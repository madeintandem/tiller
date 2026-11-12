package mutate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
)

// CompactOptions selects which days Compact folds. Now dates the delay; All
// ignores the delay and takes every day, today included; DryRun plans every
// day and commits nothing.
type CompactOptions struct {
	Now    time.Time
	All    bool
	DryRun bool
}

// CompactedDay is one day Compact folded: how many loose event files it
// removed, the day directory they lived in, and the segment it wrote. Segment
// is empty when every event was already in a segment.
type CompactedDay struct {
	Day       string `json:"day"`
	Events    int    `json:"events"`
	Directory string `json:"directory"`
	Segment   string `json:"segment,omitempty"`
}

// CompactResult reports the days folded, oldest first, the loose events
// removed, the loose events left where they were because a segment could not
// carry them faithfully, and the paths to stage, project-relative: per day the
// directory when compaction removed it, or else each loose file it removed,
// and then the segment it wrote.
type CompactResult struct {
	Days    []CompactedDay `json:"days"`
	Events  int            `json:"events"`
	Skipped int            `json:"skipped,omitempty"`
	Stage   []string       `json:"stage,omitempty"`
}

// commitDay commits one day's transaction, a seam so a test can make the tree
// change under the run.
var commitDay = func(tree *store.Tree, writes []store.PlannedWrite) error {
	return tree.Commit("compact", writes)
}

// Compact folds each eligible day of loose event files into one new,
// immutable JSONL segment beside the day directory, then deletes the loose
// files it carried and the directory once it is empty. A day is eligible when
// it is at least the manifest's events.compact_after_days whole UTC days old,
// or always under All. Each day is its own transaction, oldest first. A day
// that refuses is left untouched and the run goes on to the next day; the
// returned error joins every refused day's error, each naming its day. A tree
// that changed under the run (mutation_stale) stops it at once. An existing
// segment is never written or deleted: an event a segment already holds has
// its loose copy deleted, and a copy that differs from the segment's refuses
// the day. An event a segment cannot carry exactly stays loose and is counted
// in Skipped. Compaction records no event.
func Compact(root string, opts CompactOptions) (CompactResult, error) {
	res := CompactResult{Days: []CompactedDay{}}
	tree, err := store.Load(root)
	if err != nil {
		return res, err
	}
	if err := tree.Manifest.Events.Validate(); err != nil {
		return res, err
	}
	delay := tree.Manifest.CompactAfterDays()
	days, err := eventDays(tree.Root)
	if err != nil {
		return res, err
	}
	now := opts.Now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var refused []error
	for _, day := range days {
		if !opts.All && day.AddDate(0, 0, delay).After(today) {
			continue
		}
		plan, err := planCompactDay(tree.Root, day)
		if err == nil && plan.day.Events > 0 && !opts.DryRun {
			err = commitDay(tree, plan.writes)
		}
		if err != nil {
			err = fmt.Errorf("mutate: compact %s: %w", day.Format("2006-01-02"), err)
			var env *store.EnvError
			if errors.As(err, &env) && env.Reason == store.ReasonMutationStale {
				// First, so a caller's errors.As finds the stale refusal.
				refused = append([]error{err}, refused...)
				break
			}
			refused = append(refused, err)
			continue
		}
		res.Skipped += plan.skipped
		if plan.day.Events == 0 {
			continue
		}
		res.Days = append(res.Days, plan.day)
		res.Events += plan.day.Events
		res.Stage = append(res.Stage, plan.stage...)
	}
	return res, errors.Join(refused...)
}

// dayPlan is one day's compaction: what it reports, the paths that stage it,
// and the transaction that carries it out.
type dayPlan struct {
	day     CompactedDay
	skipped int
	stage   []string
	writes  []store.PlannedWrite
}

func planCompactDay(root string, day time.Time) (dayPlan, error) {
	monthRel := filepath.Join(".tiller", "events", day.Format("2006"), day.Format("01"))
	dirRel := filepath.Join(monthRel, day.Format("02"))
	dirAbs := filepath.Join(root, dirRel)
	plan := dayPlan{day: CompactedDay{Day: day.Format("2006-01-02"), Directory: filepath.ToSlash(dirRel)}}

	held, guards, err := readDaySegments(root, monthRel, day)
	if err != nil {
		return dayPlan{}, err
	}
	dirSnapshot, err := store.ReadDirectorySnapshot(root, dirAbs)
	if err != nil {
		return dayPlan{}, err
	}
	entries, err := store.ReadRepositoryDirectory(root, dirRel)
	if err != nil {
		return dayPlan{}, err
	}

	var carried []events.Event
	var deletes []store.PlannedWrite
	var deleted []string
	removed := map[string]bool{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		rel := filepath.Join(dirRel, entry.Name())
		abs := filepath.Join(root, rel)
		raw, snapshot, err := store.ReadGuardedFileSnapshot(root, abs)
		if err != nil {
			return dayPlan{}, err
		}
		e, ok := carriable(rel, raw)
		if !ok {
			plan.skipped++
			continue
		}
		if h, found := held[e.ID]; found {
			if err := events.Conflict(h, events.Located{Event: e, Path: rel}); err != nil {
				return dayPlan{}, err
			}
		} else {
			carried = append(carried, e)
		}
		deletes = append(deletes, store.PlanDeleteFile(abs, snapshot))
		deleted = append(deleted, filepath.ToSlash(rel))
		removed[entry.Name()] = true
	}
	plan.day.Events = len(deletes)
	if plan.day.Events == 0 {
		return plan, nil
	}

	plan.writes = guards
	if len(carried) > 0 {
		write, segmentRel, err := planSegment(root, day, carried)
		if err != nil {
			return dayPlan{}, err
		}
		plan.writes = append(plan.writes, write)
		plan.day.Segment = filepath.ToSlash(segmentRel)
	}
	plan.writes = append(plan.writes, deletes...)
	// The directory is staged whole only when it goes: one that survives
	// holds something compaction left, which staging it would sweep in.
	plan.stage = deleted
	if allRemoved(dirSnapshot.Names, removed) {
		plan.writes = append(plan.writes, store.PlanRemoveEmptyDir(dirAbs, dirSnapshot))
		plan.stage = []string{plan.day.Directory}
	}
	if plan.day.Segment != "" {
		plan.stage = append(plan.stage, plan.day.Segment)
	}
	return plan, nil
}

// readDaySegments reads every existing segment for day in its month
// directory, keyed by event id, with a read guard per segment so the day's
// transaction refuses if one changed before it commits. Two segments that
// disagree about an event refuse the day, as they refuse every reader.
func readDaySegments(root, monthRel string, day time.Time) (map[string]events.Located, []store.PlannedWrite, error) {
	entries, err := store.ReadRepositoryDirectory(root, monthRel)
	if err != nil {
		return nil, nil, err
	}
	held := map[string]events.Located{}
	var guards []store.PlannedWrite
	for _, entry := range entries {
		rel := filepath.Join(monthRel, entry.Name())
		segmentDay, _ := events.SegmentDay(entry.Name())
		if !events.IsSegmentPath(rel) || segmentDay != day.Day() || !entry.Type().IsRegular() {
			continue
		}
		abs := filepath.Join(root, rel)
		raw, snapshot, err := store.ReadGuardedFileSnapshot(root, abs)
		if err != nil {
			return nil, nil, err
		}
		evs, err := events.DecodeSegment(rel, raw)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range evs {
			next := events.Located{Event: e, Path: rel}
			if h, found := held[e.ID]; found {
				if err := events.Conflict(h, next); err != nil {
					return nil, nil, err
				}
			}
			held[e.ID] = next
		}
		guards = append(guards, store.PlanReadGuard(abs, snapshot))
	}
	return held, guards, nil
}

// carriable decodes a loose event file and reports whether a segment can hold
// it exactly: it decodes strictly, its id is a valid EV- token, it lives at its
// own dated path, and it survives a trip through the segment codec unchanged.
// The id check also keeps a crafted file name out of the stage list.
func carriable(rel string, raw []byte) (events.Event, bool) {
	e, err := events.DecodeLooseStrict(rel, raw)
	if err != nil || !id.Valid(e.ID) || !strings.HasPrefix(e.ID, "EV-") || events.RelPath(e) != rel {
		return events.Event{}, false
	}
	encoded, err := events.EncodeSegment([]events.Event{e})
	if err != nil {
		return events.Event{}, false
	}
	back, err := events.DecodeSegment(rel, encoded)
	if err != nil || len(back) != 1 || !events.Equal(back[0], e) {
		return events.Event{}, false
	}
	return e, true
}

// planSegment encodes carried as one segment for day and proves it by value
// before anything is deleted. It refuses when anything already sits at the
// segment's path, since an existing segment is never rewritten and anything
// else there is not a segment this day's read counted.
func planSegment(root string, day time.Time, carried []events.Event) (store.PlannedWrite, string, error) {
	events.Sort(carried)
	data, err := events.EncodeSegment(carried)
	if err != nil {
		return store.PlannedWrite{}, "", err
	}
	rel := events.SegmentRelPath(day, data)
	decoded, err := events.DecodeSegment(rel, data)
	if err != nil {
		return store.PlannedWrite{}, "", err
	}
	if len(decoded) != len(carried) {
		return store.PlannedWrite{}, "", fmt.Errorf("segment %s holds %d events, want %d", rel, len(decoded), len(carried))
	}
	for i := range carried {
		if !events.Equal(decoded[i], carried[i]) {
			return store.PlannedWrite{}, "", fmt.Errorf("%s does not survive segment %s", carried[i].ID, rel)
		}
	}
	abs := filepath.Join(root, rel)
	_, snapshot, err := store.ReadGuardedFileSnapshot(root, abs)
	if err != nil {
		return store.PlannedWrite{}, "", err
	}
	if snapshot.Exists() {
		// Every segment of the day was already read into held, and carried
		// excludes what held holds, so anything at this path is something no
		// reader counts (a case-folded name, a directory, a late arrival).
		// Deleting loose files on its strength could lose them.
		return store.PlannedWrite{}, "", fmt.Errorf("%s exists but was not read as a segment of the day", rel)
	}
	return store.PlanWrite(abs, snapshot, data), rel, nil
}

func allRemoved(names []string, removed map[string]bool) bool {
	for _, name := range names {
		if !removed[name] {
			return false
		}
	}
	return true
}

// eventDays lists the YYYY/MM/DD directories under .tiller/events, oldest
// first, ignoring any name that is not a real date. A missing events
// directory has no days.
func eventDays(root string) ([]time.Time, error) {
	eventsRel := filepath.Join(".tiller", "events")
	years, err := subdirectories(root, eventsRel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []time.Time
	for _, year := range years {
		months, err := subdirectories(root, filepath.Join(eventsRel, year))
		if err != nil {
			return nil, err
		}
		for _, month := range months {
			dayNames, err := subdirectories(root, filepath.Join(eventsRel, year, month))
			if err != nil {
				return nil, err
			}
			for _, dayName := range dayNames {
				text := year + "-" + month + "-" + dayName
				day, err := time.Parse("2006-01-02", text)
				if err != nil || day.Format("2006-01-02") != text {
					continue
				}
				days = append(days, day)
			}
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	return days, nil
}

func subdirectories(root, rel string) ([]string, error) {
	entries, err := store.ReadRepositoryDirectory(root, rel)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

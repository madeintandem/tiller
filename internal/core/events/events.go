// Package events is the append-only, date-partitioned event stream.
//
// Every mutation records one immutable YAML file under
// .tiller/events/YYYY/MM/DD/EV-xxxx.yml. The files are never the source of
// truth for current state (the node files are); they are the source of truth
// for recaps, node timelines, and stale-review protection, because commit
// history can squash several transitions into one commit and uncommitted
// working-tree changes have no commit yet. Appending always adds a new file
// rather than editing a shared log, so parallel branches merge event histories
// by adding files. Sorting is by timestamp, then event id. See
// docs/spec/artifact-schema.md, "Event stream".
package events

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Clock yields the current time. It is the single seam the stream reads wall
// time through, so a test can inject a fixed instant and a recorded event's
// dated path becomes deterministic. Production code uses SystemClock.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock, backed by time.Now in UTC.
type SystemClock struct{}

// Now returns the current time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FileChange records one file a command touched: its repo-relative path, the
// content revision the command read before writing (Before), and the revision
// it wrote (After). Both are RevisionRefs (store.RevisionRef); an empty ref
// serializes to nothing, so a pure-create change can leave Before unset and a
// pure-delete can leave After unset.
type FileChange struct {
	Path   string            `yaml:"path"`
	Before store.RevisionRef `yaml:"before,omitempty"`
	After  store.RevisionRef `yaml:"after,omitempty"`
}

// Event is one recorded mutation. It carries the minted EV- id, the event type
// and the literal command, the acting contributor, the node ids it targeted, the
// before and after states as opaque maps, the timestamp it occurred at, and the
// files it touched with their before/after revisions. The shape matches
// docs/spec/artifact-schema.md, "Event stream".
type Event struct {
	ID          string         `yaml:"id"`
	Type        string         `yaml:"type"`
	At          time.Time      `yaml:"at"`
	Contributor string         `yaml:"contributor"`
	Command     string         `yaml:"command"`
	Targets     []string       `yaml:"targets,omitempty"`
	Before      map[string]any `yaml:"before,omitempty"`
	After       map[string]any `yaml:"after,omitempty"`
	Files       []FileChange   `yaml:"files,omitempty"`
}

// UnmarshalYAML preserves attribution from immutable pre-schema-9 events.
// Writers emit only contributor; reading history never rewrites an old event.
func (e *Event) UnmarshalYAML(node *yaml.Node) error {
	type eventData Event
	var data struct {
		Event       eventData `yaml:",inline"`
		LegacyOwner *string   `yaml:"owner"`
	}
	if err := node.Decode(&data); err != nil {
		return err
	}
	if data.LegacyOwner != nil {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == "contributor" {
				return errors.New("event has both owner and contributor attribution")
			}
		}
		data.Event.Contributor = *data.LegacyOwner
	}
	*e = Event(data.Event)
	return nil
}

// NewEvent builds an Event stamped with clk.Now() in UTC. The id is left empty
// for File to mint, and Before, After, and Files are filled in by the caller.
// Constructing every event through NewEvent keeps the injected Clock the only
// wall-time source the stream ever reads.
func NewEvent(clk Clock, typ, contributor, command string, targets ...string) Event {
	return Event{
		Type:        typ,
		At:          clk.Now().UTC(),
		Contributor: contributor,
		Command:     command,
		Targets:     targets,
	}
}

// File resolves where an event lands and what it serializes to, without writing
// it. Mutations take the path and bytes here so events participate in the same
// transaction, journal, ordering, and rollback as their source-state writes.
func File(tree *store.Tree, e Event) (path string, data []byte, err error) {
	if e.At.IsZero() {
		return "", nil, errors.New("events: event has a zero timestamp; stamp At from a Clock (see NewEvent) before File")
	}
	if e.ID == "" {
		e.ID = id.Mint("EV")
	}

	data, err = yaml.Marshal(e)
	if err != nil {
		return "", nil, fmt.Errorf("events: marshal %s: %w", e.ID, err)
	}

	return filepath.Join(tree.Root, RelPath(e)), data, nil
}

// RelPath is an event's project-relative file path,
// .tiller/events/YYYY/MM/DD/<id>.yml, dated by its UTC timestamp.
func RelPath(e Event) string {
	t := e.At.UTC()
	return filepath.Join(
		".tiller", "events",
		fmt.Sprintf("%04d", t.Year()),
		fmt.Sprintf("%02d", int(t.Month())),
		fmt.Sprintf("%02d", t.Day()),
		e.ID+".yml",
	)
}

// Located is an event with the repository-relative file it was read from.
type Located struct {
	Event Event
	Path  string
}

// List reads every event under .tiller/events/, loose YAML files and compacted
// JSONL segments alike, and returns them sorted by timestamp, then event id
// (artifact-schema.md, "Event stream"). An event held by a loose file and a
// segment, or by several segments, is one event. A missing events/ directory
// yields an empty slice and no error. The walk uses one confined reader, so
// each dated directory is verified once rather than once per event. Returned
// events are shared read-only values; callers must not modify their maps or
// slices.
func List(tree *store.Tree) ([]Event, error) {
	located, err := ListLocated(tree)
	if err != nil {
		return nil, err
	}
	return eventsOf(located), nil
}

// ListLocated is List with the file each event came from. When several files
// hold one event, Path is a segment in preference to a loose file, and the
// lexically first segment among several.
func ListLocated(tree *store.Tree) ([]Located, error) {
	cached := processCache.Load()
	walked, listed, _, err := listWhere(tree, cached, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	if cached != nil {
		cached.replace(tree.Root, listed)
	}
	return dedupe(walked)
}

// ListSince is List restricted to events strictly after since. It reads only
// the YYYY/MM/DD directories, and the segments, whose UTC date is on or after
// since's UTC date, so a caller that wants the last few days never touches
// older partitions. Directories are named by UTC date (RelPath), so the
// boundary day is matched by its UTC date and the exact instant is checked on
// the decoded event. The confined reader, the missing-directory rule, and the
// sort are List's.
//
// A windowed read never evicts what the process cache already holds: it adds
// the files it newly parsed and leaves every other entry alone, so a List
// after it stays as cheap as before. Replacing the cache with only the window
// would make the next List reparse the whole history.
func ListSince(tree *store.Tree, since time.Time) ([]Event, error) {
	cached := processCache.Load()
	walked, _, fresh, err := listWhere(tree, cached, keepSince(since))
	if err != nil {
		return nil, err
	}
	if cached != nil {
		cached.add(tree.Root, fresh)
	}
	located, err := dedupe(walked)
	if err != nil {
		return nil, err
	}
	var evs []Event
	for _, l := range located {
		if l.Event.At.After(since) {
			evs = append(evs, l.Event)
		}
	}
	return evs, nil
}

func eventsOf(located []Located) []Event {
	evs := make([]Event, len(located))
	for i, l := range located {
		evs[i] = l.Event
	}
	return evs
}

func isSegment(path string) bool { return strings.HasSuffix(path, ".jsonl") }

// Fold is the one count-once rule every reader of the stream keeps: it folds
// records into one per event id, in the order each id was first seen. Records
// of one id that are Equal are one event, kept at its preferred home, a
// segment over a loose file and then the lexically first segment. Records
// that differ are refused, naming both files (Conflict).
func Fold(records []Located) ([]Located, error) {
	byID := make(map[string]int, len(records))
	out := make([]Located, 0, len(records))
	for _, l := range records {
		i, seen := byID[l.Event.ID]
		if !seen {
			byID[l.Event.ID] = len(out)
			out = append(out, l)
			continue
		}
		held := out[i]
		if err := Conflict(held, l); err != nil {
			return nil, err
		}
		if isSegment(l.Path) && (!isSegment(held.Path) || l.Path < held.Path) {
			out[i].Path = l.Path
		}
	}
	return out, nil
}

// Conflict is the refusal for two records of one event id that differ, naming
// both files, or nil when they are Equal.
func Conflict(held, next Located) error {
	if Equal(held.Event, next.Event) {
		return nil
	}
	return fmt.Errorf("events: %s differs between %s and %s", next.Event.ID, held.Path, next.Path)
}

// dedupe folds walked records (Fold) and sorts them in stream order.
func dedupe(walked []Located) ([]Located, error) {
	out, err := Fold(walked)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return Less(out[i].Event, out[j].Event) })
	return out, nil
}

// keepSince reports whether a dated directory, named relative to the events
// directory (2026, 2026/10, or 2026/10/21), can hold an event on or after
// since's UTC date. A directory that does not parse as a date part is kept so
// the walk treats it exactly as List does.
func keepSince(since time.Time) func(relative string) bool {
	utc := since.UTC()
	boundary := []int{utc.Year(), int(utc.Month()), utc.Day()}
	return func(relative string) bool {
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) > len(boundary) {
			return true
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil {
				return true
			}
			if n != boundary[i] {
				return n > boundary[i]
			}
		}
		return true
	}
}

// listWhere walks .tiller/events/ through one confined reader, descending only
// into directories keep accepts (named relative to the events directory), and
// returns every record found, every cache entry the walk saw, and the entries
// it had to parse because cached did not already hold them. A segment is kept
// by asking keep about its day, as if it were a day directory. The records are
// not yet deduplicated or sorted. A missing events directory is an empty
// stream.
func listWhere(tree *store.Tree, cached *cache, keep func(relative string) bool) (found []Located, listed, fresh map[string]cachedFile, err error) {
	reader, err := store.OpenRepositoryReader(tree.Root)
	if err != nil {
		return nil, nil, nil, err
	}
	defer reader.Close()
	var known map[string]cachedFile
	if cached != nil {
		known = cached.known(tree.Root)
	}
	listed = map[string]cachedFile{}
	fresh = map[string]cachedFile{}
	eventsRoot := filepath.Join(".tiller", "events")
	var walk func(string) error
	walk = func(relative string) error {
		entries, err := reader.ReadDirectory(relative)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := filepath.Join(relative, entry.Name())
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("events: %s is a symlink", path)
			}
			if entry.IsDir() {
				if inside, err := filepath.Rel(eventsRoot, path); err == nil && !keep(inside) {
					continue
				}
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			parse, ok := parserFor(eventsRoot, relative, entry.Name(), keep)
			if !ok {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			file, hit := known[path]
			if !hit || !file.matches(info) {
				raw, err := reader.ReadFile(path)
				if err != nil {
					return err
				}
				evs, err := parse(path, raw)
				if err != nil {
					return err
				}
				if cached != nil {
					cached.countParsed()
				}
				file = cachedFile{info: info, events: evs}
				fresh[path] = file
			}
			listed[path] = file
			for _, e := range file.events {
				found = append(found, Located{Event: e, Path: path})
			}
		}
		return nil
	}
	if err := walk(eventsRoot); err != nil {
		// Only an absent lazy events directory is an empty stream. A missing
		// file below an existing directory still indicates an incomplete read.
		if errors.Is(err, fs.ErrNotExist) {
			present, inspectErr := store.PathExistsNoSymlinks(tree.Root, filepath.Join(tree.Root, ".tiller", "events"))
			if inspectErr == nil && !present {
				return nil, map[string]cachedFile{}, nil, nil
			}
		}
		return nil, nil, nil, err
	}
	return found, listed, fresh, nil
}

// parserFor picks how to read a file the walk met in directory relative, or
// reports false when the walk ignores it: an event file (IsEventFile) is read
// by DecodeFile, except a segment the window does not keep, which is skipped
// when keep refuses its day, as is anything else.
func parserFor(eventsRoot, relative, name string, keep func(string) bool) (func(path string, raw []byte) ([]Event, error), bool) {
	rel := filepath.Join(relative, name)
	if !IsEventFile(rel) {
		return nil, false
	}
	if IsSegmentPath(rel) {
		inside, err := filepath.Rel(eventsRoot, relative)
		day, _ := SegmentDay(name)
		if err != nil || !keep(filepath.Join(inside, fmt.Sprintf("%02d", day))) {
			return nil, false
		}
	}
	return DecodeFile, true
}

// IsEventFile reports whether rel, a project-relative path under
// .tiller/events, names a file the stream reads: a loose .yml event or a
// segment (IsSegmentPath).
func IsEventFile(rel string) bool {
	return strings.HasSuffix(rel, ".yml") || IsSegmentPath(rel)
}

// DecodeFile is the one decode-by-path rule: it returns the events the file
// at relative holds, many for a segment (IsSegmentPath) and one for any other
// file, which is a loose event, all in the one normalized shape.
func DecodeFile(relative string, raw []byte) ([]Event, error) {
	if IsSegmentPath(relative) {
		return DecodeSegment(relative, raw)
	}
	e, err := DecodeLoose(relative, raw)
	if err != nil {
		return nil, err
	}
	return []Event{e}, nil
}

// Load reads and parses one repository-relative loose event file without
// following symlinks. A parse failure names the file.
func Load(root, relative string) (Event, error) {
	raw, err := store.ReadRepositoryFile(root, relative)
	if err != nil {
		return Event{}, err
	}
	return DecodeLoose(relative, raw)
}

// DecodeLoose parses one loose event file, named relative in an error, into
// the normalized shape a segment decodes to, so a consumer never sees a
// different Go type because an event was compacted.
func DecodeLoose(relative string, raw []byte) (Event, error) {
	var e Event
	if err := yaml.Unmarshal(raw, &e); err != nil {
		return Event{}, fmt.Errorf("events: parse %s: %w", relative, err)
	}
	normalize(&e)
	return e, nil
}

// Sort orders events in place by timestamp, then event id, the canonical event
// stream order (artifact-schema.md, "Event stream").
func Sort(evs []Event) {
	sort.SliceStable(evs, func(i, j int) bool { return Less(evs[i], evs[j]) })
}

// Less reports whether a sorts before b: earlier timestamp first, and id as the
// tiebreak when two events share a timestamp.
func Less(a, b Event) bool {
	if a.At.Equal(b.At) {
		return a.ID < b.ID
	}
	return a.At.Before(b.At)
}

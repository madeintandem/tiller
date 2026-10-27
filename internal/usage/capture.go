package usage

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// SettleAfter is how long a session must have been quiet before capture
// records it, so a running session is never captured mid-flight.
const SettleAfter = 30 * time.Minute

// Options configures one capture.
type Options struct {
	Root    string // checkout root the usage files and cursor live under
	Roots   Roots
	Readers []Reader
	Resolve Resolver
	Now     time.Time
	// Since returns the project's earliest event; a session whose first call
	// is before it is not captured. A zero time means the project has no
	// event yet: then nothing is captured and the sessions stay out of the
	// cursor, so they are judged once an event exists. It is asked at most
	// once, and only when a changed, settled session would be parsed, so a
	// capture with nothing new never reads the event stream. Nil means no
	// lower bound.
	Since func() (time.Time, error)
	// Budget bounds the capture's wall time, checked between sessions. Zero
	// means none.
	Budget time.Duration
}

// HarnessResult is what capture did for one harness. Sessions counts the
// top-level sessions found for this project, Written the files planned,
// Tokens their total, and Tied the part credited to a work item. Unreadable
// counts the folders and logs that could not be read; they were skipped,
// stay out of the cursor, and are tried again next time.
type HarnessResult struct {
	Harness    string
	Found      bool
	Sessions   int
	Written    int
	Tokens     int64
	Tied       int64
	Unreadable int
}

// Result is a capture's planned writes. Partial is true when the budget
// stopped it before every changed session was read.
type Result struct {
	Harnesses []HarnessResult
	Writes    []store.PlannedWrite
	Partial   bool
}

// Capture reads this project's harness logs and plans the usage files to
// create or grow. It takes no lock and writes nothing: the caller commits
// Result.Writes through tree.Commit, whose snapshot guards refuse a file
// changed since planning, and saves the returned cursor only after that
// commit succeeds. Now is injected; the wall clock is read only to measure
// the budget.
func Capture(o Options) (Result, Cursor, error) {
	prev := LoadCursor(o.Root)
	memo := newCwdMemo(prev.Cwd)
	o.Roots.cwd = memo
	if o.Resolve == nil {
		o.Resolve = func(string) string { return "" }
	}
	c := &capture{o: o, prev: prev, next: emptyCursor(), examined: map[string]bool{}, started: time.Now()}
	for _, rd := range o.Readers {
		h := HarnessResult{Harness: rd.Harness(), Found: anyDir(rd.Dirs(o.Roots))}
		if !c.res.Partial {
			if err := c.harness(rd, &h); err != nil {
				return Result{}, Cursor{}, err
			}
		}
		c.res.Harnesses = append(c.res.Harnesses, h)
	}
	// Working directories of rollouts no longer on disk are dropped.
	c.next.Cwd = memo.seen
	if c.res.Partial {
		// What the budget left unread keeps what the old cursor knew of it.
		for p, e := range prev.Logs {
			if !c.examined[p] {
				c.next.Logs[p] = e
			}
		}
		for p, cwd := range prev.Cwd {
			if _, ok := c.next.Cwd[p]; !ok {
				c.next.Cwd[p] = cwd
			}
		}
	}
	return c.res, c.next, nil
}

type capture struct {
	o        Options
	prev     Cursor
	next     Cursor
	examined map[string]bool
	started  time.Time
	parsed   int
	res      Result
	files    map[string]map[string]bool // harness -> usage file names on disk
	since    *time.Time                 // Options.Since's answer, once asked
}

// earliest answers Options.Since, asking it the first time only.
func (c *capture) earliest() (time.Time, error) {
	if c.since == nil {
		var at time.Time
		if c.o.Since != nil {
			var err error
			if at, err = c.o.Since(); err != nil {
				return time.Time{}, err
			}
		}
		c.since = &at
	}
	return *c.since, nil
}

// logStats is one group's logs as stat'd this run, keyed by path.
type logStats map[string]cursorEntry

func anyDir(dirs []string) bool {
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func (c *capture) harness(rd Reader, h *HarnessResult) error {
	refs, unreadable := rd.Discover(c.o.Roots)
	h.Unreadable = unreadable
	groups := groupLogs(refs)
	h.Sessions = len(groups)
	for _, g := range groups {
		seen, changed, newest, err := c.stat(g)
		if err != nil {
			h.Unreadable++ // skipped; tried again next time
			continue
		}
		if seen == nil {
			continue // a log vanished since discovery
		}
		top := g[0]
		hadFile := c.prev.Logs[top.Path].File
		if !changed && hadFile && !c.usageFileExists(rd.Harness(), top.Session) {
			changed = true // its usage file was discarded before commit
		}
		if !changed {
			c.record(seen, top.Path, hadFile)
			continue
		}
		// A log is last written no earlier than its last call, so a group
		// whose newest log is recent may still be running: it is not opened,
		// and stays out of the cursor until it settles.
		if newest.After(c.o.Now.Add(-SettleAfter)) {
			continue
		}
		// The first changed session always runs, so every capture makes
		// progress; after it, the budget is checked at each session boundary.
		if c.parsed > 0 && c.o.Budget > 0 && time.Since(c.started) > c.o.Budget {
			c.res.Partial = true
			return nil
		}
		c.parsed++
		if err := c.session(rd, g, seen, h); err != nil {
			return err
		}
	}
	return nil
}

// groupLogs gathers each top-level log with its subagent logs, top-level log
// first, ordered by the top-level log's path. Subagent logs whose parent log
// is absent are dropped, since there is no session to fold them into.
func groupLogs(refs []LogRef) [][]LogRef {
	bySession := map[string][]LogRef{}
	var tops []LogRef
	for _, r := range refs {
		if r.Parent == "" {
			tops = append(tops, r)
		} else {
			bySession[r.Parent] = append(bySession[r.Parent], r)
		}
	}
	sort.Slice(tops, func(i, j int) bool { return tops[i].Path < tops[j].Path })
	groups := make([][]LogRef, 0, len(tops))
	for _, t := range tops {
		groups = append(groups, append([]LogRef{t}, bySession[t.Session]...))
	}
	return groups
}

// stat reads each member's size and modification time without opening it.
// changed is true when any member differs from the cursor or is new to it;
// newest is the latest modification time. A member that no longer exists
// yields nil: the group is skipped this time.
func (c *capture) stat(g []LogRef) (seen logStats, changed bool, newest time.Time, err error) {
	seen = logStats{}
	for _, l := range g {
		c.examined[l.Path] = true
		info, err := os.Stat(l.Path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, time.Time{}, nil
		}
		if err != nil {
			return nil, false, time.Time{}, err
		}
		e := cursorEntry{Size: info.Size(), ModTime: info.ModTime()}
		if old, ok := c.prev.Logs[l.Path]; !ok || !old.same(e) {
			changed = true
		}
		if e.ModTime.After(newest) {
			newest = e.ModTime
		}
		seen[l.Path] = e
	}
	return seen, changed, newest, nil
}

// usageFileExists answers from one listing of the harness's usage folder,
// read the first time it is asked. An unreadable folder counts as empty, so
// the session is recaptured.
func (c *capture) usageFileExists(harness, session string) bool {
	if c.files == nil {
		c.files = map[string]map[string]bool{}
	}
	names, ok := c.files[harness]
	if !ok {
		names = map[string]bool{}
		entries, _ := store.ReadRepositoryDirectory(c.o.Root, filepath.FromSlash(path.Dir(Path(harness, session))))
		for _, e := range entries {
			if !e.IsDir() {
				names[e.Name()] = true
			}
		}
		c.files[harness] = names
	}
	return names[path.Base(Path(harness, session))]
}

// record enters a decided group into the next cursor; file marks, on the
// top-level log, that the session has a usage file.
func (c *capture) record(seen logStats, top string, file bool) {
	for p, e := range seen {
		e.File = file && p == top
		c.next.Logs[p] = e
	}
}

// session parses one changed group and plans its file. The group enters the
// cursor once it is decided, unless the session is still active, so an
// unchanged but now-settled log is read again next time.
func (c *capture) session(rd Reader, g []LogRef, seen logStats, h *HarnessResult) error {
	top := g[0]
	since, err := c.earliest()
	if err != nil {
		return err
	}
	if c.o.Since != nil && since.IsZero() {
		return nil // no event yet: nothing to judge the session by
	}
	s, err := rd.Parse(g)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // removed mid-capture; retried next time
	}
	if err != nil {
		h.Unreadable++ // left out of the cursor, so tried again next time
		return nil
	}
	first, last, ok := callSpan(s)
	if !ok {
		c.record(seen, top.Path, false)
		return nil
	}
	if last.After(c.o.Now.Add(-SettleAfter)) {
		return nil
	}
	if !since.IsZero() && first.Before(since) {
		c.record(seen, top.Path, false)
		return nil
	}

	f := File{
		Harness: rd.Harness(), Session: top.Session, Reader: ReaderVersion, CapturedAt: c.o.Now,
		FirstCall: first, LastCall: last, Rows: Credit(s, c.o.Resolve),
	}
	rel := Path(f.Harness, f.Session)
	target := filepath.Join(c.o.Root, filepath.FromSlash(rel))
	existing, snapshot, err := store.ReadGuardedFileSnapshot(c.o.Root, target)
	if err != nil {
		return err
	}
	data, err := Encode(f)
	if err != nil {
		return err
	}
	if snapshot.Exists() {
		old, err := Decode(existing)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if old.Calls() > f.Calls() {
			c.record(seen, top.Path, true) // the log lost calls; the file keeps them
			return nil
		}
		same := f
		same.CapturedAt = old.CapturedAt
		if again, err := Encode(same); err == nil && bytes.Equal(again, existing) {
			c.record(seen, top.Path, true) // nothing new since the file was written
			return nil
		}
	}

	c.res.Writes = append(c.res.Writes, store.PlanWrite(target, snapshot, data))
	h.Written++
	for _, r := range f.Rows {
		h.Tokens += r.Total()
		if r.Node != "" {
			h.Tied += r.Total()
		}
	}
	c.record(seen, top.Path, true)
	return nil
}

// callSpan is the earliest and latest call across a session and its
// subagents; ok is false when there are no calls.
func callSpan(s Session) (first, last time.Time, ok bool) {
	for _, call := range s.Calls {
		if !ok || call.At.Before(first) {
			first = call.At
		}
		if !ok || call.At.After(last) {
			last = call.At
		}
		ok = true
	}
	for _, sub := range s.Subagents {
		f, l, sok := callSpan(sub)
		if !sok {
			continue
		}
		if !ok || f.Before(first) {
			first = f
		}
		if !ok || l.After(last) {
			last = l
		}
		ok = true
	}
	return first, last, ok
}

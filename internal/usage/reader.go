package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LogRef names one harness log file. Parent is the session id of the
// top-level log a subagent log belongs to, and "" for a top-level log.
type LogRef struct {
	Harness, Path, Session, Parent string
}

// Roots says where to look. Project is the project's directory (Project[0])
// plus that same directory in each live worktree, absolute and cleaned. Env
// reads an environment variable and may be nil.
type Roots struct {
	Home    string
	Env     func(string) string
	Project []string
	// cwd remembers working directories across captures: each Codex
	// rollout's first-line one, and the first one each top-level Claude Code
	// or Pi log records, for every log in a folder named like a project
	// path. Capture sets it from the cursor. Nil reads them every time.
	cwd *cwdMemo
}

// cwdMemo holds the working directories learned by earlier captures (prev)
// and those looked up or read in this discovery (seen), keyed by log path. A
// rollout's first line never changes, and neither does the first working
// directory a log records, so a known log is never opened again for it.
type cwdMemo struct {
	prev, seen map[string]string
}

func newCwdMemo(prev map[string]string) *cwdMemo {
	return &cwdMemo{prev: prev, seen: map[string]string{}}
}

// lookup answers path's working directory from the memo or, the first time,
// from read. A read error is not remembered, so the file is tried again.
func (m *cwdMemo) lookup(path string, read func(string) (string, error)) (string, error) {
	if m == nil {
		return read(path)
	}
	cwd, ok := m.prev[path]
	if !ok {
		var err error
		if cwd, err = read(path); err != nil {
			return "", err
		}
	}
	m.seen[path] = cwd
	return cwd, nil
}

func (r Roots) env(key string) string {
	if r.Env == nil {
		return ""
	}
	return r.Env(key)
}

// Reader turns one harness's logs into neutral sessions.
type Reader interface {
	Harness() string
	// Dirs names the harness folders Discover reads; when none exists the
	// harness is not on this machine.
	Dirs(r Roots) []string
	// Discover lists the logs whose working directory is under a project
	// root. A folder or log it cannot read is skipped and counted in
	// unreadable, never fatal, so one bad file cannot stop every capture.
	Discover(r Roots) (refs []LogRef, unreadable int)
	// Parse receives one top-level log and its subagent logs.
	Parse(logs []LogRef) (Session, error)
}

const (
	scanBufferBytes = 1 << 20
	maxLineBytes    = 8 << 20
)

// scanLines calls visit with every line of the file for which keep returns
// true. A line longer than maxLineBytes is skipped by draining to its newline;
// it is never held whole. The line passed to keep and visit is only valid for
// the call.
func scanLines(path string, keep func(line []byte) bool, visit func(line []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, scanBufferBytes)
	var pending []byte
	oversized := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) && err != io.EOF {
			return err
		}
		if !oversized {
			if len(pending)+len(chunk) > maxLineBytes {
				oversized = true
				pending = nil
			} else if err == nil || err == io.EOF {
				var line []byte
				if len(pending) == 0 {
					line = chunk
				} else {
					line = append(pending, chunk...)
				}
				pending = pending[:0]
				line = trimEOL(line)
				if len(line) > 0 && keep(line) {
					if verr := visit(line); verr != nil {
						return verr
					}
				}
			} else {
				pending = append(pending, chunk...)
			}
		}
		if err == nil || err == io.EOF {
			oversized = false
			pending = pending[:0]
		}
		if err == io.EOF {
			return nil
		}
	}
}

func trimEOL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// folderMatcher picks the harness folders worth reading: one whose name is
// the encoding of a project path, or begins with that encoding plus "-",
// which is how a path beneath it encodes (/p/app/ui as -p-app-ui, and a
// removed worktree /p/app/.worktrees/x as -p-app--worktrees-x).
//
// A name never proves whose sessions a folder holds. The encoding turns
// every separator and dot into a dash, so /p/app.ui, /p/app-ui and /p/app/ui
// share the folder -p-app-ui. A match only nominates the folder; each
// session in it is judged by its own recorded working directory
// (Roots.sessionLogs).
type folderMatcher struct {
	exact map[string]bool
	under []string
}

// newFolderMatcher encodes each path with enc.
func newFolderMatcher(paths []string, enc func(string) string) folderMatcher {
	m := folderMatcher{exact: map[string]bool{}}
	for _, p := range paths {
		e := enc(p)
		if e == "" || e == "-" {
			continue
		}
		m.exact[e] = true
		m.under = append(m.under, e+"-")
	}
	return m
}

func (m folderMatcher) match(s string) bool {
	if m.exact[s] {
		return true
	}
	for _, u := range m.under {
		if strings.HasPrefix(s, u) {
			return true
		}
	}
	return false
}

// sessionLogs lists the sessions in a matched folder that ran at or beneath
// a project path. Each top-level log is judged alone, by the first working
// directory it records, compared by whole path segments; a subagent log goes
// with the session it sits under and is never judged itself. The answer
// comes from the memo after a log's first read.
//
// A log that records no working directory yet establishes nothing, so its
// session is left out and the log is read again next time. A log that cannot
// be opened is left out too and counts as one unreadable.
func (r Roots) sessionLogs(harness, folder, subagentDir string) (refs []LogRef, unreadable int) {
	found, unreadable := folderLogs(harness, folder, subagentDir)
	ours := map[string]bool{}
	for _, l := range found {
		if l.Parent != "" {
			continue
		}
		cwd, err := r.cwd.lookup(l.Path, logCwd)
		switch {
		case err == nil:
			ours[l.Session] = underAny(cwd, r.Project)
		case !errors.Is(err, errNoCwd):
			unreadable++
		}
	}
	for _, l := range found {
		session := l.Parent
		if session == "" {
			session = l.Session
		}
		if ours[session] {
			refs = append(refs, l)
		}
	}
	return refs, unreadable
}

var errNoCwd = errors.New("usage: no working directory recorded")

var cwdMark = []byte(`"cwd"`)

// logCwd reads a log until a line records a top-level cwd (Claude Code's
// message lines, Pi's session header) and returns it.
func logCwd(path string) (string, error) {
	var cwd string
	errFound := errors.New("found")
	err := scanLines(path,
		func(line []byte) bool { return bytes.Contains(line, cwdMark) },
		func(line []byte) error {
			var l struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(line, &l) == nil && l.Cwd != "" {
				cwd = l.Cwd
				return errFound
			}
			return nil
		})
	if errors.Is(err, errFound) {
		return cwd, nil
	}
	if err != nil {
		return "", err
	}
	return "", errNoCwd
}

// listHarnessDir reads a harness's base folder. A missing folder is empty;
// one that cannot be read is empty and counts as one unreadable.
func listHarnessDir(dir string) (entries []os.DirEntry, unreadable int) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0
	}
	if err != nil {
		return nil, 1
	}
	return entries, 0
}

// folderLogs lists a folder's top-level *.jsonl logs and, for each
// <stem> subdirectory, the *.jsonl logs in <stem>/<subagentDir> as subagents
// of that stem. Other files are ignored. A folder that cannot be read counts
// as one unreadable; so does a session whose subagent folder cannot be read,
// and that session is left out rather than captured without its subagents.
func folderLogs(harness, folder, subagentDir string) (refs []LogRef, unreadable int) {
	files, err := os.ReadDir(folder)
	if err != nil {
		return nil, 1
	}
	broken := map[string]bool{}
	for _, f := range files {
		name := f.Name()
		switch {
		case !f.IsDir() && strings.HasSuffix(name, ".jsonl"):
			refs = append(refs, LogRef{Harness: harness, Path: filepath.Join(folder, name), Session: strings.TrimSuffix(name, ".jsonl")})
		case f.IsDir():
			dir := filepath.Join(folder, name, subagentDir)
			subs, err := os.ReadDir(dir)
			if errors.Is(err, fs.ErrNotExist) {
				continue // no subagents for this session
			}
			if err != nil {
				broken[name] = true
				continue
			}
			for _, s := range subs {
				if s.IsDir() || !strings.HasSuffix(s.Name(), ".jsonl") {
					continue
				}
				refs = append(refs, LogRef{Harness: harness, Path: filepath.Join(dir, s.Name()), Session: strings.TrimSuffix(s.Name(), ".jsonl"), Parent: name})
			}
		}
	}
	if len(broken) == 0 {
		return refs, 0
	}
	kept := refs[:0]
	for _, r := range refs {
		if r.Parent == "" && broken[r.Session] {
			continue
		}
		kept = append(kept, r)
	}
	return kept, len(broken)
}

// parseSession builds a Session from one top-level log and its subagent logs.
// A call whose ID an earlier log of the session already holds (the top-level
// log first, then the subagent logs in order) is dropped, so a response
// repeated across the session's logs counts once.
func parseSession(harness string, logs []LogRef, parseLog func(path string) ([]Call, error)) (Session, error) {
	seen := map[string]bool{}
	parse := func(path string) ([]Call, error) {
		calls, err := parseLog(path)
		if err != nil {
			return nil, err
		}
		kept := calls[:0]
		for _, c := range calls {
			if c.ID != "" {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
			}
			kept = append(kept, c)
		}
		return kept, nil
	}
	var top *LogRef
	for i := range logs {
		if logs[i].Parent == "" {
			top = &logs[i]
			break
		}
	}
	if top == nil {
		return Session{}, errors.New("usage: no top-level " + harness + " log")
	}
	s := Session{Harness: harness, ID: top.Session}
	var err error
	if s.Calls, err = parse(top.Path); err != nil {
		return Session{}, err
	}
	for _, l := range logs {
		if l.Parent == "" {
			continue
		}
		calls, err := parse(l.Path)
		if err != nil {
			return Session{}, err
		}
		s.Subagents = append(s.Subagents, Session{Harness: harness, ID: l.Session, Calls: calls})
	}
	return s, nil
}

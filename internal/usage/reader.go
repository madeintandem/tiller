package usage

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LogRef names one harness log file. Parent is the session id of the
// top-level log a subagent log belongs to, and "" for a top-level log.
type LogRef struct {
	Harness, Path, Session, Parent string
}

// Roots says where to look. Project is the checkout root (Project[0]) plus
// live worktree paths, absolute and cleaned. Env reads an environment variable and may be nil.
type Roots struct {
	Home    string
	Env     func(string) string
	Project []string
	// cwd remembers each Codex rollout's first-line working directory across
	// captures; Capture sets it from the cursor. Nil reads every first line.
	cwd *cwdMemo
}

// cwdMemo holds the working directories learned by earlier captures (prev)
// and those looked up or read in this discovery (seen). A rollout's first
// line never changes, so a known path is never opened again.
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
	// Discover lists the logs whose working directory is under a project root.
	Discover(r Roots) ([]LogRef, error)
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

// folderMatcher accepts a harness folder that equals the encoding of a
// project path, or the root's encoding plus "--", which is how a path under
// one of the root's dot directories (a removed worktree) encodes. A sibling
// project such as /p/app-two never matches /p/app.
type folderMatcher struct {
	exact  map[string]bool
	prefix string
}

// newFolderMatcher encodes each path with enc; paths[0] is the checkout root.
func newFolderMatcher(paths []string, enc func(string) string) folderMatcher {
	m := folderMatcher{exact: map[string]bool{}}
	for i, p := range paths {
		e := enc(p)
		if e == "" || e == "-" {
			continue
		}
		m.exact[e] = true
		if i == 0 {
			m.prefix = e + "--"
		}
	}
	return m
}

func (m folderMatcher) match(s string) bool {
	return m.exact[s] || (m.prefix != "" && strings.HasPrefix(s, m.prefix))
}

// folderLogs lists a matched folder's top-level *.jsonl logs and, for each
// <stem> subdirectory, the *.jsonl logs in <stem>/<subagentDir> as subagents
// of that stem. Other files are ignored.
func folderLogs(harness, folder, subagentDir string) ([]LogRef, error) {
	files, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var refs []LogRef
	for _, f := range files {
		name := f.Name()
		switch {
		case !f.IsDir() && strings.HasSuffix(name, ".jsonl"):
			refs = append(refs, LogRef{Harness: harness, Path: filepath.Join(folder, name), Session: strings.TrimSuffix(name, ".jsonl")})
		case f.IsDir():
			dir := filepath.Join(folder, name, subagentDir)
			subs, err := os.ReadDir(dir)
			if err != nil {
				continue // no subagents for this session
			}
			for _, s := range subs {
				if s.IsDir() || !strings.HasSuffix(s.Name(), ".jsonl") {
					continue
				}
				refs = append(refs, LogRef{Harness: harness, Path: filepath.Join(dir, s.Name()), Session: strings.TrimSuffix(s.Name(), ".jsonl"), Parent: name})
			}
		}
	}
	return refs, nil
}

// parseSession builds a Session from one top-level log and its subagent logs.
func parseSession(harness string, logs []LogRef, parseLog func(path string) ([]Call, error)) (Session, error) {
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
	if s.Calls, err = parseLog(top.Path); err != nil {
		return Session{}, err
	}
	for _, l := range logs {
		if l.Parent == "" {
			continue
		}
		calls, err := parseLog(l.Path)
		if err != nil {
			return Session{}, err
		}
		s.Subagents = append(s.Subagents, Session{Harness: harness, ID: l.Session, Calls: calls})
	}
	return s, nil
}

package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const piHarness = "pi"

type piReader struct{}

// NewPiReader reads Pi and OhMyPi session logs.
func NewPiReader() Reader { return piReader{} }

func (piReader) Harness() string { return piHarness }

var piEncoder = strings.NewReplacer("/", "-", ".", "-")

// ompFolder encodes a path the way OhMyPi names its session folders: the
// home-relative path with every separator and dot as a dash. A path outside
// home keeps its full path.
func ompFolder(home, abs string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, abs); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			abs = "/" + filepath.ToSlash(rel)
		}
	}
	return piEncoder.Replace(abs)
}

// piInner encodes a path the way Pi does inside its "--...--" wrapper: the
// full path without its leading separator.
func piInner(abs string) string {
	return piEncoder.Replace(strings.TrimPrefix(abs, "/"))
}

// folderMatcher accepts a folder that equals the encoding of a project path,
// or the root's encoding plus "--", which is how a path under one of the
// root's dot directories (a removed worktree) encodes. A sibling project such
// as /p/app-two never matches /p/app.
type folderMatcher struct {
	exact  map[string]bool
	prefix string
}

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

func (piReader) Discover(r Roots) ([]LogRef, error) {
	omp := newFolderMatcher(r.Project, func(p string) string { return ompFolder(r.Home, p) })
	pi := newFolderMatcher(r.Project, piInner)
	var refs []LogRef
	for _, base := range []struct {
		dir     string
		matches func(name string) bool
	}{
		{filepath.Join(r.Home, ".omp", "agent", "sessions"), omp.match},
		{filepath.Join(r.Home, ".pi", "agent", "sessions"), func(name string) bool {
			if len(name) <= 4 || !strings.HasPrefix(name, "--") || !strings.HasSuffix(name, "--") {
				return false
			}
			return pi.match(name[2 : len(name)-2])
		}},
	} {
		entries, err := os.ReadDir(base.dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() || !base.matches(e.Name()) {
				continue
			}
			found, err := piFolderLogs(filepath.Join(base.dir, e.Name()))
			if err != nil {
				return nil, err
			}
			refs = append(refs, found...)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs, nil
}

func piFolderLogs(folder string) ([]LogRef, error) {
	files, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var refs []LogRef
	for _, f := range files {
		name := f.Name()
		switch {
		case !f.IsDir() && strings.HasSuffix(name, ".jsonl"):
			refs = append(refs, LogRef{Harness: piHarness, Path: filepath.Join(folder, name), Session: strings.TrimSuffix(name, ".jsonl")})
		case f.IsDir():
			subs, err := os.ReadDir(filepath.Join(folder, name))
			if err != nil {
				continue
			}
			for _, s := range subs {
				if s.IsDir() || !strings.HasSuffix(s.Name(), ".jsonl") {
					continue // the folder also holds .md, .json and .log files
				}
				refs = append(refs, LogRef{
					Harness: piHarness,
					Path:    filepath.Join(folder, name, s.Name()),
					Session: strings.TrimSuffix(s.Name(), ".jsonl"),
					Parent:  name,
				})
			}
		}
	}
	return refs, nil
}

func (piReader) Parse(logs []LogRef) (Session, error) {
	var top *LogRef
	for i := range logs {
		if logs[i].Parent == "" {
			top = &logs[i]
			break
		}
	}
	if top == nil {
		return Session{}, errors.New("usage: no top-level Pi log")
	}
	s := Session{Harness: piHarness, ID: top.Session}
	var err error
	if s.Calls, err = parsePiLog(top.Path); err != nil {
		return Session{}, err
	}
	for _, l := range logs {
		if l.Parent == "" {
			continue
		}
		calls, err := parsePiLog(l.Path)
		if err != nil {
			return Session{}, err
		}
		s.Subagents = append(s.Subagents, Session{Harness: piHarness, ID: l.Session, Calls: calls})
	}
	return s, nil
}

type piLine struct {
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments struct {
				Command string `json:"command"`
				Path    string `json:"path"`
			} `json:"arguments"`
		} `json:"content"`
		Usage *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheRead  int64 `json:"cacheRead"`
			CacheWrite int64 `json:"cacheWrite"`
			Reasoning  int64 `json:"reasoningTokens"`
		} `json:"usage"`
	} `json:"message"`
}

var piAssistantMark = []byte(`"role":"assistant"`)

const piSkillScheme = "skill://"

func parsePiLog(path string) ([]Call, error) {
	var calls []Call
	err := scanLines(path,
		func(line []byte) bool { return bytes.Contains(line, piAssistantMark) },
		func(line []byte) error {
			var l piLine
			if json.Unmarshal(line, &l) != nil {
				return nil // a malformed line is skipped, not fatal
			}
			m := l.Message
			if m.Role != "assistant" || m.Usage == nil {
				return nil
			}
			at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
			if err != nil {
				return nil
			}
			var markers []Marker
			for _, b := range m.Content {
				if b.Type != "toolCall" {
					continue
				}
				switch b.Name {
				case "bash":
					markers = append(markers, ShellMarkers(b.Arguments.Command)...)
				case "read":
					if name, ok := strings.CutPrefix(b.Arguments.Path, piSkillScheme); ok {
						if mk, ok := SkillMarker(name, ""); ok {
							markers = append(markers, mk)
						}
					}
				}
			}
			u := m.Usage
			calls = append(calls, Call{
				At:      at,
				Model:   m.Model,
				Tokens:  Tokens{Input: u.Input, CacheWrite: u.CacheWrite, CacheRead: u.CacheRead, Output: u.Output, Reasoning: u.Reasoning},
				Markers: markers,
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].At.Before(calls[j].At) })
	return calls, nil
}

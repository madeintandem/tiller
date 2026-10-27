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

// Dirs is the OhMyPi folder, then the Pi folder.
func (piReader) Dirs(r Roots) []string {
	return []string{
		filepath.Join(r.Home, ".omp", "agent", "sessions"),
		filepath.Join(r.Home, ".pi", "agent", "sessions"),
	}
}

func (pr piReader) Discover(r Roots) ([]LogRef, error) {
	omp := newFolderMatcher(r.Project, func(p string) string { return ompFolder(r.Home, p) })
	pi := newFolderMatcher(r.Project, piInner)
	dirs := pr.Dirs(r)
	// accept takes a folder the matcher accepts by name, or one that only
	// begins like a path beneath a project path and whose recorded working
	// directory is beneath one.
	accept := func(m folderMatcher, name, folder string) bool {
		return m.match(name) || (m.maybe(name) && r.beneath(folder))
	}
	var refs []LogRef
	for _, base := range []struct {
		dir     string
		matches func(name, folder string) bool
	}{
		{dirs[0], func(name, folder string) bool { return accept(omp, name, folder) }},
		{dirs[1], func(name, folder string) bool {
			if len(name) <= 4 || !strings.HasPrefix(name, "--") || !strings.HasSuffix(name, "--") {
				return false
			}
			return accept(pi, name[2:len(name)-2], folder)
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
			folder := filepath.Join(base.dir, e.Name())
			if !e.IsDir() || !base.matches(e.Name(), folder) {
				continue
			}
			found, err := folderLogs(piHarness, folder, "")
			if err != nil {
				return nil, err
			}
			refs = append(refs, found...)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs, nil
}

func (piReader) Parse(logs []LogRef) (Session, error) {
	return parseSession(piHarness, logs, parsePiLog)
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

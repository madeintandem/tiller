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

const claudeHarness = "claude-code"

type claudeReader struct{}

// NewClaudeReader reads Claude Code session logs.
func NewClaudeReader() Reader { return claudeReader{} }

func (claudeReader) Harness() string { return claudeHarness }

// claudeFolder encodes an absolute path the way Claude Code names its project
// folders: every separator and dot becomes a dash.
func claudeFolder(abs string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(abs)
}

func (claudeReader) Dirs(r Roots) []string {
	base := r.env("CLAUDE_CONFIG_DIR")
	if base == "" {
		base = filepath.Join(r.Home, ".claude")
	}
	return []string{filepath.Join(base, "projects")}
}

func (c claudeReader) Discover(r Roots) ([]LogRef, error) {
	projects := c.Dirs(r)[0]
	entries, err := os.ReadDir(projects)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := newFolderMatcher(r.Project, claudeFolder)
	var refs []LogRef
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		folder := filepath.Join(projects, e.Name())
		if !m.match(e.Name()) && !(m.maybe(e.Name()) && r.beneath(folder)) {
			continue
		}
		found, err := folderLogs(claudeHarness, folder, "subagents")
		if err != nil {
			return nil, err
		}
		refs = append(refs, found...)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs, nil
}

func (claudeReader) Parse(logs []LogRef) (Session, error) {
	return parseSession(claudeHarness, logs, parseClaudeLog)
}

type claudeLine struct {
	Timestamp string `json:"timestamp"`
	GitBranch string `json:"gitBranch"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string `json:"type"`
			Name  string `json:"name"`
			Input struct {
				Command string `json:"command"`
				Skill   string `json:"skill"`
				Args    string `json:"args"`
			} `json:"input"`
		} `json:"content"`
		Usage *struct {
			Input         int64 `json:"input_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
			Output        int64 `json:"output_tokens"`
			Details       struct {
				Thinking int64 `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	} `json:"message"`
}

var assistantMark = []byte(`"type":"assistant"`)

func parseClaudeLog(path string) ([]Call, error) {
	var calls []Call
	byID := map[string]int{}
	err := scanLines(path,
		func(line []byte) bool { return bytes.Contains(line, assistantMark) },
		func(line []byte) error {
			var l claudeLine
			if json.Unmarshal(line, &l) != nil {
				return nil // a malformed line is skipped, not fatal
			}
			m := l.Message
			if m.Usage == nil || m.Model == "<synthetic>" {
				return nil
			}
			var markers []Marker
			for _, b := range m.Content {
				if b.Type != "tool_use" {
					continue
				}
				switch b.Name {
				case "Bash":
					markers = append(markers, ShellMarkers(b.Input.Command)...)
				case "Skill":
					if mk, ok := SkillMarker(b.Input.Skill, b.Input.Args); ok {
						markers = append(markers, mk)
					}
				}
			}
			// One response is logged as several lines sharing its message
			// id (and request id); they are one call.
			var id string
			if m.ID != "" {
				id = m.ID
				if l.RequestID != "" {
					id += " " + l.RequestID
				}
				if i, seen := byID[id]; seen {
					calls[i].Markers = append(calls[i].Markers, markers...)
					return nil
				}
			}
			at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
			if err != nil {
				return nil
			}
			if id != "" {
				byID[id] = len(calls)
			}
			u := m.Usage
			calls = append(calls, Call{
				At:    at,
				Model: m.Model,
				Tokens: Tokens{
					Input: u.Input, CacheWrite: u.CacheCreation, CacheRead: u.CacheRead,
					Output: u.Output, Reasoning: u.Details.Thinking,
				},
				Markers: markers,
				Branch:  l.GitBranch,
				ID:      id,
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].At.Before(calls[j].At) })
	return calls, nil
}

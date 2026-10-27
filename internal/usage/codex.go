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
	"sort"
	"strings"
	"time"
)

const codexHarness = "codex"

type codexReader struct{}

// NewCodexReader reads Codex rollout logs.
func NewCodexReader() Reader { return codexReader{} }

func (codexReader) Harness() string { return codexHarness }

// codexSessionCwd returns the working directory recorded on the first line of
// a rollout file, the session_meta line. It reads only that line; a line over
// maxLineBytes is an error, so the file is not ours.
func codexSessionCwd(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var first struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	br := bufio.NewReaderSize(f, 64<<10)
	var line []byte
	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxLineBytes {
			return "", errors.New("usage: first line of " + path + " is too long")
		}
		if err == nil || errors.Is(err, io.EOF) {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", err
		}
	}
	if err := json.Unmarshal(trimEOL(line), &first); err != nil {
		return "", err
	}
	if first.Type != "session_meta" {
		return "", errors.New("usage: first line of " + path + " is not session_meta")
	}
	return first.Payload.Cwd, nil
}

// underAny reports whether cwd equals a root or lies beneath one, by path
// segment: /p/app/sub is under /p/app, /p/app-two is not.
func underAny(cwd string, roots []string) bool {
	if !filepath.IsAbs(cwd) {
		return false
	}
	cwd = filepath.Clean(cwd)
	for _, r := range roots {
		r = filepath.Clean(r)
		if cwd == r || strings.HasPrefix(cwd, strings.TrimSuffix(r, string(filepath.Separator))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (codexReader) Dirs(r Roots) []string {
	base := r.env("CODEX_HOME")
	if base == "" {
		base = filepath.Join(r.Home, ".codex")
	}
	return []string{filepath.Join(base, "sessions")}
}

func (c codexReader) Discover(r Roots) ([]LogRef, error) {
	dir := c.Dirs(r)[0]
	var refs []LogRef
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		name := d.Name()
		if d.IsDir() || !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		cwd, err := r.cwd.lookup(path, codexSessionCwd)
		if err != nil || !underAny(cwd, r.Project) {
			return nil // unreadable or foreign: not ours
		}
		refs = append(refs, LogRef{Harness: codexHarness, Path: path, Session: strings.TrimSuffix(name, ".jsonl")})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Path < refs[j].Path })
	return refs, nil
}

func (codexReader) Parse(logs []LogRef) (Session, error) {
	return parseSession(codexHarness, logs, parseCodexLog)
}

type codexLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type      string `json:"type"`
		Model     string `json:"model"`
		Arguments string `json:"arguments"`
		Input     string `json:"input"`
		Info      *struct {
			Total *codexUsage `json:"total_token_usage"`
			Last  codexUsage  `json:"last_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
	Total      int64 `json:"total_tokens"`
}

var codexKeep = [][]byte{
	[]byte(`"token_count"`), []byte(`"turn_context"`),
	[]byte(`"function_call"`), []byte(`"custom_tool_call"`),
}

// codexCommand reads a shell tool's arguments. A JSON object with a command
// array becomes the array joined with spaces, one with a cmd string (Codex's
// exec_command) yields that string; any other string passes through.
func codexCommand(args string) string {
	var a struct {
		Command []string `json:"command"`
		Cmd     string   `json:"cmd"`
	}
	if json.Unmarshal([]byte(args), &a) == nil {
		if len(a.Command) > 0 {
			return strings.Join(a.Command, " ")
		}
		if a.Cmd != "" {
			return a.Cmd
		}
	}
	return args
}

// parseCodexLog reads one rollout. Each token_count is one call, except one
// whose running total_token_usage equals the previous one's: Codex writes
// such a repeat with the last call's usage again, and counting it would
// double that call.
func parseCodexLog(path string) ([]Call, error) {
	var calls []Call
	var model string
	var pending []Marker
	var prevTotal *codexUsage
	err := scanLines(path,
		func(line []byte) bool {
			for _, k := range codexKeep {
				if bytes.Contains(line, k) {
					return true
				}
			}
			return false
		},
		func(line []byte) error {
			var l codexLine
			if json.Unmarshal(line, &l) != nil {
				return nil // a malformed line is skipped, not fatal
			}
			p := l.Payload
			switch {
			case l.Type == "turn_context":
				if p.Model != "" {
					model = p.Model
				}
			case l.Type == "response_item" && p.Type == "function_call":
				pending = append(pending, ShellMarkers(codexCommand(p.Arguments))...)
			case l.Type == "response_item" && p.Type == "custom_tool_call":
				pending = append(pending, ShellMarkers(p.Input)...)
			case l.Type == "event_msg" && p.Type == "token_count" && p.Info != nil:
				at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
				if err != nil {
					return nil
				}
				total := p.Info.Total
				if total != nil && prevTotal != nil && *total == *prevTotal {
					return nil // a repeat of the previous call's usage
				}
				prevTotal = total
				u := p.Info.Last
				calls = append(calls, Call{
					At:      at,
					Model:   model,
					Tokens:  Tokens{Input: u.Input - u.Cached, CacheWrite: u.CacheWrite, CacheRead: u.Cached, Output: u.Output, Reasoning: u.Reasoning},
					Markers: pending,
				})
				pending = nil
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].At.Before(calls[j].At) })
	return calls, nil
}

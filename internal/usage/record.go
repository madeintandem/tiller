// Package usage holds the neutral token-usage record committed under
// .tiller/usage/. Nothing here knows any one harness, and no field names a
// person.
package usage

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// ReaderVersion is the version of the harness readers; a new version may
// recompute files an older one wrote.
const ReaderVersion = 1

const usageDir = ".tiller/usage"

// Tokens are the four non-overlapping token counts plus informational
// reasoning, which is part of Output and never added to a total.
type Tokens struct {
	Input      int64 `yaml:"input"`
	CacheWrite int64 `yaml:"cache_write"`
	CacheRead  int64 `yaml:"cache_read"`
	Output     int64 `yaml:"output"`
	Reasoning  int64 `yaml:"reasoning,omitempty"`
}

// Total sums the four counts, never Reasoning.
func (t Tokens) Total() int64 { return t.Input + t.CacheWrite + t.CacheRead + t.Output }

// Add accumulates o into t.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.CacheWrite += o.CacheWrite
	t.CacheRead += o.CacheRead
	t.Output += o.Output
	t.Reasoning += o.Reasoning
}

// Row is the calls of one model in one UTC hour, credited to one node (or
// none) and one stage.
type Row struct {
	Hour   time.Time `yaml:"hour"`
	Node   string    `yaml:"node,omitempty"`
	Stage  Stage     `yaml:"stage"`
	Model  string    `yaml:"model"`
	Calls  int       `yaml:"calls"`
	Tokens `yaml:",inline"`
}

// File is one top-level harness session's usage, subagents folded in.
type File struct {
	Harness    string    `yaml:"harness"`
	Session    string    `yaml:"session"`
	Reader     int       `yaml:"reader"`
	CapturedAt time.Time `yaml:"captured_at"`
	FirstCall  time.Time `yaml:"first_call"`
	LastCall   time.Time `yaml:"last_call"`
	Rows       []Row     `yaml:"rows"`
}

// Calls is the number of model calls the file records.
func (f File) Calls() int {
	n := 0
	for _, r := range f.Rows {
		n += r.Calls
	}
	return n
}

// Path is the repository-relative path of a session's usage file.
func Path(harness, session string) string {
	return path.Join(usageDir, harness, session+".yml")
}

// Encode renders f with rows sorted by hour, node, stage, model.
func Encode(f File) ([]byte, error) {
	rows := append([]Row(nil), f.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.Hour.Equal(b.Hour) {
			return a.Hour.Before(b.Hour)
		}
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.Stage != b.Stage {
			return a.Stage < b.Stage
		}
		return a.Model < b.Model
	})
	f.Rows = rows
	f.CapturedAt = f.CapturedAt.UTC()
	f.FirstCall = f.FirstCall.UTC()
	f.LastCall = f.LastCall.UTC()
	for i := range f.Rows {
		f.Rows[i].Hour = f.Rows[i].Hour.UTC()
	}
	return yaml.Marshal(f)
}

// Decode parses a usage file.
func Decode(data []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

// LoadFiles reads every .tiller/usage/<harness>/*.yml under root. A missing
// usage directory is an empty result.
func LoadFiles(root string) ([]File, error) {
	harnesses, err := store.ReadRepositoryDirectory(root, usageDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []File
	for _, h := range harnesses {
		if !h.IsDir() {
			continue
		}
		entries, err := store.ReadRepositoryDirectory(root, path.Join(usageDir, h.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
				continue
			}
			rel := path.Join(usageDir, h.Name(), e.Name())
			data, err := store.ReadRepositoryFile(root, rel)
			if err != nil {
				return nil, err
			}
			f, err := Decode(data)
			if err != nil {
				return nil, errors.New(rel + ": " + err.Error())
			}
			files = append(files, f)
		}
	}
	return files, nil
}

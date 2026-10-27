package usage

import (
	"path/filepath"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

var cursorPath = filepath.Join(".tiller", ".cache", "usage.yml")

// cursorEntry is what capture last saw of one log file: its size and
// modification time. File, on a top-level log, says the session has a usage
// file (planned or found), so one discarded before commit is noticed.
type cursorEntry struct {
	Size    int64     `yaml:"size"`
	ModTime time.Time `yaml:"mod_time"`
	File    bool      `yaml:"file,omitempty"`
}

// Cursor is capture's memory between runs. Logs records, per absolute log
// path, the log as last captured; a log whose size and modification time
// still match is not opened again. Cwd records, per path, a working
// directory that never changes: each Codex rollout's first-line one, and the
// one recorded in each Claude Code or Pi folder whose name only begins like
// a project path's encoding (a subdirectory's sessions, or a sibling's).
type Cursor struct {
	Logs map[string]cursorEntry `yaml:"logs"`
	Cwd  map[string]string      `yaml:"cwd,omitempty"`
}

type cursorFile struct {
	Reader int `yaml:"reader"`
	Cursor `yaml:",inline"`
}

func emptyCursor() Cursor {
	return Cursor{Logs: map[string]cursorEntry{}, Cwd: map[string]string{}}
}

// LoadCursor reads the gitignored cursor under root. A missing, unreadable,
// or corrupt cursor, or one written by another reader version, is empty, so
// every log is examined afresh.
func LoadCursor(root string) Cursor {
	data, err := store.ReadRepositoryFile(root, cursorPath)
	if err != nil {
		return emptyCursor()
	}
	var f cursorFile
	if yaml.Unmarshal(data, &f) != nil || f.Reader != ReaderVersion {
		return emptyCursor()
	}
	c := f.Cursor
	if c.Logs == nil {
		c.Logs = map[string]cursorEntry{}
	}
	if c.Cwd == nil {
		c.Cwd = map[string]string{}
	}
	return c
}

// Save writes the cursor under root. The caller saves only after the
// capture's writes are committed.
func (c Cursor) Save(root string) error {
	data, err := yaml.Marshal(cursorFile{Reader: ReaderVersion, Cursor: c})
	if err != nil {
		return err
	}
	return store.WriteRepositoryFileAtomic(root, cursorPath, data)
}

func (e cursorEntry) same(o cursorEntry) bool {
	return e.Size == o.Size && e.ModTime.Equal(o.ModTime)
}

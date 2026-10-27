package usage

import (
	"path/filepath"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

var cursorPath = filepath.Join(".tiller", ".cache", "usage.yml")

// cursorEntry is what capture last saw of one log file: its size and
// modification time, and whether it belongs to this project.
type cursorEntry struct {
	Size    int64     `yaml:"size"`
	ModTime time.Time `yaml:"mod_time"`
	Match   bool      `yaml:"match"`
}

// Cursor records, per absolute log path, the log as last captured. A log
// whose size and modification time still match is not opened again.
type Cursor map[string]cursorEntry

type cursorFile struct {
	Reader int    `yaml:"reader"`
	Logs   Cursor `yaml:"logs"`
}

// LoadCursor reads the gitignored cursor under root. A missing, unreadable,
// or corrupt cursor, or one written by another reader version, is empty, so
// every log is examined afresh.
func LoadCursor(root string) Cursor {
	data, err := store.ReadRepositoryFile(root, cursorPath)
	if err != nil {
		return Cursor{}
	}
	var f cursorFile
	if yaml.Unmarshal(data, &f) != nil || f.Reader != ReaderVersion || f.Logs == nil {
		return Cursor{}
	}
	return f.Logs
}

// Save writes the cursor under root. The caller saves only after the
// capture's writes are committed.
func (c Cursor) Save(root string) error {
	data, err := yaml.Marshal(cursorFile{Reader: ReaderVersion, Logs: c})
	if err != nil {
		return err
	}
	return store.WriteRepositoryFileAtomic(root, cursorPath, data)
}

func (e cursorEntry) same(o cursorEntry) bool {
	return e.Size == o.Size && e.ModTime.Equal(o.ModTime)
}

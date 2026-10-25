package execution

import (
	"io/fs"
	"os"
	"time"
)

// indexRacyWindow mirrors git's racy-index rule: an index written this close
// to the moment it was observed cannot be told apart from a same-size rewrite
// in the same timestamp tick, so its identity is not trusted and every check
// compares content.
const indexRacyWindow = 2 * time.Second

// listIndex is readIndex; tests count calls through it.
var listIndex = readIndex

// sameIndexFile reports whether two Stats name the same unchanged index file.
// The Stats follow a symlinked index to the file git actually replaces: git
// renames index.lock over the link's target, so any rewrite is a different
// file.
func sameIndexFile(a, b fs.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) &&
		a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

// captureIndex records the worktree index identity and its listing. The Stat
// comes before the listing: if the index is replaced in between, the stored
// metadata names the older file and the next check compares content. A
// recently written index keeps no identity (see indexRacyWindow).
func (s *source) captureIndex(index string) error {
	s.indexPath = index
	s.indexInfo = nil
	if info, err := os.Stat(index); err == nil && time.Since(info.ModTime()).Abs() >= indexRacyWindow {
		s.indexInfo = info
	}
	var err error
	s.gitlinks, s.indexDigest, err = listIndex(s.worktreeRoot, true)
	return err
}

// indexUnchanged confirms the worktree index still matches the one captured,
// by file identity when the file is untouched, else by content digest.
func (s *source) indexUnchanged() (bool, error) {
	if info, err := os.Stat(s.indexPath); err == nil && sameIndexFile(s.indexInfo, info) {
		return true, nil
	}
	_, digest, err := listIndex(s.worktreeRoot, false)
	if err != nil {
		return false, err
	}
	return digest == s.indexDigest, nil
}

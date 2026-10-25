package execution

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ls-files inspects the existing index without refreshing or writing it. Keep
// its fingerprint so a staged boundary cannot change during an observation.
func readIndex(root string, keepGitlinks bool) (map[string]bool, [sha256.Size]byte, error) {
	out, err := gitRead(root, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	digest := sha256.Sum256(out)
	if !keepGitlinks {
		return nil, digest, nil
	}
	gitlinks := map[string]bool{}
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		metadata, path, ok := strings.Cut(string(entry), "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 {
			return nil, [sha256.Size]byte{}, errors.New("invalid Git index inventory")
		}
		if fields[0] == "160000" {
			gitlinks[path] = true
		}
	}
	return gitlinks, digest, nil
}

func (s *source) gitPath(path string) string {
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(s.repo.projectPrefix), path))
}

func (s *source) checkWorktreePath(path string) error {
	for component := s.gitPath(path); component != "."; component = filepath.ToSlash(filepath.Dir(component)) {
		if s.gitlinks[component] || s.entries[component].mode == "160000" {
			return fmt.Errorf("execution path %s crosses a Git gitlink at %s", path, component)
		}
	}
	return nil
}

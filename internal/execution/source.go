package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

type source struct {
	repo         *repository
	kind         string
	branch       string
	root         string
	worktreeRoot string
	taskPath     string
	ref          string
	oid          string
	remoteRef    string
	remoteOID    string
	freshness    string
	generation   fileState
	uncommitted  bool
	entries      map[string]treeEntry
	gitlinks     map[string]bool
	indexDigest  [sha256.Size]byte
	indexPath    string
	indexInfo    fs.FileInfo
	files        map[string]fileState
	directories  map[string]string
}

type fileState struct {
	exists bool
	digest [sha256.Size]byte
	info   fs.FileInfo
}

func (f fileState) equal(other fileState) bool {
	if f.exists != other.exists || f.digest != other.digest {
		return false
	}
	if !f.exists {
		return true
	}
	return os.SameFile(f.info, other.info) && f.info.Mode() == other.info.Mode() && f.info.Size() == other.info.Size() && f.info.ModTime().Equal(other.info.ModTime())
}

// ErrTooLarge reports a file larger than the byte limit its reader was given.
var ErrTooLarge = errors.New("execution file exceeds the read limit")

// Read through os.Root and the established segment walker. The descriptor is
// regular and nonblocking; a second open detects replacement or content changes
// while the first read was in flight, even without a Tiller generation update.
func regularFile(root, path string, keepBytes bool) ([]byte, fileState, error) {
	return regularFileAtMost(root, path, keepBytes, 0)
}

// regularFileAtMost is regularFile with a byte limit: a positive limit refuses
// a larger file with ErrTooLarge, counting bytes as both reads consume them
// rather than trusting a stat, so a growing file is caught too and no read
// ever consumes more than limit+1 bytes. Zero means no limit.
func regularFileAtMost(root, path string, keepBytes bool, limit int64) ([]byte, fileState, error) {
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return nil, fileState{}, err
	}
	defer anchored.Close()
	read := func(keepBytes bool) ([]byte, fileState, error) {
		file, err := (rootpath.Walker{}).Open(anchored, path, false)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fileState{}, nil
		}
		if err != nil {
			return nil, fileState{}, err
		}
		defer file.Close()
		before, err := file.Stat()
		if err != nil {
			return nil, fileState{}, err
		}
		var content io.Reader = file
		if limit > 0 {
			content = io.LimitReader(file, limit+1)
		}
		var data []byte
		var digest [sha256.Size]byte
		var read int64
		if keepBytes {
			data, err = io.ReadAll(content)
			if err == nil {
				read = int64(len(data))
				digest = sha256.Sum256(data)
			}
		} else {
			hash := sha256.New()
			read, err = io.Copy(hash, content)
			if err == nil {
				hash.Sum(digest[:0])
			}
		}
		if err != nil {
			return nil, fileState{}, err
		}
		if limit > 0 && read > limit {
			return nil, fileState{}, fmt.Errorf("execution file %s: %w", path, ErrTooLarge)
		}
		after, err := file.Stat()
		if err != nil {
			return nil, fileState{}, err
		}
		if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return nil, fileState{}, fmt.Errorf("execution file %s changed while being read", path)
		}
		return data, fileState{exists: true, digest: digest, info: after}, nil
	}
	data, before, err := read(keepBytes)
	if err != nil {
		return nil, fileState{}, err
	}
	_, after, err := read(false)
	if err != nil {
		return nil, fileState{}, err
	}
	if !before.equal(after) {
		return nil, fileState{}, fmt.Errorf("execution file %s changed while being read", path)
	}
	return data, after, nil
}

func generation(root, prefix string) (fileState, error) {
	for _, path := range []string{".tiller/.cache/lock", ".tiller/.cache/journal.yml"} {
		path = filepath.Join(prefix, path)
		exists, err := store.PathExistsNoSymlinks(root, filepath.Join(root, path))
		if err != nil {
			return fileState{}, err
		}
		if exists {
			return fileState{}, fmt.Errorf("execution state is unavailable while %s is present", path)
		}
	}
	_, state, err := regularFile(root, filepath.Join(prefix, ".tiller/.cache/generation"), false)
	return state, err
}

// captureWorktree records the worktree boundary an observation is read under.
// It does not recheck it: the caller closes every read with one stability pass.
func (s *source) captureWorktree() error {
	common, head, branch, index, err := worktreeHead(s.worktreeRoot)
	if err != nil {
		return err
	}
	if common != s.repo.common {
		return errors.New("execution worktree does not belong to the served Git common directory")
	}
	if branch != "refs/heads/"+s.branch {
		return errors.New("execution worktree no longer holds its discovered task branch")
	}
	if head != s.oid {
		return errors.New("execution worktree HEAD changed since ref discovery")
	}
	s.entries, err = commitTrees.inventory(s.worktreeRoot, s.repo.common, s.oid)
	if err != nil {
		return err
	}
	if err := s.captureIndex(index); err != nil {
		return err
	}
	if err := s.checkWorktreePath(s.taskPath); err != nil {
		return err
	}
	s.generation, err = generation(s.worktreeRoot, filepath.FromSlash(s.repo.projectPrefix))
	if err != nil {
		return err
	}
	_, state, err := regularFile(s.worktreeRoot, s.gitPath(s.taskPath), false)
	if err != nil {
		return err
	}
	if !state.exists {
		return fmt.Errorf("execution task record %s is unavailable", s.taskPath)
	}
	s.files[s.taskPath] = state
	// Only the project's own Tiller state decides whether the observation is
	// uncommitted; unrelated code edits in the worktree do not.
	status, err := gitRead(s.worktreeRoot, "--literal-pathspecs", "status", "--porcelain", "-z", "--untracked-files=normal", "--ignore-submodules=all", "--", s.gitPath(".tiller"))
	if err != nil {
		return err
	}
	s.uncommitted = len(status) != 0
	return nil
}

func (s *source) revision(taskID string) string {
	hash := sha256.New()
	for _, value := range []string{s.kind, s.repo.common, s.repo.projectPrefix, s.branch, s.ref, s.oid, s.root, taskID} {
		hash.Write([]byte(value))
		hash.Write([]byte{0})
	}
	if s.kind == "working_tree" {
		task := s.files[s.taskPath]
		hash.Write(task.digest[:])
		hash.Write(s.generation.digest[:])
	}
	return s.kind + ":" + s.oid + ":" + hex.EncodeToString(hash.Sum(nil))
}

// watchedRefs names both tips of a source, including the absent local
// counterpart of a tracking observation, so a new or moved ref cannot silently
// switch the selected source.
func (s *source) watchedRefs() []string {
	names := []string{"refs/heads/" + s.branch}
	if s.remoteRef != "" {
		names = append(names, s.remoteRef)
	}
	return names
}

func (s *source) checkRefs(oids map[string]string) error {
	want := map[string]string{"refs/heads/" + s.branch: s.repo.refs["refs/heads/"+s.branch].oid}
	if s.remoteRef != "" {
		want[s.remoteRef] = s.remoteOID
	}
	for name, oid := range want {
		if oids[name] != oid {
			return fmt.Errorf("execution ref %s changed; refresh the observation", name)
		}
	}
	return nil
}

func (s *source) checkWorktreeIdentity() error {
	common, head, branch, _, err := worktreeHead(s.worktreeRoot)
	if err != nil {
		return err
	}
	if common != s.repo.common {
		return errors.New("execution worktree Git identity changed")
	}
	if branch != "refs/heads/"+s.branch {
		return errors.New("execution worktree branch changed; refresh the observation")
	}
	if head != s.oid {
		return errors.New("execution worktree HEAD changed; refresh the observation")
	}
	unchanged, err := s.indexUnchanged()
	if err != nil {
		return err
	}
	if !unchanged {
		return errors.New("execution Git index changed; refresh the observation")
	}
	return nil
}

// checkFiles confirms, without Git, that every worktree file and event
// directory read under this source is unchanged, bracketed by its generation.
func (s *source) checkFiles() error {
	if s.kind != "working_tree" {
		return nil
	}
	currentGeneration, err := generation(s.worktreeRoot, filepath.FromSlash(s.repo.projectPrefix))
	if err != nil {
		return err
	}
	if !s.generation.equal(currentGeneration) {
		return errors.New("execution generation changed; refresh the observation")
	}
	for path, expected := range s.files {
		if err := s.checkWorktreePath(path); err != nil {
			return err
		}
		_, actual, err := regularFile(s.worktreeRoot, s.gitPath(path), false)
		if err != nil {
			return err
		}
		if !expected.equal(actual) {
			return fmt.Errorf("execution file %s changed; refresh the observation", path)
		}
	}
	for path, expected := range s.directories {
		if err := s.checkWorktreePath(path); err != nil {
			return err
		}
		_, actual, err := directory(s.worktreeRoot, s.gitPath(path))
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("execution event directory %s changed; refresh the observation", path)
		}
	}
	// Recheck the generation after supplemental file reads as well.
	currentGeneration, err = generation(s.worktreeRoot, filepath.FromSlash(s.repo.projectPrefix))
	if err != nil {
		return err
	}
	if !s.generation.equal(currentGeneration) {
		return errors.New("execution generation changed while being read")
	}
	return nil
}

func (s *source) read(path string) ([]byte, error) {
	return s.readAtMost(path, 0)
}

// readAtMost reads one source file, refusing with ErrTooLarge a file larger
// than a positive limit before its bytes are loaded. Zero means no limit.
func (s *source) readAtMost(path string, limit int64) ([]byte, error) {
	if !fs.ValidPath(filepath.ToSlash(path)) || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\\\x00") {
		return nil, fmt.Errorf("unsafe execution path %q", path)
	}
	if s.kind == "working_tree" {
		if err := s.checkWorktreePath(path); err != nil {
			return nil, err
		}
		data, state, err := regularFileAtMost(s.worktreeRoot, s.gitPath(path), true, limit)
		if err != nil {
			return nil, err
		}
		if !state.exists {
			return nil, fmt.Errorf("execution file %s: %w", path, fs.ErrNotExist)
		}
		if expected, ok := s.files[path]; ok && !expected.equal(state) {
			return nil, fmt.Errorf("execution file %s changed; refresh the observation", path)
		}
		s.files[path] = state
		return data, nil
	}
	name := s.gitPath(path)
	entry, ok := s.entries[name]
	if !ok {
		return nil, fmt.Errorf("execution file %s: %w", path, fs.ErrNotExist)
	}
	if entry.kind != "blob" || (entry.mode != "100644" && entry.mode != "100755") {
		return nil, fmt.Errorf("execution file %s is not a regular Git blob", path)
	}
	for parent := filepath.ToSlash(filepath.Dir(name)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
		entry, ok := s.entries[parent]
		if !ok || entry.kind != "tree" || entry.mode != "040000" {
			return nil, fmt.Errorf("execution file %s has an unsafe Git tree component", path)
		}
	}
	if limit > 0 {
		// A blob is immutable under its object id, so its recorded size is
		// the size the read below returns. That read still caps its own
		// output, so the limit never rests on the size check alone.
		size, err := gitText(s.repo.root, "cat-file", "-s", entry.oid)
		if err != nil {
			return nil, err
		}
		blobSize, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("execution file %s has an unreadable Git blob size: %w", path, err)
		}
		if blobSize > limit {
			return nil, fmt.Errorf("execution file %s: %w", path, ErrTooLarge)
		}
		data, err := gitReadAtMost(s.repo.root, limit, "cat-file", "blob", entry.oid)
		if errors.Is(err, ErrTooLarge) {
			return nil, fmt.Errorf("execution file %s: %w", path, ErrTooLarge)
		}
		return data, err
	}
	return gitRead(s.repo.root, "cat-file", "blob", entry.oid)
}

func evidencePath(path string) error {
	if !fs.ValidPath(path) || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path || strings.ContainsAny(path, "\\\x00") {
		return fmt.Errorf("unsafe execution evidence path %q", path)
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("execution evidence path %q enters a hidden component", path)
		}
	}
	return nil
}

func directory(root, path string) ([]fs.DirEntry, string, error) {
	entries, err := store.ReadRepositoryDirectory(root, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "absent", nil
	}
	if err != nil {
		return nil, "", err
	}
	var state strings.Builder
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("execution event path %s/%s is a symlink", path, entry.Name())
		}
		fmt.Fprintf(&state, "%s\x00%d\x00", entry.Name(), entry.Type())
	}
	return entries, "present\x00" + state.String(), nil
}

func (s *source) eventPaths() ([]string, error) {
	eventsRoot := s.gitPath(".tiller/events")
	var paths []string
	if s.kind != "working_tree" {
		for path, entry := range s.entries {
			if !strings.HasPrefix(path, eventsRoot+"/") {
				continue
			}
			if entry.kind != "tree" && entry.kind != "blob" {
				return nil, fmt.Errorf("execution event path %s is not a regular Git blob or directory", path)
			}
			if entry.kind == "blob" && entry.mode != "100644" && entry.mode != "100755" {
				return nil, fmt.Errorf("execution event path %s is not a regular Git blob", path)
			}
			if entry.kind == "blob" && strings.HasSuffix(path, ".yml") {
				projectPath := strings.TrimPrefix(path, s.repo.projectPrefix+"/")
				if s.repo.projectPrefix == "" {
					projectPath = path
				}
				paths = append(paths, filepath.FromSlash(projectPath))
			}
		}
		// A symlink/gitlink at the events root must not become an empty history.
		if entry, ok := s.entries[eventsRoot]; ok && (entry.kind != "tree" || entry.mode != "040000") {
			return nil, errors.New("execution events root is not a Git directory")
		}
		sort.Strings(paths)
		return paths, nil
	}
	var walk func(string) error
	walk = func(path string) error {
		if err := s.checkWorktreePath(path); err != nil {
			return err
		}
		entries, state, err := directory(s.worktreeRoot, s.gitPath(path))
		if err != nil {
			return err
		}
		if state == "absent" && path != ".tiller/events" {
			return fmt.Errorf("execution event directory %s disappeared", path)
		}
		if expected, ok := s.directories[path]; ok && expected != state {
			return fmt.Errorf("execution event directory %s changed", path)
		}
		s.directories[path] = state
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			if err := s.checkWorktreePath(child); err != nil {
				return err
			}
			if entry.IsDir() {
				if err := walk(child); err != nil {
					return err
				}
			} else if strings.HasSuffix(entry.Name(), ".yml") {
				paths = append(paths, child)
			}
		}
		return nil
	}
	if err := walk(".tiller/events"); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *source) events(taskID string) ([]events.Event, error) {
	paths, err := s.eventPaths()
	if err != nil {
		return nil, err
	}
	out := make([]events.Event, 0)
	for _, path := range paths {
		data, err := s.read(path)
		if err != nil {
			return nil, err
		}
		var event events.Event
		if err := yaml.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("execution event %s: %w", path, err)
		}
		// Another task's event cannot make this task's history incomplete.
		if !slices.Contains(event.Targets, taskID) {
			continue
		}
		if !id.Valid(event.ID) || !strings.HasPrefix(event.ID, "EV-") || filepath.Base(path) != event.ID+".yml" || event.At.IsZero() || event.Type == "" {
			return nil, fmt.Errorf("execution event %s has invalid identity, timestamp, or type", path)
		}
		out = append(out, event)
	}
	events.Sort(out)
	return out, nil
}

// hasEvent reports whether the source carries an event file at its dated path.
func (s *source) hasEvent(event events.Event) (bool, error) {
	_, err := s.read(events.RelPath(event))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

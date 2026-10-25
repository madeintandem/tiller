package vcs

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
)

// WorktreeSurface observes current bytes beneath a caller-approved directory
// anchor. relativePath must be canonical and cannot escape that anchor;
// recordedPath supplies the project's coordinates for the returned inventory.
// A canonical declaration alias may be used as the anchor itself.
//
// Directory inventories include tracked and untracked nonignored Git files,
// excluding vanished files. Without Git they include every regular file. An
// explicitly named file is included even when ignored. Both policies prune
// .git and .tiller at every depth and omit empty nested directories. Future
// baseline comparisons must use this same policy, not HEAD's tree inventory.
// Hashes use raw Git SHA1 blob/tree framing, without filters or object/index
// writes. This observation does not lock arbitrary editors or external Git.
func WorktreeSurface(anchorRoot, relativePath, recordedPath string, at time.Time) (store.SurfaceFingerprint, error) {
	anchor, file, canonicalRoot, err := openWorktreeSurface(anchorRoot, relativePath, recordedPath)
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	defer anchor.Close()
	defer file.Close()
	anchorRoot = canonicalRoot
	info, err := file.Stat()
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	observer := surfaceObserver{recorded: filepath.ToSlash(recordedPath), base: relativePath}
	result := store.SurfaceFingerprint{Path: observer.recorded, ObservedAt: at}
	if info.Mode().IsRegular() {
		hash, err := observer.blob(file, info)
		if err != nil {
			return store.SurfaceFingerprint{}, err
		}
		result.Kind = store.KindGitBlob
		result.Value = "git:" + hex.EncodeToString(hash[:])
		result.Inventory = []string{observer.recorded}
		return result, nil
	}

	observer.included, err = surfaceGitInventory(anchorRoot, relativePath)
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	if observer.included != nil {
		observer.directories = make(map[string]bool)
		observer.folded = make(map[string][]string)
		for name := range observer.included {
			observer.folded[strings.ToLower(name)] = append(observer.folded[strings.ToLower(name)], name)
			for parent := path.Dir(name); ; parent = path.Dir(parent) {
				if !observer.directories[parent] {
					observer.directories[parent] = true
					observer.folded[strings.ToLower(parent)] = append(observer.folded[strings.ToLower(parent)], parent)
				}
				if parent == "." {
					break
				}
			}
		}
	}
	root, err := (rootpath.Walker{}).OpenRoot(anchor, relativePath)
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	defer root.Close()
	bound, err := root.Stat(".")
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	if !os.SameFile(info, bound) {
		return store.SurfaceFingerprint{}, fmt.Errorf("%w: surface %q changed while opening", rootpath.ErrUnsafe, relativePath)
	}
	hash, _, err := observer.tree(root, relativePath)
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	sort.Strings(observer.inventory)
	result.Kind = store.KindGitTree
	result.Value = "git:" + hex.EncodeToString(hash[:])
	result.Inventory = observer.inventory
	return result, nil
}

// CheckWorktreeSurface checks the same safe named path and metadata boundary as
// WorktreeSurface without computing an inventory or content hashes.
func CheckWorktreeSurface(anchorRoot, relativePath, recordedPath string) error {
	anchor, file, _, err := openWorktreeSurface(anchorRoot, relativePath, recordedPath)
	if err != nil {
		return err
	}
	file.Close()
	anchor.Close()
	return nil
}

func openWorktreeSurface(anchorRoot, relativePath, recordedPath string) (*os.Root, *os.File, string, error) {
	if surfaceMetadata(anchorRoot) || surfaceMetadata(relativePath) || surfaceMetadata(recordedPath) {
		return nil, nil, "", fmt.Errorf("surface %q is Tiller or Git metadata", recordedPath)
	}
	// Resolve only the caller-approved declaration alias.
	canonical, err := filepath.Abs(anchorRoot)
	if err != nil {
		return nil, nil, "", err
	}
	canonical, err = filepath.EvalSymlinks(canonical)
	if err != nil {
		return nil, nil, "", err
	}
	metadata, err := surfaceMetadataAnchor(canonical)
	if err != nil {
		return nil, nil, "", err
	}
	if metadata {
		return nil, nil, "", fmt.Errorf("surface %q is Tiller or Git metadata", recordedPath)
	}
	anchor, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, nil, "", err
	}
	file, err := (rootpath.Walker{}).Open(anchor, relativePath, true)
	if err != nil {
		anchor.Close()
		return nil, nil, "", err
	}
	metadata, err = surfaceMetadataWithin(anchor, relativePath)
	if err == nil && metadata {
		err = fmt.Errorf("surface %q is Tiller or Git metadata", recordedPath)
	}
	if err != nil {
		file.Close()
		anchor.Close()
		return nil, nil, "", err
	}
	return anchor, file, canonical, nil
}

type surfaceObserver struct {
	base        string
	recorded    string
	included    map[string]string // Git index mode; nil means external, non-Git policy
	directories map[string]bool
	folded      map[string][]string // lowercased path to every inventory spelling; consulted only on a miss
	inventory   []string
	buffer      [32 * 1024]byte
}

type surfaceTreeEntry struct {
	name string
	mode string
	hash [sha1.Size]byte
}

func (o *surfaceObserver) tree(root *os.Root, relative string) ([sha1.Size]byte, bool, error) {
	var zero [sha1.Size]byte
	if o.included[filepath.ToSlash(relative)] == "160000" {
		return zero, false, fmt.Errorf("surface %q is a Git submodule; declare its working tree as a separate code root", relative)
	}
	dir, err := root.Open(".")
	if err != nil {
		return zero, false, err
	}
	children, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return zero, false, err
	}
	entries := make([]surfaceTreeEntry, 0, len(children))
	walker := rootpath.Walker{}
	for _, child := range children {
		metadata, err := surfaceMetadataWithin(root, child.Name())
		if err != nil {
			return zero, false, err
		}
		if metadata {
			continue
		}
		rel := path.Join(filepath.ToSlash(relative), child.Name())
		if o.included != nil {
			mode, member, err := o.member(root, rel, child.Name())
			if err != nil {
				return zero, false, err
			}
			if !member {
				continue
			}
			if mode == "160000" {
				return zero, false, fmt.Errorf("surface %q is a Git submodule; declare its working tree as a separate code root", rel)
			}
		}
		file, err := walker.Open(root, child.Name(), true)
		if err != nil {
			return zero, false, err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return zero, false, err
		}
		entry := surfaceTreeEntry{name: child.Name(), mode: "100644"}
		if info.IsDir() {
			file.Close()
			nested, err := walker.OpenRoot(root, child.Name())
			if err != nil {
				return zero, false, err
			}
			bound, err := nested.Stat(".")
			if err != nil || !os.SameFile(info, bound) {
				nested.Close()
				if err != nil {
					return zero, false, err
				}
				return zero, false, fmt.Errorf("%w: directory %q changed while opening", rootpath.ErrUnsafe, rel)
			}
			hash, nonempty, err := o.tree(nested, rel)
			nested.Close()
			if err != nil {
				return zero, false, err
			}
			if !nonempty {
				continue
			}
			entry.mode, entry.hash = "40000", hash
		} else {
			entry.hash, err = o.blob(file, info)
			file.Close()
			if err != nil {
				return zero, false, err
			}
			if info.Mode()&0o100 != 0 {
				entry.mode = "100755"
			}
			within, err := filepath.Rel(o.base, filepath.FromSlash(rel))
			if err != nil {
				return zero, false, err
			}
			o.inventory = append(o.inventory, path.Join(o.recorded, filepath.ToSlash(within)))
		}
		entries = append(entries, entry)
	}
	// Git sorts directory entries as though their names end in '/', not NUL.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i].name, entries[j].name
		if entries[i].mode == "40000" {
			a += "/"
		}
		if entries[j].mode == "40000" {
			b += "/"
		}
		return a < b
	})
	size := 0
	for _, entry := range entries {
		size += len(entry.mode) + 1 + len(entry.name) + 1 + sha1.Size
	}
	h := sha1.New()
	fmt.Fprintf(h, "tree %d\x00", size)
	for _, entry := range entries {
		io.WriteString(h, entry.mode)
		io.WriteString(h, " ")
		io.WriteString(h, entry.name)
		io.WriteString(h, "\x00")
		h.Write(entry.hash[:])
	}
	h.Sum(zero[:0])
	return zero, len(entries) != 0, nil
}

// member reports whether the directory entry name, at inventory path rel, is a
// tracked or untracked nonignored file (with its index mode) or an ancestor of
// one. Git's inventory keeps the index spelling, so after an unstaged case-only
// rename on a case-insensitive filesystem the listing and the inventory
// disagree. A case variant counts only when this directory resolves both
// spellings to the same entry; folding alone would merge distinct files on a
// case-sensitive filesystem.
func (o *surfaceObserver) member(root *os.Root, rel, name string) (string, bool, error) {
	if mode := o.included[rel]; mode != "" {
		return mode, true, nil
	}
	if o.directories[rel] {
		return "", true, nil
	}
	// Every ancestor of root was verified the same way, so an alias differing
	// only in an ancestor's spelling is checked here by its own base name.
	for _, alias := range o.folded[strings.ToLower(rel)] {
		aliasName := path.Base(alias)
		actual, err := root.Lstat(name)
		if err != nil {
			return "", false, err
		}
		indexed, err := root.Lstat(aliasName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", false, err
		}
		if !os.SameFile(actual, indexed) {
			continue
		}
		if mode := o.included[alias]; mode != "" {
			return mode, true, nil
		}
		return "", true, nil
	}
	return "", false, nil
}

func (o *surfaceObserver) blob(file *os.File, info os.FileInfo) ([sha1.Size]byte, error) {
	var hash [sha1.Size]byte
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", info.Size())
	// Hide File.WriteTo so every file shares the observer's copy buffer.
	n, err := io.CopyBuffer(h, struct{ io.Reader }{file}, o.buffer[:])
	if err != nil {
		return hash, err
	}
	after, err := file.Stat()
	if err != nil {
		return hash, err
	}
	if n != info.Size() || after.Size() != info.Size() || after.Mode() != info.Mode() || !after.ModTime().Equal(info.ModTime()) {
		return hash, fmt.Errorf("surface file %q changed while reading", file.Name())
	}
	h.Sum(hash[:0])
	return hash, nil
}

func surfaceMetadata(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == ".git" || part == ".tiller" {
			return true
		}
	}
	return false
}

// The anchor is absolute and resolved; only metadata-shaped ancestors need an
// identity check for filesystem case aliases. Other external roots stay valid.
func surfaceMetadataAnchor(anchor string) (bool, error) {
	if surfaceMetadata(anchor) {
		return true, nil
	}
	for {
		parent := filepath.Dir(anchor)
		if parent == anchor {
			return false, nil
		}
		name := filepath.Base(anchor)
		if strings.EqualFold(name, ".git") || strings.EqualFold(name, ".tiller") {
			root, err := os.OpenRoot(parent)
			if err != nil {
				return false, err
			}
			metadata, err := surfaceMetadataWithin(root, name)
			root.Close()
			if err != nil || metadata {
				return metadata, err
			}
		}
		anchor = parent
	}
}

// A case variant is metadata only when this filesystem resolves it to the
// canonical metadata entry. EqualFold alone would hide distinct code on a
// case-sensitive filesystem. Each parent is opened beneath the existing anchor.
func surfaceMetadataWithin(root *os.Root, name string) (bool, error) {
	if surfaceMetadata(name) {
		return true, nil
	}
	prefix := "."
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		prefix = path.Join(prefix, part)
		canonical := ""
		switch {
		case strings.EqualFold(part, ".git"):
			canonical = ".git"
		case strings.EqualFold(part, ".tiller"):
			canonical = ".tiller"
		default:
			continue
		}
		parent, err := (rootpath.Walker{}).OpenRoot(root, path.Dir(prefix))
		if err != nil {
			return false, err
		}
		actual, actualErr := parent.Lstat(part)
		metadata, metadataErr := parent.Lstat(canonical)
		parent.Close()
		if actualErr != nil {
			return false, actualErr
		}
		if errors.Is(metadataErr, os.ErrNotExist) {
			continue
		}
		if metadataErr != nil {
			return false, metadataErr
		}
		if os.SameFile(actual, metadata) {
			return true, nil
		}
	}
	return false, nil
}

// A failed Git invocation is never an empty inventory. Only Git's specific
// no-repository discovery failure, with no repository marker or explicit Git
// environment, permits the external-tree policy.
func surfaceGitInventory(anchorRoot, relativePath string) (map[string]string, error) {
	// Discover from the selected directory so an explicitly described submodule
	// uses its own working-tree inventory, not its parent's gitlink.
	g := New(filepath.Join(anchorRoot, relativePath))
	inside, err := g.run("rev-parse", "--is-inside-work-tree")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && strings.Contains(ge.stderr, "not a git repository (or any") && surfaceHasNoGit(g.Dir) {
			return nil, nil
		}
		return nil, g.asEnv(err)
	}
	if inside != "true" {
		return nil, g.unavailable("surface is not in a Git working tree")
	}
	tracked, err := g.runRaw("--literal-pathspecs", "ls-files", "--cached", "--stage", "-z", "--", ".")
	if err != nil {
		return nil, g.asEnv(err)
	}
	untracked, err := g.runRaw("--literal-pathspecs", "ls-files", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return nil, g.asEnv(err)
	}
	files := make(map[string]string)
	include := func(name, mode string) error {
		if surfaceMetadata(name) {
			return nil
		}
		if !filepath.IsLocal(name) || filepath.ToSlash(filepath.Clean(name)) != name {
			return g.unavailable("Git returned a noncanonical surface path")
		}
		files[path.Join(filepath.ToSlash(relativePath), name)] = mode
		return nil
	}
	for _, record := range strings.Split(tracked, "\x00") {
		if record == "" {
			continue
		}
		header, name, ok := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 {
			return nil, g.unavailable("Git returned an invalid index entry")
		}
		if err := include(name, fields[0]); err != nil {
			return nil, err
		}
	}
	for _, name := range strings.Split(untracked, "\x00") {
		if name != "" {
			if err := include(name, "100644"); err != nil {
				return nil, err
			}
		}
	}
	return files, nil
}

func surfaceHasNoGit(dir string) bool {
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM"} {
		if _, set := os.LookupEnv(key); set {
			return false
		}
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	// Follow only the caller-approved anchor alias, so marker discovery agrees
	// with Git's physical working directory. Content reads remain anchored.
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Lstat(filepath.Join(absolute, ".git")); !errors.Is(err, os.ErrNotExist) {
			return false
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return true
		}
		absolute = parent
	}
}

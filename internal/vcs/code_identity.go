package vcs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CodeIdentity returns "sha256:<64 hex>", a content digest of the project's code
// that gate reuse compares to decide whether a command already passed over these
// same files. It answers "same code?", where WorktreeFingerprint answers "which
// exact tree?": it hashes the content of every tracked or untracked non-ignored
// file, not HEAD, so the same bytes give the same value whether committed or
// dirty, and moving HEAD without touching a file does not move it.
//
// Tiller's own writes are left out so recording a result never invalidates it:
// the project's state tree (`<prefix>.tiller/`) and its gate evidence
// (`<prefix>artifacts/tiller/`), where prefix is the project's directory inside
// the repository ("" for a project at the root). Another project's state in the
// same repository is code to this one and stays in.
//
// It uses WorktreeFingerprint's coordinate system, the repository root, and finds
// the project directory by walking up from g.Dir to the nearest `.tiller/`, so
// the value is the same from any directory in the project. It starts from HEAD's full
// tree (`ls-tree -r -z --full-tree`, skipped when HEAD is unborn), mapping each
// path to "<mode> <oid>", then overlays every path that differs from HEAD or is
// untracked and not ignored, read through worktreeIdentity with a mode taken from
// Lstat (a deleted path drops out). The sorted "path\x00mode oid" lines are
// hashed. Read-only like WorktreeFingerprint: no index, object, stash, or ref is
// written, and a changed path that exists but cannot be read is the same
// worktree_unreadable error, never a deletion.
//
// HEAD blobs are a safe shortcut only when Git cannot normalize away working-tree
// byte differences. If check-in conversion is configured for tracked code, it
// returns "" instead of trusting Git's normalized diff or hashing the entire
// checkout on every check. Attributes are resolved by Git from the current
// worktree (with its index fallback), including local and global overrides.
//
// A repository inside the checkout is opaque to it: a submodule's gitlink
// records only a commit, and an untracked nested repository reads as one
// directory, so edits inside either would never move the identity. Rather
// than grant reuse over code it cannot see, it returns "" (no identity, which
// never matches: every gate runs, and resume stales every gate) whenever
// HEAD's tree, or on an unborn HEAD the index, carries a 160000 gitlink
// outside the excluded prefixes, or the worktree overlay meets a directory
// standing at a path (directoryIdentity).
func (g *Git) CodeIdentity() (string, error) {
	top, err := g.runRaw("rev-parse", "--show-toplevel")
	if err != nil {
		return "", g.asEnv(err)
	}
	repo := &Git{Dir: strings.TrimSuffix(top, "\n"), Clock: g.Clock}

	prefix, err := g.runRaw("rev-parse", "--show-prefix")
	if err != nil {
		return "", g.asEnv(err)
	}
	prefix = projectPrefix(repo.Dir, strings.TrimSuffix(prefix, "\n"))
	excluded := []string{prefix + ".tiller/", prefix + "artifacts/tiller/"}

	entries := map[string]string{}
	if _, err := repo.run("rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		tree, err := repo.runRaw("--no-optional-locks", "ls-tree", "-r", "-z", "--full-tree", "HEAD")
		if err != nil {
			return "", g.asEnv(err)
		}
		for _, rec := range strings.Split(tree, "\x00") {
			// "<mode> SP <type> SP <oid> TAB <path>"; the path is everything after
			// the first tab, so a path containing spaces or tabs stays whole.
			meta, p, ok := strings.Cut(rec, "\t")
			if !ok {
				continue
			}
			fields := strings.Fields(meta)
			if len(fields) != 3 {
				continue
			}
			if fields[0] == gitlinkMode && !isExcluded(p, excluded) {
				return "", nil
			}
			entries[p] = fields[0] + " " + fields[2]
		}
		if converts, err := repo.codeCheckinConversion(entries, excluded); err != nil || converts {
			return "", err
		}

		// --ignore-submodules=none overrides diff.ignoreSubmodules and a
		// .gitmodules ignore setting, so a gitlink staged since HEAD is listed
		// and the overlay meets it as an opaque directory.
		diff, err := repo.runRaw("--no-optional-locks", "diff", "--name-only", "--no-renames", "--ignore-submodules=none", "-z", "HEAD")
		if err != nil {
			return "", g.asEnv(err)
		}
		changed := map[string]struct{}{}
		addNULPaths(changed, diff)
		if opaque, err := overlayWorktree(repo.Dir, entries, changed, excluded); err != nil || opaque {
			return "", err
		}
	} else {
		// An unborn HEAD has no tree to diff against, so a file staged for the first
		// commit is in neither `diff HEAD` nor `ls-files --others`. Read every
		// indexed path from the working tree instead.
		cached, err := repo.runRaw("--no-optional-locks", "ls-files", "--stage", "-z")
		if err != nil {
			return "", g.asEnv(err)
		}
		staged := map[string]struct{}{}
		for _, rec := range strings.Split(cached, "\x00") {
			// "<mode> SP <oid> SP <stage> TAB <path>".
			meta, p, ok := strings.Cut(rec, "\t")
			if !ok {
				continue
			}
			if strings.HasPrefix(meta, gitlinkMode+" ") && !isExcluded(p, excluded) {
				return "", nil
			}
			staged[p] = struct{}{}
		}
		if opaque, err := overlayWorktree(repo.Dir, entries, staged, excluded); err != nil || opaque {
			return "", err
		}
	}

	others, err := repo.runRaw("--no-optional-locks", "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", g.asEnv(err)
	}
	untracked := map[string]struct{}{}
	addNULPaths(untracked, others)
	if opaque, err := overlayWorktree(repo.Dir, entries, untracked, excluded); err != nil || opaque {
		return "", err
	}

	sorted := make([]string, 0, len(entries))
	for p := range entries {
		if !isExcluded(p, excluded) {
			sorted = append(sorted, p)
		}
	}
	sort.Strings(sorted)

	h := sha256.New()
	// Retire records produced before conversion safety was established, including
	// records made under conversion settings that have since been removed.
	h.Write([]byte("tiller-code-identity-v2\x00"))
	for _, p := range sorted {
		fmt.Fprintf(h, "%s\x00%s\n", p, entries[p])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// codeCheckinConversion checks the HEAD paths whose raw bytes the fast path
// would otherwise trust without reading. New/index-only and untracked paths are
// already read raw by the overlay, including on an unborn HEAD.
func (g *Git) codeCheckinConversion(entries map[string]string, excluded []string) (bool, error) {
	config, err := g.runRaw("config", "--null", "--get-regexp", `^(core\.autocrlf|filter\.(unset|unspecified)\.(clean|process))$`)
	if err != nil {
		var ge *gitError
		if !errors.As(err, &ge) || ge.exitCode != 1 {
			return false, g.asEnv(err)
		}
	}
	autocrlf := "false"
	for _, record := range strings.Split(config, "\x00") {
		key, value, _ := strings.Cut(record, "\n")
		switch key {
		case "":
		case "core.autocrlf":
			autocrlf = value
		default:
			// check-attr cannot distinguish its unset/unspecified sentinels
			// from literal filter names. A configured driver for either name
			// therefore makes the normalized-diff shortcut unsafe.
			return true, nil
		}
	}
	switch strings.ToLower(autocrlf) {
	case "false", "no", "off", "0":
	default:
		return true, nil
	}

	var paths bytes.Buffer
	for p := range entries {
		if !isExcluded(p, excluded) {
			paths.WriteString(p)
			paths.WriteByte(0)
		}
	}
	if paths.Len() == 0 {
		return false, nil
	}
	var attrs bytes.Buffer
	if err := g.runCmd(&paths, &attrs, "check-attr", "-z", "--stdin",
		"text", "eol", "crlf", "filter", "ident", "working-tree-encoding"); err != nil {
		return false, g.asEnv(err)
	}
	// -z yields path, attribute, value triples. Any effective conversion
	// attribute costs reuse, even when a filter is not currently configured.
	fields := strings.Split(strings.TrimSuffix(attrs.String(), "\x00"), "\x00")
	for i := 2; i < len(fields); i += 3 {
		if fields[i] != "unspecified" && fields[i] != "unset" {
			return true, nil
		}
	}
	return false, nil
}

// projectPrefix is the repository-relative directory (trailing slash, "" at the
// root) of the Tiller project that observed lies in, observed being g.Dir's own
// prefix. It is the nearest enclosing directory holding a `.tiller/`, so a
// subdirectory of the project resolves to the project and not to itself; with no
// `.tiller/` on the way up it is observed unchanged.
func projectPrefix(top, observed string) string {
	for p := observed; ; {
		if info, err := os.Stat(filepath.Join(top, filepath.FromSlash(p), ".tiller")); err == nil && info.IsDir() {
			return p
		}
		if p == "" {
			return observed
		}
		p = strings.TrimSuffix(p, "/")
		if i := strings.LastIndex(p, "/"); i >= 0 {
			p = p[:i+1]
		} else {
			p = ""
		}
	}
}

// gitlinkMode is the Git tree mode of a submodule's commit entry.
const gitlinkMode = "160000"

// overlayWorktree replaces entries[p] with each path's working-tree "<mode> <oid>"
// (deleting it when the path is gone), skipping excluded paths so Tiller's own
// state is never even read. It reports opaque, and stops, when a path is a
// directory (a submodule or an untracked nested repository), whose contents
// it cannot see.
func overlayWorktree(dir string, entries map[string]string, paths map[string]struct{}, excluded []string) (opaque bool, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, unreadable(".", err)
	}
	defer root.Close()

	for p := range paths {
		if isExcluded(p, excluded) {
			continue
		}
		oid, err := worktreeIdentity(root, p)
		if err != nil {
			return false, unreadable(p, err)
		}
		switch oid {
		case deletedIdentity:
			delete(entries, p)
			continue
		case directoryIdentity:
			return true, nil
		}
		mode, err := worktreeMode(root, p)
		if err != nil {
			return false, unreadable(p, err)
		}
		entries[p] = mode + " " + oid
	}
	return false, nil
}

// worktreeMode is the Git file mode of the entry at name beneath root: 120000 for
// a symlink, 160000 for a directory (a submodule or nested repository), 100755
// when any execute bit is set, else 100644.
func worktreeMode(root *os.Root, name string) (string, error) {
	info, err := root.Lstat(strings.TrimSuffix(name, "/"))
	if err != nil {
		return "", err
	}
	switch mode := info.Mode(); {
	case mode&fs.ModeSymlink != 0:
		return "120000", nil
	case mode.IsDir():
		return "160000", nil
	case mode.Perm()&0o111 != 0:
		return "100755", nil
	}
	return "100644", nil
}

func isExcluded(p string, excluded []string) bool {
	for _, prefix := range excluded {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

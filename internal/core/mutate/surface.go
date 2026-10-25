package mutate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
)

const (
	eventLinkSurfaceEpic      = "link.surface.epic"
	eventLinkSurfaceFeature   = "link.surface.feature"
	eventUnlinkSurfaceEpic    = "unlink.surface.epic"
	eventUnlinkSurfaceFeature = "unlink.surface.feature"
)

// SurfaceResult reports a direct surface recording on an already-resolved node.
type SurfaceResult struct {
	Noun    string
	ID      string
	Key     string
	Surface []string
	Changed bool
}

// SetSurface records existing repo-relative paths on an already-resolved epic
// or feature. Link uses the same planner for combined dependency/surface writes.
func SetSurface(tree *store.Tree, deps Deps, node store.Node, paths []string) (SurfaceResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return SurfaceResult{}, err
	}
	if err := validateSurfaceTarget(node, "link"); err != nil {
		return SurfaceResult{}, err
	}
	if len(paths) == 0 {
		return SurfaceResult{}, fmt.Errorf("mutate: link needs at least one --surface path")
	}
	res, err := linkNode(tree, deps, owner, node, nil, paths, "")
	if err != nil {
		return SurfaceResult{}, err
	}
	return SurfaceResult{Noun: res.Noun, ID: res.ID, Key: res.Key, Surface: res.Surface, Changed: res.SurfaceChanged}, nil
}

func validateSurfaceTarget(node store.Node, command string) error {
	if node.Noun() != store.NounEpic && node.Noun() != store.NounFeature {
		return &usageError{reason: "surface_target", msg: fmt.Sprintf("%s --surface applies to an epic or a feature, not a %s", command, node.Noun())}
	}
	return nil
}

// guardSurfaceRemoval refuses the two removals that would make the recorded
// surface lie. A done node's completion record already pins its surface
// fingerprints (and an archived epic is closed), so the list is history. And a
// specified, live replace or migrate feature must keep naming the code it changes,
// the same rule spec feature enforces: when the work moved that code, link the
// new location before unlinking the old one.
func guardSurfaceRemoval(node store.Node, edit dependsOnEdit) error {
	ref := node.NodeID()
	if key := deref(node.NodeKey()); key != "" {
		ref = key
	}
	switch edit.status {
	case "done", "archived":
		return &usageError{reason: "surface_locked", msg: fmt.Sprintf("%s %s is %s; its completion record pins its surface, so the surface cannot change", node.Noun(), ref, edit.status)}
	}
	if (edit.changeType == "replace" || edit.changeType == "migrate") && edit.status != "draft" && edit.status != "cancelled" && len(edit.surface) == 0 {
		return &usageError{reason: "surface_required", msg: fmt.Sprintf("%s %s is a %s feature and must keep a surface; link the code's current location with `tiller link feature %s --surface <path>` before unlinking the last recorded path", node.Noun(), ref, edit.changeType, ref)}
	}
	return nil
}

func unlinkSurfaceEvent(deps Deps, owner store.Contributor, node store.Node, relYml string, before, after, removed []string) events.Event {
	verb := eventUnlinkSurfaceEpic
	if node.Noun() == store.NounFeature {
		verb = eventUnlinkSurfaceFeature
	}
	ev := events.NewEvent(deps.clock(), verb, owner.ID, "unlink "+node.Noun()+" "+node.NodeID(), node.NodeID())
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"surface": before}
	ev.After = map[string]any{"surface": after, "removed": removed}
	ev.Files = []events.FileChange{{Path: relYml}}
	return ev
}

func surfaceEvent(deps Deps, owner store.Contributor, node store.Node, relYml string, after []string) events.Event {
	verb := eventLinkSurfaceEpic
	if node.Noun() == store.NounFeature {
		verb = eventLinkSurfaceFeature
	}
	ev := events.NewEvent(deps.clock(), verb, owner.ID, "link "+node.Noun()+" "+node.NodeID(), node.NodeID())
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"surface": after}
	ev.Files = []events.FileChange{{Path: relYml}}
	return ev
}

func validateSurfacePaths(tree *store.Tree, node store.Node, paths []string) ([]string, error) {
	if err := validateSurfaceTarget(node, "link"); err != nil {
		return nil, err
	}
	clean, err := cleanSurfacePaths(paths)
	if err != nil {
		return nil, err
	}
	hasReference := false
	for _, root := range tree.Manifest.Inputs.Code {
		if err := root.ValidateMode(filepath.Join(tree.Root, ".tiller", "tiller.yml")); err != nil {
			return nil, err
		}
		hasReference = hasReference || root.Mode == store.CodeModeReference
	}
	repo, err := filepath.Abs(tree.Root)
	if err != nil {
		return nil, surfaceUnavailable(paths[0], err)
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return nil, surfaceUnavailable(paths[0], err)
	}
	candidates := make([]struct {
		declared, canonical string
		info                os.FileInfo
	}, len(clean))
	// Validate requests, not only additions: repeating a vanished path refuses,
	// while an unrelated old surface is left to the drift sweep.
	for i, path := range clean {
		candidates[i].declared = filepath.Join(repo, path)
		candidates[i].canonical = filepath.Join(canonicalRepo, path)
		exists, err := store.PathExistsNoSymlinks(repo, candidates[i].declared)
		if err != nil {
			if errors.Is(err, rootpath.ErrUnsafe) {
				return nil, &usageError{reason: "surface_path", msg: fmt.Sprintf("surface path %q is unsafe: %v; name a path without symlink components", paths[i], err)}
			}
			return nil, surfaceUnavailable(paths[i], err)
		}
		if !exists {
			return nil, &usageError{reason: "surface_missing", msg: fmt.Sprintf("surface path %q does not exist; record an existing file or directory", paths[i])}
		}
		if hasReference {
			candidates[i].info, err = os.Stat(candidates[i].canonical)
			if err != nil {
				return nil, surfaceUnavailable(paths[i], err)
			}
		}
	}
	// Resolve each reference only once. Compare both its declared location and
	// its identity after aliases resolve; a surface root never overrides either.
	for _, root := range tree.Manifest.Inputs.Code {
		if root.Mode != store.CodeModeReference {
			continue
		}
		declared := root.Root
		if !filepath.IsAbs(declared) {
			declared = filepath.Join(repo, declared)
		}
		declared = filepath.Clean(declared)
		resolved, err := filepath.EvalSymlinks(declared)
		if err != nil {
			return nil, surfaceUnavailable(paths[0], fmt.Errorf("resolve reference root %q: %w", root.Root, err))
		}
		referenceInfo, err := os.Stat(resolved)
		if err != nil {
			return nil, surfaceUnavailable(paths[0], fmt.Errorf("inspect reference root %q: %w", root.Root, err))
		}
		for i, candidate := range candidates {
			declaredOverlap, err := surfaceOverlap(candidate.declared, declared)
			if err != nil {
				return nil, surfaceUnavailable(paths[i], fmt.Errorf("compare reference root %q: %w", root.Root, err))
			}
			resolvedOverlap, err := surfaceOverlap(candidate.canonical, resolved)
			if err != nil {
				return nil, surfaceUnavailable(paths[i], fmt.Errorf("compare reference root %q: %w", root.Root, err))
			}
			if !declaredOverlap && !resolvedOverlap {
				resolvedOverlap, err = hasAncestorIdentity(candidate.canonical, candidate.info, referenceInfo)
				if err == nil && !resolvedOverlap {
					resolvedOverlap, err = hasAncestorIdentity(resolved, referenceInfo, candidate.info)
				}
				if err != nil {
					return nil, surfaceUnavailable(paths[i], fmt.Errorf("compare reference root %q: %w", root.Root, err))
				}
			}
			if declaredOverlap || resolvedOverlap {
				return nil, &usageError{reason: "surface_reference", msg: fmt.Sprintf("surface path %q overlaps reference root %q; name existing code outside that reference root and its containing directories", paths[i], root.Root)}
			}
		}
	}
	return clean, nil
}

func surfaceUnavailable(path string, cause error) *store.EnvError {
	return &store.EnvError{Reason: "surface_unavailable", Msg: fmt.Sprintf("cannot inspect surface path %q: %v", path, cause)}
}

func surfaceOverlap(candidate, reference string) (bool, error) {
	within := func(parent, child string) (bool, error) {
		rel, err := filepath.Rel(parent, child)
		if err != nil {
			return false, err
		}
		return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
	}
	if overlaps, err := within(reference, candidate); overlaps || err != nil {
		return overlaps, err
	}
	return within(candidate, reference)
}

// hasAncestorIdentity compares metadata, not spelling: case-insensitive aliases
// and hard links can name the same protected code without a lexical overlap.
// Only the path and its ancestors are inspected; no directory is enumerated.
func hasAncestorIdentity(path string, info, target os.FileInfo) (bool, error) {
	for {
		if os.SameFile(info, target) {
			return true, nil
		}
		parent := filepath.Dir(path)
		if !target.IsDir() || parent == path {
			return false, nil
		}
		var err error
		info, err = os.Stat(parent)
		if err != nil {
			return false, err
		}
		path = parent
	}
}

// cleanSurfacePaths preserves every requested path for metadata validation.
// Existence is required when recording; later deletion remains drift's concern.
func cleanSurfacePaths(paths []string) ([]string, error) {
	clean := make([]string, len(paths))
	for i, raw := range paths {
		if raw == "" {
			return nil, &usageError{reason: "surface_path", msg: "a surface path cannot be empty"}
		}
		if filepath.IsAbs(raw) {
			return nil, &usageError{reason: "surface_path", msg: fmt.Sprintf("surface path %q must be repo-relative, not absolute", raw)}
		}
		c := filepath.Clean(raw)
		if c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
			return nil, &usageError{reason: "surface_path", msg: fmt.Sprintf("surface path %q escapes the repo root", raw)}
		}
		clean[i] = c
	}
	return clean, nil
}

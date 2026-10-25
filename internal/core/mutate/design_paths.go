package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/rootpath"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

type designSurfacePath struct {
	anchor   string
	relative string
	recorded string
}

func designPathError(reason, format string, args ...any) error {
	return &usageError{reason: reason, msg: fmt.Sprintf(format, args...)}
}

func designDocPath(raw string) (string, error) {
	if err := store.ValidateDesignPath(raw); err != nil {
		return "", designPathError("design_path", "%v", err)
	}
	return raw, nil
}

func designCoordinate(root, raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00\r\n") {
		return "", designPathError("design_path", "invalid empty or unsafe path %q", raw)
	}
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute := raw
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, absolute)
	}
	absolute = filepath.Clean(absolute)
	rel, err := filepath.Rel(root, absolute)
	if err != nil {
		return filepath.ToSlash(absolute), nil
	}
	return filepath.ToSlash(rel), nil
}

func designWithin(parent, child string) (string, bool) {
	rel, err := filepath.Rel(parent, child)
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Only declaration aliases are resolved. Every remaining component is walked
// beneath the selected root, so a candidate symlink cannot expand membership.
func resolveDesignSurface(root string, manifest *store.Manifest, raw string) (designSurfacePath, error) {
	if _, err := designCoordinate(root, raw); err != nil {
		return designSurfacePath{}, err
	}
	repo, err := filepath.Abs(root)
	if err != nil {
		return designSurfacePath{}, err
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return designSurfacePath{}, err
	}
	candidate := raw
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(repo, candidate)
	}
	candidate = filepath.Clean(candidate)
	// Recorded relative coordinates are based on the canonical project root,
	// including paths outside it. Do not resolve candidate symlinks: only a
	// declared root may authorize an alias.
	canonicalCandidate := raw
	if !filepath.IsAbs(canonicalCandidate) {
		canonicalCandidate = filepath.Join(canonicalRepo, canonicalCandidate)
	} else if rel, within := designWithin(repo, candidate); within {
		canonicalCandidate = filepath.Join(canonicalRepo, rel)
	}
	var unavailable error
	for _, declaration := range manifest.Inputs.Code {
		if err := declaration.ValidateMode(filepath.Join(root, ".tiller", "tiller.yml")); err != nil {
			return designSurfacePath{}, err
		}
		if declaration.Root == "" {
			return designSurfacePath{}, designPathError("design_code_root", "a declared code root cannot be empty")
		}
		declared := declaration.Root
		if !filepath.IsAbs(declared) {
			declared = filepath.Join(repo, declared)
		}
		declared = filepath.Clean(declared)
		resolved, resolveErr := filepath.EvalSymlinks(declared)
		rel, within := designWithin(declared, candidate)
		if !within && resolveErr == nil {
			rel, within = designWithin(resolved, canonicalCandidate)
		}
		if !within {
			continue
		}
		if resolveErr != nil {
			unavailable = designSurfaceUnavailable(raw, resolveErr)
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil {
			unavailable = designSurfaceUnavailable(raw, err)
			continue
		}
		anchor, relative := resolved, rel
		if !info.IsDir() {
			if !info.Mode().IsRegular() || rel != "." {
				unavailable = designPathError("design_path", "describes path %q is not beneath a regular code root", raw)
				continue
			}
			anchor, relative = filepath.Dir(resolved), filepath.Base(resolved)
		}
		opened, err := os.OpenRoot(anchor)
		if err != nil {
			unavailable = designSurfaceUnavailable(raw, err)
			continue
		}
		file, err := (rootpath.Walker{}).Open(opened, relative, true)
		opened.Close()
		if err != nil {
			unavailable = designSurfaceUnavailable(raw, err)
			continue
		}
		file.Close()
		recorded, err := designCoordinate(canonicalRepo, filepath.Join(anchor, relative))
		if err != nil {
			return designSurfacePath{}, err
		}
		return designSurfacePath{anchor: anchor, relative: filepath.ToSlash(relative), recorded: recorded}, nil
	}
	if unavailable != nil {
		return designSurfacePath{}, unavailable
	}
	return designSurfacePath{}, designPathError("design_membership", "describes path %q must equal or lie beneath a declared code root", raw)
}

func designSourcePath(root, raw string) (string, error) {
	p, err := designCoordinate(root, raw)
	if err != nil {
		return "", err
	}
	if p == "." || p == ".." || strings.HasPrefix(p, "../") || filepath.IsAbs(p) {
		return "", designPathError("design_source", "source path %q must name a file in this project", raw)
	}
	return p, nil
}

// Removal uses coordinates alone, not existence: disappeared code and retired
// sources must remain unlinkable even when their declaration no longer resolves.
func designRemovalPaths(root string, paths []string, source bool) ([]string, error) {
	result := make([]string, 0, len(paths))
	for _, raw := range paths {
		var p string
		var err error
		if source {
			p, err = designSourcePath(root, raw)
		} else {
			p, err = designCoordinate(root, raw)
		}
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}

// Retain the printed coordinate even when an alias has vanished. A declaration
// that still resolves additionally lets the original alias spelling unlink a
// vanished descendant, without opening that descendant.
func designRemovalAliases(root string, manifest *store.Manifest, paths []string) []string {
	repo, err := filepath.Abs(root)
	if err != nil {
		return paths
	}
	canonicalRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return paths
	}
	result := append([]string(nil), paths...)
	for _, p := range paths {
		candidate := filepath.FromSlash(p)
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(repo, candidate)
		}
		for _, declaration := range manifest.Inputs.Code {
			declared := declaration.Root
			if declared == "" {
				continue
			}
			if !filepath.IsAbs(declared) {
				declared = filepath.Join(repo, declared)
			}
			relative, within := designWithin(declared, candidate)
			if !within {
				continue
			}
			resolved, err := filepath.EvalSymlinks(declared)
			if err != nil {
				continue
			}
			recorded, err := designCoordinate(canonicalRepo, filepath.Join(resolved, relative))
			if err == nil && recorded != p {
				result = append(result, recorded)
			}
		}
	}
	return result
}

// ObserveDesignSurface uses exactly the root safety and working-tree inventory
// policy used by approval. Errors retain filesystem identity so drift can
// distinguish a vanished path from an unsafe or unreadable one.
func ObserveDesignSurface(root string, manifest *store.Manifest, described string, at time.Time) (store.SurfaceFingerprint, error) {
	surface, err := resolveDesignSurface(root, manifest, described)
	if err != nil {
		return store.SurfaceFingerprint{}, err
	}
	return vcs.WorktreeSurface(surface.anchor, surface.relative, described, at)
}

func designSurfaceUnavailable(path string, cause error) error {
	return fmt.Errorf("%w: %w", &store.EnvError{Reason: "surface_unavailable", Msg: fmt.Sprintf("cannot inspect surface path %q", path)}, cause)
}

// DesignSurfaceCoordinates retains recorded coordinates and adds canonical
// declaration aliases, without requiring descendants to still exist.
func DesignSurfaceCoordinates(root string, manifest *store.Manifest, paths []string) []string {
	coordinates := designRemovalAliases(root, manifest, paths)
	base := root
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		base = canonical
	}
	for i, coordinate := range coordinates {
		if filepath.IsAbs(coordinate) {
			if relative, within := designWithin(root, coordinate); within {
				coordinate = filepath.Join(base, relative)
			}
		}
		if normalized, err := designCoordinate(base, coordinate); err == nil {
			coordinates[i] = normalized
		}
	}
	return coordinates
}

// CheckDesignSurface checks membership and safe existence without inventorying
// bytes; stale_design needs absence, not a second content comparison.
func CheckDesignSurface(root string, manifest *store.Manifest, described string) error {
	surface, err := resolveDesignSurface(root, manifest, described)
	if err != nil {
		return err
	}
	return vcs.CheckWorktreeSurface(surface.anchor, surface.relative, described)
}

package store

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const BaselineRelPath = ".tiller/design/baseline.yml"

// Design documents are path-addressed prose, not work nodes.
const NounDesign = "design"

type BaselineLedger struct {
	Docs []BaselineDoc `yaml:"docs" json:"docs"`
}

type BaselineDoc struct {
	Path        string                `yaml:"path" json:"path"`
	Describes   []string              `yaml:"describes" json:"describes"`
	DerivedFrom []RevisionRef         `yaml:"derived_from,omitempty" json:"derived_from,omitempty"`
	Confirmed   *BaselineConfirmation `yaml:"confirmed,omitempty" json:"confirmed,omitempty"`
}

type BaselineConfirmation struct {
	Commit   RevisionRef          `yaml:"commit" json:"commit"`
	Surfaces []SurfaceFingerprint `yaml:"surfaces" json:"surfaces"`
	By       string               `yaml:"by" json:"by"`
}

// HasConfirmedDocs is deliberately I/O-free. Parsing validates stamp contents;
// neither a retired confirming owner nor subsequently missing prose erases history.
func (b BaselineLedger) HasConfirmedDocs() bool {
	for _, doc := range b.Docs {
		if doc.Confirmed != nil {
			return true
		}
	}
	return false
}

// ValidateDesignPath accepts canonical paths relative to .tiller/design.
func ValidateDesignPath(p string) error {
	if !baselineCoordinate(p) || path.IsAbs(p) || p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.ToLower(path.Ext(p)) != ".md" {
		return fmt.Errorf("design path %q must be a canonical Markdown path beneath .tiller/design", p)
	}
	return nil
}

func baselineCoordinate(p string) bool {
	return p != "" && !strings.ContainsAny(p, "\\\x00\r\n") && path.Clean(p) == p
}

// ParseBaseline uses the project's YAML conventions with baseline-local semantic
// validation. Incomplete stamps must never open the first-confirmation gate.
func ParseBaseline(data []byte) (BaselineLedger, error) {
	var ledger BaselineLedger
	if err := yaml.Unmarshal(data, &ledger); err != nil {
		return BaselineLedger{}, fmt.Errorf("parse %s: %w", BaselineRelPath, err)
	}
	if ledger.Docs == nil {
		return BaselineLedger{}, fmt.Errorf("parse %s: docs must be a list (use docs: [] for an empty ledger)", BaselineRelPath)
	}
	seen := make(map[string]bool, len(ledger.Docs))
	for _, doc := range ledger.Docs {
		if err := validateBaselineDoc(doc); err != nil {
			return BaselineLedger{}, fmt.Errorf("parse %s: %w", BaselineRelPath, err)
		}
		if seen[doc.Path] {
			return BaselineLedger{}, fmt.Errorf("parse %s: duplicate design path %q", BaselineRelPath, doc.Path)
		}
		seen[doc.Path] = true
	}
	return ledger, nil
}

func validateBaselineDoc(doc BaselineDoc) error {
	if err := ValidateDesignPath(doc.Path); err != nil {
		return err
	}
	if len(doc.Describes) == 0 {
		return fmt.Errorf("design %q needs at least one describes path", doc.Path)
	}
	paths := make(map[string]bool, len(doc.Describes))
	for _, p := range doc.Describes {
		if !baselineCoordinate(p) || paths[p] {
			return fmt.Errorf("design %q has invalid or duplicate describes path %q", doc.Path, p)
		}
		paths[p] = true
	}
	sources := make(map[string]bool, len(doc.DerivedFrom))
	for _, ref := range doc.DerivedFrom {
		if ref.Kind != KindGitBlob || !validBaselineGitValue(ref.Value) || ref.ObservedAt.IsZero() || !baselineCoordinate(ref.Path) || path.IsAbs(ref.Path) || ref.Path == "." || ref.Path == ".." || strings.HasPrefix(ref.Path, "../") || sources[ref.Path] {
			return fmt.Errorf("design %q has incomplete, invalid, or duplicate source revision %q", doc.Path, ref.Path)
		}
		sources[ref.Path] = true
	}
	if doc.Confirmed == nil {
		return nil
	}
	c := doc.Confirmed
	if strings.TrimSpace(c.By) == "" || c.Commit.Kind != KindGitCommit || !validBaselineGitValue(c.Commit.Value) || c.Commit.Path != "" || c.Commit.ObservedAt.IsZero() || len(c.Surfaces) != len(doc.Describes) {
		return fmt.Errorf("design %q has an incomplete confirmation", doc.Path)
	}
	observed := make(map[string]bool, len(c.Surfaces))
	for _, surface := range c.Surfaces {
		if !paths[surface.Path] || observed[surface.Path] || (surface.Kind != KindGitBlob && surface.Kind != KindGitTree) || !validBaselineGitValue(surface.Value) || surface.ObservedAt.IsZero() {
			return fmt.Errorf("design %q has an invalid confirmed surface %q", doc.Path, surface.Path)
		}
		observed[surface.Path] = true
		if surface.Kind == KindGitBlob && (len(surface.Inventory) != 1 || surface.Inventory[0] != surface.Path) {
			return fmt.Errorf("design %q has invalid blob inventory", doc.Path)
		}
		inventory := make(map[string]bool, len(surface.Inventory))
		for _, p := range surface.Inventory {
			// Inventories contain filesystem names, not command coordinates:
			// backslashes and control characters other than NUL are literal bytes.
			if p == "" || strings.ContainsRune(p, '\x00') || path.Clean(p) != p || inventory[p] || !baselineContains(surface.Path, p) || (surface.Kind == KindGitTree && p == surface.Path) {
				return fmt.Errorf("design %q has invalid inventory path %q", doc.Path, p)
			}
			inventory[p] = true
		}
	}
	return nil
}

func baselineContains(parent, child string) bool {
	if parent == child {
		return true
	}
	if parent == "." {
		return !path.IsAbs(child) && child != ".." && !strings.HasPrefix(child, "../")
	}
	return strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}

func validBaselineGitValue(value string) bool {
	if !strings.HasPrefix(value, "git:") {
		return false
	}
	hash := strings.TrimPrefix(value, "git:")
	if len(hash) < 4 || len(hash) > 64 {
		return false
	}
	for _, c := range hash {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LoadBaseline does not read the prose: missing documents remain repairable by
// unlink and diagnosable by readiness. The ledger itself is opened without symlinks.
func LoadBaseline(root string) (BaselineLedger, error) {
	data, snapshot, err := ReadRegularFileSnapshot(root, filepath.Join(root, BaselineRelPath))
	if err != nil {
		return BaselineLedger{}, fmt.Errorf("read %s: %w", BaselineRelPath, err)
	}
	if !snapshot.Exists() {
		return BaselineLedger{}, nil
	}
	return ParseBaseline(data)
}

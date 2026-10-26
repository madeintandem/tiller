package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/madeintandem/tiller/internal/rootpath"
	"gopkg.in/yaml.v3"
)

// WorkItem is the lifecycle body shared by shelf work. attached_to is
// provenance only; resolved_by is the live carrier pointer sync crosses on.
// resolution and duplicate_of describe cancellation. Each shelf noun embeds
// this body and may grow its own fields independently.
type WorkItem struct {
	ID           string   `yaml:"id"`
	Key          *string  `yaml:"key,omitempty"`
	Title        string   `yaml:"title,omitempty"`
	Status       string   `yaml:"status,omitempty"`
	Blocked      bool     `yaml:"blocked"`
	Started      bool     `yaml:"started"`
	Owner        string   `yaml:"owner,omitempty"`
	ProposedBy   string   `yaml:"proposed_by,omitempty"`
	AttachedTo   string   `yaml:"attached_to,omitempty"`
	ResolvedBy   string   `yaml:"resolved_by,omitempty"`
	Resolution   string   `yaml:"resolution,omitempty"`
	DuplicateOf  string   `yaml:"duplicate_of,omitempty"`
	DependsOn    []string `yaml:"depends_on,omitempty"`
	Source       string   `yaml:"source,omitempty"`
	Supersedes   []string `yaml:"supersedes,omitempty"`
	SupersededBy *string  `yaml:"superseded_by,omitempty"`

	// AcceptedAgainst pins the contract document the human triaged, with
	// the same pointer-valued pin Task carries.
	AcceptedAgainst *BasisRefs `yaml:"accepted_against,omitempty"`

	Evidence  []Evidence `yaml:"evidence,omitempty"`
	Branch    string     `yaml:"branch,omitempty"`
	PR        *int       `yaml:"pr,omitempty"`
	ReviewRef *ReviewRef `yaml:"review_ref,omitempty"`
	// DoneBasis is ancestry, content, human_attested, or carried.
	DoneBasis  string `yaml:"done_basis,omitempty"`
	GatesBlock `yaml:",inline"`
}

// Bug is a defect noticed against shipped or in-flight work. On disk it is
// .tiller/bugs/<key>/bug.yml beside bug.md.
type Bug struct {
	WorkItem `yaml:",inline"`
}

// Chore is upkeep no feature owns. On disk it is .tiller/chores/<key>/chore.yml
// beside chore.md.
type Chore struct {
	WorkItem `yaml:",inline"`
}

// Tweak is a minor adjustment, potentially involving code, on its own shelf.
type Tweak struct {
	WorkItem `yaml:",inline"`
}

func (b *Bug) NodeID() string     { return b.ID }
func (b *Bug) NodeKey() *string   { return b.Key }
func (b *Bug) Noun() string       { return NounBug }
func (b *Bug) NodeStatus() string { return b.Status }
func (b *Bug) Body() *WorkItem    { return &b.WorkItem }

func (c *Chore) NodeID() string     { return c.ID }
func (c *Chore) NodeKey() *string   { return c.Key }
func (c *Chore) Noun() string       { return NounChore }
func (c *Chore) NodeStatus() string { return c.Status }
func (c *Chore) Body() *WorkItem    { return &c.WorkItem }

func (t *Tweak) NodeID() string     { return t.ID }
func (t *Tweak) NodeKey() *string   { return t.Key }
func (t *Tweak) Noun() string       { return NounTweak }
func (t *Tweak) NodeStatus() string { return t.Status }
func (t *Tweak) Body() *WorkItem    { return &t.WorkItem }

// WorkNode is a shelf Node whose lifecycle state is a WorkItem.
type WorkNode interface {
	Node
	Body() *WorkItem
}

// AsWorkNode narrows a Node to the shelf nouns.
func AsWorkNode(n Node) (WorkNode, bool) {
	switch v := n.(type) {
	case *Bug:
		return v, true
	case *Chore:
		return v, true
	case *Tweak:
		return v, true
	default:
		return nil, false
	}
}

// NewWorkNode returns an empty node of the shelf noun, for the loader and
// the writers to fill.
func NewWorkNode(noun string) (WorkNode, error) {
	switch noun {
	case NounBug:
		return &Bug{}, nil
	case NounChore:
		return &Chore{}, nil
	case NounTweak:
		return &Tweak{}, nil
	default:
		return nil, fmt.Errorf("store: %q is not a shelf noun", noun)
	}
}

// ShelfOrder is a shelf's recorded soft order: intent, not constraint,
// written only by `tiller order <shelf noun>`.
type ShelfOrder struct {
	Order []string `yaml:"order,omitempty"`
}

// LoadShelfOrder reads a shelf's order.yml; absent is no recorded order.
func LoadShelfOrder(root, noun string) (ShelfOrder, error) {
	var so ShelfOrder
	rel := ShelfOrderRelPath(noun)
	data, err := ReadRepositoryFile(root, rel)
	if errors.Is(err, os.ErrNotExist) {
		return so, nil
	}
	if err != nil {
		return so, fmt.Errorf("read %s: %w", rel, err)
	}
	if err := yaml.Unmarshal(data, &so); err != nil {
		return so, fmt.Errorf("parse %s: %w", rel, err)
	}
	return so, nil
}

// The cancel resolutions a shelf node may carry (the design's Resolution).
const (
	ResolutionWontfix     = "wontfix"
	ResolutionInvalid     = "invalid"
	ResolutionDuplicate   = "duplicate"
	ResolutionTransferred = "transferred"
)

// ValidResolution reports whether r is a persisted cancellation resolution.
func ValidResolution(r string) bool {
	return r == ResolutionWontfix || r == ResolutionInvalid || r == ResolutionDuplicate || r == ResolutionTransferred
}

// WorkNode looks one shelf node up by id across all work shelves.
func (t *Tree) WorkNode(id string) (WorkNode, bool) {
	if b, ok := t.Bugs[id]; ok {
		return b, true
	}
	if c, ok := t.Chores[id]; ok {
		return c, true
	}
	if w, ok := t.Tweaks[id]; ok {
		return w, true
	}
	return nil, false
}

// WorkNodesOf returns one shelf's nodes in id order. An unknown noun panics.
func (t *Tree) WorkNodesOf(noun string) []WorkNode {
	var out []WorkNode
	switch noun {
	case NounBug:
		for _, b := range t.Bugs {
			out = append(out, b)
		}
	case NounChore:
		for _, c := range t.Chores {
			out = append(out, c)
		}
	case NounTweak:
		for _, w := range t.Tweaks {
			out = append(out, w)
		}
	default:
		panic(notAShelf(noun))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID() < out[j].NodeID() })
	return out
}

// ShelfOrderOf returns the recorded soft order for a shelf noun. An unknown
// noun panics.
func (t *Tree) ShelfOrderOf(noun string) []string {
	return t.shelfOrder(noun).Order
}

// shelfOrder is the Tree field holding one shelf's soft order.
func (t *Tree) shelfOrder(noun string) *ShelfOrder {
	switch noun {
	case NounBug:
		return &t.BugOrder
	case NounChore:
		return &t.ChoreOrder
	case NounTweak:
		return &t.TweakOrder
	default:
		panic(notAShelf(noun))
	}
}

// loadShelf walks one shelf: every directory under .tiller/<shelf>/ holds
// <noun>.yml, and the shelf's own order.yml is a file the walk skips.
func (t *Tree) loadShelf(noun string, ids *nodeIDRegistry) error {
	dir := filepath.Join(t.Root, ".tiller", ShelfDir(noun))
	entries, err := t.stateDirectory(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink", rootpath.ErrUnsafe, filepath.Join(dir, e.Name()))
		}
		if !e.IsDir() {
			continue
		}
		yml := filepath.Join(dir, e.Name(), ShelfStateFile(noun))
		node, err := NewWorkNode(noun)
		if err != nil {
			return err
		}
		if err := t.stateYAML(yml, node); err != nil {
			return err
		}
		w := node.Body()
		if err := ids.claim(noun, w.ID, yml); err != nil {
			return err
		}
		if err := checkKeyNamesItsFolder(yml, e.Name(), w.Key); err != nil {
			return err
		}
		if w.Key != nil && *w.Key != ShelfCanonicalKey(noun, e.Name()) {
			return fmt.Errorf("parse %s: key %q must be canonical under %s/ (want %q)", yml, *w.Key, ShelfDir(noun), ShelfCanonicalKey(noun, e.Name()))
		}
		if w.Resolution != "" && !ValidResolution(w.Resolution) {
			return fmt.Errorf("parse %s: resolution %q is not wontfix, invalid, duplicate, or transferred", yml, w.Resolution)
		}
		switch n := node.(type) {
		case *Bug:
			t.Bugs[n.ID] = n
		case *Chore:
			t.Chores[n.ID] = n
		case *Tweak:
			t.Tweaks[n.ID] = n
		}
	}
	order, err := LoadShelfOrder(t.Root, noun)
	if err != nil {
		return err
	}
	*t.shelfOrder(noun) = order
	return nil
}

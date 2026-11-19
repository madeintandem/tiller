package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// Batch is one shared review of several bugs, chores, and tweaks: one branch,
// one pull request, one merge. On disk it is .tiller/batches/<key>/batch.yml
// with no contract file, because its members' contracts are the contract.
//
// It embeds the shelf lifecycle body so the claim, start, check, submit,
// resume, and done-crossing code that serves a shelf node serves a batch
// unchanged. The fields that describe one shelf node's own story (severity,
// attachment, carrier, dependencies, accepted basis, recovered review,
// resolution, lineage) are refused on load. Membership is not stored here:
// each member names its batch in WorkItem.BatchID.
type Batch struct {
	WorkItem `yaml:",inline"`
}

func (b *Batch) NodeID() string     { return b.ID }
func (b *Batch) NodeKey() *string   { return b.Key }
func (b *Batch) Noun() string       { return NounBatch }
func (b *Batch) NodeStatus() string { return b.Status }
func (b *Batch) Body() *WorkItem    { return &b.WorkItem }

// BatchLive reports a batch whose review can still land: any status but done
// or cancelled.
func BatchLive(b *Batch) bool {
	return b != nil && b.Status != "done" && b.Status != "cancelled"
}

// BatchOf returns the batch a shelf node names, nil when it names none.
func (t *Tree) BatchOf(w WorkNode) *Batch {
	if t == nil || w == nil {
		return nil
	}
	return t.Batches[w.Body().BatchID]
}

// BatchMembers returns the shelf nodes naming a batch, bugs then chores then
// tweaks, each shelf in key order.
func (t *Tree) BatchMembers(batchID string) []WorkNode {
	var out []WorkNode
	if batchID == "" {
		return out
	}
	for _, noun := range ShelfNouns() {
		var members []WorkNode
		for _, w := range t.WorkNodesOf(noun) {
			if w.Body().BatchID == batchID {
				members = append(members, w)
			}
		}
		sort.SliceStable(members, func(i, j int) bool { return keyOrEmpty(members[i]) < keyOrEmpty(members[j]) })
		out = append(out, members...)
	}
	return out
}

// BatchesInOrder returns every batch in key order.
func (t *Tree) BatchesInOrder() []*Batch {
	out := make([]*Batch, 0, len(t.Batches))
	for _, b := range t.Batches {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if left, right := keyOrEmpty(out[i]), keyOrEmpty(out[j]); left != right {
			return left < right
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func keyOrEmpty(n Node) string {
	if k := n.NodeKey(); k != nil {
		return *k
	}
	return ""
}

// batchStatuses are the states a batch record may hold. It is born ready, so
// proposed is not one of them.
var batchStatuses = map[string]bool{
	"ready": true, "in_progress": true, "check": true, "in_review": true, "done": true, "cancelled": true,
}

// shelfOnlyField names the first field set on a batch record that only a
// shelf node carries, "" when the record is clean.
func shelfOnlyField(w *WorkItem) string {
	switch {
	case w.Severity != "":
		return "severity"
	case w.AttachedTo != "":
		return "attached_to"
	case w.ResolvedBy != "":
		return "resolved_by"
	case w.BatchID != "":
		return "batch"
	case w.Resolution != "":
		return "resolution"
	case w.DuplicateOf != "":
		return "duplicate_of"
	case len(w.DependsOn) > 0:
		return "depends_on"
	case w.Source != "":
		return "source"
	case len(w.Supersedes) > 0:
		return "supersedes"
	case w.SupersededBy != nil:
		return "superseded_by"
	case w.AcceptedAgainst != nil:
		return "accepted_against"
	case w.RecoveredReview != nil:
		return "recovered_review"
	}
	return ""
}

// loadBatches walks .tiller/batches/: every directory holds batch.yml. It runs
// after the shelves, so it can check each member's pointer in the same pass.
func (t *Tree) loadBatches(ids *nodeIDRegistry) error {
	dir := filepath.Join(t.Root, ".tiller", ShelfDir(NounBatch))
	entries, err := t.stateDirectory(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink", rootpath.ErrUnsafe, filepath.Join(dir, e.Name()))
		}
		if !e.IsDir() {
			continue
		}
		yml := filepath.Join(dir, e.Name(), ShelfStateFile(NounBatch))
		b := &Batch{}
		if err := t.stateYAML(yml, b); err != nil {
			return err
		}
		if err := ids.claim(NounBatch, b.ID, yml); err != nil {
			return err
		}
		if err := checkKeyNamesItsFolder(yml, e.Name(), b.Key); err != nil {
			return err
		}
		if b.Key != nil && *b.Key != ShelfCanonicalKey(NounBatch, e.Name()) {
			return fmt.Errorf("parse %s: key %q must be canonical under %s/ (want %q)", yml, *b.Key, ShelfDir(NounBatch), ShelfCanonicalKey(NounBatch, e.Name()))
		}
		if !batchStatuses[b.Status] {
			return fmt.Errorf("parse %s: status %q is not ready, in_progress, check, in_review, done, or cancelled", yml, b.Status)
		}
		if field := shelfOnlyField(&b.WorkItem); field != "" {
			return fmt.Errorf("parse %s: a batch carries no %s", yml, field)
		}
		t.Batches[b.ID] = b
	}
	for _, noun := range ShelfNouns() {
		for _, w := range t.WorkNodesOf(noun) {
			body := w.Body()
			if body.BatchID == "" {
				continue
			}
			if t.Batches[body.BatchID] == nil {
				return fmt.Errorf("%s %s names batch %s, which is not under .tiller/%s/", noun, body.ID, body.BatchID, ShelfDir(NounBatch))
			}
			if body.DoneBasis == "batched" && body.Status != "done" {
				return fmt.Errorf("%s %s records done_basis batched but is %s", noun, body.ID, body.Status)
			}
		}
	}
	return nil
}

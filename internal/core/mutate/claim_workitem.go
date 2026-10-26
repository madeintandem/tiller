package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Shelf claims reserve one execution unit and never cascade.

func resolveWorkRef(tree *store.Tree, noun, ref string) (store.WorkNode, error) {
	if !store.IsWorkNoun(noun) {
		return nil, notShelfNoun(noun)
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: ref})
	if err != nil {
		return nil, err
	}
	w, ok := store.AsWorkNode(n)
	if !ok {
		return nil, fmt.Errorf("mutate: %s resolved to a %s", ref, n.Noun())
	}
	return w, nil
}

// commitWorkOwner writes one shelf node's state file with its owner changed
// and one event.
func commitWorkOwner(tree *store.Tree, operation string, doc store.WorkNode, relYml string, snapshot store.FileSnapshot, ev events.Event) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitMutation(tree, operation, []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data),
	}, []events.Event{ev}); err != nil {
		return err
	}
	return nil
}

// ClaimWork takes the claim on a bug or chore, or takes it over with from.
func ClaimWork(tree *store.Tree, deps Deps, noun, ref, from string) (ClaimResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ClaimResult{}, err
	}
	treeNode, err := resolveWorkRef(tree, noun, ref)
	if err != nil {
		return ClaimResult{}, err
	}
	doc, relYml, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return ClaimResult{}, err
	}
	w := doc.Body()
	key := deref(w.Key)
	prior := w.Owner
	takeover := false
	if prior != owner.ID {
		if err := guardOwnerMutable(noun, w.ID, key, w.Status, w.Started, prior); err != nil {
			return ClaimResult{}, err
		}
	}
	switch {
	case prior == owner.ID:
		if from != "" && from != prior {
			return ClaimResult{}, claimHolderMismatchErr(noun, w.ID, key, prior)
		}
		return ClaimResult{Noun: noun, ID: w.ID, Key: key, Owner: owner.ID}, nil
	case prior == "":
		if from != "" {
			return ClaimResult{}, claimHolderMismatchErr(noun, w.ID, key, prior)
		}
	case from == "":
		return ClaimResult{}, claimHeldErr(noun, w.ID, key, prior)
	case from != prior:
		return ClaimResult{}, claimHolderMismatchErr(noun, w.ID, key, prior)
	default:
		takeover = true
	}
	w.Owner = owner.ID

	command := "claim " + noun + " " + key
	if from != "" {
		command += " --from " + from
	}
	ev := events.NewEvent(deps.clock(), workItemEventType("claim", noun), owner.ID, command, w.ID)
	ev.ID = deps.mintRaw("EV")
	if prior != "" {
		ev.Before = map[string]any{"owner": prior}
	}
	after := map[string]any{"owner": owner.ID}
	if takeover {
		after["from"], after["to"] = prior, owner.ID
	}
	ev.After = after
	if err := commitWorkOwner(tree, "claim "+noun, doc, relYml, snapshot, ev); err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Noun: noun, ID: w.ID, Key: key, Owner: owner.ID, Changed: true, Takeover: takeover, From: prior}, nil
}

// ReleaseWork returns a bug's or chore's claim to the pool.
func ReleaseWork(tree *store.Tree, deps Deps, noun, ref string) (ReleaseResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ReleaseResult{}, err
	}
	treeNode, err := resolveWorkRef(tree, noun, ref)
	if err != nil {
		return ReleaseResult{}, err
	}
	doc, relYml, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return ReleaseResult{}, err
	}
	w := doc.Body()
	key := deref(w.Key)
	prior := w.Owner
	if err := guardOwnerMutable(noun, w.ID, key, w.Status, w.Started, prior); err != nil {
		return ReleaseResult{}, err
	}
	if prior == "" {
		return ReleaseResult{Noun: noun, ID: w.ID, Key: key}, nil
	}
	if prior != owner.ID {
		return ReleaseResult{}, notClaimantErr(noun, w.ID, key, prior)
	}
	w.Owner = ""
	ev := events.NewEvent(deps.clock(), workItemEventType("release", noun), owner.ID, "release "+noun+" "+key, w.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"owner": prior}
	ev.After = map[string]any{"owner": ""}
	if err := commitWorkOwner(tree, "release "+noun, doc, relYml, snapshot, ev); err != nil {
		return ReleaseResult{}, err
	}
	return ReleaseResult{Noun: noun, ID: w.ID, Key: key, From: prior, Changed: true}, nil
}

// ReassignWork hands a bug or chore to another registered owner, human-only,
// with a required reason.
func ReassignWork(tree *store.Tree, deps Deps, noun, ref, to, reason string) (ReassignResult, error) {
	if strings.TrimSpace(to) == "" {
		return ReassignResult{}, &usageError{reason: "reassign_to_required", msg: "reassign needs --to <contributor-id>"}
	}
	if strings.TrimSpace(reason) == "" {
		return ReassignResult{}, &usageError{reason: "reassign_reason_required", msg: "reassign needs --reason"}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ReassignResult{}, err
	}
	target, err := store.ResolveContributor(tree.Manifest, to)
	if err != nil {
		return ReassignResult{}, err
	}
	treeNode, err := resolveWorkRef(tree, noun, ref)
	if err != nil {
		return ReassignResult{}, err
	}
	doc, relYml, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return ReassignResult{}, err
	}
	w := doc.Body()
	key := deref(w.Key)
	prior := w.Owner
	if err := guardOwnerMutable(noun, w.ID, key, w.Status, w.Started, prior); err != nil {
		return ReassignResult{}, err
	}
	w.Owner = target.ID
	ev := events.NewEvent(deps.clock(), workItemEventType("reassign", noun), owner.ID,
		fmt.Sprintf("reassign %s %s --to %s --reason %q", noun, key, target.ID, reason), w.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"owner": prior}
	ev.After = map[string]any{"owner": target.ID, "reason": reason}
	if err := commitWorkOwner(tree, "reassign "+noun, doc, relYml, snapshot, ev); err != nil {
		return ReassignResult{}, err
	}
	return ReassignResult{Noun: noun, ID: w.ID, Key: key, From: prior, To: target.ID, Reason: reason}, nil
}

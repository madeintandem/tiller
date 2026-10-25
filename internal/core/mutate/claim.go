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

const ReasonClaimHeld = "claim_held"

// Claims reserve individual execution units. Starting permanently fixes their owner.
type ClaimResult struct {
	Noun     string
	ID       string
	Key      string
	Owner    string
	Changed  bool
	Takeover bool
	From     string
}

type ReleaseResult struct {
	Noun    string
	ID      string
	Key     string
	From    string
	Changed bool
}

type ReassignResult struct {
	Noun   string
	ID     string
	Key    string
	From   string
	To     string
	Reason string
}

func claimTaskFile(tree *store.Tree, ref string) (*store.Task, string, store.FileSnapshot, error) {
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounTask, Raw: ref})
	if err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	task := n.(*store.Task)
	feature := tree.Features[task.FeatureID]
	if feature == nil {
		return nil, "", store.FileSnapshot{}, fmt.Errorf("mutate: task %s has no parent feature", task.ID)
	}
	return loadTaskFile(tree, feature, task)
}

// ClaimTask reserves an unstarted task; --from must explicitly name an existing holder.
func ClaimTask(tree *store.Tree, deps Deps, ref, from string) (ClaimResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ClaimResult{}, err
	}
	doc, rel, snapshot, err := claimTaskFile(tree, ref)
	if err != nil {
		return ClaimResult{}, err
	}
	prior, key := doc.Owner, deref(doc.Key)
	if prior == owner.ID {
		if from != "" && from != prior {
			return ClaimResult{}, claimHolderMismatchErr(store.NounTask, doc.ID, key, prior)
		}
		return ClaimResult{Noun: store.NounTask, ID: doc.ID, Key: key, Owner: prior}, nil
	}
	if err := guardOwnerMutable(store.NounTask, doc.ID, key, doc.Status, doc.Started, prior); err != nil {
		return ClaimResult{}, err
	}
	switch {
	case prior == "" && from != "", from != "" && from != prior:
		return ClaimResult{}, claimHolderMismatchErr(store.NounTask, doc.ID, key, prior)
	case prior != "" && from == "":
		return ClaimResult{}, claimHeldErr(store.NounTask, doc.ID, key, prior)
	}
	doc.Owner = owner.ID
	command := "claim task " + doc.ID
	if from != "" {
		command += " --from " + from
	}
	ev := events.NewEvent(deps.clock(), "claim.task", owner.ID, command, doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"owner": prior}
	ev.After = map[string]any{"owner": owner.ID}
	if prior != "" {
		ev.After["from"], ev.After["to"] = prior, owner.ID
	}
	if err := commitTaskOwner(tree, "claim task", doc, rel, snapshot, ev); err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Noun: store.NounTask, ID: doc.ID, Key: key, Owner: owner.ID, Changed: true, Takeover: prior != "", From: prior}, nil
}

func ReleaseTask(tree *store.Tree, deps Deps, ref string) (ReleaseResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ReleaseResult{}, err
	}
	doc, rel, snapshot, err := claimTaskFile(tree, ref)
	if err != nil {
		return ReleaseResult{}, err
	}
	prior, key := doc.Owner, deref(doc.Key)
	if err := guardOwnerMutable(store.NounTask, doc.ID, key, doc.Status, doc.Started, prior); err != nil {
		return ReleaseResult{}, err
	}
	if prior == "" {
		return ReleaseResult{Noun: store.NounTask, ID: doc.ID, Key: key}, nil
	}
	if prior != owner.ID {
		return ReleaseResult{}, notClaimantErr(store.NounTask, doc.ID, key, prior)
	}
	doc.Owner = ""
	ev := events.NewEvent(deps.clock(), "release.task", owner.ID, "release task "+doc.ID, doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before, ev.After = map[string]any{"owner": prior}, map[string]any{"owner": ""}
	if err := commitTaskOwner(tree, "release task", doc, rel, snapshot, ev); err != nil {
		return ReleaseResult{}, err
	}
	return ReleaseResult{Noun: store.NounTask, ID: doc.ID, Key: key, From: prior, Changed: true}, nil
}

func ReassignTask(tree *store.Tree, deps Deps, ref, to, reason string) (ReassignResult, error) {
	if strings.TrimSpace(to) == "" {
		return ReassignResult{}, &usageError{reason: "reassign_to_required", msg: "reassign needs --to <owner>"}
	}
	if strings.TrimSpace(reason) == "" {
		return ReassignResult{}, &usageError{reason: "reassign_reason_required", msg: "reassign needs --reason <text>"}
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ReassignResult{}, err
	}
	target, err := store.ResolveContributor(tree.Manifest, to)
	if err != nil {
		return ReassignResult{}, err
	}
	doc, rel, snapshot, err := claimTaskFile(tree, ref)
	if err != nil {
		return ReassignResult{}, err
	}
	prior, key := doc.Owner, deref(doc.Key)
	if err := guardOwnerMutable(store.NounTask, doc.ID, key, doc.Status, doc.Started, prior); err != nil {
		return ReassignResult{}, err
	}
	doc.Owner = target.ID
	ev := events.NewEvent(deps.clock(), "reassign.task", owner.ID, fmt.Sprintf("reassign task %s --to %s --reason %q", doc.ID, target.ID, reason), doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before, ev.After = map[string]any{"owner": prior}, map[string]any{"from": prior, "to": target.ID, "reason": reason}
	if err := commitTaskOwner(tree, "reassign task", doc, rel, snapshot, ev); err != nil {
		return ReassignResult{}, err
	}
	return ReassignResult{Noun: store.NounTask, ID: doc.ID, Key: key, From: prior, To: target.ID, Reason: reason}, nil
}

func commitTaskOwner(tree *store.Tree, operation string, doc *store.Task, rel string, snapshot store.FileSnapshot, ev events.Event) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	ev.Files = []events.FileChange{{Path: rel}}
	if err := commitMutation(tree, operation, []store.PlannedWrite{store.PlanWrite(filepath.Join(tree.Root, rel), snapshot, data)}, []events.Event{ev}); err != nil {
		return err
	}
	return nil
}

// OwnerImmutableError distinguishes a permanent execution boundary from a mutable reservation.
type OwnerImmutableError struct {
	Noun         string
	NodeID       string
	Key          string
	CurrentState string
	Owner        string
	Reason       string
	Seam         string
}

func (e *OwnerImmutableError) Error() string {
	return fmt.Sprintf("mutate: %s %s has immutable owner %q; started or terminal ownership cannot change", e.Noun, e.NodeID, e.Owner)
}

func guardOwnerMutable(noun, id, key, status string, started bool, owner string) error {
	if !started && !reviewGrainTaskInFlight(status) && status != statusDone && status != statusCancelled {
		return nil
	}
	seam := ""
	if status != statusDone && status != statusCancelled {
		seam = fmt.Sprintf(`tiller transfer %s %s --to <owner> --reason "<why>"`, noun, key)
	}
	return &OwnerImmutableError{Noun: noun, NodeID: id, Key: key, CurrentState: status, Owner: owner, Reason: "owner_immutable", Seam: seam}
}

type ClaimHeldError struct {
	Noun   string
	NodeID string
	Key    string
	Reason string
	Holder string
	Seam   string
}

func (e *ClaimHeldError) Error() string {
	return fmt.Sprintf("mutate: %s %s is claimed by %s", e.Noun, e.NodeID, e.Holder)
}
func claimHeldErr(noun, id, key, holder string) error {
	return &ClaimHeldError{Noun: noun, NodeID: id, Key: key, Reason: ReasonClaimHeld, Holder: holder, Seam: "--from " + holder}
}

type ClaimHolderMismatchError struct {
	Noun   string
	NodeID string
	Key    string
	Reason string
	Holder string
	Seam   string
}

func (e *ClaimHolderMismatchError) Error() string {
	if e.Holder == "" {
		return fmt.Sprintf("mutate: %s %s is unclaimed; --from names no holder to take over from", e.Noun, e.NodeID)
	}
	return fmt.Sprintf("mutate: %s %s's holder does not match --from (holder: %s)", e.Noun, e.NodeID, e.Holder)
}
func claimHolderMismatchErr(noun, id, key, holder string) error {
	seam := ""
	if holder != "" {
		seam = "--from " + holder
	}
	return &ClaimHolderMismatchError{Noun: noun, NodeID: id, Key: key, Reason: "claim_holder_mismatch", Holder: holder, Seam: seam}
}

type NotClaimantError struct {
	Noun   string
	NodeID string
	Key    string
	Reason string
	Holder string
	Seam   string
}

func (e *NotClaimantError) Error() string {
	return fmt.Sprintf("mutate: %s %s is claimed by %s, not the acting contributor", e.Noun, e.NodeID, e.Holder)
}
func notClaimantErr(noun, id, key, holder string) error {
	return &NotClaimantError{Noun: noun, NodeID: id, Key: key, Reason: "not_claimant", Holder: holder, Seam: fmt.Sprintf(`tiller reassign %s %s --to <owner> --reason "<why>"`, noun, key)}
}

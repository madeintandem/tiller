package mutate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// eventAmendmentProposed is the hand-over event: draft -> proposed.
const eventAmendmentProposed = "amendment_proposed"

// ProposeResult is the outcome of handing an amendment over. Empty reports an
// amendment that proposes nothing (approve will refuse it; withdraw it).
// Warnings name each stale precondition the hand-over let through.
type ProposeResult struct {
	ID       string
	Key      string
	Status   string
	Empty    bool
	Warnings []string
}

// ProposeAmendment hands a draft amendment over: draft -> proposed, the agent's
// "I have finished" move, as `spec feature` is for a feature. It runs the
// checks that do not depend on the human's review (source honesty and every
// operation's citation, the same ones approve runs) and refuses on them with
// nothing written. A stale precondition is not a refusal here, since the tree
// may move again before approval; it comes back as a warning. A proposed
// amendment proposes again as a no-op; a terminal one is refused.
func ProposeAmendment(tree *store.Tree, deps Deps, amendment *store.Amendment) (ProposeResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ProposeResult{}, err
	}
	doc, relYml, snapshot, err := loadAmendmentDoc(tree, amendment)
	if err != nil {
		return ProposeResult{}, err
	}
	if !store.AmendmentOpen(doc) {
		return ProposeResult{}, amendmentNotOpenErr(doc)
	}
	empty := store.AmendmentEmpty(doc)
	result := ProposeResult{ID: doc.ID, Key: deref(doc.Key), Status: doc.Status, Empty: empty}
	if doc.Status == store.AmendmentStatusProposed {
		return result, nil
	}
	if err := store.ValidateAmendment(doc); err != nil {
		return ProposeResult{}, err
	}
	if err := checkHeldNodes(tree, doc); err != nil {
		return ProposeResult{}, err
	}
	if err := checkSource(tree, deps.rev(tree.Root), doc); err != nil {
		return ProposeResult{}, err
	}
	if err := checkOpSources(doc); err != nil {
		return ProposeResult{}, err
	}
	validity, err := EvaluateAmendmentPreconditions(tree, deps, amendment)
	if err != nil {
		return ProposeResult{}, err
	}
	for _, pv := range validity {
		if pv.Status == PreconditionStale {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"operation %d: %s is stale (expected %s, now %s)", pv.Index, pv.Precondition, pv.Expected, pv.Actual))
		}
	}

	doc.Status = store.AmendmentStatusProposed
	data, err := yaml.Marshal(doc)
	if err != nil {
		return ProposeResult{}, err
	}
	ev := events.NewEvent(deps.clock(), eventAmendmentProposed, owner.ID, "propose amendment "+doc.ID, doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": store.AmendmentStatusProposed, "empty": empty}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitNodeWrites(tree, "propose.amendment", []nodeWrite{
		{rel: relYml, data: data, before: snapshot, event: ev},
	}); err != nil {
		return ProposeResult{}, err
	}
	result.Status = doc.Status
	return result, nil
}

// checkHeldNodes refuses an amendment one of whose add operations names a node
// it does not hold: one the tree no longer carries (deleted outside Tiller, or
// a hand-edited id), or one the hold index gives to a different amendment (two
// amendments naming one node, after a hand edit or a merge). Either way the
// human would be reviewing a draft this amendment cannot promote. An operation
// marked removed is exempt, since its node is gone by design, and an add
// staged before schema 15 carries no node to check.
//
// Holding a node is not enough: the amendment must also have staged it. A
// hand-edited node: naming a draft nobody holds wins the hold on load, and
// promoting it would approve work this amendment never proposed, so the event
// log's record of who staged the node is checked too (stagedNodes).
func checkHeldNodes(tree *store.Tree, doc *store.Amendment) error {
	var staged map[string]string
	for i, op := range doc.Operations {
		if op.Node == "" || op.Removed != "" {
			continue
		}
		n, err := heldDraftOf(tree, doc, i, op)
		if err != nil {
			return err
		}
		if staged == nil {
			if staged, err = stagedNodes(tree); err != nil {
				return err
			}
		}
		if staged[op.Node] != doc.ID {
			return &usageError{
				reason: "held_node_missing",
				msg: fmt.Sprintf("operation %d (%s %s) names %s, which this amendment did not stage; withdraw this amendment (the node stays) and stage it again",
					i, op.Op, op.Key, gen.Ref(tree, n)),
			}
		}
	}
	return nil
}

// stagedNodes maps each node an amendment's staging created to that
// amendment's id, read from the add events staging writes (planStagedAdd puts
// the node first on Targets and the amendment on After). It is the provenance
// behind a hold: the hold index is derived from the amendment files, which a
// hand edit or a merge can point at any node, while the event log records
// which amendment actually created the draft.
func stagedNodes(tree *store.Tree) (map[string]string, error) {
	history, err := events.List(tree)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, ev := range history {
		if !strings.HasPrefix(ev.Type, "add.") || len(ev.Targets) == 0 {
			continue
		}
		if amendmentID, _ := ev.After["amendment"].(string); amendmentID != "" {
			out[ev.Targets[0]] = amendmentID
		}
	}
	return out, nil
}

// heldDraftOf is the draft the add operation at index holds for doc, the one
// node approval may promote for it. It refuses held_node_missing when the
// node is gone or when the hold index (store.Tree.AmendmentHolds) names a
// different holder, naming that holder.
func heldDraftOf(tree *store.Tree, doc *store.Amendment, index int, op store.AmendmentOp) (store.Node, error) {
	n := tree.AddedNode(op)
	if n == nil {
		return nil, &usageError{
			reason: "held_node_missing",
			msg: fmt.Sprintf("operation %d (%s %s) holds %s, which is not in the tree; withdraw the amendment and stage it again",
				index, op.Op, op.Key, op.Node),
		}
	}
	holderID := tree.AmendmentHolds[op.Node]
	if holderID == doc.ID {
		return n, nil
	}
	holder := "no amendment holds it"
	if a := tree.Amendments[holderID]; a != nil {
		holder = "amendment " + gen.Ref(tree, a) + " holds it, not this one"
	}
	return nil, &usageError{
		reason: "held_node_missing",
		msg: fmt.Sprintf("operation %d (%s %s) names %s, but %s; withdraw this amendment and stage it again",
			index, op.Op, op.Key, gen.Ref(tree, n), holder),
	}
}

// operationRefusal prefixes a refusal approval found on one operation with
// its index, keeping the refusal's reason.
func operationRefusal(index int, err error) error {
	var ue *usageError
	if errors.As(err, &ue) {
		return &usageError{reason: ue.reason, msg: fmt.Sprintf("operation %d: %s", index, ue.msg), cause: ue.cause}
	}
	return err
}

package mutate

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/core/events"
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

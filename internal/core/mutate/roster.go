package mutate

// Contributor registration and retirement own the committed roster. Removal
// preserves historical attribution and refuses live claims, open questions,
// and self-lockout.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// Event type discriminators for new contributor roster mutations.
const (
	eventContributorAdd    = "contributor.add"
	eventContributorRemove = "contributor.remove"
)

// ContributorAddOutcome distinguishes why AddContributor did or did not change the
// roster: a fresh registration, an existing id's display name updated, or a
// no-op because the id and name already matched. The CLI's own summary line
// branches on it.
type ContributorAddOutcome int

const (
	ContributorAddNoChange ContributorAddOutcome = iota
	ContributorAddRegistered
	ContributorAddNameUpdated
)

// AddContributorResult reports the registered contributor, resulting roster,
// and whether registration or a display-name update changed it.
type AddContributorResult struct {
	Contributor store.Contributor
	Roster      []store.Contributor
	Outcome     ContributorAddOutcome
}

// AddContributor registers a contributor or updates their display name.
// Registration needs no acting contributor, including on an empty roster:
// the event attributes to the contributor being added.
// A no-op writes neither state nor an event.
func AddContributor(tree *store.Tree, deps Deps, owner store.Contributor) (AddContributorResult, error) {
	manifestRelPath := filepath.Join(".tiller", "tiller.yml")
	manifestPath := filepath.Join(tree.Root, manifestRelPath)
	manifestData, manifestSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, manifestPath)
	if err != nil {
		return AddContributorResult{}, err
	}
	if !manifestSnapshot.Exists() {
		return AddContributorResult{}, fmt.Errorf("read %s: %w", manifestPath, os.ErrNotExist)
	}
	manifest, err := store.ParseManifest(tree.Root, manifestData)
	if err != nil {
		return AddContributorResult{}, err
	}

	var priorName string
	existed := false
	for _, o := range manifest.Contributors {
		if o.ID == owner.ID {
			existed = true
			priorName = o.Name
			break
		}
	}

	// The CLI rejects blank names before this point, so false means no change.
	if !store.AddContributor(manifest, owner) {
		return AddContributorResult{Contributor: owner, Roster: manifest.Contributors, Outcome: ContributorAddNoChange}, nil
	}
	rendered, err := store.RenderContributors(manifestData, manifest.Contributors)
	if err != nil {
		return AddContributorResult{}, err
	}

	outcome := ContributorAddRegistered
	if existed {
		outcome = ContributorAddNameUpdated
	}

	ev := events.NewEvent(deps.clock(), eventContributorAdd, owner.ID, "add contributor "+owner.ID, owner.ID)
	ev.ID = deps.mintRaw("EV")
	if existed {
		ev.Before = map[string]any{"name": priorName}
	}
	ev.After = map[string]any{"name": owner.Name}
	ev.Files = []events.FileChange{{Path: manifestRelPath}}

	if err := commitMutation(tree, "add contributor", []store.PlannedWrite{
		store.PlanWrite(manifestPath, manifestSnapshot, rendered),
	}, []events.Event{ev}); err != nil {
		return AddContributorResult{}, err
	}

	return AddContributorResult{Contributor: owner, Roster: manifest.Contributors, Outcome: outcome}, nil
}

// RemoveContributorResult reports the retired contributor and resulting roster.
// An absent id refuses rather than silently succeeding.
type RemoveContributorResult struct {
	Removed store.Contributor
	Roster  []store.Contributor
}

// RemoveContributor retires a roster entry, preserving all historical IDs.
// Live work claims and open questions block removal; another contributor must
// retire the acting contributor to prevent self-lockout.
func RemoveContributor(tree *store.Tree, deps Deps, id string) (RemoveContributorResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return RemoveContributorResult{}, err
	}
	target, err := store.ResolveContributor(tree.Manifest, id)
	if err != nil {
		return RemoveContributorResult{}, err
	}
	if target.ID == owner.ID {
		return RemoveContributorResult{}, &RemoveContributorSelfError{ID: target.ID, Reason: ReasonRemoveContributorSelf}
	}
	if holdings := contributorHoldings(tree, target.ID); holdings.blocking() {
		return RemoveContributorResult{}, holdings.asError(target.ID)
	}

	manifestRelPath := filepath.Join(".tiller", "tiller.yml")
	manifestPath := filepath.Join(tree.Root, manifestRelPath)
	manifestData, manifestSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, manifestPath)
	if err != nil {
		return RemoveContributorResult{}, err
	}
	if !manifestSnapshot.Exists() {
		return RemoveContributorResult{}, fmt.Errorf("read %s: %w", manifestPath, os.ErrNotExist)
	}
	manifest, err := store.ParseManifest(tree.Root, manifestData)
	if err != nil {
		return RemoveContributorResult{}, err
	}
	if !store.RemoveContributor(manifest, target.ID) {
		// The roster changed between the ResolveContributor check above and this
		// snapshot-safe read (a concurrent remove, or a hand edit): the id this
		// call resolved against is no longer there to remove. A tool failure,
		// not the census refusal or a silent success, since neither is honest
		// about what actually happened.
		return RemoveContributorResult{}, fmt.Errorf("mutate: contributor %s vanished from the roster after it was resolved; reload and retry", target.ID)
	}
	rendered, err := store.RenderContributors(manifestData, manifest.Contributors)
	if err != nil {
		return RemoveContributorResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventContributorRemove, owner.ID, "remove contributor "+target.ID, target.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"name": target.Name}
	ev.Files = []events.FileChange{{Path: manifestRelPath}}

	if err := commitMutation(tree, "remove contributor", []store.PlannedWrite{
		store.PlanWrite(manifestPath, manifestSnapshot, rendered),
	}, []events.Event{ev}); err != nil {
		return RemoveContributorResult{}, err
	}

	return RemoveContributorResult{Removed: target, Roster: manifest.Contributors}, nil
}

// ReasonRemoveContributorSelf and ReasonRemoveContributorHoldings are the stable tokens
// contributor removal's two refusals carry, minted here rather than at the CLI
// layer: the same convention ReasonClaimHeld (claim.go) already keeps, so
// the core names the fact and the CLI only ever composes it.
const (
	ReasonRemoveContributorSelf     = "cannot_remove_self"
	ReasonRemoveContributorHoldings = "contributor_holds_live_work"
)

// RemoveContributorSelfError refuses self-removal by the acting contributor:
// design's own no-self-lockout rule ("The verbs"). A well-formed request,
// refused by who is asking rather than by the id's shape, so this carries
// the same blocked-answer shape as ClaimHeldError's family: exit 1,
// rendered through the CLI's emitReport as an ok:false data block, and it
// deliberately does not implement ExitCode, because the report sets that.
type RemoveContributorSelfError struct {
	ID     string
	Reason string
}

func (e *RemoveContributorSelfError) Error() string {
	return fmt.Sprintf("mutate: %s cannot remove themselves from the roster", e.ID)
}

// RemoveContributorHoldings counts unfinished execution assignments and open questions.
// Terminal ownership remains historical attribution and does not block removal.
type RemoveContributorHoldings struct {
	Tasks     int
	Bugs      int
	Chores    int
	Tweaks    int
	Questions int
}

func (h RemoveContributorHoldings) blocking() bool {
	return h.Tasks > 0 || h.Bugs > 0 || h.Chores > 0 || h.Tweaks > 0 || h.Questions > 0
}

func (h RemoveContributorHoldings) asError(id string) error {
	return &RemoveContributorHoldingsError{ID: id, Reason: ReasonRemoveContributorHoldings, Holdings: h}
}

// RemoveContributorHoldingsError refuses retirement while the contributor holds
// live work or open questions. Like RemoveContributorSelfError, it is rendered
// as a blocked answer (exit 1), not an environment failure.
type RemoveContributorHoldingsError struct {
	ID       string
	Reason   string
	Holdings RemoveContributorHoldings
}

func (e *RemoveContributorHoldingsError) Error() string {
	h := e.Holdings
	return fmt.Sprintf("mutate: contributor %s still holds %d live task(s), %d live bug(s), %d live chore(s), %d live tweak(s), %d open question(s)",
		e.ID, h.Tasks, h.Bugs, h.Chores, h.Tweaks, h.Questions)
}

// contributorHoldings includes every nonterminal assignment, not only active work.
func contributorHoldings(tree *store.Tree, id string) RemoveContributorHoldings {
	var h RemoveContributorHoldings
	for _, tk := range tree.Tasks {
		if tk.Owner == id && tk.Status != statusDone && tk.Status != statusCancelled {
			h.Tasks++
		}
	}
	for _, w := range tree.WorkNodesOf(store.NounBug) {
		if b := w.Body(); b.Owner == id && b.Status != statusDone && b.Status != statusCancelled {
			h.Bugs++
		}
	}
	for _, w := range tree.WorkNodesOf(store.NounChore) {
		if b := w.Body(); b.Owner == id && b.Status != statusDone && b.Status != statusCancelled {
			h.Chores++
		}
	}
	for _, w := range tree.WorkNodesOf(store.NounTweak) {
		if b := w.Body(); b.Owner == id && b.Status != statusDone && b.Status != statusCancelled {
			h.Tweaks++
		}
	}
	for _, q := range tree.Questions {
		if q.Owner == id && gen.QuestionOpen(q) {
			h.Questions++
		}
	}
	return h
}

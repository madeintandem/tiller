package gen

// This file holds the derived-activity rules for epics and features. An epic has
// no stored 'active' state (artifact-schema.md, Epics States): the stored
// lifecycle is draft -> ready -> done -> archived, plus cancelled. Whether an
// approved epic is being worked is COMPUTED from its children rather than stored
// and mutated, so the .yml keeps the stored 'ready'/'done' while the status
// oracle and console can show the epic as 'active'. The derivation is a pure
// function of the loaded tree, shared by CLI status and live console reads.

import "github.com/madeintandem/tiller/internal/store"

// UnfinishedAssignment includes every nonterminal execution assignment,
// including proposed reservations, checked work, and work awaiting review.
// Callers must also enforce execution noun and a nonempty scalar owner.
func UnfinishedAssignment(status string) bool {
	return status != "done" && status != "cancelled"
}

// ReviewState is the single in-flight state the gates lens reports on, kept out
// of the active-work lens so the two do not double-count a node.
const ReviewState = "in_review"

// OnActiveLens excludes review only because the gates lens displays it once.
func OnActiveLens(status string) bool {
	return UnfinishedAssignment(status) && status != ReviewState
}

// Epic activity values: the derived, display-only working status of an epic.
const (
	EpicActivityActive = "active"
	EpicActivityIdle   = "idle"
)

// inFlightFeatureStates are the feature states that mean a feature is actively
// being worked. There is exactly one: a feature is started (active) and stays
// there while its task PRs move through review, since a PR always represents a
// task (artifact-schema.md, Features States).
var inFlightFeatureStates = map[string]bool{"active": true}

// inFlightTaskStates are the task states that mean code work is underway under a
// feature, even one whose own status has not moved past ready yet.
var inFlightTaskStates = map[string]bool{"in_progress": true, "check": true, "in_review": true}

// EpicActivity returns the epic's derived working status: EpicActivityActive when
// the epic is approved (stored status 'ready') AND at least one child feature is
// in flight, EpicActivityIdle otherwise. "In flight" means a child feature that
// is itself being worked (active) or one with a task in progress or in review.
// The derivation never reads or writes a stored 'active' epic state; it is the
// display-only answer to "is this approved epic being worked", computed fresh
// from the children every time. A non-ready epic (draft, done, archived,
// cancelled) is always idle.
func EpicActivity(t *store.Tree, e *store.Epic) string {
	if e.Status != "ready" {
		return EpicActivityIdle
	}
	if epicHasWorkInFlight(t, e) {
		return EpicActivityActive
	}
	return EpicActivityIdle
}

// epicHasWorkInFlight reports whether any feature under the epic, or any task
// under such a feature, is actively being worked. A feature is a child of the
// epic when its EpicKey equals the epic's key (the folder link the store sets on
// Load).
func epicHasWorkInFlight(t *store.Tree, e *store.Epic) bool {
	if e.Key == nil {
		return false
	}
	epicKey := *e.Key
	for _, f := range t.Features {
		if f.EpicKey != epicKey {
			continue
		}
		if inFlightFeatureStates[f.Status] || featureHasTaskInFlight(t, f) {
			return true
		}
	}
	return false
}

func featureHasTaskInFlight(t *store.Tree, f *store.Feature) bool {
	for _, tk := range t.Tasks {
		if tk.FeatureID == f.ID && inFlightTaskStates[tk.Status] {
			return true
		}
	}
	return false
}

// FeatureBoardStatus is the status column a feature occupies on a status
// board: its stored status, except that a ready feature with a task in flight
// reads "active". Under feature-grain review the start that flips the feature
// to active is committed on the unmerged review branch, while the console
// observes its tasks' execution from their branches, so the served feature
// can still read ready with work underway. Display-only, like EpicActivity:
// the stored status is never replaced.
func FeatureBoardStatus(t *store.Tree, f *store.Feature) string {
	if f.Status == "ready" && featureHasTaskInFlight(t, f) {
		return "active"
	}
	return f.Status
}

// Feature activity values: the derived, display-only rollup phase of an active
// feature (docs/spec/cli-surface.md). Like the
// epic's derived status above, the phase is computed on every read and never
// stored; feature.yml keeps the stored "active" either way.
const (
	FeatureActivityAwaitingCheck = "awaiting_check"
	FeatureActivityAwaitingSync  = "awaiting_sync"
)

// FeatureActivity returns the feature's derived rollup phase: awaiting_check
// when the feature is active, every non-cancelled task under it is done (and
// there is at least one, mirroring the rollup's refusal to fire over an empty
// set), and some required feature-grain gate has no recorded result;
// awaiting_sync when every such gate carries one, which is exactly the state
// the rollup fires on (mutate.featureRollupReady delegates here, so what the
// console shows as awaiting sync is precisely what the next sync will cross);
// "" otherwise. Presence is the bar, not green: a recorded fail or stale
// result still counts as checked, matching advisory enforcement.
func FeatureActivity(t *store.Tree, f *store.Feature) string {
	if f.Status != "active" {
		return ""
	}
	nonCancelled, done := 0, 0
	for _, tk := range t.Tasks {
		if tk.FeatureID != f.ID || tk.Status == "cancelled" {
			continue
		}
		nonCancelled++
		if tk.Status == "done" {
			done++
		}
	}
	if nonCancelled == 0 || done != nonCancelled {
		return ""
	}
	for name, spec := range t.Manifest.Gates.Required {
		if !spec.On.Includes(store.NounFeature) {
			continue
		}
		if _, ok := f.GatesBlock.Gates[name]; !ok {
			return FeatureActivityAwaitingCheck
		}
	}
	return FeatureActivityAwaitingSync
}

// FeatureDisplay is a feature's after-the-colon status for the human lenses
// that already render the epic's derived status this way ("ready, active"):
// the stored status with the derived phase appended, "active, awaiting check"
// or "active, awaiting sync". The stored status is never replaced.
func FeatureDisplay(t *store.Tree, f *store.Feature) string {
	switch FeatureActivity(t, f) {
	case FeatureActivityAwaitingCheck:
		return f.Status + ", awaiting check"
	case FeatureActivityAwaitingSync:
		return f.Status + ", awaiting sync"
	}
	return f.Status
}

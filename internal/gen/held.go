package gen

// This file derives the held-by-retired-ancestor annotation the read surfaces
// show before a human runs a doing verb that would refuse (issue #36,
// "annotate, don't subtract"): legal_next and NextOwner stay pure lifecycle
// projections, untouched by this file, while a caller that wants to say WHY
// those verbs would refuse reads this derived predicate instead.
// mutate.guardParentState (internal/core/mutate/transition.go) is the arm that
// actually refuses; its own doc comment points back here, and this one points
// at it, so an edit to either walk's shape has to update its cross-referenced
// twin.
//
// The predicate fires ONLY for a RETIRED ancestor: a cancelled feature, or a
// cancelled-or-archived epic. A cancelled feature answers first, since its
// remedy (cancel the task too) is reachable and it is the deepest actionable
// node, so it wins even when the epic above it is also retired. Past that,
// the epic question runs BEFORE the pre-gate parent states (draft, specced):
// a proposed task under a draft or specced feature is ordinarily the
// mid-specify shape, ahead of the plan gate rather than past a retirement,
// but that pre-gate seam is a one-hop dead end exactly when the epic itself
// is retired, since the remedy it would name refuses ParentEpicRetiredError
// the moment it runs. So the epic answer outranks the pre-gate arms there,
// and only there (issue #38, JC's call 2026-08-03). A pre-gate feature under
// a LIVE epic still holds nothing, guardParentState's own live route to the
// gate. And a node that is itself terminal (done or cancelled) is never
// held: a cancelled task under a cancelled feature already finished, it was
// not left holding anything.

import "github.com/madeintandem/tiller/internal/store"

// Held names the nearest retired ancestor a live node sits under: the noun
// ("feature" or "epic") and human reference of the cancelled feature, or the
// cancelled-or-archived epic, whose retirement is why the doing verbs refuse
// from here, plus that ancestor's own state ("cancelled" or "archived").
type Held struct {
	Noun  string
	ID    string
	Ref   string
	State string
}

// RetiredAncestor answers gen's derived half of guardParentState's guard: the
// nearest retired ancestor of n, cancelled feature before retired epic before
// pre-gate for a task, exactly the order guardParentState's own checks run (a
// feature's cancelled arm answers before the walk ever reaches the epic, and
// the epic answer outranks a merely pre-gate parent, issue #38). nil when n
// is itself terminal, when every ancestor is live, or when the immediate
// parent is pre-gate (draft or specced) under a LIVE epic. Only a task and a
// feature can come back held; an epic has no ancestor to retire under, and a
// question sits outside the work tree, so both answer nil unconditionally.
func RetiredAncestor(t *store.Tree, n store.Node) *Held {
	switch node := n.(type) {
	case *store.Task:
		return retiredAncestorOfTask(t, node)
	case *store.Feature:
		return retiredAncestorOfFeature(t, node)
	default:
		return nil
	}
}

// retiredAncestorOfTask mirrors guardParentState's own control flow over the
// parent feature's status, reordered for issue #38: cancelled names the
// feature directly, the same early return guardParentState's own cancelled
// check makes first. Otherwise the epic question runs next, exactly the way
// the guard's own call to epicRetired now runs ahead of its pre-gate switch;
// only once that comes back nil (a live epic) does draft or specced stop
// here, pre-gate rather than retired. Anything else (ready, active, done)
// already fell through the epic check above, so there is nothing left to ask.
func retiredAncestorOfTask(t *store.Tree, tk *store.Task) *Held {
	if terminalStates[tk.Status] {
		return nil
	}
	feat := t.Features[tk.FeatureID]
	if feat == nil {
		return nil
	}
	if feat.Status == "cancelled" {
		return &Held{Noun: store.NounFeature, ID: feat.ID, Ref: Ref(t, feat), State: "cancelled"}
	}
	return retiredEpicAncestor(t, feat)
}

// retiredAncestorOfFeature answers a live feature's own held state: a feature
// has no feature ancestor, so only its epic can retire it.
func retiredAncestorOfFeature(t *store.Tree, feat *store.Feature) *Held {
	if terminalStates[feat.Status] {
		return nil
	}
	return retiredEpicAncestor(t, feat)
}

// retiredEpicAncestor is the epic-grain half both callers share: nil unless
// the feature's own epic is cancelled or archived, mirroring
// mutate.epicRetired's identical rule (transition.go).
func retiredEpicAncestor(t *store.Tree, feat *store.Feature) *Held {
	epic := heldEpicByKey(t, feat.EpicKey)
	if epic == nil {
		return nil
	}
	if epic.Status != "cancelled" && epic.Status != "archived" {
		return nil
	}
	return &Held{Noun: store.NounEpic, ID: epic.ID, Ref: Ref(t, epic), State: epic.Status}
}

// heldEpicByKey finds a tree's epic by its canonical key, mirrored from
// mutate.epicByKey and views.recapEpicByKey (each package keeps its own copy
// rather than importing across the package graph: gen must not import mutate
// or views, and the walk is three lines).
func heldEpicByKey(t *store.Tree, key string) *store.Epic {
	if key == "" {
		return nil
	}
	for _, e := range t.Epics {
		if e.Key != nil && *e.Key == key {
			return e
		}
	}
	return nil
}

// terminalStates must not regain holds from retired ancestors.
var terminalStates = map[string]bool{"done": true, "cancelled": true}

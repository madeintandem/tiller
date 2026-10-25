package drift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// ForDesign shares Run's per-document comparison. Design docs are addressed by
// ledger path, never by a synthetic work-node id.
func ForDesign(t *store.Tree, rev vcs.Revisions, doc store.BaselineDoc) ([]Candidate, []Uncovered, error) {
	enabled, err := enabledSet(t.Manifest.Drift.Checks)
	if err != nil {
		return nil, nil, err
	}
	rep := Report{Coverage: map[string]map[string]int{}}
	for _, marker := range []string{MarkerStaleDesign, MarkerShiftedDesign} {
		if enabled[marker] {
			reportDesign(t, rev, &rep, doc, marker)
		}
	}
	return rep.Candidates, rep.Uncovered, nil
}

func sweepDesign(t *store.Tree, rev vcs.Revisions, rep *Report, marker string) {
	docs := append([]store.BaselineDoc(nil), t.Baseline.Docs...)
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	for _, doc := range docs {
		reportDesign(t, rev, rep, doc, marker)
	}
}

func reportDesign(t *store.Tree, rev vcs.Revisions, rep *Report, doc store.BaselineDoc, marker string) {
	before := len(rep.Uncovered)
	uncovered := func(reason, detail string) {
		rep.Uncovered = append(rep.Uncovered, Uncovered{Marker: marker, Noun: "design", Key: doc.Path, Reason: reason, Detail: detail})
	}
	if doc.Confirmed == nil {
		uncovered("unconfirmed", "no human confirmation of the described code is recorded")
		bumpCoverage(rep, marker, "unresolvable")
		return
	}
	candidate := Candidate{Marker: marker, Noun: "design", Key: doc.Path, Since: &doc.Confirmed.Commit}
	affected := map[string]bool{}
	var missing []string
	stamps := make(map[string]store.SurfaceFingerprint, len(doc.Confirmed.Surfaces))
	for _, stamp := range doc.Confirmed.Surfaces {
		stamps[stamp.Path] = stamp
	}
	for _, described := range doc.Describes {
		var err error
		if marker == MarkerStaleDesign {
			err = mutate.CheckDesignSurface(t.Root, t.Manifest, described)
		} else {
			var current store.SurfaceFingerprint
			current, err = mutate.ObserveDesignSurface(t.Root, t.Manifest, described, time.Time{})
			if err == nil {
				stamp, ok := stamps[described]
				if !ok {
					uncovered("surface_unrecorded", fmt.Sprintf("%s has no confirmation fingerprint", described))
				} else if current.Kind != stamp.Kind || current.Value != stamp.Value {
					affected[described] = true
				}
			}
		}
		if errors.Is(err, os.ErrNotExist) {
			affected[described] = true
			missing = append(missing, described)
		} else if err != nil {
			uncovered("surface_unreadable", fmt.Sprintf("%s: %v", described, err))
		}
	}

	historyBefore := len(rep.Uncovered)
	history := designHistory{rev: rev, resolved: map[string]error{}}
	if err := history.resolve(doc.Confirmed.Commit); err != nil {
		uncovered("history_unavailable", err.Error())
	} else if marker == MarkerStaleDesign {
		var crossings []store.RevisionRef
		for _, id := range sortedFeatureIDs(t) {
			feature := t.Features[id]
			if feature.Status != "done" {
				continue
			}
			paths := designOverlaps(t, doc.Describes, feature.Surface.Paths)
			if len(paths) == 0 {
				continue
			}
			crossing := feature.VerifiedAgainst.Commit
			relation, err := history.relation(doc.Confirmed.Commit, crossing)
			if err != nil {
				uncovered("history_unavailable", fmt.Sprintf("feature %s: %v", gen.Ref(t, feature), err))
				continue
			}
			if relation == designDiverged {
				uncovered("history_diverged", fmt.Sprintf("feature %s crossing %s and confirmation %s have diverged; no bounded range", gen.Ref(t, feature), crossing.Value, doc.Confirmed.Commit.Value))
				continue
			}
			if relation == designAfter {
				continue // crossing is already an ancestor of confirmation
			}
			if relation == designEqual {
				if crossing.ObservedAt.IsZero() || doc.Confirmed.Commit.ObservedAt.IsZero() {
					uncovered("history_unavailable", fmt.Sprintf("feature %s shares the confirmation commit but has no ordering observation", gen.Ref(t, feature)))
					continue
				}
				if !crossing.ObservedAt.After(doc.Confirmed.Commit.ObservedAt) {
					continue
				}
			}
			for _, p := range paths {
				affected[p] = true
			}
			spec, _ := gen.ProsePath(t, feature)
			candidate.Features = append(candidate.Features, DesignFeature{ID: feature.ID, Key: nodeKey(feature), Spec: spec, Commit: crossing})
			crossings = append(crossings, crossing)
		}
		if len(crossings) > 0 {
			latest := crossings[0]
			bounded := true
			for _, crossing := range crossings[1:] {
				relation, err := history.relation(latest, crossing)
				if err != nil {
					uncovered("history_unavailable", err.Error())
					bounded = false
					break
				}
				if relation == designBefore || (relation == designEqual && crossing.ObservedAt.After(latest.ObservedAt)) {
					latest = crossing
				}
			}
			for _, crossing := range crossings {
				if !bounded {
					break
				}
				relation, err := history.relation(crossing, latest)
				if err != nil {
					uncovered("history_unavailable", err.Error())
					bounded = false
				} else if relation != designBefore && relation != designEqual {
					uncovered("history_diverged", "matching feature crossings have diverged; no single latest crossing bounds all work")
					bounded = false
				}
			}
			if bounded && len(rep.Uncovered) == historyBefore {
				candidate.Until = &latest
			}
		}
	} else {
		head, err := rev.CurrentCommit()
		if err != nil {
			uncovered("history_unavailable", fmt.Sprintf("current project HEAD: %v", err))
		} else if relation, err := history.relation(doc.Confirmed.Commit, head); err != nil {
			uncovered("history_unavailable", err.Error())
		} else if relation != designBefore && relation != designEqual {
			uncovered("history_diverged", "confirmation is not an ancestor of current project HEAD; no bounded range")
		} else {
			candidate.Until = &head
		}
	}

	if len(affected) > 0 {
		for p := range affected {
			candidate.Files = append(candidate.Files, p)
		}
		sort.Strings(candidate.Files)
		if marker == MarkerStaleDesign {
			candidate.Detail = fmt.Sprintf("%d done feature(s) crossed after confirmation on described code", len(candidate.Features))
			if len(candidate.Features) == 0 {
				candidate.Detail = "described code no longer exists"
			}
		} else {
			candidate.Detail = "described working-tree code no longer matches confirmation"
		}
		if len(missing) > 0 {
			candidate.Context = append(candidate.Context, "missing described paths: "+strings.Join(missing, ", "))
		}
		candidate.Context = append(candidate.Context, "Review and update the prose against current code, then have a human reconfirm the named doc.")
		candidate.Context = append(candidate.Context, "History anchors are project commits, not recoverable confirmation bytes: dirty or external code may never have existed in those commits.")
		rep.Candidates = append(rep.Candidates, candidate)
	}
	if len(rep.Uncovered) > before {
		bumpCoverage(rep, marker, "unresolvable")
	} else {
		bumpCoverage(rep, marker, "checked")
		if rep.comparedDesign == nil {
			rep.comparedDesign = map[string]bool{}
		}
		rep.comparedDesign[doc.Path] = true
	}
}

// Normalize both lists in the same coordinates the ledger writer records.
// filepath.Rel's component boundary avoids app matching apple, and lets '.'
// cover project code without incorrectly covering ../external.
func designOverlaps(t *store.Tree, described, surfaces []string) []string {
	var matches []string
	surfaces = mutate.DesignSurfaceCoordinates(t.Root, t.Manifest, surfaces)
	for _, p := range described {
		matched := false
		for _, a := range mutate.DesignSurfaceCoordinates(t.Root, t.Manifest, []string{p}) {
			for _, b := range surfaces {
				if designContains(a, b) || designContains(b, a) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			matches = append(matches, p)
		}
	}
	return matches
}

func designContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

const (
	designEqual = iota
	designBefore
	designAfter
	designDiverged
)

type designHistory struct {
	rev      vcs.Revisions
	resolved map[string]error
}

func (h *designHistory) resolve(ref store.RevisionRef) error {
	if ref.Kind != store.KindGitCommit || !strings.HasPrefix(ref.Value, "git:") || strings.TrimPrefix(ref.Value, "git:") == "" {
		return fmt.Errorf("missing or invalid project commit anchor %q", ref.Value)
	}
	if err, ok := h.resolved[ref.Value]; ok {
		return err
	}
	var err error
	if h.rev == nil {
		err = fmt.Errorf("project history is unavailable")
	} else if exists, failure := h.rev.RefExists(strings.TrimPrefix(ref.Value, "git:") + "^{commit}"); failure != nil {
		err = failure
	} else if !exists {
		err = fmt.Errorf("commit %s is unavailable", ref.Value)
	}
	h.resolved[ref.Value] = err
	return err
}

func (h *designHistory) relation(a, b store.RevisionRef) (int, error) {
	if err := h.resolve(a); err != nil {
		return 0, err
	}
	if err := h.resolve(b); err != nil {
		return 0, err
	}
	left, right := strings.TrimPrefix(a.Value, "git:"), strings.TrimPrefix(b.Value, "git:")
	if left == right {
		return designEqual, nil
	}
	before, err := h.rev.IsMergedInto(left, right)
	if err != nil {
		return 0, err
	}
	after, err := h.rev.IsMergedInto(right, left)
	if err != nil {
		return 0, err
	}
	switch {
	case before && after:
		return designEqual, nil
	case before:
		return designBefore, nil
	case after:
		return designAfter, nil
	default:
		return designDiverged, nil
	}
}

// Package readiness evaluates the pre-decomposition readiness gate
// (cli-surface.md, "Bootstrapping a project"): whether a project's inputs and
// manifest are coherent enough to begin decomposition. Most of that is a thin
// wrapper over store.Manifest's validation that shapes the result for
// reporting, but Check also reads .tiller/orientation.md off disk itself and
// compares its stamped generation against gen.OrientationGeneration()
// (issue #41): that check is not a store.Manifest shape concern, since store
// cannot import gen without an import cycle (gen already imports store), so
// it lives here instead. It holds no cobra dependency and never writes to
// stdout; the CLI renders the Report into a JSON envelope.
package readiness

import (
	"errors"
	"fmt"
	"sort"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// ReasonStaleOrientation is the Gap reason when the on-disk
// .tiller/orientation.md carries a generation stamp that disagrees with the
// binary's, whichever side is ahead. It is not a store.Manifest shape concern
// (store.Manifest.Gaps cannot import gen without an import cycle, since gen
// already imports store), so it lives here beside the check that produces it.
const ReasonStaleOrientation = "stale_orientation"

// ReasonDependencyCycle names a loop in actual execution obligations.
const ReasonDependencyCycle = "dependency_cycle"

// ReasonProjectedEpicOrderCycle names a nonblocking grouped-render limitation.
const ReasonProjectedEpicOrderCycle = "projected_epic_order_cycle"

// ReasonNoConfirmedBaseline is the independent existing-code startup gate.
const ReasonNoConfirmedBaseline = "no_confirmed_baseline"

// ReasonMissingDesignDoc names a registered doc whose prose needs repair.
const ReasonMissingDesignDoc = "missing_design_doc"

// Gap is one readiness shortfall: a stable machine-readable Reason and a
// human-readable Message. The reasons are the store.Reason* discriminators.
type Gap struct {
	Reason  string `json:"reason" yaml:"reason"`
	Message string `json:"message" yaml:"message"`
}

// Report is the outcome of the readiness gate. Ready is true only when there are
// no gaps; Gaps names each shortfall in a stable order when it is not.
type Report struct {
	Ready      bool  `json:"ready" yaml:"ready"`
	Gaps       []Gap `json:"gaps" yaml:"gaps"`
	Advisories []Gap `json:"advisories" yaml:"advisories"`
}

// Check reports manifest, orientation, execution-cycle, ownership, and baseline
// gaps in stable order. Projected grouping cycles are independent advisories
// and never change Ready.
// A nil tree skips tree-dependent checks rather than inventing an answer for
// a tree nobody loaded.
func Check(m *store.Manifest, t *store.Tree) Report {
	gaps := m.Gaps()
	report := Report{Ready: len(gaps) == 0, Gaps: make([]Gap, 0, len(gaps)), Advisories: []Gap{}}
	for _, g := range gaps {
		report.Gaps = append(report.Gaps, gapFrom(g))
	}
	if orientationGap, stale := staleOrientationGap(m.Root()); stale {
		report.Ready = false
		report.Gaps = append(report.Gaps, orientationGap)
	}
	if t != nil {
		cycles := DependencyDiagnostics(t)
		report.Gaps = append(report.Gaps, cycles.Gaps...)
		report.Advisories = append(report.Advisories, cycles.Advisories...)
		report.Ready = report.Ready && len(cycles.Gaps) == 0
		for _, g := range danglingOwnerGaps(m, t) {
			report.Ready = false
			report.Gaps = append(report.Gaps, g)
		}
		if (m.Project.Kind == store.KindBrownfield || m.Project.Kind == store.KindLegacyMigration) && !t.Baseline.HasConfirmedDocs() {
			report.Ready = false
			report.Gaps = append(report.Gaps, Gap{
				Reason:  ReasonNoConfirmedBaseline,
				Message: "no design baseline has been confirmed; derive or review the existing baseline, then have a human run `tiller approve design <doc>`",
			})
		}
		docs := make([]string, 0, len(t.Baseline.Docs))
		for _, doc := range t.Baseline.Docs {
			docs = append(docs, doc.Path)
		}
		sort.Strings(docs)
		for _, doc := range docs {
			if !store.BaselineDocPresent(m.Root(), doc) {
				report.Ready = false
				report.Gaps = append(report.Gaps, Gap{
					Reason:  ReasonMissingDesignDoc,
					Message: fmt.Sprintf("registered design doc .tiller/design/%s is missing or unreadable; restore its prose or remove its links with `tiller unlink design`", doc),
				})
			}
		}
	}
	return report
}

// ReasonDanglingOwner names unfinished execution work or an open question
// whose recorded owner is absent from the roster. Historical contributors
// do not reserve unfinished work and are not readiness gaps.
const ReasonDanglingOwner = "dangling_owner"

// danglingOwnerGaps reports live scalar assignments, never ancestor contributors.
func danglingOwnerGaps(m *store.Manifest, t *store.Tree) []Gap {
	roster := make(map[string]bool, len(m.Contributors))
	for _, o := range m.Contributors {
		roster[o.ID] = true
	}

	var gaps []Gap
	for _, id := range sortedKeys(t.Tasks) {
		if tk := t.Tasks[id]; tk.Owner != "" && !roster[tk.Owner] && tk.Status != "done" && tk.Status != "cancelled" {
			gaps = append(gaps, danglingOwnerGap(t, store.NounTask, id, tk.Owner))
		}
	}
	for _, noun := range store.ShelfNouns() {
		for _, node := range t.WorkNodesOf(noun) {
			w := node.Body()
			if w.Owner != "" && !roster[w.Owner] && w.Status != "done" && w.Status != "cancelled" {
				gaps = append(gaps, danglingOwnerGap(t, noun, w.ID, w.Owner))
			}
		}
	}
	for _, id := range sortedKeys(t.Questions) {
		if q := t.Questions[id]; q.Owner != "" && !roster[q.Owner] && gen.QuestionOpen(q) {
			gaps = append(gaps, danglingOwnerGap(t, store.NounQuestion, id, q.Owner))
		}
	}
	return gaps
}

func danglingOwnerGap(t *store.Tree, noun, id, owner string) Gap {
	remedy := fmt.Sprintf("run `tiller add contributor %s --name \"<name>\"` to re-register them; started ownership cannot be rewritten", owner)
	return Gap{
		Reason: ReasonDanglingOwner,
		Message: fmt.Sprintf(
			"%s %s is owned by %s, who is not on the roster; %s",
			noun, gen.RefByID(t, id), owner, remedy),
	}
}

// sortedKeys returns m's keys in ascending order, so a caller walking a Go
// map (randomized iteration) gets a deterministic sequence.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// memberLabel names one cycle member the way a human addresses it: canonical
// key where the noun carries one, id otherwise (and for any member the tree
// no longer resolves, the id is the only honest handle left).
func memberLabel(t *store.Tree, noun, id string) string {
	deref := func(k *string) string {
		if k != nil {
			return *k
		}
		return ""
	}
	switch noun {
	case store.NounEpic:
		if e := t.Epics[id]; e != nil {
			if k := deref(e.Key); k != "" {
				return k
			}
		}
	case store.NounFeature:
		if f := t.Features[id]; f != nil {
			if k := deref(f.Key); k != "" {
				return k
			}
		}
	case store.NounTask:
		if tk := t.Tasks[id]; tk != nil {
			if k := deref(tk.Key); k != "" {
				return k
			}
		}
	}
	return id
}

// staleOrientationGap compares the on-disk .tiller/orientation.md generation
// stamp against the binary's (gen.OrientationGeneration()), reporting a gap
// when they disagree in either direction. A missing file, or one carrying no
// stamp gen.OrientationStampGeneration recognizes, reports stale=false:
// readiness names no new failure for a file it cannot read a generation out
// of (issue #41's explicit carve-out), the same silence it already keeps
// about a missing orientation.md today.
//
// A behind artifact needs only offline project maintenance. An ahead artifact
// requires a newer binary; refresh must never downgrade its generation.
func staleOrientationGap(root string) (Gap, bool) {
	data, err := store.ReadRepositoryFile(root, ".tiller/orientation.md")
	if err != nil {
		return Gap{}, false
	}
	onDisk, ok := gen.OrientationStampGeneration(data)
	if !ok {
		return Gap{}, false
	}
	current := gen.OrientationGeneration()
	if onDisk == current {
		return Gap{}, false
	}
	remedy := "run `tiller upgrade --project-only` to refresh it"
	if onDisk > current {
		remedy = "run `tiller upgrade` to update the binary and this checkout"
	}
	return Gap{
		Reason: ReasonStaleOrientation,
		Message: fmt.Sprintf(
			"orientation.md is generation %d but the binary is on generation %d; %s",
			onDisk, current, remedy),
	}, true
}

// gapFrom turns a store readiness error into a reportable Gap, preserving the
// machine-readable reason when the error carries one.
func gapFrom(err error) Gap {
	var envErr *store.EnvError
	if errors.As(err, &envErr) {
		return Gap{Reason: envErr.Reason, Message: envErr.Error()}
	}
	return Gap{Reason: "error", Message: err.Error()}
}

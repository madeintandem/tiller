package views

import (
	"fmt"
	"sort"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// DesignApprovalAct records this approval's actual path-level results.
type DesignApprovalAct struct {
	Confirmed []string
	Unchanged []string
}

// BaselineReadout is the complete ledger review surface. Its source mappings
// are recorded linkage facts, not an inventory of all possible source input.
// Unlike Produced, these rows are never subject to the work entity cap.
type BaselineReadout struct {
	Docs          []BaselineDocReadout    `json:"docs"`
	Sources       []BaselineSourceReadout `json:"sources"`
	Confirmed     int                     `json:"confirmed"`
	Pending       int                     `json:"pending"`
	Missing       int                     `json:"missing"`
	OpenQuestions int                     `json:"open_questions"`
	Review        *ReadoutAction          `json:"review,omitempty"`
	Repair        *ReadoutAction          `json:"repair,omitempty"`
}

// BaselineDocView is the path-addressed JSON face of a ledger document.
type BaselineDocView struct {
	Path        string                    `json:"path"`
	Describes   []string                  `json:"describes"`
	DerivedFrom []RevisionRefView         `json:"derived_from,omitempty"`
	Confirmed   *BaselineConfirmationView `json:"confirmed,omitempty"`
}

type BaselineConfirmationView struct {
	Commit   RevisionRefView       `json:"commit"`
	Surfaces []BaselineSurfaceView `json:"surfaces"`
	By       string                `json:"by"`
}

type BaselineSurfaceView struct {
	RevisionRefView
	Inventory []string `json:"inventory"`
}

// BaselineDocReadout adds observed prose presence to the recorded document.
type BaselineDocReadout struct {
	BaselineDocView
	Present *bool `json:"present,omitempty"`
}

func BaselineDocOf(doc store.BaselineDoc) BaselineDocView {
	view := BaselineDocView{Path: doc.Path, Describes: doc.Describes}
	if len(doc.DerivedFrom) > 0 {
		view.DerivedFrom = make([]RevisionRefView, len(doc.DerivedFrom))
		for i, ref := range doc.DerivedFrom {
			if projected := revisionRefOf(ref); projected != nil {
				view.DerivedFrom[i] = *projected
			}
		}
	}
	if doc.Confirmed != nil {
		stamp := doc.Confirmed
		view.Confirmed = &BaselineConfirmationView{By: stamp.By, Surfaces: make([]BaselineSurfaceView, len(stamp.Surfaces))}
		if commit := revisionRefOf(stamp.Commit); commit != nil {
			view.Confirmed.Commit = *commit
		}
		for i, surface := range stamp.Surfaces {
			view.Confirmed.Surfaces[i] = BaselineSurfaceView{
				RevisionRefView: RevisionRefView{Kind: surface.Kind, Value: surface.Value, Path: surface.Path, ObservedAt: formatTime(surface.ObservedAt)},
				Inventory:       surface.Inventory,
			}
		}
	}
	return view
}

// BaselineSourceReadout groups links by source path AND blob identity. A source
// path may legitimately appear several times with different recorded revisions.
type BaselineSourceReadout struct {
	Path  string   `json:"path"`
	Kind  string   `json:"kind"`
	Value string   `json:"value"`
	Docs  []string `json:"docs"`
}

// The baseline route's reasons: derive a baseline when no design doc is
// registered, otherwise review the registered docs.
const (
	baselineDeriveReason = "existing code needs a human-confirmed design baseline before decomposition or execution"
	baselineReviewReason = "registered design docs await human review and confirmation; review the existing docs rather than regenerate them"
)

func baselineRequired(t *store.Tree) bool {
	return t.Manifest != nil &&
		(t.Manifest.Project.Kind == store.KindBrownfield || t.Manifest.Project.Kind == store.KindLegacyMigration) &&
		!t.Baseline.HasConfirmedDocs()
}

func baselineReviewAction(pending int) ReadoutAction {
	return ReadoutAction{
		Text:  fmt.Sprintf("review the %d unconfirmed design %s under .tiller/design/ and confirm accepted named docs with `tiller approve design <doc>`", pending, plural(pending, "doc", "docs")),
		Owner: "human",
	}
}

func baselineReadoutFor(t *store.Tree, moment Moment, present map[string]bool, approval bool) *BaselineReadout {
	b := &BaselineReadout{}
	for _, doc := range t.Baseline.Docs {
		if doc.Confirmed == nil {
			b.Pending++
		} else {
			b.Confirmed++
		}
		if exists, checked := present[doc.Path]; checked && !exists {
			b.Missing++
		}
	}
	if b.Pending > 0 {
		action := baselineReviewAction(b.Pending)
		b.Review = &action
	}
	if b.Missing > 0 {
		b.Repair = &ReadoutAction{
			Text:  fmt.Sprintf("restore the %d missing or unsafe registered design %s under .tiller/design/ or remove their links with `tiller unlink design`; inspect affected docs with `tiller readout derive-baseline`", b.Missing, plural(b.Missing, "doc", "docs")),
			Owner: "human",
		}
	}
	if moment != MomentDeriveBaseline && !approval {
		if b.Pending == 0 && b.Missing == 0 {
			return nil
		}
		return b
	}
	b.Docs = []BaselineDocReadout{}
	b.Sources = []BaselineSourceReadout{}
	type sourceKey struct{ path, kind, value string }
	sources := make(map[sourceKey]map[string]bool)
	for _, doc := range t.Baseline.Docs {
		row := BaselineDocReadout{BaselineDocView: BaselineDocOf(doc)}
		if exists, checked := present[doc.Path]; checked {
			row.Present = &exists
		}
		b.Docs = append(b.Docs, row)
		for _, ref := range doc.DerivedFrom {
			key := sourceKey{ref.Path, ref.Kind, ref.Value}
			if sources[key] == nil {
				sources[key] = make(map[string]bool)
			}
			sources[key][doc.Path] = true
		}
	}
	sort.Slice(b.Docs, func(i, j int) bool { return b.Docs[i].Path < b.Docs[j].Path })
	for key, docs := range sources {
		row := BaselineSourceReadout{Path: key.path, Kind: key.kind, Value: key.value, Docs: make([]string, 0, len(docs))}
		for doc := range docs {
			row.Docs = append(row.Docs, doc)
		}
		sort.Strings(row.Docs)
		b.Sources = append(b.Sources, row)
	}
	sort.Slice(b.Sources, func(i, j int) bool {
		a, z := b.Sources[i], b.Sources[j]
		if a.Path != z.Path {
			return a.Path < z.Path
		}
		if a.Kind != z.Kind {
			return a.Kind < z.Kind
		}
		return a.Value < z.Value
	})
	for _, q := range t.Questions {
		if gen.QuestionOpen(q) {
			b.OpenQuestions++
		}
	}
	return b
}

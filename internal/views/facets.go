package views

import (
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// The filter axes, spelled as the /api/ls query params and the CLI's ls flags.
const (
	AxisStatus      = "status"
	AxisOwner       = "owner"
	AxisContributor = "contributor"
	AxisEpic        = "epic"
	AxisFeature     = "feature"
)

// statusUnfinished is the pseudo-status Matches expands for the assignment
// nouns (task, bug, chore).
const statusUnfinished = "unfinished"

// filterAxes is the axes each noun can answer, in render order. An axis a
// noun cannot answer would only ever exclude every row (Matches), so the
// console never offers it.
var filterAxes = map[string][]string{
	store.NounEpic:      {AxisStatus, AxisContributor},
	store.NounFeature:   {AxisStatus, AxisContributor, AxisEpic},
	store.NounTask:      {AxisStatus, AxisOwner, AxisEpic, AxisFeature},
	store.NounQuestion:  {AxisStatus, AxisOwner},
	store.NounAmendment: {AxisStatus},
	store.NounBug:       {AxisStatus, AxisOwner},
	store.NounChore:     {AxisStatus, AxisOwner},
	store.NounTweak:     {AxisStatus, AxisOwner},
}

// assignmentStatuses is the task lifecycle the shelf nouns share
// (lifecycle/table.go, workItemRows), led by the unfinished pseudo-status.
var assignmentStatuses = []string{statusUnfinished, "proposed", "ready", "in_progress", "check", "in_review", "done", "cancelled"}

// statusOrder is each noun's stored statuses in lifecycle order
// (artifact-schema.md, the per-node status enums).
var statusOrder = map[string][]string{
	store.NounEpic:      {"draft", "ready", "done", "archived", "cancelled"},
	store.NounFeature:   {"draft", "specced", "ready", "active", "done", "cancelled"},
	store.NounTask:      assignmentStatuses,
	store.NounBug:       assignmentStatuses,
	store.NounChore:     assignmentStatuses,
	store.NounTweak:     assignmentStatuses,
	store.NounQuestion:  {"open", "answered", "cancelled"},
	store.NounAmendment: {"proposed", "approved", "withdrawn"},
}

// FilterAxes is the axes noun answers, in render order; nil for an unknown noun.
func FilterAxes(noun string) []string {
	return filterAxes[noun]
}

// FilterAxesByNoun is a copy of the whole axis table, keyed by singular noun,
// so the console can drop inapplicable values on a noun switch without a
// second copy of the table.
func FilterAxesByNoun() map[string][]string {
	out := make(map[string][]string, len(filterAxes))
	for noun, axes := range filterAxes {
		out[noun] = append([]string(nil), axes...)
	}
	return out
}

// FacetOption is one pickable value on an axis. Value is what the URL and
// the API carry (a status word, a person id, a node id); Label is display
// only. Key and Parent apply to epic and feature options; You to people.
type FacetOption struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Key       string `json:"key,omitempty"`
	Parent    string `json:"parent,omitempty"`
	Count     int    `json:"count"`
	Selected  bool   `json:"selected,omitempty"`
	You       bool   `json:"you,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// Facet is one axis's options in display order. Options is never nil.
type Facet struct {
	Axis    string        `json:"axis"`
	Options []FacetOption `json:"options"`
}

// Facets computes every axis noun answers against the resolved filter f
// (ResolveParentFilters or its lenient twin already ran). Each axis counts
// the rows matching every other active axis; zero-count options are dropped
// except the selected value, which always shows. viewer is the acting contributor
// id, or "" when the checkout has none. unresolved is the lenient resolver's
// result: failed parents exclude all rows even if their raw value matches a
// parent link. Clearing that axis still offers valid recovery options.
func Facets(t *store.Tree, noun string, f ListFilters, viewer string, unresolved map[string]string) []Facet {
	nodes := Collect(t, noun)
	axes := FilterAxes(noun)
	var order *gen.Order
	var epicsByKey map[string]*store.Epic
	out := make([]Facet, 0, len(axes))
	for _, axis := range axes {
		others := withAxis(f, axis, "")
		counts := map[string]int{}
		nodesToCount := nodes
		for failedAxis := range unresolved {
			if failedAxis != axis {
				nodesToCount = nil
				break
			}
		}
		for _, n := range nodesToCount {
			if !Matches(t, n, others) {
				continue
			}
			switch axis {
			case AxisStatus:
				counts[n.NodeStatus()]++
				if (noun == store.NounTask || store.IsWorkNoun(noun)) && gen.UnfinishedAssignment(n.NodeStatus()) {
					counts[statusUnfinished]++
				}
			case AxisContributor:
				for _, person := range Contributors(t, n) {
					counts[person.ID]++
				}
			case AxisOwner:
				if value := Owner(n); value != "" {
					counts[value]++
				}
			case AxisEpic:
				if value := EpicKey(n); value != "" {
					counts[value]++
				}
			case AxisFeature:
				if value := FeatureID(n); value != "" {
					counts[value]++
				}
			}
		}
		selected := axisValue(f, axis)
		raw, failed := unresolved[axis]
		if failed {
			// A failed identity is not a pickable parent, even when a
			// stored link or ambiguous key happens to share its spelling.
			delete(counts, selected)
			selected = ""
		} else if selected != "" {
			if _, ok := counts[selected]; !ok {
				counts[selected] = 0
			}
		}
		if axis == AxisEpic || axis == AxisFeature {
			if order == nil {
				order = gen.DependencyOrder(t)
				epicsByKey = make(map[string]*store.Epic, len(t.Epics))
				for _, epic := range t.Epics {
					if key := Key(epic); key != "" {
						epicsByKey[key] = epic
					}
				}
			}
		}
		var opts []FacetOption
		switch axis {
		case AxisStatus:
			opts = statusOptions(noun, counts, selected)
		case AxisOwner, AxisContributor:
			opts = peopleOptions(t, counts, selected, viewer)
		case AxisEpic:
			opts = epicOptions(epicsByKey, order, counts, selected)
		case AxisFeature:
			opts = featureOptions(t, epicsByKey, order, counts, selected)
		}
		if failed {
			opts = append(opts, FacetOption{Value: raw, Label: raw, Selected: true})
		}
		if opts == nil {
			opts = []FacetOption{}
		}
		out = append(out, Facet{Axis: axis, Options: opts})
	}
	return out
}

func axisValue(f ListFilters, axis string) string {
	switch axis {
	case AxisStatus:
		return f.Status
	case AxisOwner:
		return f.Owner
	case AxisContributor:
		return f.Contributor
	case AxisEpic:
		return f.Epic
	case AxisFeature:
		return f.Feature
	}
	return ""
}

func withAxis(f ListFilters, axis, value string) ListFilters {
	switch axis {
	case AxisStatus:
		f.Status = value
	case AxisOwner:
		f.Owner = value
	case AxisContributor:
		f.Contributor = value
	case AxisEpic:
		f.Epic = value
	case AxisFeature:
		f.Feature = value
	}
	return f
}

func statusOptions(noun string, counts map[string]int, selected string) []FacetOption {
	rank := map[string]int{}
	for i, s := range statusOrder[noun] {
		rank[s] = i
	}
	opts := make([]FacetOption, 0, len(counts))
	for v, c := range counts {
		opts = append(opts, FacetOption{Value: v, Label: v, Count: c, Selected: v == selected})
	}
	sort.Slice(opts, func(i, j int) bool {
		ri, iKnown := rank[opts[i].Value]
		rj, jKnown := rank[opts[j].Value]
		if iKnown != jKnown {
			return iKnown
		}
		if iKnown {
			return ri < rj
		}
		return opts[i].Value < opts[j].Value
	})
	return opts
}

func peopleOptions(t *store.Tree, counts map[string]int, selected, viewer string) []FacetOption {
	names := map[string]string{}
	if t.Manifest != nil {
		for _, o := range t.Manifest.Contributors {
			names[o.ID] = strings.TrimSpace(o.Name)
		}
	}
	opts := make([]FacetOption, 0, len(counts))
	for id, c := range counts {
		label := names[id]
		if label == "" {
			label = id
		}
		opts = append(opts, FacetOption{Value: id, Label: label, Count: c, Selected: id == selected, You: viewer != "" && id == viewer})
	}
	sort.Slice(opts, func(i, j int) bool {
		if opts[i].You != opts[j].You {
			return opts[i].You
		}
		li, lj := strings.ToLower(opts[i].Label), strings.ToLower(opts[j].Label)
		if li != lj {
			return li < lj
		}
		return opts[i].Value < opts[j].Value
	})
	return opts
}

// parentRank groups parent options: live first, then cancelled, then values
// naming no node (a stale selected value).
func parentRank(o FacetOption, known bool) int {
	switch {
	case !known:
		return 2
	case o.Cancelled:
		return 1
	}
	return 0
}

func sortParents(opts []FacetOption, known map[string]bool, less func(a, b string) bool) {
	sort.SliceStable(opts, func(i, j int) bool {
		ri, rj := parentRank(opts[i], known[opts[i].Value]), parentRank(opts[j], known[opts[j].Value])
		if ri != rj {
			return ri < rj
		}
		if ri == 2 {
			return opts[i].Value < opts[j].Value
		}
		return less(opts[i].Value, opts[j].Value)
	})
}

// epicOptions turns counts keyed by epic key into options carrying the epic's
// id, so a pick writes the immutable id while an old key URL still selects.
func epicOptions(byKey map[string]*store.Epic, order *gen.Order, counts map[string]int, selected string) []FacetOption {
	known := map[string]bool{}
	opts := make([]FacetOption, 0, len(counts))
	for key, c := range counts {
		o := FacetOption{Value: key, Label: key, Count: c, Selected: key == selected}
		if e, ok := byKey[key]; ok {
			o.Value, o.Label, o.Key, o.Cancelled = e.ID, e.Title, key, e.Status == "cancelled"
			known[e.ID] = true
		} else if o.Selected {
			o.Count = 0
		} else {
			continue
		}
		opts = append(opts, o)
	}
	sortParents(opts, known, func(a, b string) bool { return order.Less(store.NounEpic, a, b) })
	return opts
}

func featureOptions(t *store.Tree, epicsByKey map[string]*store.Epic, order *gen.Order, counts map[string]int, selected string) []FacetOption {
	known := map[string]bool{}
	opts := make([]FacetOption, 0, len(counts))
	for id, c := range counts {
		o := FacetOption{Value: id, Label: id, Count: c, Selected: id == selected}
		if fe, ok := t.Features[id]; ok {
			o.Label, o.Key, o.Parent, o.Cancelled = fe.Title, Key(fe), "", fe.Status == "cancelled"
			if epic := epicsByKey[fe.EpicKey]; epic != nil {
				o.Parent = epic.ID
			}
			known[id] = true
		} else if o.Selected {
			o.Count = 0
		} else {
			continue
		}
		opts = append(opts, o)
	}
	sortParents(opts, known, func(a, b string) bool { return order.Less(store.NounFeature, a, b) })
	return opts
}

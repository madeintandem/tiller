package serve

// The list endpoint is the CLI's ls made fetchable (ui-surface.md, Explore's
// filter mode): the same views projections, the same filter axes, with the
// human reference carried explicitly since the console renders it (the CLI's
// ListItem keeps Ref out of its JSON for golden stability). The console alone
// also receives filter facets, the axis table, and unresolved parent values.

import (
	"net/http"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

// listNode is one list row with the reference the console renders. Embedding
// ListItem keeps the field set identical to the CLI's payload; the outer Ref
// shadows the embedded render-only one. Column is a feature row's Build board
// column (gen.FeatureBoardStatus), which Home's funnel counts so it agrees
// with Build; it is empty for every other noun.
type listNode struct {
	views.ListItem
	Ref    string `json:"ref"`
	Column string `json:"column,omitempty"`
}

// listPayload adds console filter-bar data to the CLI's list envelope.
type listPayload struct {
	Noun       string              `json:"noun"`
	Count      int                 `json:"count"`
	Nodes      []listNode          `json:"nodes"`
	Facets     []views.Facet       `json:"facets"`
	Axes       map[string][]string `json:"axes"`
	Unresolved map[string]string   `json:"unresolved"`
}

// listNouns accepts the CLI's singular and plural forms.
var listNouns = map[string]string{
	"epic": store.NounEpic, "epics": store.NounEpic,
	"feature": store.NounFeature, "features": store.NounFeature,
	"task": store.NounTask, "tasks": store.NounTask,
	"question": store.NounQuestion, "questions": store.NounQuestion,
	"amendment": store.NounAmendment, "amendments": store.NounAmendment,
	"bug": store.NounBug, "bugs": store.NounBug,
	"chore": store.NounChore, "chores": store.NounChore,
	"tweak": store.NounTweak, "tweaks": store.NounTweak,
}

// ListHandler answers GET /api/ls?noun=features&status=&owner=&contributor=&feature=&epic=.
// A bad noun is a 400. An unresolved parent answers 200 with no rows and the
// raw value in unresolved, allowing the console to explain a stale link on
// that field. The CLI retains strict parent resolution and its exit-2 refusal.
func ListHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, r *http.Request) (any, error) {
		q := r.URL.Query()
		noun, ok := listNouns[q.Get("noun")]
		if !ok {
			return nil, &httpError{http.StatusBadRequest, "noun must be one of epic(s), feature(s), task(s), question(s), amendment(s), bug(s), chore(s), tweak(s)"}
		}
		f := views.ListFilters{
			Status:      q.Get("status"),
			Owner:       q.Get("owner"),
			Contributor: q.Get("contributor"),
			Feature:     q.Get("feature"),
			Epic:        q.Get("epic"),
		}
		unresolved := views.ResolveParentFiltersLeniently(t, &f)
		items := []views.ListItem{}
		if len(unresolved) == 0 {
			for _, n := range views.Collect(t, noun) {
				if !views.Matches(t, n, f) {
					continue
				}
				items = append(items, views.Item(t, n))
			}
			views.SortListItems(t, noun, items)
		}
		if noun == store.NounFeature || noun == store.NounEpic {
			if owners := views.RecoveredOwners(t, noun, items); len(owners) > 0 {
				views.EnrichRecoveredList(t, noun, items, readinput.WorkflowForNodes(t, owners))
			}
		}

		// views.Item already resolved each row's reference; the wrapper only
		// lifts it into the JSON.
		nodes := make([]listNode, len(items))
		for i, it := range items {
			nodes[i] = listNode{ListItem: it, Ref: it.Ref}
			if f, ok := t.Features[it.ID]; ok && noun == store.NounFeature {
				nodes[i].Column = gen.FeatureBoardStatus(t, f)
			}
		}
		return listPayload{
			Noun: noun, Count: len(nodes), Nodes: nodes,
			Facets: views.Facets(t, noun, f, actingContributor(root), unresolved),
			Axes:   views.FilterAxesByNoun(), Unresolved: unresolved,
		}, nil
	})
}

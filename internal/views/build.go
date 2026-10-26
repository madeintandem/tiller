package views

// This file is the Build view's server projection: epics in the shared epic
// order, each with its own features in the shared feature order, each
// feature's own tasks in the shared task order — the identical
// gen.DependencyOrder computation `ls`, the tree, and the streams already
// consult (issue #27), reshaped for the board's epic panel and six-column
// layout instead of a flat list. The point of running it here rather than
// leaving it to the client is the phase's own acceptance line: "no
// client-side ordering code exists". Like every other file in this package,
// it is a pure function of the loaded store.Tree, no reasoning, no mutation,
// no rendering (node.go's package comment).
//
// Two shape questions the plan text left open, both settled here:
//
//   - feature_counts vs mix. The plan's own shape sketch listed an epic row
//     as {noun,id,key,title,ref,status,feature_counts,mix}, but its Verified
//     Facts elaboration describes mix as "the epic's child-state composition
//     for the panel's mix bar" — exactly what feature_counts would also have
//     to be, since the panel's feature-count badge is just that
//     composition's sum. Shipping both is two spellings of one fact, which
//     the Bounds section calls out by name and asks to be
//     collapsed, so BuildEpic carries it once, as FeatureCounts (the plan's
//     own field name for it), keyed by each feature's board column
//     (gen.FeatureBoardStatus) rather than the client's seven-state legend bucket: bucket folding and
//     segment order are the client's own presentation call
//     (ui/src/views/home/stat.ts's composeStat runs the identical fold for
//     the header tiles from raw rows), not an ordering fact this projection
//     owes.
//
//   - drift. The plan left the mechanism open: "ids from /api/drift or a
//     drift boolean on the projection, whichever is cheaper server-side."
//     A boolean here would mean running drift.Run — serve.go's own comment
//     calls it "deliberately the most expensive read this API makes" and
//     refuses to cache it on purpose — inline on every /api/build request.
//     Keeping it out is what lets the board paint from a pure-tree read
//     that never blocks on that git-plus-stat sweep. The client has no
//     fetch cache or dedupe to lean on either way (api.ts's fetchJSON is a
//     bare fetch per call, and hooks.ts's useFetch deliberately blanks its
//     data to null on every refetch, generation bump included, rather than
//     holding the stale value): baking drift into this payload would mean
//     the whole board goes blank for the sweep's own duration on every ↻,
//     not just a drift flag lagging. So BuildFeature below carries no drift
//     field at all; Task 13 joins /api/drift's candidate ids onto cards by
//     node id client-side instead, the same overlay idiom it already runs
//     for /api/queue (this package's StreamsOf carries no queue data for
//     the identical reason, independent of any caching question).

import (
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// BuildBoard is the Build view's board: non-cancelled epics in the shared
// epic order, panel-and-columns ready. Epic selection within that set is the
// client's concern, riding the URL.
type BuildBoard struct {
	Epics []BuildEpic `json:"epics"`
}

// BuildEpic is one epic panel row: enough identity to link and select it,
// plus its features' column composition for the panel's count badge
// and mix bar (FeatureCounts; see this file's package comment for why that
// is one field, not two), and its own features in the shared feature order.
type BuildEpic struct {
	Noun         string              `json:"noun"`
	ID           string              `json:"id"`
	Key          string              `json:"key,omitempty"`
	Title        string              `json:"title,omitempty"`
	Ref          string              `json:"ref"`
	Status       string              `json:"status"`
	Contributors []store.Contributor `json:"contributors"`
	// FeatureCounts is this epic's own features grouped by their board
	// column (BuildFeature.Column), so the mix bar agrees with the columns,
	// absent (never an empty object) for an epic with no
	// features. Values are counts, not ids: the panel's badge sums them and
	// the mix bar paints one segment per key, and neither needs the members
	// themselves.
	FeatureCounts map[string]int `json:"feature_counts,omitempty"`
	Features      []BuildFeature `json:"features"`
}

// BuildFeature is one board card: a feature row in the same shape `ls`
// gives one (embedded ListItem — identity, status, activity, contributors),
// plus the task rollup
// (TaskDone/TaskTotal), the open attached shelf-work count, the task list
// itself in the shared task order, and the two blocked-register conditions
// a card renders distinctly (issue #39: the manual flag and a held ancestor
// are never folded into one flag). Ref
// rides explicitly because the console (unlike the CLI's golden-pinned ls)
// renders it, the same shadow-the-embedded-render-only-Ref idiom
// serve/list.go's listNode already uses for the identical reason.
type BuildFeature struct {
	ListItem
	Ref string `json:"ref"`
	// Column is the board column the card lands in (gen.FeatureBoardStatus):
	// the stored status, except a ready feature with a task in flight reads
	// active. Status itself stays the stored value.
	Column        string `json:"column"`
	TaskDone      int    `json:"task_done"`
	TaskTotal     int    `json:"task_total"`
	OpenWorkItems int    `json:"open_work_items"`
	// Tasks is this feature's own tasks in the shared task order
	// (featureTasks, nodepage.go — the identical walk the node page's
	// Relationships.Tasks already runs), each row the node page's own
	// RelatedNode shape (noun/id/key/ref/status/title) rather than an
	// invented one, per the plan's "reuse existing projections... rather
	// than inventing field names." Always an array, never null, even for a
	// feature with no tasks yet, so a caller parses one shape.
	Tasks []RelatedNode `json:"tasks"`
	// Blocked is the manual blocked flag (store.Feature.Blocked, read
	// through views.Blocked — no new derivation), orthogonal to the card's
	// stored status the same way it is everywhere else this flag renders
	// (streams.go's BlockedItem, issue #39): a blocked active feature is
	// still active, so it stays in its status column and only carries the
	// flag. False omits the key rather than emitting "blocked": false.
	Blocked bool `json:"blocked,omitempty"`
	// Held names the nearest retired ancestor this feature sits under
	// (gen.RetiredAncestor), nil for a live feature, the same additive fact
	// NodeView.Held and streams.go's BlockedItem.Held already carry.
	Held *HeldView `json:"held,omitempty"`
}

// BuildBoardOf projects the loaded tree onto the Build view's board.
func BuildBoardOf(t *store.Tree) BuildBoard {
	order := gen.DependencyOrder(t)
	epicIDs := order.Epics()
	board := BuildBoard{Epics: make([]BuildEpic, 0, len(epicIDs))}
	for _, eid := range epicIDs {
		e, ok := t.Epics[eid]
		if !ok || e.Status == "cancelled" {
			continue
		}
		board.Epics = append(board.Epics, buildEpicOf(t, order, e))
	}
	return board
}

// buildEpicOf projects one epic and its own features, gathered by the
// layout link (order.Features, scoped to this epic's key) exactly the way
// nodepage.go's childFeatures and decompositionOf already do, including
// their identical guard against a keyless epic: the degenerate case that
// should not exist for a folder-backed noun, but a defensive read never
// crashes on it, it just reports no children.
func buildEpicOf(t *store.Tree, order *gen.Order, e *store.Epic) BuildEpic {
	be := BuildEpic{
		Noun:         e.Noun(),
		ID:           e.ID,
		Key:          Key(e),
		Title:        e.Title,
		Ref:          gen.Ref(t, e),
		Status:       e.Status,
		Contributors: Contributors(t, e),
	}
	var featureIDs []string
	if e.Key != nil && *e.Key != "" {
		featureIDs = order.Features(*e.Key)
	}
	be.Features = make([]BuildFeature, 0, len(featureIDs))
	for _, fid := range featureIDs {
		f, ok := t.Features[fid]
		if !ok {
			continue
		}
		if be.FeatureCounts == nil {
			be.FeatureCounts = map[string]int{}
		}
		card := buildFeatureOf(t, order, f)
		be.FeatureCounts[card.Column]++
		be.Features = append(be.Features, card)
	}
	return be
}

// buildFeatureOf projects one feature and its own tasks.
func buildFeatureOf(t *store.Tree, order *gen.Order, f *store.Feature) BuildFeature {
	item := Item(t, f)
	tasks := featureTasks(t, order, f.ID)
	if tasks == nil {
		tasks = []RelatedNode{}
	}
	// task_total and task_done both drop a cancelled task, the identical
	// exclusion gen.FeatureActivity already applies when it decides whether
	// "all tasks done" holds (activity.go): a cancelled task is scope the
	// feature no longer carries, not unfinished work, so counting it toward
	// the total would make the board's rollup disagree with the "awaiting
	// check" phase Home already renders for the very same feature. The
	// Tasks list above stays unfiltered: it is the full in-place expand,
	// not a rollup.
	var done, total int
	for _, tk := range tasks {
		if tk.Status == "cancelled" {
			continue
		}
		total++
		if tk.Status == "done" {
			done++
		}
	}

	return BuildFeature{
		ListItem:      item,
		Ref:           item.Ref,
		Column:        gen.FeatureBoardStatus(t, f),
		TaskDone:      done,
		TaskTotal:     total,
		OpenWorkItems: OpenAttachedWorkCount(t, f.ID),
		Tasks:         tasks,
		Blocked:       Blocked(f),
		Held:          heldViewOf(gen.RetiredAncestor(t, f)),
	}
}

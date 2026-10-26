package views

// The action queue derives team attention from recorded state. Approval and
// question gates remain individual asks; shared execution closeout is classified
// once per review unit through workflow.AdviceOf. Actor describes the next human
// or agent responsibility, never a developer claim. Commands are copyable CLI
// instructions; an empty command means the advice requires contextual judgment.

import (
	"fmt"
	"sort"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Queue is the JSON payload of /api/queue. Entries is always an array, never
// null, so a caller parses one shape when nothing needs the human.
type Queue struct {
	Entries []QueueEntry `json:"entries"`
}

// QueueFilter selects entries already admitted to a Queue. Empty fields are
// wildcards. Eligibility remains QueueOfInput's responsibility; filtering only
// narrows that projection for a reader.
type QueueFilter struct {
	Kind string
	Noun string
}

// FilterQueue keeps Queue entries matching every nonempty filter field, in the
// order QueueOfInput produced them. Entries is allocated even when no row
// matches so the JSON contract remains an array rather than null.
func FilterQueue(q Queue, filter QueueFilter) Queue {
	if filter.Kind == "" && filter.Noun == "" && q.Entries != nil {
		return q
	}
	entries := make([]QueueEntry, 0, len(q.Entries))
	for _, entry := range q.Entries {
		if filter.Kind != "" && entry.Kind != filter.Kind {
			continue
		}
		if filter.Noun != "" && entry.Noun != filter.Noun {
			continue
		}
		entries = append(entries, entry)
	}
	return Queue{Entries: entries}
}

// QueueKinds lists every queue entry kind in the queue's group order. The
// readout's attention categories must cover each one.
var QueueKinds = []string{
	"approve_amendment", "approve_epic", "approve_feature", "approve_tasks",
	"approve_bug", "approve_chore", "approve_tweak", "answer_question", "carrier_cancelled",
	"attest_gate", "attest_criterion", "feature_check", "workflow_closeout", "merge_epic", "merge_feature",
}

// QueueEntry is one team attention item. Kind names its workflow boundary:
//
//	approve_amendment  a proposed amendment awaiting approval
//	approve_epic       a draft epic awaiting approval
//	approve_feature    a specced feature awaiting spec approval
//	approve_tasks      a feature with proposed tasks awaiting batch approval
//	approve_bug        a proposed bug awaiting triage
//	approve_chore      a proposed chore awaiting triage
//	approve_tweak      a proposed tweak awaiting triage
//	answer_question    an open question, due first
//	carrier_cancelled  a shelf item whose recorded carrier cannot complete it
//	attest_gate        an optional review gate on an in-review node, still unmet
//	attest_criterion   an eval criterion recorded needs_attestation
//	feature_check      final task-grain feature checks after every task is done
//	workflow_closeout  shared assembly, checks, rubric, acceptance, submit or sync
//	merge_epic         an accepted epic review awaiting human review and merge
//	merge_feature      the corresponding feature-grain review
//
// There is no coordinator claim. An epic-grain closeout can direct checks or
// acceptance on a participating feature while remaining one epic queue entry.
//
// Prose is the entry's one line of human meaning: a title for the nodes that
// carry one, a question's statement, and for attest_criterion the criterion id
// the command names. Due carries a question's due hint. Command
// is the exact CLI invocation, the copy-command affordance. Gate and GateStatus
// ride only on attest_gate: the gate's name and its current stored status
// (pending, fail, skipped, or stale), the identity kept out of the command and
// prose so a renderer reads it from its own field.
type QueueEntry struct {
	Kind       string `json:"kind"`
	Noun       string `json:"noun"`
	ID         string `json:"id"`
	Key        string `json:"key,omitempty"`
	Ref        string `json:"ref"`
	Prose      string `json:"prose,omitempty"`
	Command    string `json:"command"`
	Due        string `json:"due,omitempty"`
	Gate       string `json:"gate,omitempty"`
	GateStatus string `json:"gate_status,omitempty"`
	Actor      string `json:"actor,omitempty"`
	Phase      string `json:"phase,omitempty"`
}

// QueueInput carries I/O observations across the pure view boundary. Workflow
// observes contracts, bases and aggregate inclusion through internal/readinput.
type QueueInput struct {
	// BasisDrifted names the feature ids whose accepted basis no longer
	// matches the working tree. A feature named here suppresses its
	// feature_check entry, because the copied command would refuse; the
	// remedy is the drift surface's own (approve --refresh-basis).
	BasisDrifted map[string]bool
	Workflow     workflow.Input
}

// QueueOf derives the action queue with no I/O-derived facts.
func QueueOf(t *store.Tree) Queue { return QueueOfInput(t, QueueInput{}) }

// QueueOfInput derives the action queue from the loaded tree plus the
// caller's I/O-derived facts (QueueInput). The groups come in a fixed order
// (proposed amendments first, then task and shelf approve gates, open
// questions, cancelled shelf carriers, pending attestations, and parked
// feature checks); within a group an epic, a feature, or a task orders by
// DependencyOrder (computed once here and threaded down) so a caller
// batching several rows into one CLI invocation (`tiller approve epic <a> <b>
// <c>...`) applies a dependency before what rests on it just by joining
// addresses in row order. Shelf groups use their recorded soft order and
// dependencies. The amendment group orders by id instead
// (queueApproveAmendments' own doc comment: amendments carry no dependency
// edges among themselves to rank by). The question group keeps its own
// due-then-id rule, and a mixed group leads with grain, so the same tree and
// the same QueueInput always yield the same queue.
//
// Amendments lead the list because they lead Route: a proposed amendment
// outranks even the plan gate there (a staged reshape can invalidate the very
// plan awaiting approval), and the queue's own group order mirrors that
// priority rather than disagreeing with it.
func QueueOfInput(t *store.Tree, in QueueInput) Queue {
	order := gen.DependencyOrder(t)
	entries := []QueueEntry{}
	entries = append(entries, queueApproveAmendments(t)...)
	entries = append(entries, queueApproveEpics(t, order)...)
	entries = append(entries, queueApproveFeatures(t, order)...)
	entries = append(entries, queueApproveTaskBatches(t, order)...)
	entries = append(entries, queueApproveShelf(t, store.NounBug)...)
	entries = append(entries, queueApproveShelf(t, store.NounChore)...)
	entries = append(entries, queueApproveShelf(t, store.NounTweak)...)
	entries = append(entries, queueAnswerQuestions(t)...)
	entries = append(entries, queueCarrierCancelled(t)...)
	closeout := queueWorkflowCloseout(t, order, in)
	shared := make(map[string]bool, len(closeout))
	for _, entry := range closeout {
		shared[entry.ID] = true
		if epic := t.Epics[entry.ID]; epic != nil && epic.Key != nil {
			for _, feature := range t.Features {
				if feature.EpicKey == *epic.Key && store.ResolvedReviewGrain(t.Manifest, epic, feature) == store.ReviewGrainEpic {
					shared[feature.ID] = true
				}
			}
		}
	}
	for _, group := range [][]QueueEntry{queueAttestGates(t, order), queueAttestCriteria(t, order)} {
		for _, entry := range group {
			if !shared[entry.ID] {
				entries = append(entries, entry)
			}
		}
	}
	entries = append(entries, closeout...)
	return Queue{Entries: entries}
}

// queueApproveAmendments lists the proposed amendments awaiting the human's
// approval, id-ascending: an amendment carries no dependency edges among
// itself and any other amendment the way an epic or a feature does (it is a
// flat staging document, not part of the one-tree hierarchy,
// store/amendment.go's own doc comment), so there is no gen.Order ranking to
// sort by, and id order is the only deterministic one available. It runs no
// suppression check at all, unlike queueApproveFeatures and
// queueApproveTaskBatches: an amendment has no ancestor to be held under
// (approve amendment refuses only on its own status and its own staged
// preconditions, never on a parent's), so there is nothing here for a
// retired-ancestor guard to catch.
func queueApproveAmendments(t *store.Tree) []QueueEntry {
	var out []QueueEntry
	for _, a := range t.Amendments {
		if a.Status != store.AmendmentStatusProposed {
			continue
		}
		out = append(out, QueueEntry{
			Kind: "approve_amendment", Noun: store.NounAmendment, ID: a.ID, Key: Key(a),
			Ref: gen.Ref(t, a), Prose: a.Title,
			Command: "tiller approve amendment " + queueAddr(a),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// queueApproveEpics lists the draft epics awaiting the human's approval, the
// first gate a decomposition passes through and so the queue's first group.
func queueApproveEpics(t *store.Tree, order *gen.Order) []QueueEntry {
	var out []QueueEntry
	for _, e := range t.Epics {
		if e.Status != "draft" {
			continue
		}
		out = append(out, QueueEntry{
			Kind: "approve_epic", Noun: store.NounEpic, ID: e.ID, Key: Key(e),
			Ref: gen.Ref(t, e), Prose: e.Title,
			Command: "tiller approve epic " + queueAddr(e),
		})
	}
	sortQueueByOrder(order, out)
	return out
}

// queueApproveFeatures lists the specced features awaiting the human's spec
// approval, the most common gate.
func queueApproveFeatures(t *store.Tree, order *gen.Order) []QueueEntry {
	var out []QueueEntry
	for _, f := range t.Features {
		if f.Status != "specced" {
			continue
		}
		// A retired ancestor epic suppresses the row rather than exempting the
		// feature from it: `tiller approve feature` would now refuse with
		// ParentEpicRetiredError, and a queue whose commands fail
		// when copied is worse than silence (queueFeatureChecks keeps the same
		// rule below).
		if gen.RetiredAncestor(t, f) != nil {
			continue
		}
		// An open question holding the plan (the feature, its epic, or any live
		// task under it) suppresses the row the same way: `tiller approve
		// feature` refuses while one is open, and a queue whose commands fail
		// when copied is worse than silence. The question's own answer_question
		// row is the move that takes its place.
		if len(gen.PlanQuestionBlockers(t, f)) > 0 {
			continue
		}
		out = append(out, QueueEntry{
			Kind: "approve_feature", Noun: store.NounFeature, ID: f.ID, Key: Key(f),
			Ref: gen.Ref(t, f), Prose: f.Title,
			Command: "tiller approve feature " + queueAddr(f),
		})
	}
	sortQueueByOrder(order, out)
	return out
}

// queueApproveTaskBatches lists the features owning at least one proposed task
// that is the human's to approve on its own, one entry per feature rather than
// one per task: those tasks are approved together (`tiller approve task
// --feature <key>`), so the queue offers the feature-grain command a human runs
// once. The per-task `tiller approve task <node>` stays a CLI capability; the
// queue no longer renders a row for each proposed task.
//
// A feature that has not yet passed its own plan gate is skipped, because its
// proposed tasks are not a separate decision: `approve feature` flips the
// initial set in the same act (cli-surface.md, "Specifying a feature"), so
// offering them here would ask the human to do by hand what the gate ahead of
// them does. What is left after that gate is the mid-flight addition, which is
// exactly the case the spec sends through `approve task`.
func queueApproveTaskBatches(t *store.Tree, order *gen.Order) []QueueEntry {
	var out []QueueEntry
	for _, f := range t.Features {
		if !approvesTasksIndividually(f) {
			continue
		}
		// A retired ancestor epic suppresses the row rather than exempting the
		// feature from it: `tiller approve task --feature` would now refuse the
		// same way at the first task it reached (ParentEpicRetiredError, issue
		// #36), and a queue whose commands fail when copied is worse than
		// silence (queueFeatureChecks keeps the same rule below;
		// widened it to this group).
		if gen.RetiredAncestor(t, f) != nil {
			continue
		}
		// The row stands for the batch, so it stays while any proposed task is
		// free to approve: `tiller approve task` refuses a task an open question
		// holds (itself or through its feature or epic), and the row goes only
		// when every proposed task is held, the same suppress-not-exempt rule as
		// above.
		approvable := false
		for _, tk := range t.FeatureTasks[f.ID] {
			if tk.Status == "proposed" && len(gen.TaskQuestionBlockers(t, f, tk)) == 0 {
				approvable = true
				break
			}
		}
		if !approvable {
			continue
		}
		out = append(out, QueueEntry{
			Kind: "approve_tasks", Noun: store.NounFeature, ID: f.ID, Key: Key(f),
			Ref: gen.Ref(t, f), Prose: f.Title,
			Command: "tiller approve task --feature " + queueAddr(f),
		})
	}
	sortQueueByOrder(order, out)
	return out
}

// approvesTasksIndividually reports whether a proposed task under this feature
// is its own human decision. It is once the feature is past its plan gate, the
// states a task can be added mid-flight in; before that gate the initial set
// rides the feature's approve (cli-surface.md, "Specifying a feature").
func approvesTasksIndividually(f *store.Feature) bool {
	return f.Status == "ready" || f.Status == "active"
}

// queueAnswerQuestions lists the open questions, due first: entries carrying a
// due hint sort before those without, ordered by the hint then id, and the
// undated remainder by id. The answer text is the human's, so the command
// carries a "..." placeholder to fill in.
func queueAnswerQuestions(t *store.Tree) []QueueEntry {
	var out []QueueEntry
	for _, q := range t.Questions {
		if !gen.QuestionOpen(q) {
			continue
		}
		out = append(out, QueueEntry{
			Kind: "answer_question", Noun: store.NounQuestion, ID: q.ID,
			Ref: gen.Ref(t, q), Prose: gen.QuestionTitle(q),
			Command: `tiller answer question ` + q.ID + ` --answer "..."`,
			Due:     gen.QuestionDue(q),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].Due, out[j].Due
		if (di != "") != (dj != "") {
			return di != ""
		}
		if di != dj {
			return di < dj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// queueAttestGates lists optional review gates still waiting on a human for
// tasks and shelf nodes under review. A gate queues when the node is in review
// (status gen.ReviewState), the manifest gate is attestation-backed, and it
// applies to the node's grain. A stored pass has already cleared the gate, so
// only pending (the absent or empty state), fail, skipped, and stale queue.
//
// Entries walk tasks in dependency order, then bugs and chores in their shelf
// order. Gate names sort within each node rather than inheriting map iteration
// order.
func queueAttestGates(t *store.Tree, order *gen.Order) []QueueEntry {
	var out []QueueEntry
	var optional map[string]store.GateSpec
	if t.Manifest != nil {
		optional = t.Manifest.Gates.Optional
	}
	gateNames := make([]string, 0, len(optional))
	for name := range optional {
		gateNames = append(gateNames, name)
	}
	sort.Strings(gateNames)

	add := func(n store.Node) {
		if n.NodeStatus() != gen.ReviewState {
			return
		}
		if work, ok := store.AsWorkNode(n); ok && gen.WorkActivity(t, work) == gen.ActivityCarried {
			return
		}
		stored := NodeGates(n)
		for _, name := range gateNames {
			spec := optional[name]
			if spec.Kind != store.GateKindAttestation || !optionalGateApplies(spec.On, n.Noun()) {
				continue
			}
			status := stored[name].Status
			if status == "" {
				status = store.GateStatusPending
			}
			if status == store.GateStatusPass {
				continue
			}
			out = append(out, QueueEntry{
				Kind: "attest_gate", Noun: n.Noun(), ID: n.NodeID(), Key: Key(n),
				Ref: gen.Ref(t, n), Prose: nodeProse(n),
				Command:    fmt.Sprintf("tiller attest %s %s --gate %s --status pass", n.Noun(), queueAddr(n), name),
				Gate:       name,
				GateStatus: status,
			})
		}
	}

	tasks := make([]*store.Task, 0, len(t.Tasks))
	for _, task := range t.Tasks {
		tasks = append(tasks, task)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return order.Less(store.NounTask, tasks[i].ID, tasks[j].ID)
	})
	for _, task := range tasks {
		add(task)
	}
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(t, noun) {
			add(work)
		}
	}
	return out
}

// queueAttestCriteria lists every eval criterion recorded needs_attestation
// across the features and tasks: one entry per criterion, the machine
// half settled and the human sign-off waiting. Epics are not scanned, because
// they carry no gate block to record a criterion in (artifact-schema.md,
// Epics). The command's address is the canonical key for a feature and the id
// for a task, matching how the CLI addresses each noun. Entries group by grain
// first (every feature, then every task), rank by DependencyOrder within a
// grain, and fall to criterion id between two entries on the same node. The
// leading grain term is what keeps rank and reference from deciding the same
// pair from different sides; grainOf (views/streams.go) carries the reason.
func queueAttestCriteria(t *store.Tree, order *gen.Order) []QueueEntry {
	var out []QueueEntry
	add := func(n store.Node) {
		g, ok := NodeGates(n)["evals"]
		if !ok {
			return
		}
		ids := make([]string, 0, len(g.Criteria))
		for cid, c := range g.Criteria {
			if c.Status == store.CriterionStatusNeedsAttestation {
				ids = append(ids, cid)
			}
		}
		sort.Strings(ids)
		for _, cid := range ids {
			out = append(out, QueueEntry{
				Kind: "attest_criterion", Noun: n.Noun(), ID: n.NodeID(), Key: Key(n),
				Ref: gen.Ref(t, n), Prose: cid,
				Command: fmt.Sprintf("tiller attest %s %s --criterion %s --status pass", n.Noun(), queueAddr(n), cid),
			})
		}
	}
	for _, f := range t.Features {
		add(f)
	}
	for _, tk := range t.Tasks {
		add(tk)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ID == b.ID {
			return a.Prose < b.Prose
		}
		if ga, gb := grainOf(a.Noun), grainOf(b.Noun); ga != gb {
			return ga < gb
		}
		if gen.RankedNoun(a.Noun) {
			return order.Less(a.Noun, a.ID, b.ID)
		}
		return a.Ref < b.Ref
	})
	return out
}

// queueAddr is the address a queue command names a node by: the canonical key
// for the keyed nouns, the id for a ledger item (or a keyless node, so the
// command never degrades to a blank address).
func queueAddr(n store.Node) string {
	if k := Key(n); k != "" {
		return k
	}
	return n.NodeID()
}

// sortQueueByOrder orders one homogeneous-noun kind group (every entry an
// epic, or every entry a feature) by DependencyOrder: the
// approve_epic, approve_feature, and approve_tasks (feature-grain) groups. A
// caller batching several rows into one CLI invocation joins addresses in this
// order, so a dependency always precedes what rests on it.
func sortQueueByOrder(order *gen.Order, entries []QueueEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return order.Less(entries[i].Noun, entries[i].ID, entries[j].ID)
	})
}

// optionalGateApplies reports whether an optional gate's `on` grains cover a
// noun: it names the noun literally, or it names "pr", which expands to tasks
// and shelf work because those grains share the in-review lifecycle. Features
// remain excluded. It is the queue's own; mutate.onIncludes is an exact-match
// over the concrete grains, so the expansion lives here beside its one use
// rather than reaching into mutate.
func optionalGateApplies(on store.StringList, noun string) bool {
	for _, g := range on {
		if g == noun {
			return true
		}
		if g == "pr" && (noun == store.NounTask || store.IsWorkNoun(noun)) {
			return true
		}
	}
	return false
}

// nodeProse is an attest_gate entry's one line of human meaning: the node's
// title. The gate identity rides the Gate field, never here.
func nodeProse(n store.Node) string {
	if work, ok := store.AsWorkNode(n); ok {
		return work.Body().Title
	}
	switch v := n.(type) {
	case *store.Feature:
		return v.Title
	case *store.Task:
		return v.Title
	}
	return ""
}

// queueWorkflowCloseout projects the shared classifier, once per applicable
// review unit. A contribution is never an assignment to perform these actions.
func queueWorkflowCloseout(t *store.Tree, order *gen.Order, in QueueInput) []QueueEntry {
	var out []QueueEntry
	for _, node := range workflow.SharedCloseoutNodes(t, order) {
		advice := workflow.AdviceOf(t, node, in.Workflow)
		if advice == nil || advice.Phase == "done" || (advice.RecoveredReview == nil && (advice.Phase == "blocked" || in.BasisDrifted[advice.FeatureID])) {
			continue
		}
		kind := "workflow_closeout"
		if advice.Phase == "review" {
			kind = "merge_" + node.Noun()
		} else if node.Noun() == store.NounFeature && advice.Phase == "checks" && store.ResolvedReviewGrain(t.Manifest, recapEpicByKey(t, node.(*store.Feature).EpicKey), node.(*store.Feature)) == store.ReviewGrainTask {
			kind = "feature_check"
		}
		command := advice.Command
		if advice.Phase == "review" {
			command = "tiller sync"
		}
		prose := advice.Reason
		if advice.Phase == "review" {
			switch reviewed := node.(type) {
			case *store.Feature:
				prose = mergeProse(reviewed.PR, reviewed.ReviewRef) + "; " + prose
			case *store.Epic:
				prose = mergeProse(reviewed.PR, reviewed.ReviewRef) + "; " + prose
			}
		}
		out = append(out, QueueEntry{
			Kind: kind, Noun: node.Noun(), ID: node.NodeID(), Key: Key(node),
			Ref: gen.Ref(t, node), Prose: prose, Command: command,
			Actor: advice.Actor, Phase: advice.Phase,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Phase == "review") != (out[j].Phase == "review") {
			return out[j].Phase == "review"
		}
		return false
	})
	return out
}

// mergeProse names a merge queue entry's own recorded review surface: a PR
// number when the repository has a remote, or the review branch when it does
// not, the same either-or ReviewRef falls back to at submit time.
func mergeProse(pr *int, ref *store.ReviewRef) string {
	switch {
	case pr != nil:
		return fmt.Sprintf("PR #%d", *pr)
	case ref != nil && ref.Branch != "":
		return "branch " + ref.Branch
	default:
		return ""
	}
}

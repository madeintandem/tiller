package mutate

// SubmitNode is submit's coarse-grain half: `submit feature` and `submit
// epic`, the record-only counterpart to task-grain submit (transitionTask's
// own "submit" arm, unchanged). At a coarse review grain the tasks under the
// reviewed node keep riding their own lifecycle exactly as they always have,
// in_review included; nothing here adds a new lifecycle state anywhere. What
// changes is that the PR (or, remote-less, the review ref) is recorded on
// the reviewed node itself rather than on each task, once every task under
// it is ready and the reviewed node's own required gates have already been
// run and recorded green (docs/spec/cli-surface.md). The status
// the reviewed node carries never moves: a feature or epic still crosses to
// done only through sync observing the human's merge, the same as today; the
// merge and the sync that follows it finish the story this function starts.
//
// SubmitNode never runs a gate itself. It reads what `tiller check feature`
// already recorded onto the node's own GatesBlock (or, at epic grain, onto
// each live feature's own), the same recorded block the console and every
// other read surface render; a caller who wants a green result runs check
// first, and this function only asks whether that already happened.
import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Event type discriminators submit_node.go writes, the coarse-grain siblings
// of transitionEvent's own "submit."+store.NounTask.
const (
	eventSubmitFeature = "submit.feature"
	eventSubmitEpic    = "submit.epic"
)

const (
	ReasonTaskReviewOpen        = "task_review_open"
	ReasonIndependentReviewOpen = "independent_review_open"
)

// SubmitNodeInput is a request to record a coarse-grain review surface: PR
// where the repository has a remote, ReviewRef the branch-plus-head fallback
// where it does not, the same either-or task-grain submit already carries
// (TransitionInput). Noun is store.NounFeature or store.NounEpic; Key is the
// node's id or canonical key.
type SubmitNodeInput struct {
	Noun      string
	Key       string
	PR        *int
	ReviewRef *store.ReviewRef
}

// SubmitNodeResult is the outcome: the resolved node identity, the review
// surface now on record (whether or not this call changed it), and whether
// this call actually wrote anything. A refusal returns an error instead, the
// same convention Configure's result carries.
type SubmitNodeResult struct {
	Noun      string
	ID        string
	Key       string
	PR        *int
	ReviewRef *store.ReviewRef
	Changed   bool
}

// SubmitNode resolves the acting contributor, resolves the node by key, and
// dispatches on the concrete noun. Only a feature or an epic can be the
// reviewed node here; a task's own PR is recorded by the ordinary
// task-grain submit, which is unaffected by this file.
func SubmitNode(tree *store.Tree, deps Deps, in SubmitNodeInput) (SubmitNodeResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	switch in.Noun {
	case store.NounFeature, store.NounEpic:
	default:
		return SubmitNodeResult{}, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("mutate: submit records a review surface on a feature or an epic, not %q; a task's own PR goes through submit task", in.Noun),
		}
	}

	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Key})
	if err != nil {
		return SubmitNodeResult{}, err
	}
	switch n := node.(type) {
	case *store.Feature:
		return submitFeature(tree, deps, owner, n, in)
	case *store.Epic:
		return submitEpic(tree, deps, owner, n, in)
	default:
		// Unreachable: in.Noun was already checked against store.NounFeature
		// and store.NounEpic above, and the resolver refuses a noun mismatch
		// before returning, so it never hands back a third concrete type.
		return SubmitNodeResult{}, fmt.Errorf("mutate: submit resolved %s to an unsupported node type", in.Noun)
	}
}

// submitFeature is `submit feature`. After grain and lifecycle legality, it
// reuses the feature work guards in their established order: retired parent,
// active basis, operative questions, reviewed assumptions, and claim. Only
// then does it reject legacy task PR history, require every live task to be
// in_review or done, and require recorded green feature gates. Past every
// refusal, an identical review surface is a no-op; otherwise one feature write
// and one submit.feature event record the surface.
func submitFeature(tree *store.Tree, deps Deps, owner store.Contributor, treeFeat *store.Feature, in SubmitNodeInput) (SubmitNodeResult, error) {
	if treeFeat.Key == nil {
		return SubmitNodeResult{}, fmt.Errorf("mutate: feature %s has no canonical key", treeFeat.ID)
	}
	relDir := featureRelDir(treeFeat)
	ymlRel := filepath.Join(relDir, "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	doc.EpicKey = treeFeat.EpicKey
	key := deref(doc.Key)
	if doc.RecoveredReview != nil {
		return SubmitNodeResult{}, recoveredSubmitLocked(store.NounFeature, doc.ID, doc.Branch)
	}

	epic := epicByKey(tree, doc.EpicKey)
	grain := store.ResolvedReviewGrain(tree.Manifest, epic, &doc)
	if grain != store.ReviewGrainFeature {
		return SubmitNodeResult{}, wrongReviewGrainErr(grain)
	}
	if featureNotStarted(doc.Status) {
		return SubmitNodeResult{}, &NodeNotStartedError{Noun: store.NounFeature, NodeID: doc.ID, Key: key, Status: doc.Status}
	}
	if featureTerminal(doc.Status) {
		return SubmitNodeResult{}, &NodeTerminalError{Noun: store.NounFeature, NodeID: doc.ID, Key: key, Status: doc.Status}
	}
	if err := guardFeatureCoarseSubmit(tree, deps, treeFeat, &doc); err != nil {
		return SubmitNodeResult{}, err
	}
	fresh, guards, err := freshReviewGrainLineage(tree, &doc)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	tree = fresh
	treeFeat = &doc
	if task := taskWithOwnOpenReview(tree, treeFeat); task != nil {
		return SubmitNodeResult{}, taskReviewOpenErr(store.NounFeature, doc.ID, key, task)
	}
	if ok, blockerID, blockerKey, blockerStatus := featureTasksSubmittable(tree, treeFeat); !ok {
		return SubmitNodeResult{}, &TaskNotSubmittableError{
			FeatureID: doc.ID, FeatureKey: key,
			TaskID: blockerID, TaskKey: blockerKey, TaskStatus: blockerStatus,
		}
	}
	if ok, missing := requiredGatesRecordedPass(tree.Manifest, store.NounFeature, &doc.GatesBlock); !ok {
		return SubmitNodeResult{}, &RequiredGatesNotGreenError{Noun: store.NounFeature, NodeID: doc.ID, Key: key, Missing: missing}
	}
	rev := deps.rev(tree.Root)
	if err := guardReviewIdentity(store.NounFeature, doc.ID, doc.PR, in.PR); err != nil {
		return SubmitNodeResult{}, err
	}
	if err := guardSubmitBranch(rev, store.NounFeature, doc.ID, doc.Branch, in.ReviewRef); err != nil {
		return SubmitNodeResult{}, err
	}
	if err := guardAggregateTasks(tree, rev, store.NounFeature, doc.ID, doc.Branch, &doc); err != nil {
		return SubmitNodeResult{}, err
	}
	if in.PR == nil {
		head, err := rev.CurrentCommit()
		if err != nil {
			return SubmitNodeResult{}, err
		}
		in.ReviewRef = &store.ReviewRef{Branch: doc.Branch, Head: head}
	}

	if samePR(doc.PR, in.PR) && sameReviewRef(doc.ReviewRef, in.ReviewRef) {
		return SubmitNodeResult{Noun: store.NounFeature, ID: doc.ID, Key: key, PR: doc.PR, ReviewRef: doc.ReviewRef, Changed: false}, nil
	}

	ev := submitEvent(deps, owner, eventSubmitFeature, store.NounFeature, doc.ID, key, ymlRel,
		doc.PR, doc.ReviewRef, in.PR, in.ReviewRef)
	doc.PR = in.PR
	doc.ReviewRef = in.ReviewRef

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, ymlRel), snapshot, ydata))
	if err := commitMutation(tree, "submit.feature", writes, []events.Event{ev}); err != nil {
		return SubmitNodeResult{}, err
	}

	return SubmitNodeResult{Noun: store.NounFeature, ID: doc.ID, Key: key, PR: in.PR, ReviewRef: in.ReviewRef, Changed: true}, nil
}

// submitEpic is submitFeature's epic-grain twin. Grain and lifecycle legality
// run first, followed by the derived epic review owner guard. A non-cancelled
// task- or feature-grain override must already be done; it never rides the epic
// PR. Every feature that does resolve epic reuses the feature work guards
// before legacy task-PR, readiness, and green-gate checks. Only then can one
// epic write and one submit.epic event record the shared review surface.
func submitEpic(tree *store.Tree, deps Deps, owner store.Contributor, treeEpic *store.Epic, in SubmitNodeInput) (SubmitNodeResult, error) {
	key := deref(treeEpic.Key)
	relYml := filepath.Join(epicRelDir(treeEpic), "epic.yml")

	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	if doc.RecoveredReview != nil {
		return SubmitNodeResult{}, recoveredSubmitLocked(store.NounEpic, doc.ID, doc.Branch)
	}

	grain := store.ResolvedReviewGrain(tree.Manifest, &doc, nil)
	if grain != store.ReviewGrainEpic {
		return SubmitNodeResult{}, wrongReviewGrainErr(grain)
	}
	if doc.Status == statusDraft {
		return SubmitNodeResult{}, &NodeNotStartedError{Noun: store.NounEpic, NodeID: doc.ID, Key: key, Status: doc.Status}
	}
	if epicTerminal(doc.Status) {
		return SubmitNodeResult{}, &NodeTerminalError{Noun: store.NounEpic, NodeID: doc.ID, Key: key, Status: doc.Status}
	}
	refreshed, live, guards, err := freshEpicSubmitFeatures(tree, key)
	if err != nil {
		return SubmitNodeResult{}, err
	}

	var riding []*store.Feature
	for _, f := range live {
		if store.ResolvedReviewGrain(refreshed.Manifest, &doc, f) != store.ReviewGrainEpic {
			if f.Status != statusDone {
				return SubmitNodeResult{}, &FeatureNotSubmittableError{
					EpicID: doc.ID, EpicKey: key, FeatureID: f.ID, FeatureKey: deref(f.Key),
					Reason: ReasonIndependentReviewOpen,
				}
			}
			continue
		}
		riding = append(riding, f)
	}
	for _, f := range riding {
		if err := guardFeatureCoarseSubmit(refreshed, deps, f, f); err != nil {
			return SubmitNodeResult{}, err
		}
	}
	for _, f := range riding {
		if task := taskWithOwnOpenReview(refreshed, f); task != nil {
			return SubmitNodeResult{}, taskReviewOpenErr(store.NounEpic, doc.ID, key, task)
		}
		if ok, blockerID, _, _ := featureTasksSubmittable(refreshed, f); !ok {
			return SubmitNodeResult{}, &FeatureNotSubmittableError{
				EpicID: doc.ID, EpicKey: key, FeatureID: f.ID, FeatureKey: deref(f.Key),
				Reason: "tasks_incomplete", TaskID: blockerID,
			}
		}
	}
	for _, f := range riding {
		if ok, _ := requiredGatesRecordedPass(refreshed.Manifest, store.NounFeature, &f.GatesBlock); !ok {
			return SubmitNodeResult{}, &FeatureNotSubmittableError{
				EpicID: doc.ID, EpicKey: key, FeatureID: f.ID, FeatureKey: deref(f.Key), Reason: "gates_not_green",
			}
		}
	}
	rev := deps.rev(tree.Root)
	if err := guardReviewIdentity(store.NounEpic, doc.ID, doc.PR, in.PR); err != nil {
		return SubmitNodeResult{}, err
	}
	if err := guardSubmitBranch(rev, store.NounEpic, doc.ID, doc.Branch, in.ReviewRef); err != nil {
		return SubmitNodeResult{}, err
	}
	for _, f := range riding {
		if err := guardAggregateTasks(refreshed, rev, store.NounEpic, doc.ID, doc.Branch, f); err != nil {
			return SubmitNodeResult{}, err
		}
	}
	if in.PR == nil {
		head, err := rev.CurrentCommit()
		if err != nil {
			return SubmitNodeResult{}, err
		}
		in.ReviewRef = &store.ReviewRef{Branch: doc.Branch, Head: head}
	}

	if samePR(doc.PR, in.PR) && sameReviewRef(doc.ReviewRef, in.ReviewRef) {
		return SubmitNodeResult{Noun: store.NounEpic, ID: doc.ID, Key: key, PR: doc.PR, ReviewRef: doc.ReviewRef, Changed: false}, nil
	}

	ev := submitEvent(deps, owner, eventSubmitEpic, store.NounEpic, doc.ID, key, relYml,
		doc.PR, doc.ReviewRef, in.PR, in.ReviewRef)
	doc.PR = in.PR
	doc.ReviewRef = in.ReviewRef

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return SubmitNodeResult{}, err
	}
	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, ydata))
	if err := commitMutation(tree, "submit.epic", writes, []events.Event{ev}); err != nil {
		return SubmitNodeResult{}, err
	}

	return SubmitNodeResult{Noun: store.NounEpic, ID: doc.ID, Key: key, PR: in.PR, ReviewRef: in.ReviewRef, Changed: true}, nil
}

// notStartedFeatureStatuses are the feature statuses submit refuses before
// asking anything else: a feature that has not yet reached active carries no
// work a human could review, whatever its resolved grain. "ready" has no
// package constant (sync.go reads it the same literal way).
var notStartedFeatureStatuses = map[string]bool{statusDraft: true, statusSpecced: true, "ready": true}

func featureNotStarted(status string) bool {
	return notStartedFeatureStatuses[status]
}

// terminalFeatureStatuses are the feature statuses submit refuses because
// the node has already closed: nothing further to submit once a feature
// reached done by rollup or was cancelled by a human. This is the identical
// set Configure's own guard locks the grain on (configure.go's
// refuseReviewSurfaceLocked, configureFeature's terminal arm), so the two
// commands can never disagree about when a feature's review window has
// shut.
var terminalFeatureStatuses = map[string]bool{statusDone: true, statusCancelled: true}

func featureTerminal(status string) bool {
	return terminalFeatureStatuses[status]
}

// terminalEpicStatuses mirrors terminalFeatureStatuses one grain up: an
// epic's own done, cancelled, or archived close. This is WIDER than
// configureEpic's own terminal set (configure.go's refuseReviewSurfaceLocked),
// which omits done; that omission is harmless there only because Configure's
// guard has an independent PR-recorded arm that already refuses a done epic
// in practice (a done epic almost always carries a recorded PR by then).
// submit's own terminal guard has no such second arm, and is the only thing
// standing between a closed node and a rewrite: without done excluded here,
// an epic-grain epic submitted once and then rolled up to done could be
// submitted again with a different PR, since every other check still passes
// on a done epic (grain still resolves, done is not draft, epicLiveFeatures
// excludes only cancelled so a done feature still counts as live, its tasks
// are all done, its gates still record their old pass, and the no-op check
// only stops an identical resubmit). A real next round only becomes
// submittable again after sync's reopenEpic moves status back to ready, so
// refusing done here costs nothing legitimate (fix round 2).
var terminalEpicStatuses = map[string]bool{statusDone: true, statusCancelled: true, statusArchived: true}

func epicTerminal(status string) bool {
	return terminalEpicStatuses[status]
}

// featureTasksSubmittable reports whether every non-cancelled task under f is
// in_review or done, and there is at least one such task: the task-readiness
// half of what submit needs before its coarse grain can close over the
// feature, at either grain that calls it (submitFeature directly,
// submitEpic once per live child). Work under any review grain still rides
// each task's own lifecycle end to end (docs/spec/cli-surface.md), so a task
// still in progress, or a feature with no live task at all, both answer false.
// blockerID is empty only in the second case; otherwise it and
// blockerKey/blockerStatus name the first live task short of in_review, in
// gen.OrderTasks order, so a refusal always names the same next thing to clear.
func featureTasksSubmittable(tree *store.Tree, f *store.Feature) (ok bool, blockerID, blockerKey, blockerStatus string) {
	live := 0
	for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
		if tk.Status == statusCancelled {
			continue
		}
		live++
		if tk.Status != "in_review" && tk.Status != statusDone {
			return false, tk.ID, deref(tk.Key), tk.Status
		}
	}
	return live > 0, "", "", ""
}

// guardFeatureCoarseSubmit preserves basis, question, and assumption guards.
func guardFeatureCoarseSubmit(tree *store.Tree, deps Deps, treeFeat, doc *store.Feature) error {
	if _, err := guardFeature(tree, treeFeat, doc, TransitionInput{}, deps.rev(tree.Root)); err != nil {
		return err
	}
	guarded := *treeFeat
	guarded.AssumptionBasis = doc.AssumptionBasis
	if err := guardSubmitAssumptions(tree, &guarded); err != nil {
		return err
	}
	return nil
}

// taskWithOwnOpenReview finds task-grain review history that must finish
// before a feature or epic can open a new coarse review: a pre-hardening task
// PR, or a task that started at task grain, whose PR or remoteless review_ref
// is its own and never enters the aggregate (issue #146).
func taskWithOwnOpenReview(tree *store.Tree, f *store.Feature) *store.Task {
	for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
		if tk.Status == "in_review" && (tk.PR != nil || tk.StartedGrain == store.ReviewGrainTask) {
			return tk
		}
	}
	return nil
}

func taskReviewOpenErr(reviewedNoun, reviewedID, reviewedKey string, task *store.Task) *TaskReviewOpenError {
	return &TaskReviewOpenError{
		ReviewedNoun: reviewedNoun,
		ReviewedID:   reviewedID,
		ReviewedKey:  reviewedKey,
		TaskID:       task.ID,
		TaskKey:      deref(task.Key),
		PR:           task.PR,
		ReviewBranch: task.Branch,
		StartedGrain: task.StartedGrain,
	}
}

// requiredGatesRecordedPass reports whether every required gate the manifest
// scopes to noun (requiredGateConfigs, check.go, so a gate that grain never
// runs is never demanded of it) is recorded on gb with status pass. It is
// deliberately a
// read of what a prior `tiller check` already recorded, the read-only half
// of the same required-gate walk sync's recordPolicyViolations makes over a
// node at merge time (sync.go): that function mutates gb (it appends a
// policy_violations entry for every gate short of pass) and is not safe to
// call from a refusal path that must write nothing on a failed check, so this
// is the minimal read extracted alongside it rather than a re-implementation
// of gate evaluation, which stays check's alone. missing names each
// required gate that is absent, failed, pending, or stale, in name order, so
// a refusal can be specific about what "not green" means here; "not green"
// deliberately excludes advisory presence-only readiness (gen.FeatureActivity's
// "presence is the bar, not green"), since the design here calls for an
// actually-passing contract before a coarse-grain PR opens.
func requiredGatesRecordedPass(m *store.Manifest, noun string, gb *store.GatesBlock) (ok bool, missing []string) {
	for _, c := range requiredGateConfigs(m, noun) {
		g, recorded := gb.Gates[c.Name]
		if !recorded || g.Status != store.GateStatusPass {
			missing = append(missing, c.Name)
		}
	}
	sort.Strings(missing)
	return len(missing) == 0, missing
}

// guardSubmitGates is task-grade submit's read of the latest recorded gate
// results. Reaching check proves an earlier run was green, not that the most
// recent one was: check is an idempotent retry from check, so a failed
// recheck records its failure and leaves the node at check. Submission
// therefore reads the required results as they stand now rather than trusting
// the status. An idempotent submit from in_review moves nothing and is left
// to the engine's safe-retry rule.
func guardSubmitGates(m *store.Manifest, noun, id, key, status string, gb *store.GatesBlock) error {
	if status != "check" {
		return nil
	}
	if ok, missing := requiredGatesRecordedPass(m, noun, gb); !ok {
		return &RequiredGatesNotGreenError{Noun: noun, NodeID: id, Key: key, Missing: missing}
	}
	return nil
}

// freshEpicSubmitFeatures reads the current layout inventory and every feature
// and task document in it. The returned guards pin the checked set and bytes
// until submit.epic commits.
func freshEpicSubmitFeatures(tree *store.Tree, epicKey string) (*store.Tree, []*store.Feature, []store.PlannedWrite, error) {
	refreshed, guards, err := freshReviewGrainLineage(tree, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	var live []*store.Feature
	for _, feature := range sortedFeatures(refreshed) {
		if feature.EpicKey == epicKey && feature.Status != statusCancelled {
			live = append(live, feature)
		}
	}
	return refreshed, live, guards, nil
}

// samePR reports whether two optional PR numbers name the same submit
// payload: both absent, or both present and equal.
func samePR(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// sameReviewRef is samePR's ReviewRef twin. store.ReviewRef holds only
// comparable fields (strings and a comparable RevisionRef), so a plain value
// comparison is exact once both sides are known non-nil.
func sameReviewRef(a, b *store.ReviewRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// submitEvent builds the record-only event submitFeature and submitEpic each
// write: no status change, just the PR or review ref landing on the node
// (this file's own doc comment, "record-only by design"). pr and review_ref
// appear in Before/After only when set, the same presence-only convention
// transitionEvent already keeps for task-grain submit, so a first submit's
// Before is empty and a later submit's Before shows exactly what this call
// replaced.
func submitEvent(deps Deps, owner store.Contributor, evType, noun, targetID, key, ymlRel string,
	beforePR *int, beforeRef *store.ReviewRef, afterPR *int, afterRef *store.ReviewRef) events.Event {

	ev := events.NewEvent(deps.clock(), evType, owner.ID, fmt.Sprintf("submit %s %s", noun, key), targetID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{}
	ev.After = map[string]any{}
	if beforePR != nil {
		ev.Before["pr"] = *beforePR
	}
	if beforeRef != nil {
		ev.Before["review_ref"] = map[string]any{"branch": beforeRef.Branch, "head": beforeRef.Head.Value}
	}
	if afterPR != nil {
		ev.After["pr"] = *afterPR
	}
	if afterRef != nil {
		ev.After["review_ref"] = map[string]any{"branch": afterRef.Branch, "head": afterRef.Head.Value}
	}
	ev.Files = []events.FileChange{{Path: ymlRel}}
	return ev
}

// wrongReviewGrainErr refuses submit feature|epic when the resolved review
// grain does not name the noun submit was called on: grain and noun share
// the same three literal values (store.ReviewGrainTask/Feature/Epic equal
// store.NounTask/Feature/Epic byte for byte), so the message can name the
// grain directly as the node whose own submit applies. It is a usageError
// (exit 2), the class taskOnlyFeatureVerbs already answers with for the
// narrower "submit named the wrong noun for a PR verb" case this
// generalizes to a configurable grain.
func wrongReviewGrainErr(grain string) error {
	return &usageError{
		reason: "wrong_review_grain",
		msg:    fmt.Sprintf("mutate: resolved review grain is %s; submit the %s's own PR", grain, grain),
	}
}

// NodeNotStartedError refuses submit feature|epic because the reviewed node
// has not begun work yet: a draft, specced, or ready feature, or a draft
// epic, carries nothing a human could review (submit records the PR of work
// already run, never a plan). Like the package's other blocked answers it is
// exit 1 and deliberately does not implement ExitCode, because the report
// sets that.
type NodeNotStartedError struct {
	Noun   string
	NodeID string
	Key    string
	Status string
}

func (e *NodeNotStartedError) ident() string {
	if e.Key != "" {
		return e.Key
	}
	return e.NodeID
}

func (e *NodeNotStartedError) Error() string {
	return fmt.Sprintf("mutate: %s %s has nothing to review yet (%s)", e.Noun, e.ident(), e.Status)
}

// NodeTerminalError refuses submit feature|epic because the reviewed node
// has already closed: a done or cancelled feature, or a cancelled or
// archived epic, has no review left to record. It mirrors Configure's own
// terminal guard (configure.go's refuseReviewSurfaceLocked, its terminal
// arm), so a caller cannot open a review on a node Configure itself would
// already refuse to touch. Like the package's other blocked answers it is
// exit 1 and deliberately does not implement ExitCode, because the report
// sets that.
type NodeTerminalError struct {
	Noun   string
	NodeID string
	Key    string
	Status string
}

func (e *NodeTerminalError) ident() string {
	if e.Key != "" {
		return e.Key
	}
	return e.NodeID
}

func (e *NodeTerminalError) Error() string {
	return fmt.Sprintf("mutate: %s %s is terminal (%s); submit records nothing further once a node has closed",
		e.Noun, e.ident(), e.Status)
}

// TaskNotSubmittableError refuses `submit feature` because a task under the
// reviewed feature is neither in_review nor done: whatever grain reviews the
// work, each task still rides its own lifecycle end to end, so a task still
// being worked has nothing to review yet. A feature with no live task at all
// reads the same way, with TaskID left empty.
type TaskNotSubmittableError struct {
	FeatureID  string
	FeatureKey string
	TaskID     string
	TaskKey    string
	TaskStatus string
}

func (e *TaskNotSubmittableError) featureIdent() string {
	if e.FeatureKey != "" {
		return e.FeatureKey
	}
	return e.FeatureID
}

func (e *TaskNotSubmittableError) Error() string {
	if e.TaskID == "" {
		return fmt.Sprintf("mutate: feature %s has no live task to review", e.featureIdent())
	}
	return fmt.Sprintf("mutate: feature %s cannot submit: task %s is %s, not in_review or done",
		e.featureIdent(), e.TaskID, e.TaskStatus)
}

// RequiredGatesNotGreenError refuses submit because the reviewed node's latest
// recorded result for some gate the manifest requires at its grain is not a
// pass. Feature submit reads the feature-grain set (design doc: "check feature
// and eval feature must be green ... before submit feature"); task and shelf
// submit read the task-grade set, so a failed recheck after an earlier green
// check refuses submission even though the node still sits at check. It names
// the remedy rather than each gate individually, since `tiller check` re-runs
// and records the whole required set in one act; Missing carries the gate
// names for a caller that wants them.
type RequiredGatesNotGreenError struct {
	Noun    string
	NodeID  string
	Key     string
	Missing []string
}

func (e *RequiredGatesNotGreenError) ident() string {
	if e.Key != "" {
		return e.Key
	}
	return e.NodeID
}

func (e *RequiredGatesNotGreenError) Error() string {
	return fmt.Sprintf("mutate: %s %s has not recorded every required gate green (%s); run `tiller check %s %s` first",
		e.Noun, e.ident(), strings.Join(e.Missing, ", "), e.Noun, e.ident())
}

// TaskReviewOpenError refuses a coarse submit when a legacy in_review task
// still carries the task-grain PR that must merge first.
type TaskReviewOpenError struct {
	ReviewedNoun string
	ReviewedID   string
	ReviewedKey  string
	TaskID       string
	TaskKey      string
	// PR is the task's own PR, nil when a remoteless task-grain task holds
	// only a review_ref on ReviewBranch.
	PR           *int
	ReviewBranch string
	// StartedGrain is the grain the task started under, so the refusal can
	// say why a task under a coarse grain carries its own PR (issue #146).
	StartedGrain string
}

func (e *TaskReviewOpenError) Error() string {
	if e.PR == nil {
		return fmt.Sprintf("mutate: %s %s cannot submit while task %s has its own review open on %s; run `tiller sync` after it merges",
			e.ReviewedNoun, e.ReviewedKey, e.TaskKey, e.ReviewBranch)
	}
	return fmt.Sprintf("mutate: %s %s cannot submit while task %s has open PR #%d; run `tiller sync` after it merges",
		e.ReviewedNoun, e.ReviewedKey, e.TaskKey, *e.PR)
}

// FeatureNotSubmittableError refuses `submit epic` when a feature is not
// eligible for the epic review: an override has not reached done, its riding
// tasks are incomplete (or absent), or its required gates are not green.
type FeatureNotSubmittableError struct {
	EpicID     string
	EpicKey    string
	FeatureID  string
	FeatureKey string
	Reason     string // "tasks_incomplete", "gates_not_green", or ReasonIndependentReviewOpen

	// TaskID names the first live task short of in_review when Reason is
	// "tasks_incomplete". It is empty when the feature has no live task and
	// carries no meaning for the other reasons.
	TaskID string
}

func (e *FeatureNotSubmittableError) epicIdent() string {
	if e.EpicKey != "" {
		return e.EpicKey
	}
	return e.EpicID
}

func (e *FeatureNotSubmittableError) featureIdent() string {
	if e.FeatureKey != "" {
		return e.FeatureKey
	}
	return e.FeatureID
}

func (e *FeatureNotSubmittableError) Error() string {
	if e.Reason == ReasonIndependentReviewOpen {
		return fmt.Sprintf("mutate: epic %s cannot submit: overridden feature %s must finish its own review first",
			e.epicIdent(), e.featureIdent())
	}
	if e.Reason == "gates_not_green" {
		return fmt.Sprintf("mutate: epic %s cannot submit: feature %s has not recorded every required gate green; run `tiller check feature %s` first",
			e.epicIdent(), e.featureIdent(), e.featureIdent())
	}
	if e.TaskID == "" {
		return fmt.Sprintf("mutate: epic %s cannot submit: feature %s has no live task to review",
			e.epicIdent(), e.featureIdent())
	}
	return fmt.Sprintf("mutate: epic %s cannot submit: feature %s has a task not yet in_review or done",
		e.epicIdent(), e.featureIdent())
}

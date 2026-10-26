// Package workflow classifies shared acceptance and review work from observed facts.
package workflow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// Input contains caller-observed local facts. An absent contract is
// explicitly unread, never a valid empty acceptance result.
type Input struct {
	Contracts       map[string]*store.EvalFile
	ContractErrors  map[string]string
	ContractMissing map[string]bool
	// Eligibility carries observed basis failures only. Question holds derive
	// from the tree and must not overwrite or conceal these independent guards.
	Eligibility           map[string]string
	SubmissionEligibility map[string]string
	// Assembly lists submitted task branches still missing from each feature's
	// aggregate, including submission-state commits at their current tips.
	Assembly       map[string][]string
	AssemblyErrors map[string]string
	// RecoveryErrors retains unavailable or invalid immutable evidence; consumers
	// must never reinterpret it using an ordinary live aggregate branch.
	RecoveryErrors map[string]string
	// Observed carries task execution observed on local branches and
	// worktrees, keyed by task ID. Routing reads it only to mute the reader's
	// own in-flight task or pass over another's; eligibility never reads it.
	Observed map[string]Observation
}

// Observation names the branch a task is observed in flight on and the owner
// that branch records, which may be empty.
type Observation struct {
	Branch string
	Owner  string
}

type Work struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Actor  string `json:"actor"`
	NodeID string `json:"node_id,omitempty"`
}

// Advice is independent of legal_next and of external host state.
type Advice struct {
	Phase                string `json:"phase"`
	Actor                string `json:"actor"`
	Action               string `json:"action"`
	Command              string `json:"command,omitempty"`
	Reason               string `json:"reason"`
	ReviewID             string `json:"review_id"`
	FeatureID            string `json:"feature_id,omitempty"`
	ContractStatus       string `json:"contract_status"`
	AcceptanceReady      bool   `json:"acceptance_ready"`
	SubmissionRecorded   bool   `json:"submission_recorded"`
	Outstanding          []Work `json:"outstanding,omitempty"`
	HistoricalViolations int    `json:"historical_violations,omitempty"`
	// CrossingViolations separates source-less crossing-time warnings only for
	// recovered reviews; ordinary stored violations retain historical semantics.
	CrossingViolations        int                 `json:"crossing_violations,omitempty"`
	RecoveredReview           *RecoveredReview    `json:"recovered_review,omitempty"`
	ReviewMissing             bool                `json:"review_missing,omitempty"`
	ReviewGuidance            string              `json:"review_guidance,omitempty"`
	RecoveredCrossingsPending bool                `json:"recovered_crossings_pending"`
	CurrentAcceptance         []CurrentAcceptance `json:"current_acceptance,omitempty"`
}

// aggregateBranchOf is the recorded aggregate branch of a review node: the
// feature's at feature grain, the epic's at epic grain.
func aggregateBranchOf(review store.Node) string {
	switch n := review.(type) {
	case *store.Feature:
		return n.Branch
	case *store.Epic:
		return n.Branch
	}
	return ""
}

// Classify retains each outstanding required item even when an earlier
// machine or rubric phase determines the next actor. Authored optional criteria
// never enter the blocking set; stored Required flags are not configuration.
func Classify(m *store.Manifest, f *store.Feature, review store.Node, grain string, ready bool, in Input) (a Advice) {
	a = Advice{ReviewID: review.NodeID(), FeatureID: f.ID, ContractStatus: "unread", HistoricalViolations: len(f.PolicyViolations)}
	contract := in.Contracts[f.ID]
	if message := in.ContractErrors[f.ID]; message != "" {
		a.ContractStatus = "unavailable"
		a.Outstanding = append(a.Outstanding, Work{Kind: "contract", ID: f.ID, Status: message, Actor: "agent"})
	} else if contract == nil {
		a.Outstanding = append(a.Outstanding, Work{Kind: "contract", ID: f.ID, Status: "unread", Actor: "agent"})
	} else if err := contract.ValidateContract(); err != nil {
		a.ContractStatus = "invalid"
		a.Outstanding = append(a.Outstanding, Work{Kind: "contract", ID: f.ID, Status: err.Error(), Actor: "agent"})
	} else {
		a.ContractStatus = "valid"
	}
	var names []string
	evalsRequired := false
	if m != nil {
		for name, spec := range m.Gates.Required {
			if spec.On.Includes(store.NounFeature) {
				names = append(names, name)
				if name == "evals" {
					evalsRequired = true
				}
			}
		}
	}
	sort.Strings(names)
	for _, name := range names {
		spec, recorded := m.Gates.Required[name], f.Gates[name]
		if name != "evals" && spec.Kind == store.GateKindCommand && strings.TrimSpace(spec.Run) == "" {
			a.Outstanding = append(a.Outstanding, Work{Kind: "configuration", ID: name, Status: "unconfigured", Actor: "agent"})
			continue
		}
		if recorded.Status == store.GateStatusPass {
			continue
		}
		status := recorded.Status
		if status == "" {
			status = "missing"
		}
		kind, actor := "gate", "agent"
		if spec.Kind == store.GateKindAttestation {
			actor = "human"
		}
		// evals is the aggregate of the contract below. A missing or stale
		// aggregate still needs check even if all task records happen to pass.
		if name == "evals" && a.ContractStatus == "valid" && status != "missing" && status != store.GateStatusStale {
			continue
		}
		a.Outstanding = append(a.Outstanding, Work{Kind: kind, ID: name, Status: status, Actor: actor})
	}
	if a.ContractStatus == "valid" && evalsRequired {
		for _, c := range contract.Criteria {
			if !c.Required {
				continue
			}
			r := f.Gates["evals"].Criteria[c.ID]
			if r.Status == store.CriterionStatusPass {
				continue
			}
			status := r.Status
			if status == "" {
				status = "missing"
			}
			kind, actor := "criterion", "agent"
			switch c.Binding.Type {
			case store.BindingManual:
				actor = "human"
			case store.BindingEval:
				kind = "rubric"
				if r.Status == store.CriterionStatusNeedsAttestation && hasRubricEvidence(r.Evidence) {
					actor = "human"
				}
			}
			a.Outstanding = append(a.Outstanding, Work{Kind: kind, ID: c.ID, Status: status, Actor: actor})
		}
	}
	if evalsRequired && a.ContractStatus == "valid" && len(a.Outstanding) == 0 && f.Gates["evals"].Status != store.GateStatusPass {
		a.Outstanding = append(a.Outstanding, Work{Kind: "gate", ID: "evals", Status: f.Gates["evals"].Status, Actor: "agent"})
	}
	a.AcceptanceReady = len(a.Outstanding) == 0
	switch n := review.(type) {
	case *store.Feature:
		a.SubmissionRecorded = HasRecordedReview(n.PR, n.ReviewRef)
		a.RecoveredReview = RecoveryOf(n.PR, n.RecoveredReview)
	case *store.Epic:
		a.SubmissionRecorded = HasRecordedReview(n.PR, n.ReviewRef)
		a.RecoveredReview = RecoveryOf(n.PR, n.RecoveredReview)
	}
	if a.RecoveredReview != nil {
		a.HistoricalViolations = 0
		for _, violation := range f.PolicyViolations {
			if violation.SourceRevision.Value != "" {
				a.HistoricalViolations++
			} else {
				a.CrossingViolations++
			}
		}
	}
	a.ReviewMissing = grain != store.ReviewGrainTask && !a.SubmissionRecorded && a.RecoveredReview == nil && review.NodeStatus() != "done"
	if a.ReviewMissing {
		a.ReviewGuidance = MissingReviewGuidance(review)
	}
	defer func() {
		if ready && a.ReviewMissing {
			a.Reason += "; " + a.ReviewGuidance
		}
	}()
	a.Phase, a.Actor, a.Action = "execution", "agent", "execute"
	a.Reason = "implementation work remains"
	if !ready && a.RecoveredReview == nil {
		return a
	}
	if review.NodeStatus() == "done" {
		a.Phase, a.Actor, a.Action = "done", "none", "report"
		a.Reason = "done remains recorded"
		if a.RecoveredReview != nil {
			a.Reason += "; " + RecoverySummary(a.RecoveredReview)
		}
		if !a.AcceptanceReady {
			a.Reason += "; acceptance remains unresolved: " + workflowItems(a.Outstanding)
		}
		if a.HistoricalViolations > 0 && a.RecoveredReview == nil {
			a.Reason += fmt.Sprintf("; %d historical policy violations remain recorded", a.HistoricalViolations)
		}
		return a
	}
	if a.RecoveredReview != nil {
		a.Reason = RecoverySummary(a.RecoveredReview)
		if problem := in.RecoveryErrors[a.ReviewID]; problem != "" {
			a.Phase, a.Action, a.Command = "blocked", "inspect_recovery", ""
			a.Reason += "; cannot inspect recovered evidence: " + problem
			return a
		}
		a.Phase, a.Action, a.Command = "sync", "sync", "tiller sync"
		a.Reason += "; run ordinary sync on integration " + a.RecoveredReview.IntegrationBranch + ", then publish and read back state"
		if a.RecoveredReview.Basis == "contained" {
			a.Phase, a.Actor, a.Action = "fast_forward", "human", "accept_fast_forward"
			a.Command = "tiller sync --accept-fast-forward " + a.ReviewID
			a.Reason += "; containment is not an observed merge; the existing human fast-forward decision is required before crossing"
		}
		if !a.AcceptanceReady {
			a.Reason += "; current acceptance remains independently unresolved: " + workflowItems(a.Outstanding)
		}
		return a
	}
	if grain != store.ReviewGrainTask {
		if problem := in.AssemblyErrors[f.ID]; problem != "" {
			a.Phase, a.Action, a.Reason = "blocked", "inspect_assembly", problem
			return a
		}
		if tasks := in.Assembly[f.ID]; len(tasks) > 0 {
			a.Phase, a.Action = "assembly", "assemble"
			a.Command = "tiller guide execute"
			a.Reason = "assemble submitted task branches into the " + review.Noun() + " aggregate with ordinary Git, including each submitted head and current branch tip: " + strings.Join(tasks, ", ") + "; then rerun affected checks and acceptance"
			return a
		}
	}
	if a.ContractStatus != "valid" {
		a.Phase, a.Action = "contract", "repair_contract"
		a.Reason = "acceptance contract " + a.ContractStatus + "; read or repair the feature contract before classifying acceptance"
		return a
	}
	agentWork := -1
	for i, work := range a.Outstanding {
		if work.Actor != "agent" {
			continue
		}
		if agentWork < 0 {
			agentWork = i
		}
		if work.Kind != "rubric" {
			agentWork = i
			break
		}
	}
	if agentWork >= 0 {
		work := a.Outstanding[agentWork]
		a.Phase, a.Action, a.Command = "checks", "check", "tiller check feature "+f.ID
		if work.Kind == "rubric" {
			a.Phase, a.Action, a.Command = "rubric", "grade_rubric", "tiller eval feature "+f.ID+" --emit-rubrics"
		}
		a.Reason = "required work remains on feature " + f.ID + ": " + workflowItems(a.Outstanding)
		if work.Kind == "configuration" {
			a.Action, a.Command = "configure_gate", ""
			a.Reason += "; configure the required gate command in the manifest before running check"
		}
		return a
	}
	if len(a.Outstanding) > 0 {
		a.Phase, a.Actor, a.Action = "acceptance", "human", "attest"
		a.Reason = "human acceptance remains on feature " + f.ID + ": " + workflowItems(a.Outstanding) + "; execute must prepare and verify the draft or offline review surface before handoff; host publication is not observed here"
		if grain == store.ReviewGrainTask {
			a.Reason = "human acceptance remains on feature " + f.ID + ": " + workflowItems(a.Outstanding) + "; complete applicable checks and publish integration-branch state (or verify the local tip offline) before handoff; do not open a new PR for state-only closeout"
		}
		for _, work := range a.Outstanding {
			flag := "--criterion"
			if work.Kind == "gate" {
				flag = "--gate"
			}
			a.Reason += "; human decision: `tiller attest feature " + f.ID + " " + flag + " " + work.ID + " --status <pass|fail>`"
		}
		// A coarse review's attest is refused off its recorded aggregate
		// branch; the handoff names where the human records it (issue #150).
		if branch := aggregateBranchOf(review); grain != store.ReviewGrainTask && branch != "" {
			a.Reason += "; record each decision on aggregate branch " + branch + " (in its worktree if checked out there), commit there, then submit"
		}
		return a
	}
	if grain == store.ReviewGrainTask {
		a.Phase, a.Action, a.Command = "sync", "sync", "tiller sync"
		a.Reason = "all live tasks are done and feature acceptance passed; run ordinary sync on the integration branch, then publish and read back state"
		return a
	}
	if grain != store.ReviewGrainTask && a.SubmissionRecorded {
		a.Phase, a.Actor, a.Action = "review", "human", "review_merge"
		a.Reason = "formal submission recorded; awaiting human review and merge"
		if !a.AcceptanceReady {
			a.Reason += "; acceptance remains unresolved: " + workflowItems(a.Outstanding)
		}
		return a
	}
	a.Phase, a.Action = "submission", "submit"
	a.Command = "tiller guide execute"
	a.Reason = "required acceptance passed; formal submission not recorded"
	return a
}

func hasRubricEvidence(evidence []store.Evidence) bool {
	for _, e := range evidence {
		if e.Kind == "rubric_output" && e.Ref != "" {
			return true
		}
	}
	return false
}

func workflowItems(items []Work) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		prefix := ""
		if item.NodeID != "" {
			prefix = item.NodeID + " "
		}
		parts = append(parts, prefix+item.ID+" ("+item.Status+", "+item.Actor+")")
	}
	return strings.Join(parts, "; ")
}

func featureTasksDone(t *store.Tree, f *store.Feature) bool {
	live := 0
	for _, task := range t.FeatureTasks[f.ID] {
		if task.Status == "cancelled" {
			continue
		}
		live++
		if task.Status != "done" {
			return false
		}
	}
	return live > 0
}

// AdviceOf describes this node's resolved work, never the project route.
func AdviceOf(t *store.Tree, n store.Node, in Input) *Advice {
	var f *store.Feature
	switch node := n.(type) {
	case *store.Feature:
		f = node
	case *store.Task:
		f = t.Features[node.FeatureID]
		if node.PR != nil || node.StartedGrain == store.ReviewGrainTask {
			return nil
		} // historical and task-grain task reviews retain their own identity
	case *store.Epic:
		return EpicAdvice(t, gen.DependencyOrder(t), node, in)
	default:
		return nil
	}
	if f == nil || f.Status == "cancelled" {
		return nil
	}
	epic := epicByKey(t, f.EpicKey)
	grain := store.ResolvedReviewGrain(t.Manifest, epic, f)
	if task, ok := n.(*store.Task); ok && (grain == store.ReviewGrainTask || task.Status != "in_review") {
		return nil
	}
	if grain == store.ReviewGrainEpic && epic != nil && (f.Status != "done" || epic.RecoveredReview != nil) {
		return EpicAdvice(t, gen.DependencyOrder(t), epic, in)
	}
	var reviewed store.Node = f
	ready := FeatureTasksReadyToSubmit(t, f)
	if grain == store.ReviewGrainTask {
		ready = featureTasksDone(t, f)
	}
	if !ready && f.Status != "done" && f.RecoveredReview == nil {
		return nil
	}
	a := Classify(t.Manifest, f, reviewed, grain, ready || f.Status == "done", in)
	if f.Status == "done" && reviewed.NodeID() != f.ID {
		a = Classify(t.Manifest, f, f, grain, true, in)
	}
	applyRecoveredCloseout(t, &a)
	applyWorkflowEligibility(t, n, in, &a)
	return &a
}

func applyWorkflowEligibility(t *store.Tree, n store.Node, in Input, a *Advice) {
	if n.NodeStatus() == "done" {
		return
	}
	if a.RecoveredReview != nil {
		// Current eligibility remains visible, but cannot send already integrated
		// work back through premerge assembly, submission, or acceptance guards.
		if problem := in.Eligibility[n.NodeID()]; problem != "" {
			a.Reason += "; current eligibility: " + problem
		}
		if n.NodeID() != a.FeatureID {
			if problem := in.Eligibility[a.FeatureID]; problem != "" {
				a.Reason += "; current feature eligibility: " + problem
			}
		}
		if blocked(n) {
			a.Reason += "; current node is blocked"
		}
		if f := t.Features[a.FeatureID]; f != nil {
			if blockers := gen.FeatureQuestionBlockers(t, f); len(blockers) > 0 {
				a.Reason += "; current open question " + blockers[0].Question
			}
		}
		return
	}
	problem := in.Eligibility[n.NodeID()]
	if problem == "" {
		problem = in.Eligibility[a.FeatureID]
	}
	if problem == "" && a.Action == "submit" {
		problem = in.SubmissionEligibility[a.FeatureID]
	}
	if retired := gen.RetiredAncestor(t, n); retired != nil {
		problem = "retired ancestor " + retired.Ref
	}
	if blocked(n) {
		problem = "node is blocked"
	}
	if f := t.Features[a.FeatureID]; f != nil {
		epic := epicByKey(t, f.EpicKey)
		if epic == nil || epic.Status == "draft" {
			problem = "parent epic is not approved"
		}
		if dependency := UnmetDependency(t, f, epic); dependency != "" {
			problem = "unmet dependency " + dependency
		}
		if blockers := gen.FeatureQuestionBlockers(t, f); len(blockers) > 0 {
			problem = "open question " + blockers[0].Question
		}
	}
	if problem == "" {
		return
	}
	a.Phase, a.Actor, a.Action, a.Command = "blocked", "human", "resolve_blocker", ""
	a.Reason = problem + "; resolve the local eligibility problem before execution"
	if a.ReviewGuidance != "" {
		a.Reason += "; " + a.ReviewGuidance
	}
}

func EpicAdvice(t *store.Tree, order *gen.Order, epic *store.Epic, in Input) *Advice {
	if store.ResolvedReviewGrain(t.Manifest, epic, nil) != store.ReviewGrainEpic {
		return nil
	}
	var selected *Advice
	var participating []*store.Feature
	for _, f := range EpicLiveFeatures(t, order, epic) {
		if store.ResolvedReviewGrain(t.Manifest, epic, f) != store.ReviewGrainEpic {
			if f.Status != "done" {
				return nil
			}
			continue
		}
		if !FeatureTasksReadyToSubmit(t, f) && epic.RecoveredReview == nil {
			return nil
		}
		a := Classify(t.Manifest, f, epic, store.ReviewGrainEpic, true, in)
		if epic.RecoveredReview != nil {
			for i := range a.Outstanding {
				a.Outstanding[i].NodeID = f.ID
			}
			fact := CurrentAcceptance{NodeID: f.ID, ContractStatus: a.ContractStatus, AcceptanceReady: a.AcceptanceReady, Outstanding: a.Outstanding}
			participating = append(participating, f)
			if selected == nil {
				a.CurrentAcceptance = []CurrentAcceptance{fact}
				selected = &a
			} else {
				selected.AcceptanceReady = selected.AcceptanceReady && a.AcceptanceReady
				if selected.ContractStatus != a.ContractStatus {
					selected.ContractStatus = "mixed"
				}
				selected.Outstanding = append(selected.Outstanding, a.Outstanding...)
				selected.HistoricalViolations += a.HistoricalViolations
				selected.CrossingViolations += a.CrossingViolations
				selected.CurrentAcceptance = append(selected.CurrentAcceptance, fact)
			}
			continue
		}
		applyWorkflowEligibility(t, f, in, &a)
		if selected == nil || workflowPriority(a) < workflowPriority(*selected) {
			copy := a
			selected = &copy
		}
	}
	if selected != nil && epic.RecoveredReview != nil {
		applyRecoveredCloseout(t, selected)
		featureID := selected.FeatureID
		for _, feature := range participating {
			selected.FeatureID = feature.ID
			applyWorkflowEligibility(t, feature, in, selected)
		}
		selected.FeatureID = featureID
	}
	return selected
}

func workflowPriority(a Advice) int {
	switch a.Phase {
	case "blocked", "contract":
		return 0
	case "assembly", "checks":
		return 1
	case "rubric":
		return 2
	case "acceptance":
		return 3
	default:
		return 4
	}
}

func SharedCloseoutNodes(t *store.Tree, order *gen.Order) []store.Node {
	var nodes []store.Node
	for _, epic := range OrderedEpics(t, order) {
		if epic.Status != "done" && epic.Status != "cancelled" && epic.Status != "archived" && store.ResolvedReviewGrain(t.Manifest, epic, nil) == store.ReviewGrainEpic {
			nodes = append(nodes, epic)
		}
	}
	for _, feature := range OrderedFeatures(t, order) {
		if feature.Status != "active" || gen.RetiredAncestor(t, feature) != nil {
			continue
		}
		if store.ResolvedReviewGrain(t.Manifest, epicByKey(t, feature.EpicKey), feature) != store.ReviewGrainEpic {
			nodes = append(nodes, feature)
		}
	}
	return nodes
}

func EpicLiveFeatures(t *store.Tree, order *gen.Order, e *store.Epic) []*store.Feature {
	if e.Key == nil {
		return nil
	}
	epicKey := *e.Key
	var out []*store.Feature
	for _, f := range OrderedFeatures(t, order) {
		if f.EpicKey != epicKey || f.Status == "cancelled" {
			continue
		}
		if gen.RetiredAncestor(t, f) != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func OrderedEpics(t *store.Tree, order *gen.Order) []*store.Epic {
	var out []*store.Epic
	seen := map[string]bool{}
	for _, eid := range order.Epics() {
		if e := t.Epics[eid]; e != nil {
			out = append(out, e)
			seen[eid] = true
		}
	}
	var orphans []*store.Epic
	for id, e := range t.Epics {
		if !seen[id] {
			orphans = append(orphans, e)
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].ID < orphans[j].ID })
	return append(out, orphans...)
}

func FeatureTasksReadyToSubmit(t *store.Tree, f *store.Feature) bool {
	live := 0
	for _, tk := range t.FeatureTasks[f.ID] {
		if tk.Status == "cancelled" {
			continue
		}
		live++
		// A task PR, or a task started at task grain (issue #146, R6), is a
		// task-owned review the aggregate never absorbs; it holds the submit.
		if tk.Status == "in_review" && (tk.PR != nil || tk.StartedGrain == store.ReviewGrainTask) {
			return false
		}
		if tk.Status != "in_review" && tk.Status != "done" {
			return false
		}
	}
	return live > 0
}

func HasRecordedReview(pr *int, ref *store.ReviewRef) bool {
	return pr != nil || (ref != nil && ref.Branch != "")
}

func UnmetDependency(t *store.Tree, f *store.Feature, owning *store.Epic) string {
	if unmet := unmetTarget(t, f.DependsOn); unmet != "" {
		return unmet
	}
	// The owning epic's recorded depends_on gates its features the same way
	// (issue #46): an epic-grain edge is recorded state, and a
	// draft under an epic whose prerequisites are unfinished is not
	// whose-turn-is-now any more than a feature whose own edge is unmet. The
	// old walk honored such an edge only by flow position, which held exactly
	// until the prerequisite epic's features left draft; a gate cannot depend
	// on luck of position, and the soft order (which now outranks position)
	// must never outrank a recorded edge. The caller resolves the owning
	// epic (both walks already hold the key-to-epic map); a feature whose
	// EpicKey names no epic gates on its own edges alone.
	if owning != nil {
		return unmetTarget(t, owning.DependsOn)
	}
	return ""
}

func unmetTarget(t *store.Tree, deps []string) string {
	for _, dep := range deps {
		if df, ok := t.Features[dep]; ok {
			if df.Status != "done" && df.Status != "cancelled" {
				return gen.Ref(t, df)
			}
			continue
		}
		if de, ok := t.Epics[dep]; ok {
			if de.Status != "done" && de.Status != "cancelled" && de.Status != "archived" {
				return gen.Ref(t, de)
			}
		}
	}
	return ""
}

func OrderedFeatures(t *store.Tree, order *gen.Order) []*store.Feature {
	var out []*store.Feature
	seen := map[string]bool{}
	for _, eid := range order.Epics() {
		e := t.Epics[eid]
		if e == nil || e.Key == nil {
			continue
		}
		for _, fid := range order.Features(*e.Key) {
			if f := t.Features[fid]; f != nil {
				out = append(out, f)
				seen[fid] = true
			}
		}
	}
	var orphans []*store.Feature
	for id, f := range t.Features {
		if !seen[id] {
			orphans = append(orphans, f)
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].ID < orphans[j].ID })
	return append(out, orphans...)
}

func epicByKey(t *store.Tree, key string) *store.Epic {
	for _, epic := range t.Epics {
		if epic.Key != nil && *epic.Key == key {
			return epic
		}
	}
	return nil
}

func blocked(n store.Node) bool {
	switch node := n.(type) {
	case *store.Epic:
		return node.Blocked
	case *store.Feature:
		return node.Blocked
	case *store.Task:
		return node.Blocked
	}
	return false
}

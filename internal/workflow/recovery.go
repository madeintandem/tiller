package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
)

// RecoveredReview projects immutable historical evidence, independently of
// current acceptance and of host state. PR association is caller-supplied.
type RecoveredReview struct {
	PR                *int                       `json:"pr,omitempty"`
	Branch            string                     `json:"branch"`
	Head              string                     `json:"head"`
	MergeCommit       string                     `json:"merge_commit"`
	IntegrationBranch string                     `json:"integration_branch"`
	Basis             string                     `json:"basis"`
	RecoveredBy       string                     `json:"recovered_by"`
	RecoveredAt       time.Time                  `json:"recovered_at"`
	Reason            string                     `json:"reason"`
	Tasks             []RecoveredTask            `json:"tasks"`
	PolicySnapshots   []HistoricalPolicySnapshot `json:"policy_snapshots"`
}

type RecoveredTask struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	State string `json:"state"`
}

type HistoricalPolicySnapshot struct {
	NodeID       string                        `json:"node_id,omitempty"`
	Path         string                        `json:"path"`
	Source       string                        `json:"source"`
	Observations []HistoricalPolicyObservation `json:"observations"`
}

type HistoricalPolicyObservation struct {
	Gate               string   `json:"gate"`
	Status             string   `json:"status"`
	UnresolvedCriteria []string `json:"unresolved_criteria,omitempty"`
	EvidenceRefs       []string `json:"evidence_refs,omitempty"`
}

func RecoveryOf(pr *int, record *store.RecoveredReview) *RecoveredReview {
	if record == nil {
		return nil
	}
	out := &RecoveredReview{PR: pr, Branch: record.Branch, Head: record.Head.Value,
		MergeCommit: record.MergeCommit.Value, IntegrationBranch: record.IntegrationBranch,
		Basis: record.Basis, RecoveredBy: record.RecoveredBy, RecoveredAt: record.RecoveredAt,
		Reason: record.Reason, Tasks: make([]RecoveredTask, len(record.Tasks)),
		PolicySnapshots: make([]HistoricalPolicySnapshot, len(record.PolicySnapshots))}
	for i, task := range record.Tasks {
		out.Tasks[i] = RecoveredTask{ID: task.ID, Path: task.Path, State: task.State.Value}
	}
	for i, snapshot := range record.PolicySnapshots {
		out.PolicySnapshots[i] = HistoricalPolicySnapshot{NodeID: snapshot.NodeID, Path: snapshot.Path, Source: snapshot.Source.Value, Observations: make([]HistoricalPolicyObservation, len(snapshot.Observations))}
		for j, observation := range snapshot.Observations {
			out.PolicySnapshots[i].Observations[j] = HistoricalPolicyObservation{Gate: observation.Gate, Status: observation.Status, UnresolvedCriteria: observation.UnresolvedCriteria, EvidenceRefs: observation.EvidenceRefs}
		}
	}
	return out
}

// RecoverySummary names provenance and nonpassing recorded policy; it does not
// claim a stored pass was rerun against the integration result.
func RecoverySummary(r *RecoveredReview) string {
	if r == nil {
		return ""
	}
	identity := "branch " + r.Branch
	if r.PR != nil {
		identity = fmt.Sprintf("caller-supplied PR #%d on %s", *r.PR, r.Branch)
	}
	text := fmt.Sprintf("Recovered historical review: %s; head %s; integration result %s; basis %s; recorded by %s: %s", identity, r.Head, r.MergeCommit, r.Basis, r.RecoveredBy, r.Reason)
	var policy []string
	for _, snapshot := range r.PolicySnapshots {
		for _, observation := range snapshot.Observations {
			item := snapshot.NodeID + " " + observation.Gate + ": " + observation.Status + " (source " + snapshot.Source + ")"
			if len(observation.UnresolvedCriteria) > 0 {
				item += "; unresolved criteria " + strings.Join(observation.UnresolvedCriteria, ", ")
			}
			policy = append(policy, item)
		}
	}
	if len(policy) > 0 {
		text += "; historical policy: " + strings.Join(policy, "; ")
	}
	return text
}

// MissingReviewNotice is the read-only diagnosis without an owning node address.
const MissingReviewNotice = "Aggregate review is not recorded; sync has not inspected its merge state. If unmerged, complete ordinary submission of the owning feature or epic. If already merged, explicitly authorized historical recovery with verified head/result and review identity is required from integration, followed by ordinary sync."

// MissingReviewGuidance describes both possibilities without observing a merge
// or manufacturing the historical identity needed to recover one.
func MissingReviewGuidance(review store.Node) string {
	return "Aggregate review is not recorded; sync has not inspected its merge state. If unmerged, complete ordinary assembly/checks/publication and record `tiller submit " + review.Noun() + " " + review.NodeID() + " --pr <actual-number>` (or the permitted no-remote review ref). If already merged, only on explicit human authorization verify the historical review externally, then record on integration with `tiller submit " + review.Noun() + " " + review.NodeID() + " --recover-merged --head <historical-reviewed-commit> --merge-commit <integration-result-commit> --reason <explanation>` and `--pr <verified-number>` when a remote is configured; then run ordinary `tiller sync`."
}

// ReviewStateOf supplies list consumers with the owning coarse review state,
// including riding tasks. Independent task PRs retain ordinary behavior.
// Missing means submitted work waits on an aggregate review nobody recorded:
// the task itself is in review, or the feature or epic holds such a task.
// Unstarted and in-progress work has nothing stranded, so it never warns.
func ReviewStateOf(t *store.Tree, n store.Node) (*RecoveredReview, bool) {
	owner := ReviewOwnerOf(t, n)
	var recovered *RecoveredReview
	var recorded bool
	switch o := owner.(type) {
	case *store.Feature:
		recovered, recorded = RecoveryOf(o.PR, o.RecoveredReview), o.RecoveredReview != nil || HasRecordedReview(o.PR, o.ReviewRef)
	case *store.Epic:
		recovered, recorded = RecoveryOf(o.PR, o.RecoveredReview), o.RecoveredReview != nil || HasRecordedReview(o.PR, o.ReviewRef)
	default:
		return nil, false
	}
	if recorded || reviewOwnerClosed(owner.NodeStatus()) {
		return recovered, false
	}
	switch node := n.(type) {
	case *store.Task:
		return recovered, node.Status == "in_review"
	case *store.Feature:
		return recovered, hasSubmittedRider(t, node)
	case *store.Epic:
		for _, f := range t.Features {
			if node.Key != nil && f.EpicKey == *node.Key && ReviewOwnerOf(t, f) == owner && hasSubmittedRider(t, f) {
				return recovered, true
			}
		}
	}
	return recovered, false
}

func reviewOwnerClosed(status string) bool {
	return status == "done" || status == "cancelled" || status == "archived"
}

// hasSubmittedRider reports a task of f in review on the aggregate, not on a
// task PR of its own.
func hasSubmittedRider(t *store.Tree, f *store.Feature) bool {
	for _, task := range t.FeatureTasks[f.ID] {
		if task.Status == "in_review" && task.PR == nil {
			return true
		}
	}
	return false
}

// ReviewOwnerOf resolves the aggregate independently of closeout readiness.
// Historical task PRs stay task-owned rather than riding the current grain.
func ReviewOwnerOf(t *store.Tree, n store.Node) store.Node {
	var feature *store.Feature
	switch node := n.(type) {
	case *store.Epic:
		if node.RecoveredReview != nil || store.ResolvedReviewGrain(t.Manifest, node, nil) == store.ReviewGrainEpic {
			return node
		}
		return nil
	case *store.Feature:
		feature = node
	case *store.Task:
		if node.PR != nil {
			return nil
		}
		feature = t.Features[node.FeatureID]
	default:
		return nil
	}
	if feature == nil {
		return nil
	}
	if feature.RecoveredReview != nil {
		return feature
	}
	// A cancelled feature rides no review, so it never inherits its epic's.
	// Its own recovered review above is history and stays visible.
	if feature.Status == "cancelled" {
		return nil
	}
	epic := epicByKey(t, feature.EpicKey)
	switch store.ResolvedReviewGrain(t.Manifest, epic, feature) {
	case store.ReviewGrainFeature:
		return feature
	case store.ReviewGrainEpic:
		if epic != nil {
			return epic
		}
	}
	return nil
}

package workflow

import (
	"fmt"

	"github.com/madeintandem/tiller/internal/store"
)

// CurrentAcceptance retains feature-local acceptance when an epic's next action
// is determined by its historical review rather than by one feature's gates.
type CurrentAcceptance struct {
	NodeID          string `json:"node_id"`
	ContractStatus  string `json:"contract_status"`
	AcceptanceReady bool   `json:"acceptance_ready"`
	Outstanding     []Work `json:"outstanding,omitempty"`
}

// applyRecoveredCloseout distinguishes pending historical crossings from the
// independent acceptance work that can remain after those riders have crossed.
func applyRecoveredCloseout(t *store.Tree, a *Advice) {
	if a.RecoveredReview == nil {
		return
	}
	defer func() {
		if a.HistoricalViolations > 0 {
			a.Reason += fmt.Sprintf("; %d source-pinned historical policy warnings remain recorded", a.HistoricalViolations)
		}
		if a.CrossingViolations > 0 {
			a.Reason += fmt.Sprintf("; %d independent crossing-time policy warnings remain recorded", a.CrossingViolations)
		}
	}()
	if a.Phase == "done" {
		a.Reason = "done remains recorded; " + RecoverySummary(a.RecoveredReview)
		if !a.AcceptanceReady {
			a.Reason += "; current acceptance remains independently unresolved: " + workflowItems(a.Outstanding)
		}
		return
	}
	for _, rider := range a.RecoveredReview.Tasks {
		task := t.Tasks[rider.ID]
		if task == nil || (task.Status != "in_review" && task.Status != "done") {
			if a.Phase != "done" {
				a.Phase, a.Actor, a.Action, a.Command = "blocked", "agent", "inspect_recovery", ""
				a.Reason = RecoverySummary(a.RecoveredReview) + "; frozen rider " + rider.ID + " no longer has its recorded submitted lifecycle; restore the historical binding before sync"
			}
			return
		}
		if task.Status == "in_review" {
			a.RecoveredCrossingsPending = true
		}
	}
	if a.Phase == "done" || a.Action == "inspect_recovery" {
		return
	}
	if a.RecoveredCrossingsPending {
		// Epic aggregation may have found unresolved acceptance after the first
		// feature's initially green advice. Keep all of it independently visible.
		a.Reason = RecoverySummary(a.RecoveredReview) + "; run ordinary sync on integration " + a.RecoveredReview.IntegrationBranch + ", then publish and read back state"
		if a.RecoveredReview.Basis == "contained" {
			a.Reason += "; containment is not an observed merge; the human fast-forward decision is required before crossing"
		}
		if !a.AcceptanceReady {
			a.Reason += "; current acceptance remains independently unresolved: " + workflowItems(a.Outstanding)
		}
		return
	}
	a.Phase, a.Actor, a.Action, a.Command = "sync", "agent", "sync", "tiller sync"
	a.Reason = RecoverySummary(a.RecoveredReview) + "; historical riders have crossed; run ordinary integration sync for remaining rollup, then publish and read back state"
	if a.AcceptanceReady {
		return
	}
	a.Reason = RecoverySummary(a.RecoveredReview) + "; historical riders have crossed; resolve independent current acceptance on integration before final rollup: " + workflowItems(a.Outstanding)
	// Prefer agent preparation to a human decision when both remain outstanding.
	selected := -1
	for i, work := range a.Outstanding {
		if selected < 0 {
			selected = i
		}
		if work.Actor == "agent" {
			selected = i
			break
		}
	}
	if selected < 0 {
		return
	}
	work := a.Outstanding[selected]
	featureID := work.NodeID
	if featureID == "" {
		featureID = a.FeatureID
	}
	a.FeatureID = featureID
	a.Phase, a.Actor, a.Action, a.Command = "checks", work.Actor, "check", "tiller check feature "+featureID
	switch work.Kind {
	case "contract":
		a.Phase, a.Action, a.Command = "contract", "repair_contract", ""
	case "configuration":
		a.Action, a.Command = "configure_gate", ""
	case "rubric":
		if work.Actor == "agent" {
			a.Phase, a.Action, a.Command = "rubric", "grade_rubric", "tiller eval feature "+featureID+" --emit-rubrics"
		}
	}
	if work.Actor == "human" {
		flag := "--criterion"
		if work.Kind == "gate" {
			flag = "--gate"
		}
		a.Phase, a.Action = "acceptance", "attest"
		a.Command = "tiller attest feature " + featureID + " " + flag + " " + work.ID + " --status <pass|fail>"
	}
}

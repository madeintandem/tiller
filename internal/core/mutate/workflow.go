package mutate

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/workflow"
)

// ObserveWorkflow reads each feature contract and basis once per snapshot. Task
// basis checks use task-owned pins rather than rereading their parent feature.
func ObserveWorkflow(t *store.Tree) workflow.Input {
	return observeWorkflow(t, nil, nil)
}

// ObserveWorkflowForNode scopes observations to the addressed node's resolved review.
// Its Contracts and ContractErrors also supply the caller's eval presentation;
// do not load the same feature.eval.md again to render that presentation.
func ObserveWorkflowForNode(t *store.Tree, node store.Node) workflow.Input {
	return ObserveWorkflowForNodes(t, []store.Node{node})
}

// ObserveWorkflowForNodes unions ObserveWorkflowForNode's scopes, so a list
// that classifies a few review owners reads only their features.
func ObserveWorkflowForNodes(t *store.Tree, nodes []store.Node) workflow.Input {
	features := map[string]bool{}
	tasks := map[string]bool{}
	for _, node := range nodes {
		addWorkflowScope(t, node, features, tasks)
	}
	return observeWorkflow(t, features, tasks)
}

func addWorkflowScope(t *store.Tree, node store.Node, features, tasks map[string]bool) {
	var feature *store.Feature
	var epic *store.Epic
	switch n := node.(type) {
	case *store.Feature:
		feature = n
	case *store.Task:
		feature = t.Features[n.FeatureID]
		tasks[n.ID] = true
	case *store.Epic:
		epic = n
	}
	if feature != nil {
		features[feature.ID] = true
		for _, parent := range t.Epics {
			if parent.Key != nil && *parent.Key == feature.EpicKey {
				if store.ResolvedReviewGrain(t.Manifest, parent, feature) == store.ReviewGrainEpic {
					epic = parent
				}
				break
			}
		}
	}
	if epic != nil && epic.Key != nil {
		for id, f := range t.Features {
			if f.EpicKey == *epic.Key && f.Status != "cancelled" {
				features[id] = true
			}
		}
	}
}

func observeWorkflow(t *store.Tree, features, tasks map[string]bool) workflow.Input {
	in := workflow.Input{
		Contracts: map[string]*store.EvalFile{}, ContractErrors: map[string]string{}, ContractMissing: map[string]bool{},
		Eligibility: map[string]string{}, SubmissionEligibility: map[string]string{},
		Assembly: map[string][]string{}, AssemblyErrors: map[string]string{},
		RecoveryErrors: map[string]string{},
	}
	revision := vcs.New(t.Root)
	recoveryObserved := map[string]bool{}
	for id, f := range t.Features {
		if features != nil && !features[id] {
			continue
		}
		path, ok := gen.EvalPath(t, f)
		if !ok {
			in.ContractErrors[id] = "contract path unavailable"
		} else if document, err := corpus.Read(context.Background(), t.Root, filepath.ToSlash(path)); err != nil {
			in.ContractErrors[id] = err.Error()
			in.ContractMissing[id] = errors.Is(err, fs.ErrNotExist)
		} else {
			contract, err := store.ParseEvalFile(document.Content)
			if err != nil {
				in.ContractErrors[id] = err.Error()
			} else {
				in.Contracts[id] = contract
			}
		}
		if err := BasisDrift(t, f, revision); err != nil {
			in.Eligibility[id] = err.Error()
		}
		if assumptions := UnreviewedAssumptions(t, f); len(assumptions) > 0 {
			in.SubmissionEligibility[id] = "answered assumption " + assumptions[0].QuestionID + " awaits human review"
		}
		var epic *store.Epic
		for _, parent := range t.Epics {
			if parent.Key != nil && *parent.Key == f.EpicKey {
				epic = parent
				break
			}
		}
		grain := store.ResolvedReviewGrain(t.Manifest, epic, f)
		if grain == store.ReviewGrainTask || f.Status == "done" || f.Status == "cancelled" {
			continue
		}
		branch := f.Branch
		if grain == store.ReviewGrainEpic && epic != nil {
			branch = epic.Branch
		}
		var recovered *store.RecoveredReview
		reviewID := f.ID
		if grain == store.ReviewGrainEpic && epic != nil {
			recovered, reviewID = epic.RecoveredReview, epic.ID
		} else {
			recovered = f.RecoveredReview
		}
		if recovered != nil {
			if !recoveryObserved[reviewID] {
				recoveryObserved[reviewID] = true
				into := ""
				if t.Manifest != nil {
					into = t.Manifest.Git.IntegrationBranch
				}
				if err := checkRecoveredEvidence(revision, t.Root, recovered, into); err != nil {
					in.RecoveryErrors[reviewID] = err.Error()
				}
			}
			// A recovered review is immutable; today's aggregate tip (or its
			// absence) must never select ordinary assembly or merge advice.
			continue
		}
		for _, task := range gen.OrderTasks(t, t.FeatureTasks[id], f.Order) {
			// A task started at task grain owns its review like a task PR
			// does; the aggregate never absorbs it (issue #146, R6).
			if task.Status != "in_review" || task.PR != nil || task.StartedGrain == store.ReviewGrainTask {
				continue
			}
			included, err := TaskIncludedInAggregate(revision, task, branch)
			if err != nil {
				in.AssemblyErrors[id] = "cannot inspect aggregate inclusion for " + task.ID + ": " + err.Error()
				break
			}
			if !included {
				in.Assembly[id] = append(in.Assembly[id], task.ID)
			}
		}
	}
	for id, task := range t.Tasks {
		if tasks != nil && !tasks[id] {
			continue
		}
		if err := TaskOwnBasisDrift(t, task, revision); err != nil {
			in.Eligibility[id] = err.Error()
		}
	}
	return in
}

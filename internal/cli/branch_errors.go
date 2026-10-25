package cli

import (
	"errors"
	"fmt"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// branchBlockedData keeps branch recovery machine-readable without conflating
// an incomplete aggregate with a failed Git command.
type branchBlockedData struct {
	Reason           string `json:"reason"`
	Noun             string `json:"noun"`
	ID               string `json:"id"`
	Branch           string `json:"branch,omitempty"`
	Conflict         string `json:"conflict,omitempty"`
	CurrentBranch    string `json:"current_branch,omitempty"`
	Worktree         string `json:"worktree,omitempty"`
	RefMissing       bool   `json:"ref_missing,omitempty"`
	Task             string `json:"task,omitempty"`
	TaskKey          string `json:"task_key,omitempty"`
	Dependency       string `json:"dependency,omitempty"`
	DependencyStatus string `json:"dependency_status,omitempty"`
	// AggregateUnrecorded marks a dependency refusal whose named aggregate
	// this start would have minted: nothing records it and no ref exists.
	AggregateUnrecorded bool `json:"aggregate_unrecorded,omitempty"`
	// DependencyStartedGrain is the grain the prerequisite started under. At
	// task grain it never enters the aggregate, so only its merge unblocks.
	DependencyStartedGrain string `json:"dependency_started_grain,omitempty"`
}

func branchBlockedView(tree *store.Tree, err error) (any, string, bool) {
	var branch *mutate.BranchSafetyError
	if errors.As(err, &branch) {
		data := branchBlockedData{Reason: branch.Reason, Noun: branch.Noun, ID: branch.NodeID, Branch: branch.Branch, Conflict: branch.Conflict, CurrentBranch: branch.CurrentBranch, Worktree: branch.Worktree, RefMissing: branch.RefMissing}
		return data, fmt.Sprintf("Blocked: %s %s: %s", branch.Noun, gen.RefByID(tree, branch.NodeID), branch.Error()), true
	}
	var aggregate *mutate.AggregateContentMissingError
	if errors.As(err, &aggregate) {
		data := branchBlockedData{Reason: "aggregate_content_missing", Noun: aggregate.Noun, ID: aggregate.NodeID, Branch: aggregate.Branch, Task: aggregate.TaskID, TaskKey: aggregate.TaskKey}
		return data, fmt.Sprintf("Blocked: %s %s on %s does not contain task %s's submitted head and current tip; commit task submission state, merge its branch into the aggregate, and retry.", aggregate.Noun, gen.RefByID(tree, aggregate.NodeID), aggregate.Branch, gen.RefByID(tree, aggregate.TaskID)), true
	}
	var dependency *mutate.TaskDependencyBlockedError
	if errors.As(err, &dependency) {
		data := branchBlockedData{Reason: "task_dependency_unfinished", Noun: store.NounTask, ID: dependency.TaskID, Branch: dependency.Branch, Dependency: dependency.DependencyID, DependencyStatus: dependency.DependencyStatus, AggregateUnrecorded: dependency.AggregateUnrecorded, DependencyStartedGrain: dependency.DependencyStartedGrain}
		// A task-grain prerequisite can never enter the aggregate, so this
		// outranks the adopt text even when the start also minted it.
		if dependency.DependencyStartedGrain == store.ReviewGrainTask {
			return data, fmt.Sprintf("Blocked: task %s requires %s, which started at task grain; wait for its PR to merge and sync before starting.", gen.RefByID(tree, dependency.TaskID), gen.RefByID(tree, dependency.DependencyID)), true
		}
		if dependency.AggregateUnrecorded {
			return data, fmt.Sprintf("Blocked: task %s requires %s; aggregate %s is not recorded and does not exist. Build it containing the prerequisite's complete branch, then start with --adopt-aggregate.", gen.RefByID(tree, dependency.TaskID), gen.RefByID(tree, dependency.DependencyID), dependency.Branch), true
		}
		if dependency.Branch == "" {
			return data, fmt.Sprintf("Blocked: task %s requires %s; wait for its integration before starting.", gen.RefByID(tree, dependency.TaskID), gen.RefByID(tree, dependency.DependencyID)), true
		}
		return data, fmt.Sprintf("Blocked: task %s requires %s; wait for its integration, or at coarse grain assemble the submitted prerequisite's complete branch into %s before starting.", gen.RefByID(tree, dependency.TaskID), gen.RefByID(tree, dependency.DependencyID), dependency.Branch), true
	}
	return nil, "", false
}

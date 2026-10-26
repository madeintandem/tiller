package mutate

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// checkWorkItem is checkTask for a shelf node: the task-grade command gates
// recorded onto the node's own state file, in_progress -> check on green.
// There is no eval file to scope (a shelf node carries no contract), and the
// only guard is the node's own pinned contract.
func checkWorkItem(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, treeNode store.WorkNode, in CheckInput) (CheckResult, error) {
	noun := treeNode.Noun()
	target, ymlRel, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return CheckResult{}, err
	}
	w := target.Body()
	before := w.Status

	if legal, _ := lifecycle.Legal(target, "check"); !legal {
		return CheckResult{}, &lifecycle.IllegalTransitionError{Noun: noun, From: before, Verb: "check", Legal: lifecycle.NextMoves(target)}
	}
	if err := checkWorkBasis(tree.Root, target, rev); err != nil {
		return CheckResult{}, err
	}
	against, err := gateAgainst(rev)
	if err != nil {
		return CheckResult{}, err
	}
	run, err := newGateRun(rev, against, owner, deps, in, &w.GatesBlock)
	if err != nil {
		return CheckResult{}, err
	}
	block, outcomes, passed, err := evaluateGates(tree, noun, w.ID, nil, &w.GatesBlock, run)
	if err != nil {
		return CheckResult{}, err
	}
	mergeGates(&w.GatesBlock, block, tree.Manifest, noun)

	after := before
	if passed {
		if err := lifecycle.Apply(target, "check", lifecycle.Payload{}); err != nil {
			return CheckResult{}, err
		}
		after = w.Status
	}
	ydata, err := yaml.Marshal(target)
	if err != nil {
		return CheckResult{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "check", ReusedGates: reusedGates(outcomes, block, run.at)}, noun, w.ID, ymlRel, ydata, snapshot, before, after, ""); err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		Noun: noun, ID: w.ID, Key: deref(w.Key), Status: after, Gates: outcomes, Passed: passed, Changed: true,
		Warnings: shelfWarnings(tree.Manifest, noun),
	}, nil
}

// shelfWarnings is an advisory: a
// bug, chore, or tweak runs the gates whose on: list names task (gateNoun),
// so a manifest that scopes every command gate elsewhere silently leaves shelf
// work with no build or test gate at all.
func shelfWarnings(m *store.Manifest, noun string) []string {
	for _, c := range requiredGateConfigs(m, noun) {
		if c.Kind == store.GateKindCommand && strings.TrimSpace(c.Run) != "" {
			return nil
		}
	}
	return []string{fmt.Sprintf("no build or test gate applies to this %s: bugs, chores, and tweaks run the gates whose on: list names task in .tiller/tiller.yml, and none does", noun)}
}

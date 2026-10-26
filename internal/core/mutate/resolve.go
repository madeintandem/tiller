package mutate

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// ResolveInput is `resolve <bug|chore> <node> --by <target>` or `--clear`.
type ResolveInput struct {
	Noun  string
	Node  string
	By    string
	Clear bool
}

// ResolveResult reports the carrier now on record and what it replaced.
type ResolveResult struct {
	Noun       string
	ID         string
	Key        string
	ResolvedBy string
	Before     string
	Changed    bool
}

// WorkTerminalError refuses resolve on a done or cancelled bug or chore:
// there is nothing left to carry. Exit 1 through emitReport.
type WorkTerminalError struct {
	Noun   string
	ID     string
	Key    string
	Status string
	Reason string // "work_terminal"
}

func (e *WorkTerminalError) Error() string {
	return fmt.Sprintf("mutate: %s %s is %s; nothing is left to resolve", e.Noun, e.ID, e.Status)
}

// Resolve sets or clears the carrier pointer (docs/spec/cli-surface.md). It
// moves no status: the node keeps proposed or ready, takes the derived carried
// phase, and sync crosses it when the carrier's done lands. The carrier is a
// live epic, feature, or task; a cancelled or archived one refuses, since its
// merge will never come. It sits in the approve delegation class: human, or
// agent on an explicit instruction naming the node and the carrier.
func Resolve(tree *store.Tree, deps Deps, in ResolveInput) (ResolveResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ResolveResult{}, err
	}
	if !store.IsWorkNoun(in.Noun) {
		return ResolveResult{}, &usageError{reason: "unsupported_noun", msg: fmt.Sprintf("resolve applies to a shelf work node, not a %s", in.Noun)}
	}
	if !in.Clear && in.By == "" {
		return ResolveResult{}, &usageError{reason: "resolve_by_required", msg: "resolve needs --by <epic|feature|task> or --clear"}
	}
	if in.Clear && in.By != "" {
		return ResolveResult{}, &usageError{reason: "bad_args", msg: "resolve takes --by or --clear, never both"}
	}
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return ResolveResult{}, err
	}
	treeNode, _ := store.AsWorkNode(n)
	if s := treeNode.NodeStatus(); s == statusDone || s == statusCancelled {
		return ResolveResult{}, &WorkTerminalError{Noun: in.Noun, ID: treeNode.NodeID(), Key: deref(treeNode.NodeKey()), Status: s, Reason: "work_terminal"}
	}
	carrier := ""
	if !in.Clear {
		id, err := resolveAttachTarget(tree, in.By)
		if err != nil {
			var misuse *usageError
			if errors.As(err, &misuse) && misuse.reason == "attach_target" {
				return ResolveResult{}, &usageError{reason: "carrier_target", msg: fmt.Sprintf("carrier %q is not an epic, feature, or task", in.By)}
			}
			return ResolveResult{}, err
		}
		if state := carrierState(tree, id); state == statusCancelled || state == statusArchived {
			return ResolveResult{}, &usageError{reason: "carrier_retired", msg: fmt.Sprintf("carrier %s is %s; its merge will never come, so it cannot carry %s", id, state, treeNode.NodeID())}
		}
		carrier = id
	}
	doc, relYml, snapshot, err := loadWorkItemFile(tree, treeNode)
	if err != nil {
		return ResolveResult{}, err
	}
	w := doc.Body()
	res := ResolveResult{Noun: in.Noun, ID: w.ID, Key: deref(w.Key), ResolvedBy: carrier, Before: w.ResolvedBy}
	if w.ResolvedBy == carrier {
		return res, nil
	}
	w.ResolvedBy = carrier
	data, err := yaml.Marshal(doc)
	if err != nil {
		return ResolveResult{}, err
	}
	command := fmt.Sprintf("resolve %s %s --by %s", in.Noun, deref(w.Key), in.By)
	if in.Clear {
		command = fmt.Sprintf("resolve %s %s --clear", in.Noun, deref(w.Key))
	}
	ev := events.NewEvent(deps.clock(), workItemEventType("resolve", in.Noun), owner.ID, command, w.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"resolved_by": res.Before}
	ev.After = map[string]any{"resolved_by": carrier}
	ev.Files = []events.FileChange{{Path: relYml}}
	if err := commitMutation(tree, "resolve "+in.Noun, []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, data),
	}, []events.Event{ev}); err != nil {
		return ResolveResult{}, err
	}
	res.Changed = true

	return res, nil
}

// carrierCanCarry reports whether a recorded resolved_by still names work
// whose merge can arrive. It is the mutation-side twin of gen.WorkActivity's
// own recompute, and the two answer alike on purpose: what the reader is told
// (carried, or carrier_cancelled) and what sync does about it are one rule.
// An empty state means the id resolves to no epic, feature, or task at all,
// which cannot carry anything either.
func carrierCanCarry(state string) bool {
	return state != "" && state != statusCancelled && state != statusArchived
}

// workNodeCarried reports whether a shelf node is riding a carrier right now:
// resolved_by is set AND names work whose merge can still come. Every part of
// sync's shelf pass asks this one question, because "is it carried" decides
// both what crosses it and whether it has a branch of its own to be observed
// or asserted about, and a node whose carrier was cancelled after the fact has
// to answer the same way everywhere or it strands.
func workNodeCarried(tree *store.Tree, w store.WorkNode) bool {
	id := w.Body().ResolvedBy
	return id != "" && carrierCanCarry(carrierState(tree, id))
}

// carrierState reads a tree node's stored status by id, "" when unknown.
func carrierState(tree *store.Tree, id string) string {
	if e, ok := tree.Epics[id]; ok {
		return e.Status
	}
	if f, ok := tree.Features[id]; ok {
		return f.Status
	}
	if tk, ok := tree.Tasks[id]; ok {
		return tk.Status
	}
	return ""
}

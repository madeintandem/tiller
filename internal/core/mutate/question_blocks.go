package mutate

// Link and unlink on an open question: widen or narrow what it blocks without
// touching its prose. Only an open question's blocks can change; an answered or
// cancelled question's blocks is history.

import (
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	eventLinkQuestion   = "link.question"
	eventUnlinkQuestion = "unlink.question"
)

// QuestionBlocksInput names an open question and the blocks targets to add
// (link) or remove (unlink), each a node id or key.
type QuestionBlocksInput struct {
	Node   string
	Blocks []string
}

// QuestionBlocksResult reports the question's blocks after the call and which
// targets were added or removed. Targets is the request resolved to node ids,
// so a no-change unlink can name what was asked for. Changed is false when
// nothing moved.
type QuestionBlocksResult struct {
	ID      string
	Title   string
	Blocks  []string
	Targets []string
	Added   []string
	Removed []string
	Changed bool
}

// LinkQuestionBlocks unions the targets into an open question's blocks, in
// order, skipping those already present.
func LinkQuestionBlocks(tree *store.Tree, deps Deps, in QuestionBlocksInput) (QuestionBlocksResult, error) {
	return changeQuestionBlocks(tree, deps, in, "link", eventLinkQuestion, func(before, targets []string) ([]string, []string) {
		after := append([]string(nil), before...)
		added := unionInto(&after, targets)
		return after, added
	})
}

// UnlinkQuestionBlocks removes the targets from an open question's blocks,
// preserving the order of the rest. Removing every target is legal.
func UnlinkQuestionBlocks(tree *store.Tree, deps Deps, in QuestionBlocksInput) (QuestionBlocksResult, error) {
	return changeQuestionBlocks(tree, deps, in, "unlink", eventUnlinkQuestion, func(before, targets []string) ([]string, []string) {
		drop := make(map[string]bool, len(targets))
		for _, id := range targets {
			drop[id] = true
		}
		after := make([]string, 0, len(before))
		var removed []string
		for _, id := range before {
			if drop[id] {
				removed = append(removed, id)
				continue
			}
			after = append(after, id)
		}
		return after, removed
	})
}

// changeQuestionBlocks resolves the question and every target, checks the
// question is open, re-reads the ledger, applies change, and commits. A call
// that changes nothing writes nothing and records no event.
func changeQuestionBlocks(tree *store.Tree, deps Deps, in QuestionBlocksInput, verb, kind string, change func(before, targets []string) (after, changed []string)) (QuestionBlocksResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return QuestionBlocksResult{}, err
	}
	if len(in.Blocks) == 0 {
		return QuestionBlocksResult{}, &usageError{reason: "blocks_required", msg: fmt.Sprintf("%s question needs at least one --blocks target", verb)}
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounQuestion, Raw: in.Node})
	if err != nil {
		return QuestionBlocksResult{}, err
	}
	q := node.(*store.Question)
	if q.Status != statusOpen && q.Status != "" {
		return QuestionBlocksResult{}, &QuestionClosedError{QuestionID: q.ID, Status: q.Status, Verb: verb}
	}
	targets, err := resolveBlockTargets(tree, in.Blocks)
	if err != nil {
		return QuestionBlocksResult{}, err
	}

	path := filepath.Join(tree.Root, filepath.FromSlash(questionsRelPath))
	var qf store.QuestionsFile
	snapshot, err := readYAMLSnapshot(tree.Root, path, &qf)
	if err != nil {
		return QuestionBlocksResult{}, err
	}
	var entry *store.Question
	for _, e := range qf.Questions {
		if e.ID == q.ID {
			entry = e
			break
		}
	}
	if entry == nil {
		return QuestionBlocksResult{}, fmt.Errorf("mutate: question %s resolved but is missing from %s", q.ID, questionsRelPath)
	}

	// Re-check against the entry just read: the tree above may be stale.
	if entry.Status != statusOpen && entry.Status != "" {
		return QuestionBlocksResult{}, &QuestionClosedError{QuestionID: q.ID, Status: entry.Status, Verb: verb}
	}

	before := append([]string(nil), entry.Blocks...)
	after, changed := change(before, targets)
	res := QuestionBlocksResult{ID: q.ID, Title: gen.QuestionTitle(entry), Blocks: after, Targets: targets}
	if verb == "link" {
		res.Added = changed
	} else {
		res.Removed = changed
	}
	if len(changed) == 0 {
		res.Blocks = before
		return res, nil
	}
	entry.Blocks = after

	data, err := yaml.Marshal(qf)
	if err != nil {
		return QuestionBlocksResult{}, err
	}
	key := "added"
	if verb == "unlink" {
		key = "removed"
	}
	ev := events.NewEvent(deps.clock(), kind, owner.ID, verb+" question "+q.ID, q.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"blocks": before}
	ev.After = map[string]any{"blocks": after, key: changed}
	ev.Files = []events.FileChange{{Path: questionsRelPath}}

	if err := commitMutation(tree, verb+" question", []store.PlannedWrite{
		store.PlanWrite(path, snapshot, data),
	}, []events.Event{ev}); err != nil {
		return QuestionBlocksResult{}, err
	}
	res.Changed = true
	return res, nil
}

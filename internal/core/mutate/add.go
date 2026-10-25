// Package mutate holds the state-changing core operations the CLI drives. It
// reasons about nothing and never writes to stdout: it validates a request
// against the loaded tree, mints ids, writes the node files atomically, and
// records an event. The CLI layer translates argv into these calls and renders
// their Result.
//
// The add verbs lay nodes down in the work tree (cli-surface.md,
// "Decomposition"): an epic or a feature in `draft` with `source` provenance,
// or a `proposed` task in its own folder under a feature. Re-adding the same
// node is an idempotent no-op success, matching the mutation contract in
// cli-surface.md ("Output and exit codes").
package mutate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// Node statuses the add verbs set (artifact-schema.md: Epics, Features, Tasks).
const (
	statusDraft    = "draft"
	statusProposed = "proposed"
)

// changeTypeDefault is the only valid change_type in greenfield and the default
// the add-feature verb assumes when none is given (artifact-schema.md, Project
// archetypes).
const changeTypeDefault = "create"

// Event type discriminators, in the dotted style the event stream already uses.
const (
	eventAddEpic    = "add.epic"
	eventAddFeature = "add.feature"
	eventAddTask    = "add.task"
)

// Deps are the injectable dependencies of a mutation: the acting contributor
// resolved from --contributor or TILLER_CONTRIBUTOR and validated here against the
// manifest, the clock that stamps events, the id minter, and the read-only git
// window the active-basis comparison and basis recording read through.
// Production passes the bare Contributor and lets Clock, Mint, and Rev default to the
// system clock, id.Mint, and a vcs.Git rooted at the repo; tests inject a fixed
// clock, a deterministic minter, and (where they isolate the git seam) a stub
// Revisions so output and state are golden-stable.
type Deps struct {
	Contributor string
	Clock       events.Clock
	Mint        func(prefix string) string
	Rev         vcs.Revisions
}

// mintAttempts bounds the retry in mintIn. Sixteen is far past any real
// collision run (six base36 characters is 2,176,782,336 tokens per prefix), so
// exhausting it means the minter is not random rather than that the tree is
// full: tests inject deterministic minters, and an unbounded loop would hang
// the suite instead of failing it.
const mintAttempts = 16

// mintIn returns a new id of the given prefix that the loaded tree does not
// already carry. This is the first of the schema's two layers against a
// duplicate id (artifact-schema.md, IDs): minting against the tree drives the
// intra-tree collision probability to zero, which leaves the load-time
// refusal covering the case a single-tree retry provably cannot see, two
// branches minting the same token independently and meeting at a merge.
//
// It is worth the tree read because ids are immutable and no verb regenerates
// one: a retry here is invisible, while a collision that reaches disk is a
// hand-edit of every depends_on, question block, and event that already names
// the id.
func (d Deps) mintIn(tree *store.Tree, prefix string) (string, error) {
	for i := 0; i < mintAttempts; i++ {
		candidate := d.mintRaw(prefix)
		if !treeHoldsID(tree, candidate) {
			return candidate, nil
		}
	}
	// Every attempt collided, which a random minter cannot do: the odds are
	// (n/2176782336)^16. So this is a broken or exhausted injected minter, and
	// erroring names that. Returning the last candidate instead would write a
	// duplicate that only surfaces on the NEXT load, turning a caught fault
	// into a tree someone else has to repair by hand.
	return "", &usageError{
		reason: "mint_exhausted",
		msg:    fmt.Sprintf("could not mint a free %s- id in %d attempts", prefix, mintAttempts),
	}
}

// mintRaw draws one id from the minter with no tree check: the id.Mint default
// in production, or a test's injected Deps.Mint. mintIn loops this to retry
// past a collision; the EV and AT call sites use it directly, because those
// two id classes are not tree-keyed (see the comments at those call sites).
func (d Deps) mintRaw(prefix string) string {
	if d.Mint != nil {
		return d.Mint(prefix)
	}
	return id.Mint(prefix)
}

// treeHoldsID reports whether any of the tree's id-keyed node maps already
// carries the candidate: epics, features, tasks, questions, amendments, and
// the two shelf nouns (bugs and chores), the grains addressable by id
// (artifact-schema.md, IDs). FeatureTasks is not checked because it is a
// second index over the same *Task values already in Tasks, keyed by feature
// id rather than node id, so it names nothing this lookup would miss.
func treeHoldsID(tree *store.Tree, candidate string) bool {
	if tree == nil {
		return false
	}
	if _, ok := tree.Epics[candidate]; ok {
		return true
	}
	if _, ok := tree.Features[candidate]; ok {
		return true
	}
	if _, ok := tree.Tasks[candidate]; ok {
		return true
	}
	if _, ok := tree.Questions[candidate]; ok {
		return true
	}
	if _, ok := tree.Amendments[candidate]; ok {
		return true
	}
	if _, ok := tree.WorkNode(candidate); ok {
		return true
	}
	return false
}

func (d Deps) clock() events.Clock {
	if d.Clock != nil {
		return d.Clock
	}
	return events.SystemClock{}
}

// rev returns the read-only git window the mutation reads revisions through. A
// caller that injected one (a test stub) gets it back; otherwise a real
// vcs.Git rooted at the repo is built on demand. This is the single seam the
// active-basis guard and approve --refresh-basis read git through outside the
// Check path, which threads its own Revisions explicitly.
func (d Deps) rev(root string) vcs.Revisions {
	if d.Rev != nil {
		return d.Rev
	}
	return vcs.New(root)
}

// EpicInput is the request for AddEpic: the new epic's single-segment key, an
// optional title, the source doc the decomposition drew it from, and the work
// it depends on (ids or keys, resolved to ids at add time).
type EpicInput struct {
	Key       string
	Title     string
	Source    string
	DependsOn []string
}

// FeatureInput is the request for AddFeature: the canonical
// <epic-key>/<feature-key>, an optional title, the change_type (defaulting to
// create), the source provenance, and the work it depends on (ids or keys,
// resolved to ids at add time).
type FeatureInput struct {
	Key        string
	Title      string
	ChangeType string
	Source     string
	DependsOn  []string
}

// TaskInput is the request for AddTask: the task's canonical three-segment key,
// <epic-key>/<feature-key>/<task-key>, its title, and the sibling tasks it
// follows (ids or canonical keys, resolved to ids at add time). A task's
// depends_on names sibling tasks and nothing else (artifact-schema.md, Tasks).
type TaskInput struct {
	Key       string
	Title     string
	DependsOn []string
}

// Result describes the outcome of an add. It carries the resulting node state so
// the CLI can print it back to the agent, and Created reports whether this call
// actually wrote the node (false on an idempotent re-add). Feature is set for a
// task; ChangeType and Source are empty where they do not apply.
type Result struct {
	Noun       string
	ID         string
	Key        string
	Title      string
	Status     string
	ChangeType string
	Source     string
	Feature    string
	AttachedTo string
	DependsOn  []string
	Path       string
	Created    bool
}

// AddEpic creates an epic folder with epic.md and epic.yml in draft, carrying
// the source provenance, mints its E- id, and records an event. Re-adding the
// same key is an idempotent no-op success that returns the existing node.
func AddEpic(tree *store.Tree, deps Deps, in EpicInput) (Result, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return Result{}, err
	}
	plan, err := planAddEpic(tree, deps, owner, in)
	if err != nil {
		return Result{}, err
	}
	if !plan.result.Created {
		return plan.result, nil
	}
	if err := commitMutation(tree, "add epic", plan.writes(), []events.Event{plan.event}); err != nil {
		return Result{}, err
	}
	return plan.result, nil
}

// planAddEpic is AddEpic's whole judgment, up to but not including the
// commit: the dependency targets, the idempotent re-add arm, the minted id,
// and the two writes plus the event a fresh epic lands with. It is split out
// so one caller can commit it alone (AddEpic) and another can fold it into a
// larger batch (plannedAdd's own comment): the amendment's approve
// materializes a staged add-epic op the same way it materializes add-feature
// and add-task, through this function rather than a second add path.
func planAddEpic(tree *store.Tree, deps Deps, owner store.Contributor, in EpicInput) (plannedAdd, error) {
	if store.IsReservedEpicKey(in.Key) {
		return plannedAdd{}, &usageError{reason: "reserved_key", msg: fmt.Sprintf("%q is a shelf folder name (.tiller/%s/) and cannot be an epic key", in.Key, in.Key)}
	}
	relDir := filepath.Join(".tiller", "epics", in.Key)
	relMd := filepath.Join(relDir, "epic.md")
	relYml := filepath.Join(relDir, "epic.yml")
	mdPath := filepath.Join(tree.Root, relMd)
	ymlPath := filepath.Join(tree.Root, relYml)

	// Targets resolve before the existence check, so an unresolvable one is
	// refused whether the node is new or already there. A re-add that named a
	// bad target and succeeded would report a graph the caller never got.
	dependsOn, err := resolveDependsOn(tree, in.DependsOn, "", dependsScope{})
	if err != nil {
		return plannedAdd{}, err
	}

	if fileExists(tree.Root, ymlPath) {
		var doc store.Epic
		if err := readYAML(tree.Root, relYml, &doc); err != nil {
			return plannedAdd{}, err
		}
		if err := refuseUnrecordedOnReAdd(tree, store.NounEpic, in.Key, doc.DependsOn, dependsOn); err != nil {
			return plannedAdd{}, err
		}
		return plannedAdd{result: Result{
			Noun:      store.NounEpic,
			ID:        doc.ID,
			Key:       deref(doc.Key),
			Title:     doc.Title,
			Status:    doc.Status,
			Source:    doc.Source,
			DependsOn: doc.DependsOn,
			Path:      relDir,
			Created:   false,
		}}, nil
	}

	eid, err := deps.mintIn(tree, "E")
	if err != nil {
		return plannedAdd{}, err
	}
	key := in.Key
	doc := store.Epic{
		ID:        eid,
		Key:       &key,
		Title:     in.Title,
		Status:    statusDraft,
		Source:    in.Source,
		DependsOn: dependsOn,
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return plannedAdd{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAddEpic, owner.ID, "add epic "+in.Key, eid)
	// EV ids are appended to the event ledger, not keyed into the tree's id
	// maps, so there is nothing to retry against (artifact-schema.md, The eval
	// model: the AT- and EV- classes are separate non-node identifiers).
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": statusDraft, "key": in.Key}
	ev.Files = []events.FileChange{{Path: relMd}, {Path: relYml}}

	return plannedAdd{
		result: Result{
			Noun:      store.NounEpic,
			ID:        eid,
			Key:       in.Key,
			Title:     in.Title,
			Status:    statusDraft,
			Source:    in.Source,
			DependsOn: dependsOn,
			Path:      relDir,
			Created:   true,
		},
		prose:  store.PlanWrite(mdPath, store.AbsentSnapshot(), []byte(renderEpicMD(heading(in.Title, in.Key)))),
		state:  store.PlanWrite(ymlPath, store.AbsentSnapshot(), ydata),
		event:  ev,
		doc:    &doc,
		ymlRel: relYml,
	}, nil
}

// plannedAdd is one add verb's work, planned but not yet committed: the Result
// its caller reports, the two planned file writes, the event, and the yml's own
// document and repo-relative path. It exists so the amendment's approve can
// materialize a staged add-epic, add-feature, or add-task inside its own
// single batch: AddEpic, AddFeature, and AddTask commit the writes as they
// stand, while approve keeps the DOCUMENT instead, because a later operation
// in the same batch may write onto the node this one materializes (a
// supersedes link on a staged replacement) and store.Tree may only be handed
// one planned write per path.
//
// An idempotent re-add carries result.Created false, no writes, and no event:
// the add verbs return it as the no-op success the mutation contract promises,
// and approve refuses on it, since an amendment that reports an add it did not
// perform is a batch lying about what it applied.
type plannedAdd struct {
	result Result
	prose  store.PlannedWrite
	state  store.PlannedWrite
	event  events.Event
	doc    store.Node
	ymlRel string
}

// writes is the pair a fresh node lands with, prose first: the human-owned .md
// and the machine-owned .yml, the split every node keeps.
func (p plannedAdd) writes() []store.PlannedWrite {
	return []store.PlannedWrite{p.prose, p.state}
}

// AddFeature creates a feature folder nested under its epic, with feature.md and
// feature.yml in draft carrying the source provenance, mints its F- id, and
// records an event. The parent epic must exist; an unknown epic returns
// resolve.ErrNotFound (exit 2). Re-adding the same canonical key is an
// idempotent no-op success. The two-file write runs under the repository
// mutation lock (cli-surface.md, Working-tree posture).
func AddFeature(tree *store.Tree, deps Deps, in FeatureInput) (Result, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return Result{}, err
	}
	plan, err := planAddFeature(tree, deps, owner, in, nil)
	if err != nil {
		return Result{}, err
	}
	if !plan.result.Created {
		return plan.result, nil
	}
	if err := commitMutation(tree, "add feature", plan.writes(), []events.Event{plan.event}); err != nil {
		return Result{}, err
	}
	return plan.result, nil
}

// planAddFeature is AddFeature's whole judgment, up to but not including the
// commit: key shape, parent epic, dependency targets, the idempotent re-add
// arm, the minted id, and the two writes plus the event a fresh feature lands
// with. It is split out so one caller can commit it alone and another can fold
// it into a larger batch (plannedAdd's own comment).
//
// stagedParent is the materialize-side escape from the live-tree-only epic
// check below: nil from AddFeature's own direct caller, since a hand `tiller
// add feature` only ever sees the live tree, and a lookup over the
// approving batch's own staged nodes from amendment_approve.go's
// materialize, where an add-epic staged EARLIER in the same amendment has
// already materialized into the batch by the time a later add-feature's
// turn comes (materialize's own strict staged-order loop). It is consulted
// only once the live resolver has already answered *resolve.ErrNotFound,
// never in place of that lookup, and it never mutates tree.Epics: the
// staging-side twin of this escape is resolveStageParentEpic
// (amendment.go).
func planAddFeature(tree *store.Tree, deps Deps, owner store.Contributor, in FeatureInput, stagedParent func(noun, key string) bool) (plannedAdd, error) {
	epicKey, featureKey, err := splitFeatureKey(in.Key)
	if err != nil {
		return plannedAdd{}, err
	}

	// The parent epic must exist: live in the tree, or, only when the caller
	// passed stagedParent, an add-epic staged earlier in the same batch.
	// Reuse the resolver so an unknown epic is the same typed not-found the
	// CLI maps to exit 2 whichever way the escape falls. The staged fallback
	// triggers only on *resolve.ErrNotFound, mirroring resolveStageParentEpic's
	// own dispatch (amendment.go) exactly: any other resolver error (an
	// id-shaped key that resolves to the wrong noun, an ambiguous key)
	// propagates unrescued, the same as staging already refuses it unrescued.
	if _, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: store.NounEpic, Raw: epicKey}); err != nil {
		var notFound *resolve.ErrNotFound
		if !errors.As(err, &notFound) || stagedParent == nil || !stagedParent(store.NounEpic, epicKey) {
			return plannedAdd{}, err
		}
	}

	relDir := filepath.Join(".tiller", "epics", epicKey, "features", featureKey)
	relMd := filepath.Join(relDir, "feature.md")
	relYml := filepath.Join(relDir, "feature.yml")
	mdPath := filepath.Join(tree.Root, relMd)
	ymlPath := filepath.Join(tree.Root, relYml)

	dependsOn, err := resolveDependsOn(tree, in.DependsOn, "", dependsScope{})
	if err != nil {
		return plannedAdd{}, err
	}

	if fileExists(tree.Root, ymlPath) {
		var doc store.Feature
		if err := readYAML(tree.Root, relYml, &doc); err != nil {
			return plannedAdd{}, err
		}
		if err := refuseUnrecordedOnReAdd(tree, store.NounFeature, in.Key, doc.DependsOn, dependsOn); err != nil {
			return plannedAdd{}, err
		}
		return plannedAdd{result: Result{
			Noun:       store.NounFeature,
			ID:         doc.ID,
			Key:        deref(doc.Key),
			Title:      doc.Title,
			Status:     doc.Status,
			ChangeType: doc.ChangeType,
			Source:     doc.Source,
			DependsOn:  doc.DependsOn,
			Path:       relDir,
			Created:    false,
		}}, nil
	}

	changeType := in.ChangeType
	if changeType == "" {
		changeType = changeTypeDefault
	}

	fid, err := deps.mintIn(tree, "F")
	if err != nil {
		return plannedAdd{}, err
	}
	canonical := in.Key
	// Ancestors describe scope. Attribution remains on the creation event.
	doc := store.Feature{
		ID:         fid,
		Key:        &canonical,
		Title:      in.Title,
		Status:     statusDraft,
		ChangeType: changeType,
		Source:     in.Source,
		DependsOn:  dependsOn,
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return plannedAdd{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAddFeature, owner.ID, "add feature "+in.Key, fid)
	// EV: same non-retry rationale as AddEpic above; not tree-keyed.
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": statusDraft, "key": in.Key}
	ev.Files = []events.FileChange{{Path: relMd}, {Path: relYml}}

	return plannedAdd{
		result: Result{
			Noun:       store.NounFeature,
			ID:         fid,
			Key:        in.Key,
			Title:      in.Title,
			Status:     statusDraft,
			ChangeType: changeType,
			Source:     in.Source,
			DependsOn:  dependsOn,
			Path:       relDir,
			Created:    true,
		},
		prose:  store.PlanWrite(mdPath, store.AbsentSnapshot(), []byte(renderFeatureMD(heading(in.Title, featureKey)))),
		state:  store.PlanWrite(ymlPath, store.AbsentSnapshot(), ydata),
		event:  ev,
		doc:    &doc,
		ymlRel: relYml,
	}, nil
}

// AddTask creates a task folder under its feature, with task.md and task.yml in
// proposed, mints its T- id, and records an event. The parent feature is
// addressed by the first two segments of the task's canonical key; an unknown or
// mismatched address returns the resolver's typed error. Any --depends-on
// targets resolve against that feature's own tasks, the scope a task's edges
// live in (artifact-schema.md, Tasks: Ordering), and record the sequence the
// sibling sort then ranks on. Re-adding the same canonical key is an idempotent
// no-op success returning the existing task. The two-file write runs under the
// repository mutation lock (cli-surface.md, Working-tree posture).
func AddTask(tree *store.Tree, deps Deps, in TaskInput) (Result, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return Result{}, err
	}
	plan, err := planAddTask(tree, deps, owner, in)
	if err != nil {
		return Result{}, err
	}
	if !plan.result.Created {
		return plan.result, nil
	}
	if err := commitMutation(tree, "add task", plan.writes(), []events.Event{plan.event}); err != nil {
		return Result{}, err
	}
	return plan.result, nil
}

// planAddTask is AddTask's whole judgment up to the commit, split out for the
// same reason planAddFeature is (plannedAdd's own comment).
func planAddTask(tree *store.Tree, deps Deps, owner store.Contributor, in TaskInput) (plannedAdd, error) {
	epicKey, featureKey, taskKey, err := splitTaskKey(in.Key)
	if err != nil {
		return plannedAdd{}, err
	}

	// The parent feature must exist; reuse the resolver so an unknown feature is
	// the same typed not-found the CLI maps to exit 2.
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{
		Noun: store.NounFeature, Raw: epicKey + "/" + featureKey,
	})
	if err != nil {
		return plannedAdd{}, err
	}
	feat, ok := node.(*store.Feature)
	if !ok || feat.Key == nil {
		return plannedAdd{}, fmt.Errorf("mutate: resolved node %q is not an addressable feature", in.Key)
	}

	relDir := store.TaskRelDir(feat.EpicKey, localKey(*feat.Key), taskKey)
	relMd := filepath.Join(relDir, store.TaskProseFile)
	relYml := filepath.Join(relDir, store.TaskStateFile)
	mdPath := filepath.Join(tree.Root, relMd)
	ymlPath := filepath.Join(tree.Root, relYml)

	// A task's targets are its siblings, so resolution is scoped to the parent
	// feature (artifact-schema.md, Tasks). It runs before anything is written,
	// so a refused target leaves no folder behind.
	dependsOn, err := resolveDependsOn(tree, in.DependsOn, "", dependsScope{Sibling: true, Feature: feat.ID})
	if err != nil {
		return plannedAdd{}, err
	}

	if fileExists(tree.Root, ymlPath) {
		var doc store.Task
		if err := readYAML(tree.Root, relYml, &doc); err != nil {
			return plannedAdd{}, err
		}
		if err := refuseUnrecordedOnReAdd(tree, store.NounTask, in.Key, doc.DependsOn, dependsOn); err != nil {
			return plannedAdd{}, err
		}
		return plannedAdd{result: Result{
			Noun:      store.NounTask,
			ID:        doc.ID,
			Key:       deref(doc.Key),
			Title:     doc.Title,
			Status:    doc.Status,
			Feature:   feat.ID,
			DependsOn: doc.DependsOn,
			Path:      relDir,
			Created:   false,
		}}, nil
	}

	tid, err := deps.mintIn(tree, "T")
	if err != nil {
		return plannedAdd{}, err
	}
	canonical := in.Key
	// Creation attributes the proposal but does not reserve execution.
	doc := store.Task{
		ID:         tid,
		Key:        &canonical,
		Title:      in.Title,
		Status:     statusProposed,
		ProposedBy: owner.ID,
		DependsOn:  dependsOn,
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return plannedAdd{}, err
	}

	ev := events.NewEvent(deps.clock(), eventAddTask, owner.ID, "add task "+in.Key, tid)
	// EV: same non-retry rationale as AddEpic above; not tree-keyed.
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"status": statusProposed, "key": in.Key, "feature": feat.ID}
	ev.Files = []events.FileChange{{Path: relMd}, {Path: relYml}}

	return plannedAdd{
		result: Result{
			Noun:      store.NounTask,
			ID:        tid,
			Key:       in.Key,
			Title:     in.Title,
			Status:    statusProposed,
			Feature:   feat.ID,
			DependsOn: dependsOn,
			Path:      relDir,
			Created:   true,
		},
		prose:  store.PlanWrite(mdPath, store.AbsentSnapshot(), []byte(renderTaskMD(heading(in.Title, taskKey)))),
		state:  store.PlanWrite(ymlPath, store.AbsentSnapshot(), ydata),
		event:  ev,
		doc:    &doc,
		ymlRel: relYml,
	}, nil
}

// refuseUnrecordedOnReAdd guards the idempotent re-add arm of the three add
// verbs. Re-adding a node is a no-op success (cli-surface.md, "Output and exit
// codes"), so this arm must not write, and unioning the targets in would make
// the second call a mutation rather than the no-op the grammar promises.
// Silently dropping them is worse than either, though: the caller is handed a
// success and a depends_on that does not contain what they just named, and a
// target that would have exited 2 on a fresh add is accepted.
//
// So the two cases are split. Targets already recorded change nothing and keep
// the no-op success, which is what makes re-running the identical command safe.
// A target that is not recorded refuses as misuse (exit 2) and names link, the
// writer whose job is adding an edge to a node that already exists.
//
// The refusal names each target twice and differently on purpose. The prose
// half renders it through gen.Ref, the "<key-path> (<id>)" convention every
// other human surface uses (cli-surface.md, "A node is addressed by id or by
// key"), so a caller who typed a key is answered in the words they typed. The
// command half carries the bare address instead, because it is meant to be
// copied and run, and a reference is not an argument.
func refuseUnrecordedOnReAdd(tree *store.Tree, noun, key string, recorded, targets []string) error {
	if len(targets) == 0 {
		return nil
	}
	have := make(map[string]bool, len(recorded))
	for _, id := range recorded {
		have[id] = true
	}
	var refs, addrs []string
	r := resolve.NewResolver(tree)
	for _, target := range targets {
		if have[target] {
			continue
		}
		ref, addr := target, target
		// The targets reached here already resolved once, in resolveDependsOn.
		// The guard is for the id a tree can no longer name at all, which
		// degrades to the bare id rather than to a blank.
		if n, err := r.Resolve(resolve.Ref{Raw: target}); err == nil {
			ref = gen.Ref(tree, n)
			if k := deref(n.NodeKey()); k != "" {
				addr = k
			}
		}
		refs = append(refs, ref)
		addrs = append(addrs, addr)
	}
	if len(refs) == 0 {
		return nil
	}
	return &usageError{
		reason: "depends_on_not_recorded",
		msg: fmt.Sprintf("%s %s already exists and does not depend on %s; add re-runs as a no-op and never edits a node. Record it with: tiller link %s %s --depends-on %s",
			noun, key, strings.Join(refs, ", "), noun, key, strings.Join(addrs, " --depends-on ")),
	}
}

// splitFeatureKey breaks a canonical feature key into its epic and local feature
// segments. A key that is not exactly <epic-key>/<feature-key> is a misuse the
// CLI surfaces as exit 2.
func splitFeatureKey(canonical string) (epicKey, featureKey string, err error) {
	parts := strings.Split(canonical, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", &usageError{
			reason: "invalid_key",
			msg:    fmt.Sprintf("feature key %q must be <epic-key>/<feature-key>", canonical),
		}
	}
	return parts[0], parts[1], nil
}

// splitTaskKey breaks a canonical task key into its epic, feature, and task
// segments (artifact-schema.md, Keys and paths). A key that is not exactly
// <epic-key>/<feature-key>/<task-key> is a misuse the CLI surfaces as exit 2.
func splitTaskKey(canonical string) (epicKey, featureKey, taskKey string, err error) {
	parts := strings.Split(canonical, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", &usageError{
			reason: "invalid_key",
			msg:    fmt.Sprintf("task key %q must be <epic-key>/<feature-key>/<task-key>", canonical),
		}
	}
	return parts[0], parts[1], parts[2], nil
}

// renderEpicMD is the prose stub for a freshly added epic. Prose is human-owned
// and agent-drafted; this is only the placeholder the decompose skill fills in.
func renderEpicMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Draft epic created by `tiller add epic`. Replace this with the epic's\n"+
		"intent, scope, and definition of done.\n\n"+
		"## Intent\n\n## Scope\n\n## Definition of done\n", title)
}

// renderFeatureMD is the prose stub for a freshly added feature.
func renderFeatureMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Draft feature created by `tiller add feature`. Write the spec here, then\n"+
		"add `feature.eval.md` and run `tiller spec feature`.\n\n"+
		"## Summary\n\n## Behavior\n\n## Notes\n", title)
}

// renderTaskMD is the prose stub for a freshly added task. task.md is the
// implementation spec, the terminus of the prose chain (artifact-schema.md,
// Tasks), so the headings name what that spec has to say. The Reuses heading is
// where specify records what the task calls instead of rewriting it, so the
// reuse is written down where a reviewer reads it.
func renderTaskMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Proposed task created by `tiller add task`. Write the implementation spec\n"+
		"here: how the work will be done, before a human approves it.\n\n"+
		"## Approach\n\n## Reuses\n\n## Surfaces\n\n## Tests\n\n## Edge cases\n", title)
}

// heading picks the markdown heading for a node: its title if given, otherwise
// its key, so a stub never renders an empty heading.
func heading(title, key string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return key
}

// loadTaskFile resolves the task.yml one task-grain write will land on and
// reads it back, returning the on-disk document and its repo-relative path. The
// caller has already established feat.Key is non-nil.
//
// The path is rebuilt from the task's own canonical key, the way featureRelDir
// rebuilds a feature's folder from the feature's key. That derivation is the
// only thing between "the caller named this node" and "this file gets
// rewritten", so its result is checked against the addressed id rather than
// trusted: a rewrite that lands on a sibling's file cancels or crosses to done
// a node nobody named, and the done crossing does not reverse. store.Load
// refuses a tree whose folders and keys disagree (checkKeyNamesItsFolder),
// which is what makes the derivation sound; this check is the local half, and
// it also catches a wrong path the loader cannot see, one this package computed.
func loadTaskFile(tree *store.Tree, feat *store.Feature, tk *store.Task) (*store.Task, string, store.FileSnapshot, error) {
	if tk.Key == nil {
		return nil, "", store.FileSnapshot{}, fmt.Errorf("mutate: task %s carries no key, so its folder cannot be named", tk.ID)
	}
	ymlRel := filepath.Join(store.TaskRelDir(feat.EpicKey, localKey(*feat.Key), localKey(*tk.Key)), store.TaskStateFile)

	var doc store.Task
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	if err := validateFreshNodeIdentity(store.NounTask, tk.ID, ymlRel, doc.ID, deref(doc.Key), deref(tk.Key), deref(feat.Key)); err != nil {
		return nil, "", store.FileSnapshot{}, err
	}
	return &doc, ymlRel, snapshot, nil
}

// localKey returns the last segment of a canonical key, which is the on-disk
// folder name at that node's own grain.
func localKey(canonical string) string {
	if i := strings.LastIndex(canonical, "/"); i >= 0 {
		return canonical[i+1:]
	}
	return canonical
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fileExists(root, path string) bool {
	info, err := mutationPathInfo(root, path)
	return err == nil && info.Mode().IsRegular()
}

func readYAML(root, relative string, out any) error {
	data, err := store.ReadRepositoryFile(root, relative)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

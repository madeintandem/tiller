package mutate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// BranchSafetyError refuses a branch identity that cannot safely carry this work.
// Tiller only diagnoses refs; it never renames, deletes, creates, or moves them.
type BranchSafetyError struct {
	Reason        string
	Noun          string
	NodeID        string
	Branch        string
	Conflict      string
	CurrentBranch string
	// Worktree is the path where Branch is checked out, and RefMissing marks a
	// Branch with no local ref; both steer attest_branch_mismatch's remedy.
	Worktree   string
	RefMissing bool
}

func (e *BranchSafetyError) Error() string {
	switch e.Reason {
	case "branch_mismatch":
		return fmt.Sprintf("%s %s must submit on its recorded branch %q, not %q; check out the recorded branch", e.Noun, e.NodeID, e.Branch, e.CurrentBranch)
	case "branch_missing":
		return fmt.Sprintf("%s %s has no recorded branch; start a claimed task before submitting", e.Noun, e.NodeID)
	case "branch_review_identity":
		return fmt.Sprintf("%s %s already has a recorded PR; continue that review instead of replacing it", e.Noun, e.NodeID)
	case "branch_review_head_mismatch":
		return fmt.Sprintf("%s %s review ref must name the current head of its recorded branch %q", e.Noun, e.NodeID, e.Branch)
	case "aggregate_round_closed":
		return fmt.Sprintf("%s %s retains its completed review on branch %q; run tiller sync from integration to reopen the parent before starting new tasks", e.Noun, e.NodeID, e.Branch)
	case "aggregate_unrecorded":
		return fmt.Sprintf("branch %q exists but is not recorded on %s %s; review its history against integration (git log <integration>..%s) and, if it is this aggregate, rerun start with --adopt-aggregate; otherwise rename it aside with Git. Tiller will not delete refs", e.Branch, e.Noun, e.NodeID, e.Branch)
	case "aggregate_adopt_missing":
		return fmt.Sprintf("--adopt-aggregate needs an existing local branch %q for %s %s, and none exists; build it with Git first or start without the flag", e.Branch, e.Noun, e.NodeID)
	case "aggregate_adopt_recorded":
		return fmt.Sprintf("%s %s already records its aggregate branch or review, so there is nothing to adopt; start without --adopt-aggregate", e.Noun, e.NodeID)
	case "aggregate_adopt_task_branch":
		return fmt.Sprintf("--adopt-aggregate refuses %q because it is task %s's branch; Tiller never reinterprets a task branch as an aggregate", e.Branch, e.Conflict)
	case "aggregate_adopt_grain":
		return fmt.Sprintf("%s %s has no aggregate to adopt, since only a task under feature or epic review grain rides one; start without --adopt-aggregate", e.Noun, e.NodeID)
	case "attest_branch_mismatch":
		prefix := fmt.Sprintf("feature %s acceptance is recorded on its aggregate branch %q, not %q; ", e.NodeID, e.Branch, e.CurrentBranch)
		switch {
		case e.Worktree != "":
			return prefix + fmt.Sprintf("run this attest in the worktree at %s, then commit there before submit", e.Worktree)
		case e.RefMissing:
			return prefix + fmt.Sprintf("run git fetch && git switch %q, run this attest there, and commit and push before submit", e.Branch)
		}
		return prefix + fmt.Sprintf("check out %q, run this attest there, and commit before submit", e.Branch)
	case "branch_invalid":
		return fmt.Sprintf("git.branch_pattern must include ${node.key} and produce a valid local branch name, not %q", e.Branch)
	default:
		return fmt.Sprintf("branch %q for %s %s conflicts with %q; choose a non-conflicting git.branch_pattern for unstarted work, or explicitly preserve and rename the conflicting ref with Git after reviewing its history; Tiller will not delete refs", e.Branch, e.Noun, e.NodeID, e.Conflict)
	}
}

// validWorkingBranch follows git-check-ref-format's branch-name rules, without
// permitting revision shorthand or already-qualified refs as recorded names.
func validWorkingBranch(branch string) bool {
	if branch == "" || branch == "HEAD" || branch == "@" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "refs/") || strings.HasSuffix(branch, ".") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	for _, c := range branch {
		if c <= ' ' || c == 127 || strings.ContainsRune("~^:?*[\\", c) {
			return false
		}
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}

func branchesConflict(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func guardNewBranch(tree *store.Tree, rev vcs.Revisions, noun, id, branch string) error {
	return guardBranch(tree, rev, noun, id, branch, false)
}

// guardAdoptedBranch runs start --adopt-aggregate's checks on the derived
// aggregate name: it must not be any task's branch, must pass every
// reservation and prefix check a new branch passes, and must already exist
// locally. Only the exact-name ref the adopt names is exempt.
func guardAdoptedBranch(tree *store.Tree, rev vcs.Revisions, noun, id, branch string) error {
	for _, taskID := range sortedMapKeys(tree.Tasks) {
		task := tree.Tasks[taskID]
		if task.Branch == branch || task.ReviewRef != nil && task.ReviewRef.Branch == branch {
			return &BranchSafetyError{Reason: "aggregate_adopt_task_branch", Noun: noun, NodeID: id, Branch: branch, Conflict: task.ID}
		}
	}
	if err := guardBranch(tree, rev, noun, id, branch, true); err != nil {
		return err
	}
	exists, err := rev.RefExists(vcs.BranchRef(branch))
	if err != nil {
		return err
	}
	if !exists {
		return &BranchSafetyError{Reason: "aggregate_adopt_missing", Noun: noun, NodeID: id, Branch: branch}
	}
	return nil
}

// guardBranch is guardNewBranch with one switch: adopt ignores a local ref at
// exactly branch, the one ref start --adopt-aggregate records. Prefix
// conflicts in either direction still refuse.
func guardBranch(tree *store.Tree, rev vcs.Revisions, noun, id, branch string, adopt bool) error {
	fail := func(reason, conflict string) error {
		return &BranchSafetyError{Reason: reason, Noun: noun, NodeID: id, Branch: branch, Conflict: conflict}
	}
	pattern := tree.Manifest.Git.BranchPattern
	if (strings.TrimSpace(pattern) != "" && !strings.Contains(pattern, "${node.key}")) || !validWorkingBranch(branch) {
		return fail("branch_invalid", "")
	}
	if integration := strings.TrimPrefix(tree.Manifest.Git.IntegrationBranch, "refs/heads/"); integration != "" && branchesConflict(branch, integration) {
		return fail("branch_collision", integration)
	}
	// Include deterministic names of unstarted nodes: otherwise a bad pattern
	// can record the first branch successfully and strand its sibling later.
	type reservation struct{ id, branch string }
	var reserved []reservation
	for _, task := range tree.Tasks {
		b := task.Branch
		if b == "" && task.Key != nil {
			b = branchFor(tree.Manifest, *task.Key)
		}
		reserved = append(reserved, reservation{task.ID, b})
	}
	for _, feature := range tree.Features {
		b := feature.Branch
		epic := epicByKey(tree, feature.EpicKey)
		grain := store.ResolvedReviewGrain(tree.Manifest, epic, feature)
		if grain == store.ReviewGrainTask {
			continue
		}
		owner := feature.ID
		if grain == store.ReviewGrainEpic && epic != nil {
			owner = epic.ID
		}
		if b == "" && grain == store.ReviewGrainFeature && feature.Key != nil {
			b = branchFor(tree.Manifest, "reviews/features/"+*feature.Key)
		}
		reserved = append(reserved, reservation{owner, b})
	}
	for _, epic := range tree.Epics {
		b := epic.Branch
		if store.ResolvedReviewGrain(tree.Manifest, epic, nil) != store.ReviewGrainEpic {
			continue
		}
		if b == "" && epic.Key != nil && store.ResolvedReviewGrain(tree.Manifest, epic, nil) == store.ReviewGrainEpic {
			b = branchFor(tree.Manifest, "reviews/epics/"+*epic.Key)
		}
		reserved = append(reserved, reservation{epic.ID, b})
	}
	for _, noun := range store.ShelfNouns() {
		for _, node := range tree.WorkNodesOf(noun) {
			b := node.Body().Branch
			if b == "" && node.NodeKey() != nil {
				b = branchFor(tree.Manifest, *node.NodeKey())
			}
			reserved = append(reserved, reservation{node.NodeID(), b})
		}
	}
	sort.Slice(reserved, func(i, j int) bool { return reserved[i].id < reserved[j].id })
	for _, other := range reserved {
		if other.id != id && other.branch != "" && branchesConflict(branch, other.branch) {
			return fail("branch_collision", other.branch)
		}
	}
	if refs, ok := rev.(interface{ LocalBranches() ([]string, error) }); ok {
		branches, err := refs.LocalBranches()
		if err != nil {
			return err
		}
		for _, existing := range branches {
			if adopt && existing == branch {
				continue
			}
			if branchesConflict(branch, existing) {
				return fail("branch_ref_conflict", existing)
			}
		}
		return nil
	}
	// Revisions supports ancestor prefixes and exact refs even when a test or
	// alternate implementation does not provide the concrete enumeration API.
	for prefix := branch; prefix != ""; {
		exists, err := rev.RefExists(vcs.BranchRef(prefix))
		if err != nil {
			return err
		}
		if exists && !(adopt && prefix == branch) {
			return fail("branch_ref_conflict", prefix)
		}
		at := strings.LastIndexByte(prefix, '/')
		if at < 0 {
			break
		}
		prefix = prefix[:at]
	}
	return nil
}

func guardSubmitBranch(rev vcs.Revisions, noun, id, branch string, ref *store.ReviewRef) error {
	if branch == "" {
		return &BranchSafetyError{Reason: "branch_missing", Noun: noun, NodeID: id}
	}
	current, err := rev.Branch()
	if err != nil {
		return err
	}
	if current != branch || (ref != nil && ref.Branch != branch) {
		if current == branch {
			current = ref.Branch
		}
		return &BranchSafetyError{Reason: "branch_mismatch", Noun: noun, NodeID: id, Branch: branch, CurrentBranch: current}
	}
	if ref != nil {
		head, err := rev.CurrentCommit()
		if err != nil {
			return err
		}
		if ref.Head.Kind != store.KindGitCommit || ref.Head.Value != head.Value {
			return &BranchSafetyError{Reason: "branch_review_head_mismatch", Noun: noun, NodeID: id, Branch: branch}
		}
	}
	return nil
}

func guardReviewIdentity(noun, id string, recorded, requested *int) error {
	if recorded != nil && (requested == nil || *recorded != *requested) {
		return &BranchSafetyError{Reason: "branch_review_identity", Noun: noun, NodeID: id}
	}
	return nil
}

// TaskIncludedInAggregate verifies both the submitted work and the current
// branch tip, including later submission-state commits.
func TaskIncludedInAggregate(rev vcs.Revisions, task *store.Task, aggregate string) (bool, error) {
	if aggregate == "" || task.Branch == "" || task.Branch == aggregate || task.ReviewRef == nil || task.ReviewRef.Branch != task.Branch || task.ReviewRef.Head.Kind != store.KindGitCommit || !strings.HasPrefix(task.ReviewRef.Head.Value, "git:") {
		return false, nil
	}
	head := strings.TrimPrefix(task.ReviewRef.Head.Value, "git:")
	if head == "" {
		return false, nil
	}
	exists, err := rev.RefExists(vcs.BranchRef(aggregate))
	if err != nil || !exists {
		return false, err
	}
	for _, ref := range []string{head, vcs.BranchRef(task.Branch)} {
		included, err := rev.IsMergedInto(ref, vcs.BranchRef(aggregate))
		if err != nil || !included {
			return false, err
		}
	}
	return true, nil
}

// AggregateContentMissingError identifies the riding task whose submitted work
// or latest state commit has not yet been merged into the aggregate.
type AggregateContentMissingError struct {
	Noun, NodeID, Branch, TaskID, TaskKey string
}

func (e *AggregateContentMissingError) Error() string {
	return fmt.Sprintf("%s %s aggregate %q does not contain task %s (%s)'s submitted head and current branch tip; commit its submission state and merge the task branch into the aggregate with ordinary Git before submitting", e.Noun, e.NodeID, e.Branch, e.TaskKey, e.TaskID)
}

func guardAggregateTasks(tree *store.Tree, rev vcs.Revisions, noun, id, branch string, feature *store.Feature) error {
	for _, task := range gen.OrderTasks(tree, tree.FeatureTasks[feature.ID], feature.Order) {
		// A task-grain task never enters the aggregate;
		// taskWithOwnOpenReview refuses it before this guard runs.
		if task.Status != "in_review" || task.StartedGrain == store.ReviewGrainTask {
			continue
		}
		included, err := TaskIncludedInAggregate(rev, task, branch)
		if err != nil {
			return err
		}
		if !included {
			return &AggregateContentMissingError{Noun: noun, NodeID: id, Branch: branch, TaskID: task.ID, TaskKey: deref(task.Key)}
		}
	}
	return nil
}

func aggregateForTask(tree *store.Tree, feature *store.Feature) (branch, noun, id string) {
	epic := epicByKey(tree, feature.EpicKey)
	switch store.ResolvedReviewGrain(tree.Manifest, epic, feature) {
	case store.ReviewGrainFeature:
		return feature.Branch, store.NounFeature, feature.ID
	case store.ReviewGrainEpic:
		if epic != nil {
			return epic.Branch, store.NounEpic, epic.ID
		}
	}
	return "", "", ""
}

// taskAggregateBranch is the aggregate task rides: none when it started at
// task grain, since such a task takes its own PR and the aggregate never
// absorbs it, else its feature's aggregate.
func taskAggregateBranch(tree *store.Tree, feature *store.Feature, task *store.Task) string {
	if task.StartedGrain == store.ReviewGrainTask {
		return ""
	}
	branch, _, _ := aggregateForTask(tree, feature)
	return branch
}

// TaskDependencyBlockedError is a start refusal, not a task state transition.
type TaskDependencyBlockedError struct {
	TaskID, TaskKey, DependencyID, DependencyKey, DependencyStatus, Branch string
	AggregateUnrecorded                                                    bool
	// DependencyStartedGrain is the grain the prerequisite started under when
	// it recorded one. At task grain it takes its own PR and never enters the
	// aggregate, so the only remedy is waiting for its merge.
	DependencyStartedGrain string
}

func (e *TaskDependencyBlockedError) Error() string {
	if e.DependencyStartedGrain == store.ReviewGrainTask {
		return fmt.Sprintf("task %s depends on %s (%s), which started at task grain; wait for its PR to merge and sync before starting", e.TaskKey, e.DependencyID, e.DependencyStatus)
	}
	if e.AggregateUnrecorded {
		return fmt.Sprintf("task %s depends on %s (%s); aggregate %q is not recorded and does not exist; build it containing that prerequisite's complete branch, then start with --adopt-aggregate", e.TaskKey, e.DependencyID, e.DependencyStatus, e.Branch)
	}
	if e.Branch == "" {
		return fmt.Sprintf("task %s depends on %s (%s); wait for its integration before starting", e.TaskKey, e.DependencyID, e.DependencyStatus)
	}
	return fmt.Sprintf("task %s depends on %s (%s); wait for integration, or at coarse grain submit that prerequisite and merge its complete branch into aggregate %q first", e.TaskKey, e.DependencyID, e.DependencyStatus, e.Branch)
}

// taskStartBase names the safe base for a task start. minted reports that the
// aggregate it reads was named by this start and recorded nowhere before it,
// so a dependency refusal must not send anyone to assemble into it as if it
// existed.
func taskStartBase(tree *store.Tree, rev vcs.Revisions, feature *store.Feature, task *store.Task, minted bool) (string, error) {
	ancestors := []store.Node{feature}
	if epic := epicByKey(tree, feature.EpicKey); epic != nil {
		ancestors = append(ancestors, epic)
	}
	for _, ancestor := range ancestors {
		var dependencies []string
		switch node := ancestor.(type) {
		case *store.Feature:
			dependencies = node.DependsOn
		case *store.Epic:
			dependencies = node.DependsOn
		}
		for _, id := range dependencies {
			var dependency store.Node
			if node := tree.Features[id]; node != nil {
				dependency = node
			}
			if node := tree.Epics[id]; node != nil {
				dependency = node
			}
			if dependency != nil && (dependency.NodeStatus() == statusDone || dependency.NodeStatus() == statusCancelled || dependency.NodeStatus() == statusArchived) {
				continue
			}
			blocked := &TaskDependencyBlockedError{TaskID: task.ID, TaskKey: deref(task.Key), DependencyID: id}
			if dependency != nil {
				blocked.DependencyKey, blocked.DependencyStatus = deref(dependency.NodeKey()), dependency.NodeStatus()
			}
			return "", blocked
		}
	}
	base, source, err := resolveInto(tree.Manifest, SyncInput{}, rev.HeadBranch)
	if err != nil {
		return "", err
	}
	if err := guardIntoNotFeatureBranch(tree, base, source, rev.HeadBranch); err != nil {
		return "", err
	}
	base = strings.TrimPrefix(base, "refs/heads/")
	aggregate, noun, id := aggregateForTask(tree, feature)
	if noun != "" {
		if aggregate == "" {
			return "", &BranchSafetyError{Reason: "branch_missing", Noun: noun, NodeID: id}
		}
	}
	for _, dependencyID := range task.DependsOn {
		dependency := tree.Tasks[dependencyID]
		if dependency != nil && (dependency.Status == statusDone || dependency.Status == statusCancelled) {
			continue
		}
		// Only a submitted same-feature prerequisite can be assembled into
		// the aggregate, so only that refusal may send anyone to build and
		// adopt one; every other prerequisite still waits.
		// A prerequisite started at task grain takes its own PR and never
		// enters the aggregate, even one that holds its branch.
		includable := aggregate != "" && dependency != nil && dependency.FeatureID == feature.ID && dependency.Status == "in_review"
		if includable && dependency.StartedGrain != store.ReviewGrainTask {
			included, err := TaskIncludedInAggregate(rev, dependency, aggregate)
			if err != nil {
				return "", err
			}
			if included {
				base = aggregate
				continue
			}
		}
		blocked := &TaskDependencyBlockedError{TaskID: task.ID, TaskKey: deref(task.Key), DependencyID: dependencyID, Branch: aggregate, AggregateUnrecorded: minted && includable && dependency.StartedGrain != store.ReviewGrainTask}
		if dependency != nil {
			blocked.DependencyKey, blocked.DependencyStatus = deref(dependency.Key), dependency.Status
			blocked.DependencyStartedGrain = dependency.StartedGrain
		}
		return "", blocked
	}
	return base, nil
}

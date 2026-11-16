package mutate

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// reviewGrainRefReader is what the ref half of the safe configure window needs
// beyond vcs.Revisions: one listing of local branches, a committed file read at
// a ref, and a way to tell a directory outside git from a failing git. *vcs.Git provides both. A Revisions without them (a test stub)
// leaves the lock reading the current checkout only.
type reviewGrainRefReader interface {
	LocalBranches() ([]string, error)
	InRepository() (bool, error)
	FileAt(commit, path string) ([]byte, error)
}

// reviewGrainRefState is the part of a feature.yml or task.yml the lock reads
// at a ref. It is decoded leniently on purpose: the file on another branch may
// come from a different schema version, and only these fields decide the lock.
type reviewGrainRefState struct {
	Status    string    `yaml:"status"`
	Branch    string    `yaml:"branch"`
	PR        yaml.Node `yaml:"pr"`
	ReviewRef yaml.Node `yaml:"review_ref"`
}

func (s reviewGrainRefState) hasReviewSurface() bool {
	recorded := func(n yaml.Node) bool { return n.Kind != 0 && n.Tag != "!!null" }
	return recorded(s.PR) || recorded(s.ReviewRef)
}

// evaluateReviewGrainRefs extends the safe configure window past the current
// checkout. Work started in another checkout or worktree commits its state on
// the task's branch and on the aggregate it merges into, and none of it reaches
// integration before the review merges, so the files evaluateReviewGrainChange
// reads still show that work as unstarted.
//
// For each affected feature it looks up only the canonical local branches: the
// epic and feature aggregates (and the rotated name a later round may have
// taken), then each open task's own branch. A branch whose tip HEAD already
// contains is skipped, since the checkout holds everything it says. A state
// file absent at a ref means that branch never carried the node and locks
// nothing. One that does not parse fails the command: the lock cannot be
// proven open, and passing would be the same blindness this guard exists to
// close. Remote-tracking refs are not read; they go stale without a fetch and
// outlive the branches they track.
func evaluateReviewGrainRefs(tree *store.Tree, rev vcs.Revisions, change reviewGrainChange) error {
	reader, ok := rev.(reviewGrainRefReader)
	if !ok {
		return nil
	}
	affected := reviewGrainAffectedFeatures(tree, change)
	if len(affected) == 0 {
		return nil
	}
	branches, err := reader.LocalBranches()
	if err != nil {
		// A project not yet under git has no other branch to hold state.
		if inside, repoErr := reader.InRepository(); repoErr == nil && !inside {
			return nil
		}
		return err
	}
	local := make(map[string]bool, len(branches))
	for _, branch := range branches {
		local[branch] = true
	}
	// carriesNews reports whether branch exists and holds commits HEAD lacks.
	checked := make(map[string]bool)
	carriesNews := func(branch string) (bool, error) {
		if branch == "" || !local[branch] {
			return false, nil
		}
		if news, seen := checked[branch]; seen {
			return news, nil
		}
		merged, err := rev.IsMergedInto(vcs.BranchRef(branch), "HEAD")
		if err != nil {
			return false, err
		}
		checked[branch] = !merged
		return !merged, nil
	}
	read := func(branch, rel string) (reviewGrainRefState, bool, error) {
		var state reviewGrainRefState
		data, err := reader.FileAt(vcs.BranchRef(branch), filepath.ToSlash(rel))
		var absent *vcs.NotVersionedError
		if errors.As(err, &absent) {
			return state, false, nil
		}
		if err != nil {
			return state, false, err
		}
		if err := yaml.Unmarshal(data, &state); err != nil {
			return state, false, fmt.Errorf(
				"mutate: cannot check the review grain lock: %s on branch %s does not parse (%v); repair or delete that branch, then retry",
				filepath.ToSlash(rel), branch, err)
		}
		return state, true, nil
	}

	for _, a := range affected {
		feature := a.feature
		tasks := openReviewGrainTasks(gen.OrderTasks(tree, tree.FeatureTasks[feature.ID], feature.Order))
		for _, branch := range reviewGrainAggregateCandidates(tree, feature) {
			news, err := carriesNews(branch)
			if err != nil {
				return err
			}
			if !news {
				continue
			}
			atRef, found, err := read(branch, filepath.Join(featureRelDir(feature), "feature.yml"))
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			if atRef.hasReviewSurface() {
				return reviewGrainBlockedOnBranch(branch,
					reviewGrainBlocked(change, store.NounFeature, feature.ID, deref(feature.Key), atRef.Status, a.before, a.after))
			}
			for _, task := range tasks {
				if err := reviewGrainTaskBlockedAt(read, change, feature, task, branch, a.before, a.after); err != nil {
					return err
				}
			}
			if atRef.Branch != "" {
				blocked := reviewGrainBranchBlocked(change, feature, a.before, a.after)
				blocked.(*ReviewGrainChangeBlockedError).BlockerStatus = atRef.Status
				return reviewGrainBlockedOnBranch(branch, blocked)
			}
		}
		for _, task := range tasks {
			branch := task.Branch
			if branch == "" && task.Key != nil {
				branch = branchFor(tree.Manifest, *task.Key)
			}
			news, err := carriesNews(branch)
			if err != nil {
				return err
			}
			if !news {
				continue
			}
			if err := reviewGrainTaskBlockedAt(read, change, feature, task, branch, a.before, a.after); err != nil {
				return err
			}
		}
	}
	return nil
}

func reviewGrainTaskBlockedAt(
	read func(branch, rel string) (reviewGrainRefState, bool, error),
	change reviewGrainChange, feature *store.Feature, task *store.Task, branch, before, after string,
) error {
	rel := filepath.Join(featureRelDir(feature), store.TaskDirName, filepath.Base(deref(task.Key)), store.TaskStateFile)
	atRef, found, err := read(branch, rel)
	if err != nil {
		return err
	}
	if !found || !reviewGrainTaskInFlight(atRef.Status) {
		return nil
	}
	return reviewGrainBlockedOnBranch(branch,
		reviewGrainBlocked(change, store.NounTask, task.ID, deref(task.Key), atRef.Status, before, after))
}

func reviewGrainBlockedOnBranch(branch string, blocked error) error {
	blocked.(*ReviewGrainChangeBlockedError).BlockerBranch = branch
	return blocked
}

// openReviewGrainTasks drops tasks the checkout already records as finished: a
// branch left behind after its review merged still says in_review at its tip,
// and the checkout is the newer word on those.
func openReviewGrainTasks(tasks []*store.Task) []*store.Task {
	var open []*store.Task
	for _, task := range tasks {
		if task.Status != statusDone && task.Status != statusCancelled {
			open = append(open, task)
		}
	}
	return open
}

// reviewGrainAggregateCandidates names the aggregate branches work under
// feature could ride: the epic aggregate and the feature aggregate, each under
// its recorded name, its derived name, and the rotated name a reopened round
// takes (the derived name plus the id of the task that opened the round). Both
// grains are named because another checkout resolves the grain it can see.
func reviewGrainAggregateCandidates(tree *store.Tree, feature *store.Feature) []string {
	var candidates []string
	seen := make(map[string]bool)
	add := func(branch string) {
		if branch != "" && !seen[branch] {
			seen[branch] = true
			candidates = append(candidates, branch)
		}
	}
	if epic := epicByKey(tree, feature.EpicKey); epic != nil {
		add(epic.Branch)
		if epic.Key != nil {
			derived := branchFor(tree.Manifest, "reviews/epics/"+*epic.Key)
			add(derived)
			for _, id := range sortedMapKeys(tree.Features) {
				if tree.Features[id].EpicKey != feature.EpicKey {
					continue
				}
				for _, task := range tree.FeatureTasks[id] {
					add(derived + "-" + task.ID)
				}
			}
		}
	}
	add(feature.Branch)
	if feature.Key != nil {
		derived := branchFor(tree.Manifest, "reviews/features/"+*feature.Key)
		add(derived)
		for _, task := range tree.FeatureTasks[feature.ID] {
			add(derived + "-" + task.ID)
		}
	}
	return candidates
}

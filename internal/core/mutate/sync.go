package mutate

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/lifecycle"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// Sync observes the final human merge into the integration branch. Task grain
// observes each task's recorded branch independently. Feature and epic grain
// observe only a recorded aggregate review, and cross only riding tasks whose
// submitted head and current branch tip are included in that aggregate.
//
// Ancestry requires containment off the integration branch's first-parent line;
// content requires the complete, whitespace-faithful net patch of a squash.
// A fast-forward has no merge witness, so only a human assertion can cross it.
// Assertions are restricted to the declared integration branch and never relax
// containment or aggregate inclusion.
//
// Feature and epic completion remains a rollup over completed children. Those
// rollups may reopen when new work appears; a task's historical done never does.
// Required gates are advisory at a human merge: violations are recorded rather
// than suppressing observed work.

// contentScanLimit bounds the squash-detection fallback: only the newest 50
// first-parent commits of the integration branch are candidates. The fallback
// answers "did the human just squash-merge this branch", and 50 first-parent
// commits is more integration-line traffic than accumulates between two syncs on
// a working team. The bound is also what keeps an ancient coincidental patch-id
// match out of reach, and it keeps the scan cheap: two git processes, computed
// lazily and once per sync, and only when some feature actually failed the
// ancestry test.
const contentScanLimit = 50

// SyncInput is the request for Sync. Into is the integration branch whose merges
// are observed. An empty Into no longer means HEAD: it means "resolve it", which
// resolveInto does from the manifest and then from the branch the checkout is
// standing on.
//
// AcceptFastForward carries task, reviewed feature/epic, or shelf-node ids,
// already resolved from the human's references, whose contained branches they
// asserts were merged. It widens the basis Tiller will accept on those nodes,
// never the set of branches it will cross: a named node the integration branch
// does not contain, or whose branch is gone, refuses the whole command. An empty
// slice is the whole of today's behavior, unchanged.
type SyncInput struct {
	Into              string
	AcceptFastForward []string
}

// Crossing is one node sync moved to done: the node it crossed, the observed
// merge commit, the evidence the crossing rested on, and the names of any
// required gates that were not green at that commit (each mirrored into the
// node's policy_violations). Basis is "ancestry", "content", "rollup", or
// "human_attested", and which of those a grain may carry is set out with the
// basis constants; MatchedCommit names the integration-branch commit whose patch
// matched, and is set only on a content crossing.
type Crossing struct {
	Noun                       string
	ID                         string
	Key                        string
	Status                     string
	Observed                   string
	Basis                      string
	MatchedCommit              string
	PolicyViolations           []string
	HistoricalPolicyViolations []store.PolicyViolation
	TouchedDesign              []string
	// EvalContractMismatch is set on a feature crossing whose completion
	// record pins an integration eval blob other than the one accepted_against
	// names. It is advisory: the crossing still happens and nothing new is
	// persisted (cli-surface.md, sync).
	EvalContractMismatch *EvalContractMismatch
}

// EvalContractMismatch names the two eval blobs that disagree at a feature
// crossing: Accepted is accepted_against.eval, the working-tree contract check
// and submit judged; Verified is the integration commit's blob the completion
// record now pins.
type EvalContractMismatch struct {
	Accepted string
	Verified string
}

// evalContractMismatch compares the accepted eval with the verified one. A
// feature that never pinned an accepted eval has nothing to compare against.
func evalContractMismatch(f store.Feature) *EvalContractMismatch {
	accepted, verified := f.AcceptedAgainst.Eval.Value, f.VerifiedAgainst.Eval.Value
	if accepted == "" || accepted == verified {
		return nil
	}
	return &EvalContractMismatch{Accepted: accepted, Verified: verified}
}

// Awaiting is one node still sitting in in_review after the walk, with the
// branch sync observed for it. It is what the zero-crossing readout names, so a
// sync that recorded nothing while work waits in review says so instead of
// reading like a clean run.
//
// A task names its own branch at task grain, or its aggregate review branch at
// coarse grain. A bug or chore with no carrier owns its observation directly.
// Carried shelf nodes never appear: their merge is their carrier's.
//
// BranchMissing means the recorded branch does not resolve locally. That covers a
// branch deleted after the merge and a branch that was never created at all, and
// it deliberately does not claim to tell them apart: the observation cannot. It
// is a pointer at where to look, not a diagnosis.
//
// NoMergeWitness means the opposite situation: the branch is contained in the
// integration branch, but only on that branch's own first-parent line, where a
// fast-forward merge and a branch that never diverged are indistinguishable (see
// observeMerge). Nothing was crossed on it, and naming it apart is what makes a
// fast-forward strand diagnosable.
// ReviewMissing means coarse submission has not been recorded. AggregateIncomplete
// means the aggregate does not include this task's submitted head and current tip.
type Awaiting struct {
	Noun                string
	ID                  string
	Key                 string
	Branch              string
	BranchMissing       bool
	NoMergeWitness      bool
	ReviewMissing       bool
	AggregateIncomplete bool
}

// CheckDue is one feature the run leaves in the awaiting-check phase: every
// non-cancelled task done, a required feature-grain gate with no recorded
// result, the rollup parked until `tiller check feature` runs. It is computed
// from the post-crossing in-memory tree, so the run that crosses a feature's
// last task names the feature in the same breath — the exact run that used to
// read as finished while the feature stranded silently
// (docs/spec/cli-surface.md).
type CheckDue struct {
	Noun string
	ID   string
	Key  string
}

// SyncResult is the outcome of a sync: the crossings it recorded, in observation
// order, the integration branch it observed and where that value came from, the
// nodes still awaiting a merge, the nodes the human asserted over, and the
// checks due after the walk.
//
// Attested carries the ids the human named on --accept-fast-forward, deduped and
// in the order named. It records the INPUT to the assertion rather than its
// effect: the named aggregate may not itself cross, and an auditor
// still needs to see what was asserted.
// Reopened carries the rollup-backed nodes this run moved BACKWARD, because new
// work landed beneath them. It is kept apart from Crossings rather than folded
// into it: a crossing is a node reaching done, and a reader (or the JSON
// envelope's crossings array, whose every entry means "this reached done")
// must not have to inspect a status field to tell the two directions apart.
//
// Skipped carries the third outcome, a node whose rollup was earned but whose
// completion record could not be built, so it did not move at all.
//
// ChecksDue carries the features whose rollup is parked on an unchecked gate,
// named so a sync never strands one silently.
type SyncResult struct {
	Crossings  []Crossing
	Reopened   []Reopening
	Skipped    []Skipped
	Into       string
	IntoSource string
	Awaiting   []Awaiting
	Attested   []string
	ChecksDue  []CheckDue
}

// Reopening is one rollup-backed node sync returned to work: a done feature that
// gained a task still to do, or a done epic that gained a feature still to do.
// Status is the state it went back to (a feature to active, an epic to ready).
// There is no basis: a reverse rests on the tree's own shape, never on a git
// observation (artifact-schema.md, "History is append-only": rollup-backed done
// recomputes).
type Reopening struct {
	Noun   string
	ID     string
	Key    string
	Status string
}

// Skipped is one node whose rollup was earned but could not be RECORDED, so it
// did not cross. Path is the repo-relative path git could not resolve, which is
// the whole diagnosis: a completion record pins the node's prose blob and one
// fingerprint per surface path (a git_tree for a directory, a git_blob for a
// file), and a path that resolves to nothing has no revision to pin.
//
// It exists so the skip is never silent. cli-surface.md's done-crossing section
// settles that a crossing which did not happen must be named rather than left
// indistinguishable from a clean run, and a skipped node is exactly that case:
// the work beneath it really is complete, so nothing else on any surface would
// ever say why it is still sitting there.
type Skipped struct {
	Noun string
	ID   string
	Key  string
	Path string
	// Surface is true when Path is one of the node's recorded surface paths,
	// the case where the work itself may have moved or removed the code and
	// `unlink --surface` is the repair.
	Surface bool
}

// skipUnrecordable classifies a rollup crossing's error into "skip this node" or
// "this is the caller's to surface", and it is the one place either grain
// decides. A node whose completion record cannot be built does not cross, and
// does not take the rest of the run down with it either: sync observes a whole
// project, and one node nobody can build a record for is not a reason to abandon
// the crossings every other node earned.
//
// The catch is deliberately the WHOLE record rather than the prose file alone,
// because a completion record pins several kinds of path and any of them can be
// unresolvable: a feature's feature.md and feature.eval.md, an epic's epic.md,
// and, at both grains, every surface.paths entry fingerprinted at its own grain.
// What still fails here is a path absent at HEAD or resolving to an object
// neither read supports (a submodule gitlink), which fails both reads and skips
// on the tree read's terms, so it gets the same answer.
//
// What makes that safe rather than a silent swallow is that the skip is
// REPORTED: the node and the offending path ride out on Skipped, sync names
// them, and the node stays where it was. cli-surface.md's own decision (the done
// crossing) is that a crossing that did not happen must never be
// indistinguishable from a clean run. A skipped feature also holds the epic above
// it back on its own terms, with no extra rule: the feature never reaches done,
// so the epic's rollup arithmetic simply does not add up.
//
// Only this typed answer is caught. A git that cannot run at all is an
// environment failure and still takes the run down, since nothing this run
// observed can be trusted.
func skipUnrecordable(err error, noun, id, key string, surface []string) (Skipped, bool) {
	var missing *vcs.NotVersionedError
	if !errors.As(err, &missing) {
		return Skipped{}, false
	}
	sk := Skipped{Noun: noun, ID: id, Key: key, Path: missing.Path}
	for _, p := range surface {
		if filepath.ToSlash(p) == filepath.ToSlash(missing.Path) {
			sk.Surface = true
			break
		}
	}
	return sk, true
}

// Sync observes task and aggregate branches and records done crossings.
// It resolves the acting contributor first (the exit-3 refusal every mutation shares),
// then the integration branch, then refuses if that branch belongs to live work
// (both before any observation and before any write), then refuses
// --accept-fast-forward if that branch is not the declared integration branch,
// then classifies whatever the human asserted on --accept-fast-forward (refusing,
// again before any write, if any named node is outside the
// contained-but-unwitnessed set), then walks the features in id order. A git
// error from the merge observation or the commit lookup is the environment
// failure it surfaces; a merged branch crosses its in_review tasks to done and
// the feature rolls up behind them. rev is the read-only git window, stubbed
// in tests; in is the optional integration-branch override.
func Sync(tree *store.Tree, deps Deps, rev vcs.Revisions, in SyncInput) (SyncResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return SyncResult{}, err
	}

	// The branch HEAD is on, read at most once: resolveInto needs it as the last
	// resort, and canonicalBranch needs it to expand a literal "HEAD" or "@".
	head := ""
	headRead := false
	headBranch := func() (string, error) {
		if headRead {
			return head, nil
		}
		h, herr := rev.HeadBranch()
		if herr != nil {
			return "", herr
		}
		head, headRead = h, true
		return head, nil
	}

	into, source, err := resolveInto(tree.Manifest, in, headBranch)
	if err != nil {
		return SyncResult{}, err
	}
	if err := guardIntoNotFeatureBranch(tree, into, source, headBranch); err != nil {
		return SyncResult{}, err
	}
	if err := guardAttestIntoIsDeclared(tree, in, into, source, headBranch); err != nil {
		return SyncResult{}, err
	}

	// The integration branch resolves to one commit, once, before any
	// observation. Every ancestry, first-parent, and content read and every
	// completion fingerprint below uses that commit, so a scratch checkout's HEAD
	// never stands in for integration and a ref that moves mid-run cannot mix two
	// revisions into one sync. The name stays the identity guards, recovery, and
	// diagnostics compare.
	tip, err := resolveIntegrationTip(rev, into, source)
	if err != nil {
		return SyncResult{}, err
	}

	var crossings []Crossing
	var reopened []Reopening
	var skipped []Skipped
	seen := map[string]mergeObs{}
	cache := &patchCache{}
	recovered, err := prepareRecoveredSync(tree, rev, into, tip.Commit)
	if err != nil {
		return SyncResult{}, err
	}

	// The human's assertion is classified before the walk and before any write, so
	// a named node outside the candidate set refuses the whole command rather than
	// crossing the eligible half of it.
	attested, named, err := attestedObservations(tree, rev, in.AcceptFastForward, into, tip.Commit, cache, recovered)
	if err != nil {
		return SyncResult{}, err
	}

	for _, f := range sortedFeatures(tree) {
		if f.Key == nil {
			continue
		}

		aggregateID, aggregate, recorded := reviewedAggregate(tree, f)
		mo := mergeObs{}
		recovery := recovered[aggregateID]
		for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
			if tk.Status != "in_review" {
				continue
			}
			// A task started at task grain observes its own merge, never an
			// aggregate it does not ride.
			ownGrain := tk.StartedGrain == store.ReviewGrainTask
			if !ownGrain && aggregateRecovery(tree, aggregateID) != nil && (f.Status == statusDone || f.Status == statusCancelled || aggregateRoundDone(tree, aggregateID)) {
				continue
			}
			id, branch := tk.ID, tk.Branch
			if aggregateID != "" && !ownGrain {
				id, branch = aggregateID, aggregate
			}
			taskObs, ok := attested[id]
			if !ok {
				taskObs, ok = seen[id]
			}
			if !ok {
				if ownGrain {
					taskObs, err = observeMerge(rev, branch, tip.Commit, cache)
					if err != nil {
						return SyncResult{}, err
					}
				} else if aggregateID != "" && !recorded {
					exists := false
					if branch != "" {
						exists, err = rev.RefExists(branchRef(branch))
						if err != nil {
							return SyncResult{}, err
						}
					}
					taskObs = mergeObs{Branch: branch, BranchExists: exists, ReviewMissing: true}
				} else if recovery != nil {
					taskObs = recovery.obs
				} else {
					taskObs, err = observeMerge(rev, branch, tip.Commit, cache)
					if err != nil {
						return SyncResult{}, err
					}
				}
				seen[id] = taskObs
			}
			if aggregateID != "" && !ownGrain && recorded && recovery == nil {
				included, ierr := TaskIncludedInAggregate(rev, tk, aggregate)
				if ierr != nil {
					return SyncResult{}, ierr
				}
				if !included {
					taskObs.Merged = false
					taskObs.AggregateIncomplete = true
				}
			}
			seen[tk.ID] = taskObs
			if !taskObs.Merged {
				continue
			}
			match := ""
			if taskObs.Commit != "" {
				match = "git:" + taskObs.Commit
			}
			taskRecovery := recovery
			if ownGrain {
				taskRecovery = nil
			}
			c, cerr := crossTaskDone(tree, deps, owner, rev, f, tk, tip.Ref, taskObs.Basis, match, taskRecovery)
			if cerr != nil {
				return SyncResult{}, cerr
			}
			crossings = append(crossings, c)
			if aggregateID != "" && !ownGrain {
				mo = taskObs
			}
		}
		if recovery != nil && mo.Merged && !featureRollupReady(tree, f) {
			if err := persistRecoveredFeaturePolicy(tree, deps, owner, rev, f, tip.Ref, recovery); err != nil {
				return SyncResult{}, err
			}
		}

		// The feature rollup: an active feature crosses once all its task PRs are
		// done and the feature-level gates/eval have been checked. It rests on its
		// tasks being done rather than on the feature branch itself, and it fires
		// just as well in a run where that branch is no longer observable at all
		// (deleted after the merge that crossed the tasks, with the feature gates
		// checked later).
		if f.Status == "active" && featureRollupReady(tree, f) {
			var historical []store.HistoricalPolicySnapshot
			if recoveredFeatureParticipated(tree, recovery, f) {
				historical = historicalPolicyForNode(recovery.record, f.ID)
			}
			c, cerr := crossFeatureDone(tree, deps, owner, rev, f, tip, featureBasis(mo), historical, recovery)
			if cerr != nil {
				sk, ok := skipUnrecordable(cerr, store.NounFeature, f.ID, deref(f.Key), f.Surface.Paths)
				if !ok {
					return SyncResult{}, cerr
				}
				skipped = append(skipped, sk)
				continue
			}
			crossings = append(crossings, c)
		}

		// The reverse of that rollup: work landed under a feature that had already
		// rolled up, so the computed done no longer holds and the feature returns
		// to active.
		if f.Status == "done" && workLandedUnderFeature(tree, f) {
			r, rerr := reopenFeature(tree, deps, owner, f)
			if rerr != nil {
				return SyncResult{}, rerr
			}
			reopened = append(reopened, r)
		}
	}

	// The epic pass runs after every feature has settled, so an epic reads the
	// feature statuses this same run just wrote rather than the ones it started
	// with: the last feature crossing and the epic crossing it earns land in one
	// sync, and so do a feature reopening and the epic reopening behind it.
	epicCrossings, epicReopened, epicSkipped, err := syncEpics(tree, deps, owner, rev, tip)
	if err != nil {
		return SyncResult{}, err
	}
	crossings = append(crossings, epicCrossings...)
	reopened = append(reopened, epicReopened...)
	skipped = append(skipped, epicSkipped...)

	// The shelf pass runs last, so a carried bug or chore reads the carrier
	// status the two passes above just wrote rather than the one this run
	// started with (docs/spec/cli-surface.md).
	workCrossings, workAwaiting, err := syncWorkItems(tree, deps, owner, rev, tip, cache, attested)
	if err != nil {
		return SyncResult{}, err
	}
	crossings = append(crossings, workCrossings...)

	due := checksDue(tree)

	return SyncResult{
		Crossings:  crossings,
		Reopened:   reopened,
		Skipped:    skipped,
		Into:       into,
		IntoSource: source,
		Awaiting:   append(awaitingNodes(tree, seen), workAwaiting...),
		Attested:   named,
		ChecksDue:  due,
	}, nil
}

// The integration-branch source tokens resolveInto reports, so a refusal can name
// where the value it refused came from and the human knows what to change.
const (
	intoSourceFlag     = "--into"
	intoSourceManifest = "git.integration_branch"
	intoSourceHead     = "HEAD"
)

// resolveInto answers which branch sync observes merges into, and where that
// answer came from. First --into, then the manifest's git.integration_branch,
// then the name of the branch HEAD points at. Nothing resolves only when HEAD is
// detached and neither of the first two is set, and that refuses rather than
// guesses.
//
// The literal string "HEAD" is never the resolved value again, and that is the
// whole point: sync used to default Into to "HEAD", so an agent standing on its
// own feature branch asked git whether that branch was an ancestor of itself,
// which is trivially true, and got a silent done crossing no merge had earned.
// The name of the branch HEAD is on is a different thing from "HEAD": it can be
// compared against a feature's recorded branch, which is what makes the refusal
// below possible.
//
// Standing on the current branch is deliberately the last resort rather than the
// first, and equally deliberately still present. It is the compatibility spine:
// for every project that works today, "the branch HEAD is on" designates exactly
// the commit "HEAD" designated, so a project integrating on master or develop
// keeps observing master or develop with no manifest edit and no behavior change.
// That is also why nothing here consults origin/HEAD or assumes main: a detected
// default would silently observe merges into a branch the team does not integrate
// on and strand every crossing.
func resolveInto(m *store.Manifest, in SyncInput, headBranch func() (string, error)) (string, string, error) {
	if v := strings.TrimSpace(in.Into); v != "" {
		return v, intoSourceFlag, nil
	}
	if m != nil {
		if v := strings.TrimSpace(m.Git.IntegrationBranch); v != "" {
			return v, intoSourceManifest, nil
		}
	}
	h, err := headBranch()
	if err != nil {
		return "", "", err
	}
	if h != "" {
		return h, intoSourceHead, nil
	}
	return "", "", &IntoUnresolvedError{}
}

// integrationTip is the one integration commit a sync observes. Commit is the
// full object ID the selected integration branch resolved to when the run
// began; Ref is that commit as the git_commit RevisionRef every crossing,
// policy violation, and completion record names.
type integrationTip struct {
	Commit string
	Ref    store.RevisionRef
}

// resolveIntegrationTip resolves the selected integration name to its commit
// exactly once. A plain branch name is asked about as refs/heads/<name> first,
// so a same-named tag cannot answer for the branch; anything else (a
// remote-tracking ref such as origin/main, or an explicit --into HEAD) resolves
// as written. A name that resolves to no commit is refused: there is nothing to
// observe merges into, and the checkout's HEAD is never a stand-in.
func resolveIntegrationTip(rev vcs.Revisions, into, source string) (integrationTip, error) {
	commit, err := rev.RefCommit(branchRef(into))
	if err != nil {
		return integrationTip{}, err
	}
	if commit == "" && !strings.HasPrefix(into, "refs/") {
		if commit, err = rev.RefCommit(into); err != nil {
			return integrationTip{}, err
		}
	}
	if commit == "" {
		return integrationTip{}, &IntegrationMissingError{Into: into, Source: source}
	}
	ref, err := rev.CommitRef(commit)
	if err != nil {
		return integrationTip{}, err
	}
	return integrationTip{Commit: commit, Ref: ref}, nil
}

// guardIntoNotFeatureBranch refuses the whole command when the resolved
// integration ref is a live task or aggregate branch. A branch is always its
// own ancestor, so that comparison can never be evidence of a merge; observing it
// could only ever record a done the human's merge never earned. It runs before
// any observation and before any write, so a refused sync changes nothing.
//
// Identity is compared by name, never by sha: a fast-forward merge legitimately
// leaves the integration ref and the feature branch pointing at the same commit,
// so sha equality is not evidence of misuse and refusing on it would break the
// fast-forward path.
//
// done and cancelled features are excluded. A terminal feature cannot cross
// again, so its recorded branch can never produce a false crossing, and including
// it would let one old feature that happened to be recorded on the integration
// branch wedge sync permanently.
func guardIntoNotFeatureBranch(tree *store.Tree, into, source string, headBranch func() (string, error)) error {
	canonHead := ""
	if raw := strings.TrimSpace(into); raw == "HEAD" || raw == "@" {
		h, err := headBranch()
		if err != nil {
			return err
		}
		if h == "" {
			// A literal HEAD on a detached checkout names no branch, so there is
			// nothing to compare a feature's branch against and the guard would
			// wave the command through with its one check disabled. It is the same
			// unanswerable question resolveInto refuses on a bare detached HEAD, so
			// it gets the same refusal rather than a silent pass. The typed value
			// rides along: this project may well declare an integration branch, and
			// the bare-detached wording ("none is set") would be false here.
			return &IntoUnresolvedError{Into: raw}
		}
		canonHead = h
	}
	target := canonicalBranch(into, canonHead)
	if target == "" {
		return nil
	}

	guard := func(noun, id, key, branch string) error {
		if branch != "" && canonicalBranch(branch, canonHead) == target {
			return &IntoIsFeatureBranchError{Into: into, Source: source, Noun: noun,
				NodeID: id, Key: key, Branch: branch}
		}
		return nil
	}
	for _, f := range sortedFeatures(tree) {
		for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
			if tk.Status != statusDone && tk.Status != statusCancelled {
				if err := guard(store.NounTask, tk.ID, deref(tk.Key), tk.Branch); err != nil {
					return err
				}
			}
		}
		if f.Status != statusDone && f.Status != statusCancelled {
			if err := guard(store.NounFeature, f.ID, deref(f.Key), f.Branch); err != nil {
				return err
			}
		}
	}
	for _, e := range sortedEpics(tree) {
		if e.Status != statusDone && e.Status != statusCancelled && e.Status != "archived" {
			if err := guard(store.NounEpic, e.ID, deref(e.Key), e.Branch); err != nil {
				return err
			}
		}
	}
	return nil
}

// guardAttestIntoIsDeclared refuses --accept-fast-forward whenever the ref this
// run resolved as into is not the project's DECLARED integration branch. It runs
// before any observation and before any write, and it is inert without the flag:
// plain observation resolves into exactly as it always did.
//
// This is the half of the safety property that is about into rather than about
// the branch. attestContained will only promote an observation that already means
// "into contains this branch, with nothing to witness how", which is airtight
// about the branch and says nothing at all about which ref into is. Three shapes
// walked straight through that gap, each recording done for work no integration
// branch carried:
//
//   - into naming the feature's own branch in any spelling canonicalBranch does
//     not reduce (heads/x, x^0, x@{0}, origin/x, HEAD^0 on a detached checkout),
//     which the name-based self-merge guard cannot catch and where containment is
//     the trivial "a branch contains itself";
//   - into resolving, with no --into typed at all, to the branch the checkout
//     happened to be standing on, which in a project that declares no
//     integration branch can be a scratch branch cut off the feature itself;
//   - into naming any other real branch that merely contains the feature branch,
//     such as a release cut taken off it.
//
// Name canonicalization cannot close the second and third: those are genuinely
// different branches, and containment in them is a true observation about the
// wrong question. So the rule is stated positively instead. The human is
// asserting "this landed on our trunk", Tiller knows which branch that is only
// because the project declares it, and an assertion measured against anything
// else is not the assertion the flag means.
//
// A project declaring no git.integration_branch therefore cannot use the flag at
// all. That is a deliberate narrowing, and it fails in the safe direction: init
// seeds the field, the refusal names the one line to add, and plain observation
// is untouched. An unreducible spelling of the declared branch (heads/main for
// main) is refused for the same reason, since Tiller compares the names it can
// reduce and declines to guess about the ones it cannot.
func guardAttestIntoIsDeclared(tree *store.Tree, in SyncInput, into, source string, headBranch func() (string, error)) error {
	if len(in.AcceptFastForward) == 0 {
		return nil
	}
	declared := ""
	if tree.Manifest != nil {
		declared = strings.TrimSpace(tree.Manifest.Git.IntegrationBranch)
	}
	if declared == "" {
		return &AttestIntoNotDeclaredError{Into: into, Source: source}
	}
	canonHead := ""
	if raw := strings.TrimSpace(into); raw == "HEAD" || raw == "@" {
		h, err := headBranch()
		if err != nil {
			return err
		}
		canonHead = h
	}
	if canonicalBranch(into, canonHead) != canonicalBranch(declared, "") {
		return &AttestIntoNotDeclaredError{Into: into, Source: source, Declared: declared}
	}
	return nil
}

// canonicalBranch reduces a ref to the branch name it designates, so the
// self-merge guard compares like with like: refs/heads/x and x are one branch,
// and HEAD (or its @ alias) designates whatever branch the checkout is on. A form
// that cannot be reduced is returned as written; the comparison then simply does
// not match, which is the safe direction.
//
// A remote-tracking ref is left alone on purpose. Observing merges into
// origin/main while standing on a feature branch is a legitimate thing to ask
// for, and it is not the branch the checkout is on.
func canonicalBranch(ref, headBranch string) string {
	r := strings.TrimSpace(ref)
	if r == "HEAD" || r == "@" {
		return strings.TrimSpace(headBranch)
	}
	return strings.TrimPrefix(r, "refs/heads/")
}

// branchRef fully qualifies a recorded branch name before it reaches git, so
// every probe observeMerge makes asks about the BRANCH and not about whatever
// else that name happens to designate.
//
// git's rev-parse precedence resolves a bare name from refs/tags before
// refs/heads. A tag sharing a feature branch's name therefore answered all six of
// observeMerge's questions about the tag: `git tag tiller/foundation/thing main`
// made an entirely unmerged branch read as contained in main, which is the one
// answer an assertion is allowed to act on. git prints "warning: refname is
// ambiguous" on stderr and Tiller never sees it.
//
// This is not a change to the witness logic or to the content fallback. Every
// predicate is the one it was; only the ref they are asked about stopped being
// ambiguous. into is deliberately left alone: it may legitimately be a
// remote-tracking ref such as origin/main, which refs/heads/ would break.
func branchRef(branch string) string {
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return branch
	}
	return "refs/heads/" + branch
}

// reviewedAggregate selects only the recorded review branch at the resolved
// coarse grain. Empty id means task-grain observation; an unsubmitted aggregate
// still names its branch for diagnostics but cannot supply merge evidence.
func reviewedAggregate(tree *store.Tree, f *store.Feature) (id, branch string, recorded bool) {
	epic := epicByKey(tree, f.EpicKey)
	switch store.ResolvedReviewGrain(tree.Manifest, epic, f) {
	case store.ReviewGrainFeature:
		return f.ID, f.Branch, f.PR != nil || f.ReviewRef != nil
	case store.ReviewGrainEpic:
		if epic != nil {
			return epic.ID, epic.Branch, epic.PR != nil || epic.ReviewRef != nil
		}
		// A malformed tree must not fall back to observing task merges.
		return f.ID, "", false
	default:
		return "", "", false
	}
}

// mergeObs is one feature's merge observation: whether the integration branch
// carries the branch's work, on what evidence, and (for content evidence) the
// commit that carried it.
//
// NoMergeWitness records the one outcome that is neither "merged" nor "not
// merged": the branch is contained in the integration branch, but only on that
// branch's own first-parent line, where a fast-forward merge and a branch that
// never diverged leave byte-identical repositories (see observeMerge). It is not
// a crossing, and the zero-crossing readout names it apart from the rest so the
// human is told which one they are looking at. It is also the exact set
// --accept-fast-forward may act on, and the only field attestContained reads.
type mergeObs struct {
	Branch              string
	Merged              bool
	Basis               string // "ancestry" | "content" | "human_attested" | "" when not merged
	Commit              string // full sha of the matching commit, content basis only
	BranchExists        bool
	NoMergeWitness      bool
	ReviewMissing       bool
	AggregateIncomplete bool
}

// The evidence a done crossing can rest on. One of the four is written on every
// crossing, so a reader never has to interpret an absent basis
// (artifact-schema.md, Done basis, is canonical for the enum).
//
// What each token means:
//
//   - ancestry: the branch's tip is contained in the integration branch with a
//     merge commit to witness it.
//   - content: the branch's whole net change is present under the same stable
//     patch-id, which is what a squash leaves behind.
//   - rollup: the node's non-cancelled children are all done.
//   - human_attested: a human ran `tiller sync --accept-fast-forward <feature>`
//     and asserted a merge Tiller could observe only as containment, which is
//     what a fast-forward leaves. It is written on the node and on the event so
//     an auditor can tell an asserted crossing from an observed one without
//     inferring it from an absence.
//
// Which token each grain records, and where that is decided:
//
//	task:    ancestry, content, or human_attested (syncTasks)
//	feature: rollup or human_attested (featureBasis)
//	epic:    rollup (crossEpicDone)
//	bug, chore: ancestry, content, human_attested, or carried (syncWorkItems)
const (
	basisAncestry      = "ancestry"
	basisContent       = "content"
	basisRollup        = "rollup"
	basisHumanAttested = "human_attested"
	// carried: a shelf node crossed because its resolved_by carrier's own done
	// landed (docs/spec/cli-surface.md); sync
	// writes it and nothing else does.
	basisCarried = "carried"
)

// observeMerge answers whether into carries branch's work, and it has exactly one
// job beyond that: never to answer yes for work no merge put there. Ancestry is
// tried first, then content.
//
// ANCESTRY, and why containment alone is not evidence. "branch is an ancestor of
// into" answers "is everything on branch already on into", which is not the same
// question as "did into gain branch's work by a merge". A branch that never
// diverged from into satisfies it for a trivial reason: `git branch feat main`,
// a `git branch -f` onto the integration branch, or an agent that recorded a
// branch and then committed its work somewhere else all leave a ref that is an
// ancestor because it carries nothing. Reading that as a merge is fabrication of
// the worst kind, and it fires under a completely correct configuration.
//
// So ancestry now requires a merge witness: the branch's tip must NOT lie on
// into's own first-parent line. A commit that is contained in into but off its
// trunk can only have arrived through a merge commit's second-parent side, which
// is exactly what `git merge --no-ff` and GitHub's merge button leave behind.
//
// THE COST, stated plainly because it is a real one. A fast-forward merge puts
// the branch's commits ON into's trunk, which is byte for byte what a branch that
// never diverged also leaves: same DAG, same refs, same trees. No observation
// separates them, and Tiller records the durable meaning of "done", so it refuses
// to guess and leaves the node in review rather than cross it on a coin flip.
// Fast-forward merges are therefore no longer observed; a merge commit or a squash
// is. The zero-crossing readout names this case on its own so it is diagnosable
// rather than mysterious.
//
// Nothing below softens that. The way out is not a weaker observation, it is a
// human assertion: `tiller sync --accept-fast-forward <feature>` crosses a node in
// exactly this state on the human's word and records the basis "human_attested"
// so the record never claims otherwise. observeMerge itself is unchanged by the
// flag and answers the same question it always did; NoMergeWitness is simply the
// one answer an assertion is allowed to act on (see attestContained).
//
// CONTENT, and its false positives, which are the whole risk of the fallback. A
// content match is not proof of a merge, and that is irreducible: content carries
// no provenance. If a commit on the integration line carries the branch's entire
// net patch, the fallback fires whether that commit came from squashing this
// branch or from a human applying the identical change some other way. What it
// cannot do is fire when the branch's change is absent from the integration
// branch, so the residual failure is misattribution (which act put the work
// there), not fabrication (crossing work that landed nowhere).
//
// Five unconditional constraints keep even misattribution narrow:
//
//  1. the branch must have a non-empty net patch, so a branch with no unique
//     change cannot match trivially;
//  2. the branch's WHOLE net change must equal a SINGLE commit's patch, so
//     partial overlap with an unrelated commit that touches some of the same
//     lines never fires;
//  3. the patch-id match must survive a whitespace-faithful re-comparison of the
//     two patches, because `git patch-id` strips whitespace before hashing and
//     would otherwise call two different indentations of one line the same
//     change, which on YAML or Python they are not;
//  4. the matching commit must lie after the merge base, so a change already
//     present at the branch point is not read as evidence the branch landed;
//  5. only the newest contentScanLimit first-parent commits of into are
//     candidates, so work carried in on some other branch's second-parent side,
//     and any ancient coincidental match, are both out of reach.
//
// The residual case is a human who cherry-picks a single-commit branch onto the
// integration branch and abandons the branch. There the change was integrated by
// a human and recording done describes what actually happened, even though no
// merge did it. That reading is accepted.
func observeMerge(rev vcs.Revisions, branch, into string, cache *patchCache) (mergeObs, error) {
	return observeMergeRef(rev, branch, branchRef(branch), into, cache)
}

// observeMergeRef applies the same merge evidence rules to an explicit ref.
// Transfer uses the recorded submission head so moving or deleting its branch
// cannot erase proof that the submitted work already landed.
func observeMergeRef(rev vcs.Revisions, branch, ref, into string, cache *patchCache) (mergeObs, error) {
	obs := mergeObs{Branch: branch}
	if branch == "" {
		return obs, nil
	}

	merged, err := rev.IsMergedInto(ref, into)
	if err != nil {
		return mergeObs{}, err
	}
	if merged {
		onTrunk, terr := rev.FirstParentContains(into, ref)
		if terr != nil {
			return mergeObs{}, terr
		}
		if !onTrunk {
			return mergeObs{Branch: branch, Merged: true, Basis: basisAncestry, BranchExists: true}, nil
		}
		// Contained, but with nothing to witness how. Falling through to content
		// would be wasted work: the merge base of a contained branch is the branch
		// itself, so its net patch is empty by construction.
		obs.BranchExists = true
		obs.NoMergeWitness = true
		return obs, nil
	}

	// A branch ref that is gone has nothing to diff. That is the deleted-local-
	// branch case, reported rather than detected.
	exists, err := rev.RefExists(ref)
	if err != nil {
		return mergeObs{}, err
	}
	if !exists {
		return obs, nil
	}
	obs.BranchExists = true

	base, err := rev.MergeBase(ref, into)
	if err != nil {
		return mergeObs{}, err
	}
	if base == "" {
		return obs, nil
	}

	want, err := rev.PatchID(base, ref)
	if err != nil {
		return mergeObs{}, err
	}
	if want == "" {
		return obs, nil
	}
	wantText, err := rev.PatchDigest(base, ref)
	if err != nil {
		return mergeObs{}, err
	}
	if wantText == "" {
		return obs, nil
	}

	candidates, err := cache.lookup(rev, into, want)
	if err != nil {
		return mergeObs{}, err
	}
	for _, commit := range candidates {
		// The match has to lie after the branch point. A commit at or before the
		// merge base carries a change the branch already had, which is not evidence
		// that the branch landed.
		atOrBeforeBase, aerr := rev.IsMergedInto(commit, base)
		if aerr != nil {
			return mergeObs{}, aerr
		}
		if atOrBeforeBase {
			continue
		}

		// Confirm the id against the patch text itself. patch-id is the cheap
		// filter; this is what makes the answer mean "the same change" rather than
		// "the same change once whitespace is thrown away".
		gotText, gerr := rev.PatchDigest(commit+"^1", commit)
		if gerr != nil {
			return mergeObs{}, gerr
		}
		if gotText == "" || gotText != wantText {
			continue
		}

		obs.Merged = true
		obs.Basis = basisContent
		obs.Commit = commit
		return obs, nil
	}
	return obs, nil
}

// patchCache holds the integration branch's first-parent patch-ids, computed at
// most once per sync and only when some feature actually reaches the content
// fallback, so a sync with nothing to fall back on pays nothing at all.
//
// Every commit sharing a patch-id is kept, newest first, rather than only the
// newest. Duplicates are real (a revert and a reapply share one id, and so do two
// commits that differ only in whitespace), and the caller has two further tests
// to apply to each candidate; collapsing them here would hide a commit that
// passes those tests behind one that does not.
type patchCache struct {
	byPatch map[string][]string
	ready   bool
}

func (c *patchCache) lookup(rev vcs.Revisions, into, want string) ([]string, error) {
	if !c.ready {
		pairs, err := rev.FirstParentPatchIDs(into, contentScanLimit)
		if err != nil {
			return nil, err
		}
		c.byPatch = make(map[string][]string, len(pairs))
		for _, p := range pairs {
			if p.PatchID == "" || p.Commit == "" {
				continue
			}
			c.byPatch[p.PatchID] = append(c.byPatch[p.PatchID], p.Commit)
		}
		c.ready = true
	}
	return c.byPatch[want], nil
}

// attestContained is the whole of --accept-fast-forward's effect on an
// observation, and the whole of its safety property, in one function so there is
// one place to read and one place to test.
//
// It promotes ONLY an observation whose NoMergeWitness is set. observeMerge
// assigns that field on exactly one path, the one reached after
// rev.IsMergedInto(branch, into) answered yes, whose documented meaning is that
// every commit reachable from branch is reachable from into. So:
//
//   - work the integration branch does not contain fails --is-ancestor, never
//     reaches the assignment, and is returned unchanged (and therefore uncrossed);
//   - a branch that no longer resolves is reported as not merged by IsMergedInto,
//     which probes the ref before it errors, so it lands in the same untouched
//     bucket;
//   - an observation that already succeeded (ancestry or content) is returned
//     unchanged too, so an assertion never overwrites evidence Tiller has.
//
// The flag widens the basis, never the candidate set: no git call is added, no
// predicate is relaxed, and the promotion cannot invent containment.
func attestContained(mo mergeObs) mergeObs {
	if !mo.NoMergeWitness {
		return mo
	}
	mo.Merged = true
	mo.Basis = basisHumanAttested
	mo.NoMergeWitness = false
	return mo
}

// attestedObservations classifies every node the human named on
// --accept-fast-forward, before the walk and before any write. It returns the
// observations to use for those nodes (promoted where the assertion applies)
// and the deduped named ids in the order named, or an AttestNotEligibleError
// naming every node that is out of the candidate set.
//
// The named node must own the review surface: a task at task grain, a reviewed
// feature or epic at coarse grain, or an uncarried shelf node.
//
// The refusal is all-or-nothing. A human who named four nodes and got one wrong
// should re-run one corrected command, not go looking for which half of their
// assertion landed, and an assertion is irreversible in a way an observation is
// not: done is a merge, and you do not un-merge.
//
// An id that names no such node is a misuse rather than an ineligible node. The
// CLI resolves a reference to one of those ids before calling, so this is
// unreachable through the command; it is here so a caller that skips that step
// is told, instead of having its assertion silently dropped.
func attestedObservations(tree *store.Tree, rev vcs.Revisions, ids []string, into, intoCommit string, cache *patchCache, recovered map[string]*recoveredSync) (map[string]mergeObs, []string, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	out := make(map[string]mergeObs, len(ids))
	named := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	var ineligible []AttestNotEligible

	for _, id := range ids {
		// Naming a node twice is one assertion, and it is one line in whichever
		// answer comes back, so the dedup sits ahead of both the eligible and the
		// ineligible paths.
		if seen[id] {
			continue
		}
		seen[id] = true

		var branch, noun, key string
		var would bool
		if tk := tree.Tasks[id]; tk != nil {
			f := tree.Features[tk.FeatureID]
			noun, key, branch = store.NounTask, deref(tk.Key), tk.Branch
			if f != nil {
				aggregateID, _, _ := reviewedAggregate(tree, f)
				would = (aggregateID == "" || tk.StartedGrain == store.ReviewGrainTask) && tk.Status == "in_review"
			}
		} else if f := tree.Features[id]; f != nil {
			aggregateID, aggregate, recorded := reviewedAggregate(tree, f)
			branch, noun, key = f.Branch, store.NounFeature, deref(f.Key)
			if aggregateID == f.ID && recorded {
				var err error
				would, err = aggregateWouldCross(tree, rev, f, aggregate, recovered[id])
				if err != nil {
					return nil, nil, err
				}
			}
		} else if e := tree.Epics[id]; e != nil {
			branch, noun, key = e.Branch, store.NounEpic, deref(e.Key)
			for _, f := range sortedFeatures(tree) {
				aggregateID, aggregate, recorded := reviewedAggregate(tree, f)
				if aggregateID != e.ID || !recorded {
					continue
				}
				crosses, err := aggregateWouldCross(tree, rev, f, aggregate, recovered[id])
				if err != nil {
					return nil, nil, err
				}
				would = would || crosses
			}
		} else if w, ok := tree.WorkNode(id); ok {
			branch, noun, key, would = observationWorkBranch(tree.Manifest, w), w.Noun(), deref(w.NodeKey()), w.NodeStatus() == "in_review" && !workNodeCarried(tree, w)
		} else {
			reviewed := append([]string{store.NounTask, store.NounFeature, store.NounEpic}, store.ShelfNouns()...)
			return nil, nil, &usageError{reason: "unknown_node",
				msg: fmt.Sprintf("--accept-fast-forward names %s, which is not a %s in this project", id, englishList("or", reviewed...))}
		}
		named = append(named, id)
		if !would {
			ineligible = append(ineligible, notEligible(noun, id, key, branch, AttestReasonNothingToCross))
			continue
		}

		var mo mergeObs
		if recovery := recovered[id]; recovery != nil {
			mo = recovery.obs
		} else {
			var err error
			mo, err = observeMerge(rev, branch, intoCommit, cache)
			if err != nil {
				return nil, nil, err
			}
		}

		switch {
		case mo.Merged:
			// Already observed. Stored unchanged so the crossing records the
			// evidence rather than the assertion.
		case mo.NoMergeWitness:
			mo = attestContained(mo)
		case branch == "" || !mo.BranchExists:
			ineligible = append(ineligible, notEligible(noun, id, key, branch, AttestReasonBranchMissing))
			continue
		default:
			rec, rerr := computeRecovery(rev, branch, intoCommit)
			if rerr != nil {
				return nil, nil, rerr
			}
			n := notEligible(noun, id, key, branch, AttestReasonNotContained)
			n.Recovery = rec
			ineligible = append(ineligible, n)
			continue
		}

		out[id] = mo
	}

	if len(ineligible) > 0 {
		return nil, nil, &AttestNotEligibleError{Into: into, Nodes: ineligible}
	}
	return out, named, nil
}

// notEligible builds one refused-node entry for AttestNotEligibleError.
func notEligible(noun, id, key, branch, reason string) AttestNotEligible {
	return AttestNotEligible{Noun: noun, NodeID: id, Key: key, Branch: branch, Reason: reason}
}

// recoveryGit is the read-only surface a not_contained refusal's remediation
// needs beyond vcs.Revisions: Task 2's upstream-resolution arm (UnpushedTip),
// the short sha a prescription names a commit by (ShortSHA), and the worktree
// read the branch-f arm depends on (WorktreeFor). It is declared here rather
// than widened onto vcs.Revisions itself, for the reason UnpushedTip's own doc
// comment already gives: sync's ordinary observation never needs any of the
// three, only this one remediation path does. *vcs.Git, the sole concrete
// Revisions, satisfies it, so a rev asserted against it here never fails in
// production; the assertion exists so a hypothetical narrower stub degrades to
// no remediation rather than a panic.
type recoveryGit interface {
	UnpushedTip(branch string) (unpushed bool, upstream string, ok bool, err error)
	ShortSHA(ref string) (string, error)
	WorktreeFor(branch string) (path string, ok bool, err error)
}

// computeRecovery gathers the honest remediation facts for a not_contained
// refusal: branch's tip is not contained in into (that is the whole reason
// this is being asked), but its pushed remote-tracking counterpart might be,
// which is the shape a state commit left unpushed before its PR merged
// produces (the field case). It returns nil, nil whenever there is
// nothing honest to prescribe: no upstream resolves for branch (UnpushedTip's
// own offline answer), or one resolves but into does not contain it either,
// in which case nothing merged this branch by any reading and inventing a
// remedy would be worse than the bare refusal.
//
// rev is asserted against recoveryGit rather than widened onto the parameter
// type; see that interface's own doc for why. UnpushedTip and WorktreeFor take
// branch's bare name, matching how the own caller
// (internal/core/review's ForTask) and WorktreeFor's own doc comment already
// use them.
// The local tip's ShortSHA is asked about branchRef(branch) instead: ShortSHA
// shells rev-parse, which resolves a bare name from refs/tags before
// refs/heads, the identical ambiguity branchRef's own doc comment describes,
// so the local tip is qualified before naming it the same way every other
// predicate in this file already qualifies branch before asking git anything
// about it.
func computeRecovery(rev vcs.Revisions, branch, into string) (*Recovery, error) {
	rg, ok := rev.(recoveryGit)
	if !ok {
		return nil, nil
	}

	_, upstream, ok, err := rg.UnpushedTip(branch)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	contained, err := rev.IsMergedInto(upstream, into)
	if err != nil {
		return nil, err
	}
	if !contained {
		return nil, nil
	}

	remoteTip, err := rg.ShortSHA(upstream)
	if err != nil {
		return nil, err
	}
	localTip, err := rg.ShortSHA(branchRef(branch))
	if err != nil {
		return nil, err
	}
	worktree, _, err := rg.WorktreeFor(branch)
	if err != nil {
		return nil, err
	}

	return &Recovery{RemoteTip: remoteTip, LocalTip: localTip, Upstream: upstream, Worktree: worktree}, nil
}

// aggregateWouldCross prevents an assertion from silently accepting an aggregate
// that cannot carry any riding task across the done boundary.
func aggregateWouldCross(tree *store.Tree, rev vcs.Revisions, f *store.Feature, branch string, recovery *recoveredSync) (bool, error) {
	id, _, _ := reviewedAggregate(tree, f)
	if aggregateRecovery(tree, id) != nil && (f.Status == statusDone || f.Status == statusCancelled || aggregateRoundDone(tree, id)) {
		return false, nil
	}
	for _, tk := range tree.FeatureTasks[f.ID] {
		// A task-grain task crosses on its own merge, never the aggregate's.
		if tk.Status != "in_review" || tk.StartedGrain == store.ReviewGrainTask {
			continue
		}
		if recovery != nil {
			if recovery.tasks[tk.ID] != nil {
				return true, nil
			}
			continue
		}
		included, err := TaskIncludedInAggregate(rev, tk, branch)
		if err != nil {
			return false, err
		}
		if included {
			return true, nil
		}
	}
	return false, nil
}

// awaitingNodes names every still-reviewed task by its own key and observation.
// Coarse tasks retain the aggregate's diagnostics plus their inclusion result.
// Ordering follows the same dependency-aware sibling order as other surfaces.
func awaitingNodes(tree *store.Tree, seen map[string]mergeObs) []Awaiting {
	var out []Awaiting
	for _, f := range sortedFeatures(tree) {
		// A rendered list of sibling tasks reads in the order depends_on
		// records (artifact-schema.md, Tasks: Ordering), the same rank every
		// other surface gets from gen.OrderTasks.
		for _, tk := range gen.OrderTasks(tree, tree.FeatureTasks[f.ID], f.Order) {
			if tk.Status != "in_review" {
				continue
			}
			mo, observed := seen[tk.ID]
			if !observed && tk.StartedGrain != store.ReviewGrainTask {
				id, branch, recorded := reviewedAggregate(tree, f)
				if id != "" && !recorded {
					mo.Branch, mo.ReviewMissing = branch, true
				}
			}
			out = append(out, Awaiting{
				Noun: store.NounTask, ID: tk.ID, Key: deref(tk.Key),
				Branch: mo.Branch, BranchMissing: observed && !mo.BranchExists, NoMergeWitness: mo.NoMergeWitness,
				ReviewMissing: mo.ReviewMissing, AggregateIncomplete: mo.AggregateIncomplete,
			})
		}
	}
	return out
}

// crossTaskDone persists only the task whose own merge or aggregate inclusion
// was observed. Siblings never inherit an observation by sharing a feature.
func crossTaskDone(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, f *store.Feature, live *store.Task, observed store.RevisionRef, basis, match string, recovery *recoveredSync) (Crossing, error) {
	tk, ymlRel, snapshot, err := loadTaskFile(tree, f, live)
	if err != nil {
		return Crossing{}, err
	}
	if recovery != nil {
		tk.FeatureID = f.ID
		if !recoveredTaskIdentityMatches(tk, recovery.tasks[tk.ID]) || tk.Status != "in_review" || tk.PR != nil {
			return Crossing{}, recoveredSyncRefusal(recovery.id, tk.ID, "recovery_task_changed", "Restore the frozen task submission and scope before syncing recovered work.")
		}
	}
	before := tk.Status
	if err := lifecycle.Apply(tk, "sync", lifecycle.Payload{}); err != nil {
		return Crossing{}, err
	}
	tk.DoneBasis = basis
	violations := recordPolicyViolations(tree.Manifest, store.NounTask, &tk.GatesBlock, observed)
	if recovery != nil {
		recordHistoricalPolicyViolations(&tk.GatesBlock, historicalPolicyForNode(recovery.record, tk.ID), observed)
	}
	ydata, err := yaml.Marshal(tk)
	if err != nil {
		return Crossing{}, err
	}
	if err := commitRecoveredCrossing(tree, deps, owner, rev,
		TransitionInput{Verb: "sync", SyncBasis: basis, SyncMatch: match},
		store.NounTask, tk.ID, ymlRel, ydata, snapshot, before, tk.Status, recovery, tk); err != nil {
		return Crossing{}, err
	}
	live.Status = tk.Status
	live.DoneBasis = tk.DoneBasis
	live.PolicyViolations = tk.PolicyViolations
	return Crossing{
		Noun: store.NounTask, ID: tk.ID, Key: deref(tk.Key), Status: tk.Status,
		Observed: observed.Value, Basis: basis, MatchedCommit: match,
		PolicyViolations: violations, HistoricalPolicyViolations: historicalCrossingViolations(tk.PolicyViolations),
	}, nil
}

// featureBasis preserves coarse human assertions on the feature rollup.
// Individual task assertions do not become assertions about their whole parent.
func featureBasis(mo mergeObs) string {
	if mo.Basis == basisHumanAttested {
		return basisHumanAttested
	}
	return basisRollup
}

// crossFeatureDone records a feature's rollup crossing onto feature.yml, set
// directly rather than through the lifecycle engine (see the file's DIVERGENCE
// note). It writes the policy_violations for any required gate that was not
// green at the merge, persists the file with one event, and updates the
// in-memory feature. basis is written to the node's done_basis as well as to the
// event: an asserted crossing and a computed one are not the same record, and a
// reader of feature.yml must be able to tell them apart without inferring it
// from an absence.
//
// There is no match parameter, unlike the task crossing. A matched commit is the
// evidence pointer a content observation leaves, and no feature crossing is an
// observation, so there is never a commit for this grain to point at.
func crossFeatureDone(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, f *store.Feature, tip integrationTip, basis string, historical []store.HistoricalPolicySnapshot, recovery *recoveredSync) (Crossing, error) {
	observed := tip.Ref
	relDir := featureRelDir(f)
	ymlRel := filepath.Join(relDir, "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return Crossing{}, err
	}
	doc.EpicKey = f.EpicKey
	before := doc.Status
	doc.Status = "done"
	doc.DoneBasis = basis

	// Record the completion basis: the spec and eval blobs and the surface
	// fingerprints the done feature was verified against, so stale_spec drift can
	// fire on done work (artifact-schema.md, Drift control hooks). Every leg is
	// read from the integration commit this sync resolved, which is also the
	// since-when anchor.
	if err := recordVerified(relDir, &doc, rev, deps.clock().Now().UTC(), tip); err != nil {
		return Crossing{}, err
	}

	violations := recordPolicyViolations(tree.Manifest, store.NounFeature, &doc.GatesBlock, observed)
	recordHistoricalPolicyViolations(&doc.GatesBlock, historical, observed)

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Crossing{}, err
	}
	if err := commitRecoveredCrossing(tree, deps, owner, rev,
		TransitionInput{Verb: "sync", SyncBasis: basis},
		store.NounFeature, doc.ID, ymlRel, ydata, snapshot, before, doc.Status, recovery, nil); err != nil {
		return Crossing{}, err
	}

	f.Status = doc.Status
	f.DoneBasis = doc.DoneBasis
	f.PolicyViolations = doc.PolicyViolations
	f.VerifiedAgainst = doc.VerifiedAgainst
	return Crossing{
		Noun: store.NounFeature, ID: doc.ID, Key: deref(doc.Key),
		Status: doc.Status, Observed: observed.Value,
		Basis: basis, PolicyViolations: violations,
		HistoricalPolicyViolations: historicalCrossingViolations(doc.PolicyViolations),
		TouchedDesign:              touchedDesign(tree, doc.Surface.Paths),
		EvalContractMismatch:       evalContractMismatch(doc),
	}, nil
}

// touchedDesign reports overlap, not drift. Confirmation and comparison belong
// to the drift sweep; this read-only join includes pending baseline docs too.
func touchedDesign(tree *store.Tree, paths []string) []string {
	if len(paths) == 0 || len(tree.Baseline.Docs) == 0 {
		return nil
	}
	paths = DesignSurfaceCoordinates(tree.Root, tree.Manifest, paths)
	var docs []string
	for _, doc := range tree.Baseline.Docs {
		touched := false
		for _, described := range doc.Describes {
			for _, surface := range paths {
				_, contains := designWithin(surface, described)
				_, within := designWithin(described, surface)
				if contains || within {
					touched = true
					break
				}
			}
			if touched {
				break
			}
		}
		if touched {
			docs = append(docs, doc.Path)
		}
	}
	sort.Strings(docs)
	return docs
}

// recordVerified populates a feature's verified_against at the done crossing,
// every leg read from the one integration commit sync resolved: the spec and
// eval revisions as git_blob RevisionRefs over that commit's
// feature.md/feature.eval.md, the commit anchor itself, plus one fingerprint per
// surface path, a git_tree with the inventory of files it covered for a
// directory, a git_blob for a file. A create feature with no surface records an
// empty surfaces list. This is the completion record stale_spec drift reads
// (artifact-schema.md, Drift control hooks; The done crossing). A path the
// integration commit lacks is a NotVersionedError the caller reports as a skip;
// it is never read from the checkout instead.
func recordVerified(relDir string, feat *store.Feature, rev vcs.Revisions, at time.Time, tip integrationTip) error {
	specRef, err := committedBlobRef(rev, tip.Commit, filepath.Join(relDir, "feature.md"), at)
	if err != nil {
		return err
	}
	evalRef, err := committedBlobRef(rev, tip.Commit, filepath.Join(relDir, "feature.eval.md"), at)
	if err != nil {
		return err
	}

	surfaces := make([]store.SurfaceFingerprint, 0, len(feat.Surface.Paths))
	for _, p := range feat.Surface.Paths {
		sf, serr := surfaceFingerprint(rev, tip.Commit, p, at)
		if serr != nil {
			return serr
		}
		surfaces = append(surfaces, sf)
	}

	feat.VerifiedAgainst = store.VerifiedAgainst{Spec: specRef, Eval: evalRef, Commit: tip.Ref, Surfaces: surfaces}
	return nil
}

// committedBlobRef pins relPath, a path under the project root, as a git_blob
// RevisionRef read from commit. Committed-tree reads resolve project-relative
// paths, so a nested project pins its own file, the same file approve
// fingerprints in the working tree.
func committedBlobRef(rev vcs.Revisions, commit, relPath string, at time.Time) (store.RevisionRef, error) {
	hash, err := rev.BlobHashAt(commit, filepath.ToSlash(relPath))
	if err != nil {
		return store.RevisionRef{}, err
	}
	return store.RevisionRef{Kind: store.KindGitBlob, Value: "git:" + hash, Path: relPath, ObservedAt: at}, nil
}

// surfaceFingerprint resolves one surface path at commit into the fingerprint a
// completion record pins: a git_tree with the inventory of files it covered
// when the path is a directory, a git_blob whose inventory is the file itself
// when it is a file. link --surface accepts existing files and directories,
// so the recorder accepts both too; demanding a tree here stranded a feature whose surface
// named one file, with sync reporting a skip every run that could never heal
// (the Lantern finding, 2026-08-07).
//
// A path that resolves to neither kind returns the tree read's error: for an
// absent path that is the plain NotVersionedError skipUnrecordable turns into
// a reported skip, and for an object of some third kind (a submodule gitlink)
// it is the more precise of the two refusals. A blob read that fails as an
// environment error still surfaces as environment, never as a skip.
func surfaceFingerprint(rev vcs.Revisions, commit, p string, at time.Time) (store.SurfaceFingerprint, error) {
	treeHash, terr := rev.TreeHashAt(commit, p)
	if terr == nil {
		inventory, ierr := rev.TreeInventoryAt(commit, p)
		if ierr != nil {
			return store.SurfaceFingerprint{}, ierr
		}
		return store.SurfaceFingerprint{
			Path: p, Kind: store.KindGitTree, Value: "git:" + treeHash,
			Inventory: inventory, ObservedAt: at,
		}, nil
	}
	var notTree *vcs.NotVersionedError
	if !errors.As(terr, &notTree) {
		return store.SurfaceFingerprint{}, terr
	}
	blobHash, berr := rev.BlobHashAt(commit, p)
	if berr != nil {
		var notBlob *vcs.NotVersionedError
		if errors.As(berr, &notBlob) {
			return store.SurfaceFingerprint{}, terr
		}
		return store.SurfaceFingerprint{}, berr
	}
	return store.SurfaceFingerprint{
		Path: p, Kind: store.KindGitBlob, Value: "git:" + blobHash,
		Inventory: []string{p}, ObservedAt: at,
	}, nil
}

// syncEpics is the epic grain of the walk: the rollup crossing and its reverse,
// in epic id order so a multi-epic run is deterministic. It reads the feature
// statuses as they stand after the feature walk, so a feature that crossed or
// reopened in this same run is already counted.
//
// The two arms are deliberately not each other's negation over an empty tree. An
// epic with no non-cancelled feature at all satisfies neither, so it neither
// crosses (there is nothing to have completed: artifact-schema.md requires at
// least one) nor reopens (nothing landed under it). A ready epic nobody has
// decomposed yet therefore stays ready rather than reading as vacuously done.
//
// archived and cancelled epics are inert here, and that is the point of the
// human's remaining gate. archive is the human's deliberate close of a done epic
// (artifact-schema.md, Epics States); recomputing an archived epic back to ready
// because someone added a feature under it would silently overturn that decision,
// which no rollup gets to do. A cancelled epic is terminal for the same reason.
func syncEpics(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions,
	tip integrationTip) ([]Crossing, []Reopening, []Skipped, error) {

	var crossings []Crossing
	var reopened []Reopening
	var skipped []Skipped

	for _, e := range sortedEpics(tree) {
		if e.Key == nil {
			continue
		}
		key := *e.Key
		switch {
		case e.Status == "ready" && epicRollupReady(tree, key):
			c, err := crossEpicDone(tree, deps, owner, rev, e, tip)
			if err != nil {
				sk, ok := skipUnrecordable(err, store.NounEpic, e.ID, key, e.Surface.Paths)
				if !ok {
					return nil, nil, nil, err
				}
				skipped = append(skipped, sk)
				continue
			}
			crossings = append(crossings, c)
		case e.Status == "done" && workLandedUnderEpic(tree, key):
			r, err := reopenEpic(tree, deps, owner, e)
			if err != nil {
				return nil, nil, nil, err
			}
			reopened = append(reopened, r)
		}
	}
	return crossings, reopened, skipped, nil
}

// crossEpicDone records an epic's rollup crossing onto epic.yml: status done,
// done_basis rollup, the completion record, and one sync.epic event. The basis is
// always basisRollup because it is the only one an epic can have: nothing below
// the feature grain is observed for an epic, and the merges that put it here were
// already recorded on the tasks and features beneath it.
//
// tip is the integration commit this sync resolved, the same value the feature
// crossings in this run carry, so every crossing of one sync names one revision.
func crossEpicDone(tree *store.Tree, deps Deps, owner store.Contributor, rev vcs.Revisions, e *store.Epic, tip integrationTip) (Crossing, error) {
	relDir := epicRelDir(e)
	ymlRel := filepath.Join(relDir, "epic.yml")

	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return Crossing{}, err
	}
	before := doc.Status
	doc.Status = "done"
	doc.DoneBasis = basisRollup

	if err := recordEpicVerified(relDir, &doc, rev, deps.clock().Now().UTC(), tip); err != nil {
		return Crossing{}, err
	}

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Crossing{}, err
	}
	if err := commitTransition(tree, deps, owner,
		TransitionInput{Verb: "sync", SyncBasis: basisRollup},
		store.NounEpic, doc.ID, ymlRel, ydata, snapshot, before, doc.Status, ""); err != nil {
		return Crossing{}, err
	}

	e.Status = doc.Status
	return Crossing{
		Noun: store.NounEpic, ID: doc.ID, Key: deref(doc.Key),
		Status: doc.Status, Observed: tip.Ref.Value, Basis: basisRollup,
	}, nil
}

// reopenEpic records the reverse: a done epic that gained a feature still to do
// returns to ready, with one sync.epic event. The done_basis and the completion
// record go with it, because both describe a done this epic no longer is: leaving
// a verified_against behind would hand stale_spec drift a baseline for a node
// that is back at work, and leaving a done_basis behind would answer "what did
// this done rest on" for a done that is not there. The event stream keeps the
// history; the node file states the present.
//
// PR and ReviewRef clear too: a merge consumes the review surface it
// represents, and a reopened epic has nothing left for that surface to
// describe. Leaving them behind left three things wrong at once:
// queueMergeEpics kept rendering a permanent ghost "tiller sync" row for a PR
// that already merged (fix item 1's own predicate change only narrows WHICH
// terminal statuses suppress it, it does nothing for a node back at `ready`);
// the coarse finish line (readout.go's coarseEpicFinishLine) never fired again
// for this round of work, since hasRecordedReview still read true; and
// Configure's Changeability guard (refuseReviewSurfaceLocked) kept
// refusing `configure epic --review-grain` on a node whose actual review
// window had reopened. Branch is deliberately left alone: it is the
// deterministic workbench name derived from the epic's own key
// (branchFor), not a review record, and every feature under an
// epic-grained epic still shares it on the next round exactly as it did
// on the first.
//
// It touches nothing else below itself, and that is a decision rather than an
// omission. reopenFeature stales its node's gates because the work those gates
// measured has changed underneath them; nothing of the kind is true here. An
// epic reopens because a NEW feature appeared beside its old ones, and the done
// features are untouched by that: their code, their contracts, and the gate runs
// that certified them all still describe exactly what they described a moment
// ago. Staling them would invalidate evidence about work nobody changed, and it
// would put every already-shipped feature back in the queue for a re-check that
// could only confirm what it already said. The epic itself has no gate block to
// stale, because acceptance lives at feature grain (artifact-schema.md, Epics).
func reopenEpic(tree *store.Tree, deps Deps, owner store.Contributor, e *store.Epic) (Reopening, error) {
	relDir := epicRelDir(e)
	ymlRel := filepath.Join(relDir, "epic.yml")

	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return Reopening{}, err
	}
	before := doc.Status
	doc.Status = "ready"
	doc.DoneBasis = ""
	doc.VerifiedAgainst = store.VerifiedAgainst{}
	doc.PR = nil
	doc.ReviewRef = nil
	doc.RecoveredReview = nil

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Reopening{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "sync"},
		store.NounEpic, doc.ID, ymlRel, ydata, snapshot, before, doc.Status, ""); err != nil {
		return Reopening{}, err
	}

	e.Status = doc.Status
	e.PR, e.ReviewRef, e.RecoveredReview = nil, nil, nil
	return Reopening{Noun: store.NounEpic, ID: doc.ID, Key: deref(doc.Key), Status: doc.Status}, nil
}

// reopenFeature is the feature-grain reverse: a done feature that gained a task
// still to do returns to active, on the same terms and for the same reason as
// reopenEpic. Its completion record clears too; sync writes a fresh one when the
// feature rolls up again.
//
// It also stales the gate results and drops the policy_violations, which
// reopenEpic has no equivalent of because an epic carries no gate block at all.
// Both are the same judgment: those records describe the crossing this reopen
// just undid, and new work has landed since. Left alone, a gates block still
// reading pass against a pre-reopen revision would let the SECOND crossing
// record a clean done on evidence that predates the work it certifies, and
// recordPolicyViolations would stay silent because the stale entries read
// green. Staling goes through lifecycle.StaleGates rather than a local loop, so
// this path and `resume` (the same event, reached by a human's hand instead of
// by observation) can never answer it differently.
//
// The violations are dropped rather than appended to for the same reason
// verified_against is: each entry names the observed revision of a crossing
// that no longer stands, and appending across every reopen/re-cross cycle would
// grow one gate's history without bound inside a field whose readers treat it
// as the current record. The next crossing writes whatever is true then, which
// after a stale-all is loud rather than silent.
//
// PR and ReviewRef clear too, reopenEpic's own twin one grain down: a merge
// consumes the review surface it represents, and a reopened feature has nothing
// left for that surface to describe. Events keep the full history
// (submit.feature still names every PR this node ever carried); the node file
// states only the present, which is a feature back at work with no review
// recorded yet. Branch is the one field this reopen does NOT clear, on purpose:
// it is the deterministic workbench name derived from the feature's own key
// (or, at epic grain, stamped from the epic's), not a review record, and the
// next round of work under this feature reuses it exactly as `start` would
// recompute it fresh (branchFor is idempotent on a non-empty branch).
func reopenFeature(tree *store.Tree, deps Deps, owner store.Contributor, f *store.Feature) (Reopening, error) {
	relDir := featureRelDir(f)
	ymlRel := filepath.Join(relDir, "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, ymlRel), &doc)
	if err != nil {
		return Reopening{}, err
	}
	doc.EpicKey = f.EpicKey
	before := doc.Status
	doc.Status = "active"
	doc.DoneBasis = ""
	doc.VerifiedAgainst = store.VerifiedAgainst{}
	lifecycle.StaleGates(&doc)
	doc.PolicyViolations = nil
	doc.PR = nil
	doc.ReviewRef = nil
	doc.RecoveredReview = nil

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return Reopening{}, err
	}
	if err := commitTransition(tree, deps, owner, TransitionInput{Verb: "sync"},
		store.NounFeature, doc.ID, ymlRel, ydata, snapshot, before, doc.Status, ""); err != nil {
		return Reopening{}, err
	}

	f.Status = doc.Status
	f.PR, f.ReviewRef, f.RecoveredReview = nil, nil, nil
	return Reopening{Noun: store.NounFeature, ID: doc.ID, Key: deref(doc.Key), Status: doc.Status}, nil
}

// recordEpicVerified populates an epic's verified_against at the rollup crossing,
// every leg read from the one integration commit sync resolved: the epic.md
// revision as a git_blob in that commit, the commit anchor itself, plus one
// fingerprint per surface path, a git_tree with the inventory of files it
// covered for a directory, a git_blob for a file. An epic with no surface
// records an empty surfaces list rather than dropping the key
// (artifact-schema.md, Epics). The commit anchor lets a later drift candidate
// say since when.
//
// There is no eval leg. An epic carries no eval file of its own: its acceptance
// bar is the sum of its features' contracts, each enforced at feature grain, so
// VerifiedAgainst.Eval stays zero here rather than pointing at a file that does
// not exist.
func recordEpicVerified(relDir string, e *store.Epic, rev vcs.Revisions, at time.Time, tip integrationTip) error {
	specRef, err := committedBlobRef(rev, tip.Commit, filepath.Join(relDir, "epic.md"), at)
	if err != nil {
		return err
	}

	surfaces := make([]store.SurfaceFingerprint, 0, len(e.Surface.Paths))
	for _, p := range e.Surface.Paths {
		sf, serr := surfaceFingerprint(rev, tip.Commit, p, at)
		if serr != nil {
			return serr
		}
		surfaces = append(surfaces, sf)
	}

	e.VerifiedAgainst = store.VerifiedAgainst{Spec: specRef, Commit: tip.Ref, Surfaces: surfaces}
	return nil
}

// epicRollupReady reports whether an epic's features say it is done: at least one
// non-cancelled feature exists and every one of them is done (artifact-schema.md,
// Epics States). A cancelled feature neither counts nor blocks, so cancelling
// work under an epic never wedges it.
func epicRollupReady(tree *store.Tree, epicKey string) bool {
	nonCancelled := 0
	for _, f := range tree.Features {
		if f.EpicKey != epicKey || f.Status == "cancelled" {
			continue
		}
		nonCancelled++
		if f.Status != "done" {
			return false
		}
	}
	return nonCancelled > 0
}

// workLandedUnderEpic reports whether a non-cancelled, non-done feature sits
// under the epic: the signal that new work landed beneath a done epic and its
// computed done no longer holds. It is stated as "something is left to do"
// rather than as the negation of epicRollupReady on purpose, so an epic whose
// features all vanished from the layout is not read as reopened by their
// absence.
func workLandedUnderEpic(tree *store.Tree, epicKey string) bool {
	for _, f := range tree.Features {
		if f.EpicKey != epicKey || f.Status == "cancelled" {
			continue
		}
		if f.Status != "done" {
			return true
		}
	}
	return false
}

// workLandedUnderFeature is the same question one level down: a non-cancelled,
// non-done task under a done feature. It deliberately does not consult the
// feature-grain gates featureRollupReady checks, because an unchecked gate is not
// new work: only a task still to do is.
func workLandedUnderFeature(tree *store.Tree, f *store.Feature) bool {
	for _, tk := range tree.Tasks {
		if tk.FeatureID != f.ID || tk.Status == "cancelled" {
			continue
		}
		if tk.Status != "done" {
			return true
		}
	}
	return false
}

// recordPolicyViolations appends a policy_violations entry for every required
// gate that was NOT green at the observed merge, under advisory enforcement
// (cli-surface.md, "The done crossing"; artifact-schema.md, Gates). A clean done
// has every required gate pass; anything else (a fail, a pending evals gate whose
// required criterion still awaits attestation, a stale or never-run required gate)
// is recorded honestly, naming the gate, the observed revision, the observed
// status, and an evidence ref. The required set is requiredGateConfigs (check.go),
// the same set check runs and submit reads, so a gate that grain never runs is
// never a finding against it; the recorded status is read from the block (a
// required gate absent from the block is read as pending). It returns the
// violating gate names in gate-name order for the crossing view.
func recordPolicyViolations(m *store.Manifest, noun string, gb *store.GatesBlock, observed store.RevisionRef) []string {
	names := make([]string, 0)

	for _, c := range requiredGateConfigs(m, noun) {
		name := c.Name
		g, recorded := gb.Gates[name]
		status := g.Status
		if !recorded || status == "" {
			status = store.GateStatusPending
		}
		if status == store.GateStatusPass {
			continue
		}
		gb.PolicyViolations = append(gb.PolicyViolations, store.PolicyViolation{
			Gate:        name,
			Observed:    observed,
			Status:      status,
			EvidenceRef: violationEvidenceRef(g),
		})
		names = append(names, name)
	}
	return names
}

// violationEvidenceRef pulls the most relevant evidence pointer off a non-green
// gate for its policy_violations entry: the gate-level evidence when present
// (build, tests), else the first per-criterion evidence the evals gate carries.
func violationEvidenceRef(g store.Gate) string {
	if len(g.Evidence) > 0 {
		return g.Evidence[0].Ref
	}
	critIDs := make([]string, 0, len(g.Criteria))
	for id := range g.Criteria {
		critIDs = append(critIDs, id)
	}
	sort.Strings(critIDs)
	for _, id := range critIDs {
		if ev := g.Criteria[id].Evidence; len(ev) > 0 {
			return ev[0].Ref
		}
	}
	return ""
}

// featureRollupReady reports whether an active feature's rollup is earned:
// every non-cancelled task done and every required feature-grain gate carrying
// a recorded result. It delegates to gen.FeatureActivity, the same predicate
// status, queue, and readout render, so a surface showing "awaiting
// sync" and this walk crossing the feature are one function
// (docs/spec/cli-surface.md). The "checked,
// not necessarily green" reading of advisory enforcement lives there now.
func featureRollupReady(tree *store.Tree, f *store.Feature) bool {
	return gen.FeatureActivity(tree, f) == gen.FeatureActivityAwaitingSync
}

// sortedFeatures returns the tree's features in id order, the deterministic
// observation order sync walks.
func sortedFeatures(tree *store.Tree) []*store.Feature {
	out := make([]*store.Feature, 0, len(tree.Features))
	for _, f := range tree.Features {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// checksDue gathers the features left in the awaiting-check phase, in
// sortedFeatures order for a deterministic confirmation.
func checksDue(tree *store.Tree) []CheckDue {
	var out []CheckDue
	for _, f := range sortedFeatures(tree) {
		if gen.FeatureActivity(tree, f) == gen.FeatureActivityAwaitingCheck {
			out = append(out, CheckDue{Noun: store.NounFeature, ID: f.ID, Key: deref(f.Key)})
		}
	}
	return out
}

// featureRelDir is the repo-relative folder of a feature, the base for its
// feature.md, feature.yml, feature.eval.md, and tasks/ subtree.
func featureRelDir(f *store.Feature) string {
	return filepath.Join(".tiller", "epics", f.EpicKey, "features", localKey(*f.Key))
}

// sortedEpics returns the tree's epics in id order, the deterministic order the
// epic pass walks, matching sortedFeatures one level down.
func sortedEpics(tree *store.Tree) []*store.Epic {
	out := make([]*store.Epic, 0, len(tree.Epics))
	for _, e := range tree.Epics {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// epicRelDir is the repo-relative folder of an epic, the base for its epic.yml
// and epic.md.
func epicRelDir(e *store.Epic) string {
	return filepath.Join(".tiller", "epics", deref(e.Key))
}

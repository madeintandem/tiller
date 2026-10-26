package serve

// This file serves the console's uniform node page, GET
// /api/node/<noun>/<address> (ui-surface.md, "Node detail"). It is the same
// thin shell the CLI's show command is: validate the noun, resolve the address
// through internal/core/resolve, and render the views projection. The address
// is everything after the noun segment because a feature's canonical key
// carries a slash (<epic-key>/<feature-key>).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/core/review"
	"github.com/madeintandem/tiller/internal/corpus"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/madeintandem/tiller/internal/workflow"
)

// NodeHandler serves the node page over one repo root: the resolver's
// id-or-key lookup, the event stream for the history timeline, and the node's
// prose file read from disk. Unknown nouns and unresolved addresses answer 404
// JSON, the read API's not-found shape.
func NodeHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, r *http.Request) (any, error) {
		noun, addr, ok := nodePath(r.URL.Path)
		if !ok {
			return nil, &httpError{http.StatusNotFound, "expected /api/node/<noun>/<address>"}
		}
		if !validNoun(noun) {
			return nil, &httpError{http.StatusNotFound,
				fmt.Sprintf("unknown noun %q (want one of epic, feature, task, question, amendment, bug, chore, tweak)", noun)}
		}
		n, err := resolve.NewResolver(t).Resolve(resolve.Ref{Noun: noun, Raw: addr})
		if err != nil {
			// Everything the resolver refuses (not found, noun mismatch,
			// ambiguous key) is "no such page" to the console, with the
			// resolver's message saying why.
			return nil, &httpError{http.StatusNotFound, err.Error()}
		}
		snapshot := executionSnapshot(r)
		baseNode, err := resolve.NewResolver(snapshot.Base).Resolve(resolve.Ref{Noun: noun, Raw: addr})
		if err != nil {
			return nil, err
		}
		evs, historyErr := events.List(snapshot.Base)
		historyError := ""
		if historyErr != nil {
			historyError = historyErr.Error()
			evs = nil
		}
		executionHistoryError := ""
		if merged, err := observedEvents(snapshot, n.NodeID(), evs); err != nil {
			executionHistoryError = err.Error()
		} else {
			evs = merged
		}
		workflowInput := mutate.ObserveWorkflowForNodesLimited(snapshot.Base, []store.Node{baseNode}, maxServedFileBytes)
		index, err := readinput.Relationships(r.Context(), t, readinput.RelationshipOptions{
			Events: evs, EventsLoaded: true, HistoryError: historyError,
			Contracts: workflowInput.Contracts, ContractErrors: workflowInput.ContractErrors, ContractMissing: workflowInput.ContractMissing,
			AllowIncompleteCorpus: true,
			MaxDocumentBytes:      maxServedFileBytes,
		})
		if err != nil {
			return nil, err
		}
		page := views.NodePageOf(t, evs, n, index)
		if page.Prose != nil {
			// The stub question is asked here, where the bytes are. The page's
			// question is the strict one (mutate.IsProseUnwritten): the console
			// draws none of a stub, so it must be sure there is nothing there,
			// where the `tiller spec feature` gate only has to be sure the
			// sentinel is still in the file. It is scoped to a file that was
			// actually read: a file that is not there is a missing file, a
			// different fact with its own honest rendering.
			content, err := readProse(r.Context(), root, page.Prose.Path)
			if errors.Is(err, corpus.ErrTooLarge) {
				return nil, tooLarge(page.Prose.Path)
			}
			if err == nil {
				page.Prose.Content = content
				page.Prose.Stub = mutate.IsProseUnwritten(page.Node.Noun, content)
			}
		}
		page.Eval = evalPayload(t, n, workflowInput)
		page.Workflow = workflow.AdviceOf(snapshot.Base, baseNode, workflowInput)
		page.ActiveBasis = basisPayload(snapshot.Base, baseNode)
		if a, ok := n.(*store.Amendment); ok && page.Amendment != nil {
			fillAmendmentValidity(t, page.Amendment, a)
		}
		badges, detail, err := driftViews(snapshot.Base, baseNode)
		if err != nil {
			// An unknown drift.checks entry must not take the WHOLE node page
			// down either, the identical reasoning the broken-event-file arm
			// above already applies (this function's own comment a few lines
			// up: "the node's state renders from its own files and the
			// history is a side dish"). A Drifted chip is at least as much a
			// side dish as the history timeline is: /api/drift's own
			// loud-failure posture (serve.go's comment at that route) is
			// right for THAT endpoint because the sweep's report IS the whole
			// payload there, so failing costs nothing else; here the SAME
			// error would erase prose, relationships, gates, and eval that
			// drift.ForNode never even touched. The page serves with no
			// badges instead of a 500, and the panel detail degrades with
			// them: a chip and a panel that disagree about whether drift
			// exists would be worse than neither.
			badges, detail = nil, nil
		}
		page.Drift = badges
		page.DriftDetail = detail
		// g is built once and shared by both of this handler's own live-git
		// reads (unpushed_review below and the origin-URL lookup a few lines
		// down), the same accept-a-g-rather-than-build-one rationale
		// review.ForTask's own doc comment states: a caller that already
		// holds one never pays for a second.
		g := vcs.New(root)
		page.UnpushedReview = unpushedReviewViewOf(review.ForTask(t, n.NodeID(), g))
		if origin, err := g.OriginURL(); err == nil {
			if repo, ok := githubRepoURL(origin); ok {
				fillGitHubLinks(&page.Node, repo)
			}
		}
		result := observedNodePage{NodePage: page, ExecutionHistoryError: executionHistoryError}
		if record, ok := snapshot.Observations[n.NodeID()]; ok {
			result.Execution = &record
			result.Workflow = nil
			executionEvidence(snapshot, record, &result.Node)
		}
		return result, nil
	})
}

// fillAmendmentValidity fills every staged precondition's Live slot, and an
// intake-minted amendment's own Source.Live slot, with their current-tree
// reads: mutate.EvaluateAmendmentPreconditions and mutate.EvaluateAmendmentSource,
// the identical comparisons `tiller show amendment` renders (single-sourced,
// per the instruction and the sibling
// binding for Source). mutate.Deps{} is the same bare zero value
// runShowAmendment itself passes; its rev() method builds a real git window
// on demand from the tree's own root, so nothing here needs injecting.
//
// The two reads are independent side dishes, not gated on one another: a
// batch that cannot even resolve (a vanished target, a hand-edited epic key)
// leaves every precondition's Live slot nil the same way active_basis and
// the Drifted chip already degrade elsewhere on this page, while Source's
// own read (which needs no operation resolution at all, only doc.Source and
// a working-tree hash) still fills normally, and vice versa — and the
// converse holds too: an unreadable source artifact (permission denied, a
// symlink loop; WorktreeBlobRef's own doc comment, transition.go) leaves
// ONLY Source.Live nil, while every precondition's own Live still fills
// normally from the healthy preconditions read
// (TestNodePageAmendmentSourceErrorAloneStillFillsPreconditionLive pins
// exactly this).
//
// #57 widens the class: a hand-edit defect (a duplicate staged key, a
// staged-epic replacement, a blanked per-op source citation) was invisible
// on this page entirely, surfacing only when approve refused. The Live-slot
// degrade above still stands, unchanged, for the reason its own comment
// still gives; what changes is that BOTH evaluators that can fail here
// (EvaluateAmendmentPreconditions AND EvaluateAmendmentSource — an earlier
// pass at this fix named only the preconditions evaluator "the ONE
// evaluator that can fail here," which was wrong four lines below it, where
// Source's own error was still silently dropped; FT4 review caught it) now
// NAME why, folded into the one page.ValidityError by
// joinAmendmentValidityErrors below, the identical vocabulary the CLI's own
// validity_error already carries. That is a small, additive risk next to
// the one this function has always taken serving a stuck amendment's
// structural shape at all: a string field left empty on the common path
// costs nothing, and named on the broken path is exactly the pre-flight #57
// asks for. SourceMissing and ShapeError, #57's other two per-op signals,
// are NOT filled here: both their evaluators are pure over the doc alone
// (no tree, no error, amendment_validity.go's own doc comments), so
// views.amendmentPageOf already fills them unconditionally, before this
// function ever runs, independent of whatever this read finds.
func fillAmendmentValidity(t *store.Tree, page *views.AmendmentPageView, a *store.Amendment) {
	list, verr := mutate.EvaluateAmendmentPreconditions(t, mutate.Deps{}, a)
	if verr == nil {
		type preKey struct {
			index int
			name  string
		}
		byKey := make(map[preKey]mutate.PreconditionValidity, len(list))
		for _, pv := range list {
			byKey[preKey{pv.Index, pv.Precondition}] = pv
		}
		for i := range page.Operations {
			op := &page.Operations[i]
			for j := range op.Preconditions {
				pc := &op.Preconditions[j]
				got, ok := byKey[preKey{op.Index, pc.Name}]
				if !ok {
					continue
				}
				pc.Live = &views.AmendmentPreconditionLiveView{
					Status: got.Status, Actual: got.Actual, Note: got.Note,
				}
			}
		}
	}

	var serr error
	if page.Source != nil {
		var sv *mutate.AmendmentSourceValidity
		sv, serr = mutate.EvaluateAmendmentSource(t, mutate.Deps{}, a)
		if serr == nil && sv != nil {
			page.Source.Live = &views.AmendmentPreconditionLiveView{
				Status: sv.Status, Actual: sv.Actual, Note: sv.Note,
			}
		}
	}

	page.ValidityError = joinAmendmentValidityErrors(verr, serr)
}

// joinAmendmentValidityErrors folds fillAmendmentValidity's own two
// independent live-read errors into the one page.ValidityError string,
// deduped by message: a faithful local mirror of cli/amendment_cmds.go's own
// joinLiveReadErrors, which runShowAmendment uses for the identical reason —
// EvaluateAmendmentPreconditions and EvaluateAmendmentSource both open with
// the SAME loadAmendmentDoc read, so a doc-load failure (a corrupt or
// hand-mangled amendment yml) surfaces from BOTH evaluators as the exact
// same error text, and joining unconditionally would report the one failure
// twice ("X; X") rather than naming the single thing that actually went
// wrong. Kept as a local copy rather than exported and shared across
// packages: it is five lines with no state, and threading an import between
// cli and serve for it would cost more than the duplication does. A nil
// error is skipped; order is preserved (preconditions, then source) so a
// caller reading the joined string on either surface sees the same order
// the evaluators ran in.
func joinAmendmentValidityErrors(errs ...error) string {
	seen := make(map[string]bool, len(errs))
	var out []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		msg := err.Error()
		if seen[msg] {
			continue
		}
		seen[msg] = true
		out = append(out, msg)
	}
	return strings.Join(out, "; ")
}

// evalPayload loads and validates a node's authored acceptance contract for the
// page's eval slot from the same observation used by workflow advice. A missing
// contract keeps the existing null payload; workflow still names that uncertainty.
func evalPayload(t *store.Tree, n store.Node, in workflow.Input) any {
	rel, ok := gen.EvalPath(t, n)
	if !ok {
		return nil // the noun carries no authored contract
	}
	if in.ContractMissing[n.NodeID()] {
		return nil
	}
	if message := in.ContractErrors[n.NodeID()]; message != "" {
		return views.EvalContractErrorView{Path: rel, Error: message}
	}
	ef := in.Contracts[n.NodeID()]
	if ef == nil {
		return nil
	}
	if err := ef.ValidateContract(); err != nil {
		return views.EvalContractErrorView{Path: rel, Error: err.Error()}
	}
	return views.EvalContractOf(rel, ef)
}

// basisPayload answers the reporting half of the active-basis rule for the node
// page: the pinned files whose working-tree content no longer matches what the
// human approved, the node whose pin moved, and the move that clears it, or nil
// when nothing drifted (cli-surface.md, "Specifying a feature", which names
// serve alongside status and drift as a reporter).
//
// It calls mutate.BasisDrift, the identical predicate the start, check, and
// submit guards run, so the console cannot draw a node clean that those verbs
// would refuse. The comparison hashes working-tree files through git, which is
// why it runs here rather than in views.NodePageOf: internal/views performs no
// I/O. A task carries two pins, its own task.md and, through its parent, the
// feature's spec and contract, so the report's own node is not always the page's
// (views.ActiveBasisView's doc comment).
//
// A non-ActiveBasisError from the comparison is a git that could not answer, and
// it leaves the page silent rather than failing: the node page already serves
// through a missing prose file and a broken event stream, and a repository with
// no working git must not be the one the console cannot render. What it never
// does is assert clean; absence here reads exactly like a node with no pin.
func basisPayload(t *store.Tree, n store.Node) *views.ActiveBasisView {
	var ab *mutate.ActiveBasisError
	if err := mutate.BasisDrift(t, n, vcs.New(t.Root)); errors.As(err, &ab) {
		v := views.ActiveBasisOf(t, ab)
		return &v
	}
	return nil
}

// driftBadges answers the node page's Drifted-chip badges: every enabled
// marker drift.ForNode finds for n, projected through views.DriftBadgesOf.
// It runs fresh per request, the same live-tree posture serve.go's own
// /api/drift route takes (serve.go's own comment there: "the sweep runs
// fresh per request... rather than a cache", since a cache here would go
// stale silently the moment a file changed underneath a running console,
// exactly the failure this read-only "watching console" must not have).
// basisPayload's own BasisDrift call above already pays the active_basis
// git-hashing cost for this same node once per request; ForNode's own
// active_basis check pays it a second time, for the identical node, in the
// identical request. That doubled cost is stated here so P4's caching pass
// finds it written down rather than discovering it.
//
// ForNode's own error, same as Run's, is an unknown drift.checks entry. This
// function only returns it; the caller (NodeHandler, above) is the one that
// decides what to do with it, and does NOT ride it unwrapped the way
// /api/drift does for the identical error. /api/drift's loud-failure posture
// is right for THAT endpoint because the sweep's report IS the whole
// payload there, so failing loud costs nothing else; the node page carries
// prose, relationships, gates, and eval that drift.ForNode never touches, so
// the same failure there would take all of it down for one manifest typo.
//
// The uncovered rows ForNode returns used to be dropped here for want of a
// detail area; the drift panel is that area now (JC's ask, 2026-08-06), so
// they ride the detail payload beside the candidates. The badge contract is
// unchanged: an uncovered row still never becomes a badge, so a node with
// only uncovered rows stays chipless and its detail reaches the reader
// through the /drift page alone.
func driftViews(t *store.Tree, n store.Node) ([]views.DriftBadgeView, *views.NodeDriftView, error) {
	candidates, uncovered, err := drift.ForNode(t, vcs.NewHeadSnapshot(vcs.New(t.Root)), n)
	if err != nil {
		return nil, nil, err
	}
	return views.DriftBadgesOf(candidates, n), views.NodeDriftDetailOf(t, candidates, uncovered), nil
}

// unpushedReviewViewOf projects review.Facts onto the node page's additive
// unpushed_review object (Task 15), nil exactly when facts is
// nil, the identical projection internal/cli's own unpushedReviewView
// applies for the CLI envelopes. The live-git computation itself is
// review.ForTask (internal/core/review), called directly above with the
// page's own node id: taskID need not already be known to name a task
// (review.ForTask's own doc comment), so every other noun the node page
// addresses passes straight through here as a harmless nil rather than
// needing a type assertion first. review.ForTask already takes on the
// silent-degrade posture a vcs failure needs here: the node page already
// serves through a missing prose file, a broken event stream, and a
// git-less repo for active_basis (basisPayload's own doc comment).
func unpushedReviewViewOf(f *review.Facts) *views.UnpushedReviewView {
	if f == nil {
		return nil
	}
	return &views.UnpushedReviewView{Commit: f.Commit, Branch: f.Branch, Upstream: f.Upstream}
}

// githubRepoURL maps an origin URL onto the GitHub repository page it names.
// Exactly two origin forms qualify: HTTPS https://github.com/<owner>/<repo>
// and SCP-style SSH git@github.com:<owner>/<repo>, each with an optional .git
// suffix. The value is exactly https://github.com/<owner>/<repo>. Anything
// else, a missing, local, malformed, or non-GitHub origin, reports ok=false so
// the caller omits every derived link rather than deriving a wrong one.
func githubRepoURL(origin string) (string, bool) {
	const httpsPrefix = "https://github.com/"
	const sshPrefix = "git@github.com:"

	var repoPath string
	switch {
	case strings.HasPrefix(origin, httpsPrefix):
		repoPath = strings.TrimPrefix(origin, httpsPrefix)
	case strings.HasPrefix(origin, sshPrefix):
		repoPath = strings.TrimPrefix(origin, sshPrefix)
	default:
		return "", false
	}
	repoPath = strings.TrimSuffix(repoPath, ".git")

	// The remainder must be exactly <owner>/<repo>: a non-empty owner and repo
	// with no further path segments.
	owner, repo, ok := strings.Cut(repoPath, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", false
	}
	return fmt.Sprintf("https://github.com/%s/%s", owner, repo), true
}

// fillGitHubLinks derives every link the node page carries off one GitHub
// repository URL: the PR page (pr_url, the original), the branch's tree page
// (branch_url), and the commit page for the basis commit recorded on either
// pin (commit_url; node header pass, 2026-08-30). Each is filled only where
// its fact exists, so a page never carries a link to nothing.
func fillGitHubLinks(n *views.NodeView, repo string) {
	if n.PR != nil {
		n.PRURL = fmt.Sprintf("%s/pull/%d", repo, *n.PR)
	}
	if n.Branch != "" {
		// Tiller cuts every branch itself as tiller/<epic>/<feature>, slugs
		// only, so the name rides the path verbatim; a free-form branch name
		// would need per-segment escaping here, which no writer produces yet.
		n.BranchURL = repo + "/tree/" + n.Branch
	}
	if n.AcceptedAgainst != nil {
		n.AcceptedAgainst.CommitURL = commitURL(repo, n.AcceptedAgainst.Commit)
	}
	if n.VerifiedAgainst != nil {
		n.VerifiedAgainst.CommitURL = commitURL(repo, n.VerifiedAgainst.Commit)
	}
}

// commitURL maps a compact "git:<sha>" token onto its commit page, and
// answers "" for an absent token or one that is not a git commit (a sha256
// working-tree fingerprint has no page anywhere), which omitempty then drops.
func commitURL(repo, token string) string {
	const prefix = "git:"
	if !strings.HasPrefix(token, prefix) {
		return ""
	}
	return repo + "/commit/" + strings.TrimPrefix(token, prefix)
}

// readProse bounds node prose with the same policy as the direct file route.
// Missing and unreadable files retain the existing empty-content pointer.
func readProse(ctx context.Context, root, rel string) (string, error) {
	document, err := corpus.ReadLimited(ctx, root, rel, maxServedFileBytes)
	if err != nil {
		return "", err
	}
	return string(document.Content), nil
}

// nodePath splits /api/node/<noun>/<address>. The first segment after the
// prefix is the noun; the remainder, slashes included, is the address, so a
// feature's canonical key round-trips.
func nodePath(path string) (noun, addr string, ok bool) {
	rest, found := strings.CutPrefix(path, "/api/node/")
	if !found {
		return "", "", false
	}
	noun, addr, found = strings.Cut(rest, "/")
	if !found || noun == "" || addr == "" {
		return "", "", false
	}
	return noun, addr, true
}

// validNoun mirrors the CLI's noun validation (internal/cli, show): the seven
// addressable node classes, singular. Amendments and shelf work resolve
// through the identical NodeHandler path everything else does, id or key,
// while the page's optional helpers retain their existing absence semantics.
func validNoun(noun string) bool {
	switch noun {
	case store.NounEpic, store.NounFeature, store.NounTask, store.NounQuestion, store.NounAmendment,
		store.NounBug, store.NounChore, store.NounTweak:
		return true
	default:
		return false
	}
}

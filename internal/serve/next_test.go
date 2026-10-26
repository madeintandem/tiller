package serve_test

// These tests pin GET /api/next, the console's ranked list of legal next
// moves (ranked-next design, "Surfaces / Console"): the same views.Next
// payload `tiller next --json` prints, ranked for the served checkout's
// contributor, with Candidates cut to the shortlist. Every request rides the
// real console Handler, mux and all, so a dropped route registration fails
// these tests instead of passing them against a handler production never
// reaches (the same posture evidence_test.go's header explains). The fixture
// mirrors layApprovedPlanAheadOfADraftFeature (internal/cli/guide_test.go),
// the tree that first separated guide's walk from the readout's: one approved
// feature plan with its initial task auto-approved to ready, plus a second,
// later feature that stays draft. Contributor resolution is exercised through
// the real TILLER_CONTRIBUTOR / identity-file seam actingContributor
// (home.go) reads, never stubbed.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/cli"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestNextHandlerRanksForTheServedIdentity pins the shape and the reader
// plumbing together: read as jc (the task's owner, via TILLER_CONTRIBUTOR, the
// same env seam the CLI honors), jc's own claimed task ranks over the draft
// feature's specify move, and the node is the ready task's identity,
// noun/id/key/ref, exactly as gen.Ref would render it.
func TestNextHandlerRanksForTheServedIdentity(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	srv := nextServer(t, root)

	resp, err := srv.Client().Get(srv.URL + "/api/next")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	var got views.Next
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	taskID := readyTaskID(t, root)
	require.Equal(t, "jc", got.Reader)
	require.False(t, got.CaughtUp)
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "your task is claimed and ready to start", top.Reason)
	require.NotNil(t, top.Node, "the top move names the task it acts on")
	require.Equal(t, store.NounTask, top.Node.Noun)
	require.Equal(t, taskID, top.Node.ID)
	require.Equal(t, "form-engine/schema-engine/transcribe-the-schema", top.Node.Key)
	require.Contains(t, top.Node.Ref, taskID)
	require.Contains(t, top.Node.Ref, "form-engine/schema-engine/transcribe-the-schema")

	specify := candidateFor(t, got, "form-engine/zebra-reporting")
	require.Equal(t, "specify", specify.Skill, "the draft feature stays on the list behind jc's own task")
}

// TestNextHandlerViaIdentityFile is the twin the env-var test never
// exercised: actingContributor's OTHER resolution path, a real
// .tiller/identity.yml written by `tiller join` (store.WriteLocalIdentity),
// with no TILLER_CONTRIBUTOR set at all. The env var wins when both are
// present (actingContributor's own doc comment), so only a test that unsets
// it proves the identity-file fallback actually reads and ranks the list, not
// merely that the env var can.
func TestNextHandlerViaIdentityFile(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	require.NoError(t, store.WriteLocalIdentity(root, "jc"))
	got := getNext(t, nextServer(t, root))

	require.Equal(t, "jc", got.Reader)
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Equal(t, "execute", top.Skill, "the identity file alone must rank jc's ready task first")
	require.Equal(t, "your task is claimed and ready to start", top.Reason)
	require.NotNil(t, top.Node)
	require.Equal(t, readyTaskID(t, root), top.Node.ID)
}

// TestNextHandlerWithoutIdentityIsUnpersonalized pins the other half of the
// reader plumbing: with no TILLER_CONTRIBUTOR and no identity file,
// actingContributor resolves the empty reader, which holds no task, so the
// list is plan order and the reader field is empty.
func TestNextHandlerWithoutIdentityIsUnpersonalized(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	got := getNext(t, nextServer(t, root))

	require.Empty(t, got.Reader)
	require.NotEmpty(t, got.Candidates)
	for _, c := range got.Candidates {
		require.NotEqual(t, "your task is claimed and ready to start", c.Reason,
			"no resolvable reader means nobody holds the ready task")
	}
	specify := candidateFor(t, got, "form-engine/zebra-reporting")
	require.Equal(t, "specify", specify.Skill)
	require.Equal(t, "draft, with every dependency met", specify.Reason)
	require.Equal(t, store.NounFeature, specify.Node.Noun)
}

// TestNextHandlerCarriesDependencyDiagnostics pins that a tree with a
// dependency cycle serves the diagnostics on the payload, and that a clean
// tree omits the key rather than null-stuffing it.
func TestNextHandlerCarriesDependencyDiagnostics(t *testing.T) {
	clean := nextServer(t, nextFixture(t))
	var raw map[string]json.RawMessage
	require.Equal(t, http.StatusOK, getJSON(t, clean, "/api/next", &raw))
	_, present := raw["dependency_diagnostics"]
	require.False(t, present, "a tree with no dependency findings omits the key")

	root := t.TempDir()
	observationFile(t, root, ".tiller/tiller.yml", serveManifest)
	observationFile(t, root, ".tiller/epics/work/epic.yml", "id: E-aaaaaa\nkey: work\ntitle: Work\nstatus: ready\n")
	observationFile(t, root, ".tiller/epics/work/features/build/feature.yml", "id: F-bbbbbb\nkey: work/build\ntitle: Build\nstatus: ready\n")
	observationFile(t, root, ".tiller/epics/work/features/build/tasks/one/task.yml", "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: ready\ndepends_on: [T-dddddd]\n")
	observationFile(t, root, ".tiller/epics/work/features/build/tasks/two/task.yml", "id: T-dddddd\nkey: work/build/two\ntitle: Two\nstatus: ready\ndepends_on: [T-cccccc]\n")
	got := getNext(t, nextServer(t, root))
	require.NotNil(t, got.DependencyDiagnostics)
	require.False(t, got.DependencyDiagnostics.Ready)
	require.NotEmpty(t, got.DependencyDiagnostics.Gaps)
	require.Contains(t, got.DependencyDiagnostics.Gaps[0].Message, "dependency cycle")
}

// TestNextHandlerRefusesWrites pins the read-only posture on the next route,
// the same guard api() enforces for every other endpoint.
func TestNextHandlerRefusesWrites(t *testing.T) {
	srv := nextServer(t, nextFixture(t))

	resp, err := srv.Client().Post(srv.URL+"/api/next", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

// TestRouteEndpointIsGone pins that /api/route was removed outright, not
// kept as an adapter: it falls to the /api/ catch-all's 404.
func TestRouteEndpointIsGone(t *testing.T) {
	srv := nextServer(t, nextFixture(t))
	resp, err := srv.Client().Get(srv.URL + "/api/route")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestNextHandlerMatchesTheCLIPayload runs the real `tiller next --json`
// against the same project and identity and compares its data block with the
// handler's body, so the console and the CLI can never disagree about the
// ranked list.
func TestNextHandlerMatchesTheCLIPayload(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	t.Chdir(root)

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"next", "--json"})
	require.NoError(t, cmd.Execute(), out.String())
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope))
	require.True(t, envelope.OK)

	srv := nextServer(t, root)
	resp, err := srv.Client().Get(srv.URL + "/api/next")
	require.NoError(t, err)
	defer resp.Body.Close()
	var served json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&served))

	var cliPayload, handlerPayload views.Next
	require.NoError(t, json.Unmarshal(envelope.Data, &cliPayload))
	require.NoError(t, json.Unmarshal(served, &handlerPayload))
	require.NotEmpty(t, handlerPayload.Candidates)
	require.Equal(t, cliPayload, handlerPayload)
	require.JSONEq(t, string(envelope.Data), string(served))
}

// A question-held task is no move, so the list names the later legal task and
// its own reason, never the hold.
func TestNextHandlerNamesTheLegalTaskPastAQuestionHold(t *testing.T) {
	root, legalKey, legalID := questionHeldNextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	got := getNext(t, nextServer(t, root))

	var top views.NextCandidate
	for _, c := range got.Candidates {
		if c.Skill == "execute" {
			top = c
			break
		}
	}
	require.NotNil(t, top.Node, "an execute move is on the list")
	require.Equal(t, store.NounTask, top.Node.Noun)
	require.Equal(t, legalID, top.Node.ID)
	require.Equal(t, legalKey, top.Node.Key)
	require.Equal(t, "your task is claimed and ready to start", top.Reason,
		"the held task's questions are moves of their own, never folded into this reason")
	for _, c := range got.Candidates {
		if c.Skill == "execute" {
			require.Equal(t, legalID, c.Node.ID, "the question-held task is no execute move")
		}
	}
}

// The closeout's advice rides the candidate: a post-merge acceptance is the
// human's move, and once the gate passes the agent's move is sync.
func TestNextHandlerClassifiesPostMergeAcceptanceAndSyncFromAuthoredContract(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	tree, err := store.Load(root)
	require.NoError(t, err)
	var feature *store.Feature
	for _, f := range tree.Features {
		if f.Key != nil && *f.Key == "form-engine/schema-engine" {
			feature = f
			break
		}
	}
	require.NotNil(t, feature)
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine")
	save := func(path string, value any) {
		data, err := yaml.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
	feature.Status = "active"
	feature.AcceptedAgainst = store.BasisRefs{}
	feature.Gates = map[string]store.Gate{"evals": {Status: "pending", Criteria: map[string]store.Criterion{"C1": {Status: "needs_attestation"}}}}
	tree.Manifest.Gates.Required = map[string]store.GateSpec{"evals": {Kind: "command", On: store.StringList{"feature"}}}
	save(filepath.Join(root, ".tiller", "tiller.yml"), tree.Manifest)
	save(filepath.Join(dir, "feature.yml"), feature)
	for _, task := range tree.FeatureTasks[feature.ID] {
		task.Status = "done"
		task.Started = true
		save(filepath.Join(dir, store.TaskDirName, "transcribe-the-schema", store.TaskStateFile), task)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(fmt.Sprintf("---\nnode: %s\ncriteria:\n  - id: C1\n    statement: Human accepts behavior\n    required: true\n    binding: {type: manual}\n---\n", feature.ID)), 0o644))
	// jc added the draft feature last, so its specify move would rank first
	// as jc's current feature; this test is about the closeout's advice.
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".tiller", "epics", "form-engine", "features", "zebra-reporting")))
	srv := nextServer(t, root)

	got := getNext(t, srv)
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Empty(t, top.Skill)
	require.NotNil(t, top.Workflow)
	require.Equal(t, "human", top.Workflow.Actor)
	require.Equal(t, "acceptance", top.Workflow.Phase)
	require.Equal(t, feature.ID, top.Workflow.FeatureID)
	require.False(t, top.Workflow.SubmissionRecorded)

	feature.Gates["evals"] = store.Gate{Status: "pass", Criteria: map[string]store.Criterion{"C1": {Status: "pass"}}}
	save(filepath.Join(dir, "feature.yml"), feature)
	got = getNext(t, srv)
	require.NotEmpty(t, got.Candidates)
	top = got.Candidates[0]
	require.Equal(t, "execute", top.Skill)
	require.NotNil(t, top.Workflow)
	require.Equal(t, "sync", top.Workflow.Action)
	require.Equal(t, "tiller sync", top.Workflow.Command)
}

// A brownfield tree whose registered design docs await confirmation lists the
// human's confirmation first, a move that names no node.
func TestNextHandlerListsTheBaselineReviewMove(t *testing.T) {
	root := t.TempDir()
	manifest := strings.Replace(nextTestManifest, "kind: greenfield", "kind: brownfield", 1)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "design", "as-is"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "as-is", "app.md"), []byte("# App\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, store.BaselineRelPath),
		[]byte("docs:\n  - path: as-is/app.md\n    describes: [.]\n"), 0o644))

	got := getNext(t, nextServer(t, root))
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Equal(t, "confirm_design", top.Kind)
	require.Nil(t, top.Node, "a design doc confirmation names no node")
	require.NotEmpty(t, top.Reason)
}

// The no-work-planned end state is caught up with the zero-day hint, no
// candidates, and an empty (never null) candidates array.
func TestNextHandlerServesTheUnplannedEndState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(nextTestManifest), 0o644))
	srv := nextServer(t, root)

	var raw map[string]json.RawMessage
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/next", &raw))
	require.JSONEq(t, `[]`, string(raw["candidates"]))
	got := getNext(t, srv)
	require.True(t, got.CaughtUp)
	require.True(t, got.Unplanned)
	require.Equal(t, "No work is planned yet: add design inputs under .tiller/design/ for decompose to turn into a draft tree.", views.Sentence(got.Hint))
}

// observedNext serves /api/next over observationFixture's integration
// checkout while the task branch's worktree records the task in flight under
// owner.
func observedNext(t *testing.T, owner string) views.Next {
	t.Helper()
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	srv, _, worktree := observationFixture(t)
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: in_progress\nstarted: true\nowner: "+owner+"\nbranch: tiller/work/build/one\n")
	return getNext(t, srv)
}

// The reader's own task in flight on a branch continues there. The move
// carries no skill to run from this checkout.
func TestNextHandlerContinuesReadersObservedTask(t *testing.T) {
	got := observedNext(t, "jc")
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Equal(t, "continue", top.Kind)
	require.Equal(t, "your task, in flight on its branch", top.Reason)
	require.Equal(t, "tiller/work/build/one", top.Branch)
	require.Nil(t, top.Workflow)
	require.Equal(t, "T-cccccc", top.Node.ID)
}

// Another contributor's task in flight is no move here; with nothing else
// legal the reader is caught up.
func TestNextHandlerOmitsOthersObservedTask(t *testing.T) {
	got := observedNext(t, "robin")
	require.True(t, got.CaughtUp)
	require.Empty(t, got.Candidates)
}

// A console served from the reader's own task worktree reads that branch as
// its served tree, so the move is ordinary execution, not a branch mute.
func TestNextHandlerServedFromOwnTaskWorktreeIsOrdinaryContinue(t *testing.T) {
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	_, _, worktree := observationFixture(t)
	observationFile(t, worktree, observationTaskPath, "id: T-cccccc\nkey: work/build/one\ntitle: One\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")
	srv := httptest.NewServer(serve.Handler(worktree, nil, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	got := getNext(t, srv)
	require.NotEmpty(t, got.Candidates)
	top := got.Candidates[0]
	require.Equal(t, "execute", top.Skill)
	require.Equal(t, "your task is in progress", top.Reason)
	require.Equal(t, "T-cccccc", top.Node.ID)
}

// The payload carries the shortlist, not the full ranked list: a tree with
// more legal moves than the shortlist holds serves exactly five rows.
func TestNextHandlerCutsCandidatesToTheShortlist(t *testing.T) {
	root := nextFixture(t)
	t.Setenv("TILLER_CONTRIBUTOR", "jc")
	var questions strings.Builder
	questions.WriteString("questions:\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&questions, "  - id: Q-aaaaa%d\n    title: Open question %d\n    statement: Which way for question %d?\n    status: open\n", i, i, i)
	}
	observationFile(t, root, ".tiller/decisions/open-questions.yml", questions.String())

	tree, err := store.Load(root)
	require.NoError(t, err)
	full := views.NextOf(tree, readinput.ForNext(tree, "jc", time.Now()))
	require.Greater(t, len(full.Candidates), 5, "the fixture must legalize more moves than the shortlist holds")
	require.Len(t, getNext(t, nextServer(t, root)).Candidates, 5)
}

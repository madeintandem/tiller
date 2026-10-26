package serve_test

// Fixtures for the /api/next tests (next_test.go): a repo with one approved
// feature plan and a draft feature, a question-held tree, and the helpers
// that serve and read them.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

const nextTestManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Route Serve Test
  key: routeserve
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
  - { id: dana, name: Dana Diaz }
`

// nextFixture lays a repo carrying one approved feature plan (an
// auto-approved ready task owned by jc) and a second, later feature that
// stays draft: jc's claimed task and the frontier's specify move are both
// legal on this one tree, so the handler test can pin which one ranks first
// (the reader's own task) and how owner scoping moves the answer. It
// returns the repo root.
func nextFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(nextTestManifest), 0o644))
	deps := mutate.Deps{Contributor: "jc"}

	load := func() *store.Tree {
		tree, err := store.Load(root)
		require.NoError(t, err)
		return tree
	}
	resolveNode := func(noun, raw string) store.Node {
		n, err := resolve.NewResolver(load()).Resolve(resolve.Ref{Noun: noun, Raw: raw})
		require.NoError(t, err)
		return n
	}

	_, err := mutate.AddEpic(load(), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(load(), deps, mutate.FeatureInput{
		Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)
	dir := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Schema engine\n\nReal spec prose for the route fixture.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: F-aaaaaa\ncriteria:\n  - id: C1\n    statement: It works\n    polarity: introduce\n    binding: { type: test, ref: spec/works_spec.rb }\n---\n\n# Acceptance\n"), 0o644))

	_, err = mutate.AddTask(load(), deps, mutate.TaskInput{
		Key: "form-engine/schema-engine/transcribe-the-schema", Title: "Transcribe the schema"})
	require.NoError(t, err)
	_, err = mutate.ClaimTask(load(), deps, "form-engine/schema-engine/transcribe-the-schema", "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, store.TaskDirName, "transcribe-the-schema", store.TaskProseFile),
		[]byte("# Transcribe the schema\n\n## Approach\n\nTranscribe it.\n"), 0o644))

	// approve reads HEAD for the commit anchor (artifact-schema.md, "Accepted
	// basis"; internal/core/mutate/transition.go's recordBasis), which cannot
	// resolve before a first commit exists, the same reason
	// approvedTaskRoot (node_test.go) inits and commits before approving.
	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "lay tree")

	// The route's approve-epic wait fires for a draft epic with
	// live features, so the fixture approves its one epic to keep these
	// tests proving the arms below that wait.
	_, err = mutate.Transition(load(), deps, resolveNode(store.NounEpic, "form-engine"), mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	_, err = mutate.Transition(load(), deps, resolveNode(store.NounFeature, "form-engine/schema-engine"), mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)
	_, err = mutate.Transition(load(), deps, resolveNode(store.NounFeature, "form-engine/schema-engine"), mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	// The later, still-draft feature the frontier's specify arm would name if
	// the execute arm did not outrank it.
	_, err = mutate.AddFeature(load(), deps, mutate.FeatureInput{
		Key: "form-engine/zebra-reporting", Title: "Zebra reporting", ChangeType: "create"})
	require.NoError(t, err)

	return root
}

// nextServer mounts the real console Handler over root.
func nextServer(t *testing.T, root string) *httptest.Server {
	t.Helper()
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv
}

// getNext reads GET /api/next into the shared views.Next shape.
func getNext(t *testing.T, srv *httptest.Server) views.Next {
	t.Helper()
	var got views.Next
	require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/next", &got))
	return got
}

// readyTaskID is the one ready task the fixture's approve-feature cascade left.
func readyTaskID(t *testing.T, root string) string {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	var taskID string
	for id, tk := range tree.Tasks {
		if tk.Status == "ready" {
			taskID = id
		}
	}
	require.NotEmpty(t, taskID, "the fixture's approve-feature cascade must have left one ready task")
	return taskID
}

// candidateFor finds the ranked row acting on the node with this key.
func candidateFor(t *testing.T, next views.Next, key string) views.NextCandidate {
	t.Helper()
	for _, c := range next.Candidates {
		if c.Node != nil && c.Node.Key == key {
			return c
		}
	}
	require.FailNow(t, "no ranked candidate for "+key)
	return views.NextCandidate{}
}

func questionHeldNextFixture(t *testing.T) (root, legalKey, legalID string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(nextTestManifest), 0o644))
	deps := mutate.Deps{Contributor: "jc"}

	load := func() *store.Tree {
		tree, err := store.Load(root)
		require.NoError(t, err)
		return tree
	}
	resolveNode := func(noun, raw string) store.Node {
		n, err := resolve.NewResolver(load()).Resolve(resolve.Ref{Noun: noun, Raw: raw})
		require.NoError(t, err)
		return n
	}

	for _, epic := range []mutate.EpicInput{
		{Key: "form-engine", Title: "Form Engine"},
		{Key: "legal-engine", Title: "Legal Engine"},
	} {
		_, err := mutate.AddEpic(load(), deps, epic)
		require.NoError(t, err)
	}
	for _, feature := range []mutate.FeatureInput{
		{Key: "form-engine/route-work", Title: "Question-held route work", ChangeType: "create"},
		{Key: "legal-engine/route-work", Title: "Legal route work", ChangeType: "create"},
	} {
		_, err := mutate.AddFeature(load(), deps, feature)
		require.NoError(t, err)
	}

	heldFeatureID := resolveNode(store.NounFeature, "form-engine/route-work").NodeID()
	legalFeatureID := resolveNode(store.NounFeature, "legal-engine/route-work").NodeID()
	heldKey := "form-engine/route-work/zeta-question-held"
	legalKey = "legal-engine/route-work/alpha-legal"
	for _, task := range []struct {
		key        string
		title      string
		epicDir    string
		featureDir string
	}{
		{key: heldKey, title: "Question held task", epicDir: "form-engine", featureDir: "route-work"},
		{key: legalKey, title: "Legal task", epicDir: "legal-engine", featureDir: "route-work"},
	} {
		_, err := mutate.AddTask(load(), deps, mutate.TaskInput{Key: task.key, Title: task.title})
		require.NoError(t, err)
		_, err = mutate.ClaimTask(load(), deps, task.key, "")
		require.NoError(t, err)
		taskDir := filepath.Join(root, ".tiller", "epics", task.epicDir, "features", task.featureDir,
			store.TaskDirName, filepath.Base(task.key))
		require.NoError(t, os.WriteFile(filepath.Join(taskDir, store.TaskProseFile),
			[]byte("# "+task.title+"\n\n## Approach\n\nImplement it.\n"), 0o644))
	}

	for _, feature := range []struct {
		epicDir string
		title   string
		id      string
	}{
		{epicDir: "form-engine", title: "Question-held route work", id: heldFeatureID},
		{epicDir: "legal-engine", title: "Legal route work", id: legalFeatureID},
	} {
		dir := filepath.Join(root, ".tiller", "epics", feature.epicDir, "features", "route-work")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
			[]byte("# "+feature.title+"\n\nChoose the next legal task.\n"), 0o644))
		eval := fmt.Sprintf("---\nnode: %s\ncriteria:\n  - id: C1\n    statement: Routing stays legal\n    polarity: introduce\n    binding: { type: test, ref: internal/serve/next_test.go }\n---\n\n# Acceptance\n", feature.id)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"), []byte(eval), 0o644))
	}

	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "lay question-held route")

	for _, epicKey := range []string{"form-engine", "legal-engine"} {
		_, err := mutate.Transition(load(), deps, resolveNode(store.NounEpic, epicKey), mutate.TransitionInput{Verb: "approve"})
		require.NoError(t, err)
	}
	for _, featureKey := range []string{"form-engine/route-work", "legal-engine/route-work"} {
		_, err := mutate.Transition(load(), deps, resolveNode(store.NounFeature, featureKey), mutate.TransitionInput{Verb: "spec"})
		require.NoError(t, err)
		_, err = mutate.Transition(load(), deps, resolveNode(store.NounFeature, featureKey), mutate.TransitionInput{Verb: "approve"})
		require.NoError(t, err)
	}

	featureID := heldFeatureID
	legalID = resolveNode(store.NounTask, legalKey).NodeID()
	heldEpicID := resolveNode(store.NounEpic, "form-engine").NodeID()
	legalEpicID := resolveNode(store.NounEpic, "legal-engine").NodeID()
	_, err := mutate.SetOrder(load(), deps, mutate.SetOrderInput{
		Noun: store.NounEpic, Nodes: []string{heldEpicID, legalEpicID},
	})
	require.NoError(t, err)

	questions := fmt.Sprintf(`questions:
  - id: Q-zzzzzz
    title: Deployment shape
    statement: Which detailed deployment shape should this route use?
    status: open
    blocks: [%s, %s]
  - id: Q-aaaaaa
    title: Architecture boundary
    statement: Which detailed architecture boundary should this route use?
    status: open
    blocks: [%s]
  - id: Q-mmmmmm
    title: Migration sequence
    statement: Which detailed migration sequence should this route use?
    status: open
    blocks: [%s]
  - id: Q-iiiiii
    title: Answered detail
    statement: This answered statement must not hold the route.
    status: answered
    answer: Already decided
    blocks: [%s]
`, featureID, heldEpicID, featureID, featureID, featureID)
	decisionsDir := filepath.Join(root, ".tiller", "decisions")
	require.NoError(t, os.MkdirAll(decisionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(decisionsDir, "open-questions.yml"), []byte(questions), 0o644))
	return root, legalKey, legalID
}

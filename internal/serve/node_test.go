package serve_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// These tests pin /api/node/{noun}/{addr}, the console's uniform node page
// (plan 2026-07-05, P2): one payload shape for every noun, the relationship
// hub, and the history timeline, resolved by id or key with the CLI's noun
// validation. The fixture is driven through the real mutate layer with a fixed
// clock and a sequential minter, so every id and timestamp asserted here is
// deterministic.

// fixedClock stamps every event with one instant, so the stream's order falls
// to the event-id tiebreak (deterministic under the sequential minter) and the
// At strings are stable to assert.
type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }

// sequentialMint mints per-prefix counters shaped like real ids (E-000001,
// EV-000003), valid to the resolver and stable across runs.
func sequentialMint() func(string) string {
	counts := map[string]int{}
	return func(prefix string) string {
		counts[prefix]++
		return fmt.Sprintf("%s-%06d", prefix, counts[prefix])
	}
}

// nodeFixture lays a small real graph through mutate: an epic, two features
// (the second depending on the first), a task on the first feature, and an
// open question blocking it. Deterministic ids: E-000001, F-000001
// (schema-engine), F-000002 (form-loading), T-000001, Q-000001, events
// EV-000001..EV-000005.
func nodeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	step := func(fn func(tree *store.Tree) error) {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		require.NoError(t, fn(tree))
	}
	step(func(tree *store.Tree) error {
		_, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "form-engine/form-loading", Title: "Form loading", ChangeType: "create",
			DependsOn: []string{"form-engine/schema-engine"},
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddTask(tree, deps, mutate.TaskInput{
			Key:   "form-engine/schema-engine/parse-the-schema",
			Title: "Parse the schema",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddQuestion(tree, deps, mutate.QuestionInput{
			Title:     "Schema dialect",
			Statement: "Which schema dialect?", Blocks: []string{"form-engine/schema-engine"},
		})
		return err
	})
	return root
}

func nodeShelfFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	eventID := 0
	mint := func(prefix string) string {
		switch prefix {
		case "E":
			return "E-root00"
		case "F":
			return "F-cccccc"
		case "B":
			return "B-aaaaaa"
		case "CH":
			return "CH-bbbbbb"
		case "EV":
			eventID++
			return fmt.Sprintf("EV-%06d", eventID)
		default:
			t.Fatalf("unexpected fixture id prefix %q", prefix)
			return ""
		}
	}
	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: mint}
	step := func(fn func(tree *store.Tree) error) {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		require.NoError(t, fn(tree))
	}
	step(func(tree *store.Tree) error {
		_, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "foundation", Title: "Foundation"})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "foundation/console", Title: "Console", ChangeType: "create",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddWorkItem(tree, deps, mutate.WorkItemInput{
			Noun: store.NounBug, Key: "blank-on-reload", Title: "Blank on reload",
			AttachedTo: "foundation/console",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddWorkItem(tree, deps, mutate.WorkItemInput{
			Noun: store.NounChore, Key: "bump-go", Title: "Bump Go",
		})
		return err
	})
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.md"),
		[]byte("# Blank on reload\n\n## Expected\n\nThe form survives a reload.\n"),
		0o644,
	))
	return root
}

func nodeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(serve.NodeHandler(nodeFixture(t)))
	t.Cleanup(srv.Close)
	return srv
}

// claimTaskInFixture gives an execution unit its own reservation.
func claimTaskInFixture(t *testing.T, root, ref, owner string) {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.ClaimTask(tree, mutate.Deps{Contributor: owner}, ref, "")
	require.NoError(t, err)
}

// nodePagePayload is the client's-eye view of the page, enough structure to
// pin every relationship and the history order.
type nodePagePayload struct {
	Node struct {
		Noun      string   `json:"noun"`
		ID        string   `json:"id"`
		Key       string   `json:"key"`
		Title     string   `json:"title"`
		Status    string   `json:"status"`
		Statement string   `json:"statement"`
		DependsOn []string `json:"depends_on"`
		Order     []string `json:"order"`
		OrderRefs []string `json:"order_refs"`
	} `json:"node"`
	Ref   string `json:"ref"`
	Prose *struct {
		Content string `json:"content"`
	} `json:"prose"`
	Relationships struct {
		Epic               *relatedPayload  `json:"epic"`
		Feature            *relatedPayload  `json:"feature"`
		Features           []relatedPayload `json:"features"`
		Tasks              []relatedPayload `json:"tasks"`
		DependsOn          []relatedPayload `json:"depends_on"`
		DependedOnBy       []relatedPayload `json:"depended_on_by"`
		BlockedByQuestions []relatedPayload `json:"blocked_by_questions"`
		Blocks             []relatedPayload `json:"blocks"`
		AttachedTo         *relatedPayload  `json:"attached_to"`
		ResolvedBy         *relatedPayload  `json:"resolved_by"`
		DuplicateOf        *relatedPayload  `json:"duplicate_of"`
		AttachedWork       []relatedPayload `json:"attached_work"`
	} `json:"relationships"`
	History []struct {
		At          string `json:"at"`
		Type        string `json:"type"`
		Contributor string `json:"contributor"`
		Command     string `json:"command"`
	} `json:"history"`
}

type relatedPayload struct {
	Noun      string `json:"noun"`
	ID        string `json:"id"`
	Key       string `json:"key"`
	Ref       string `json:"ref"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Statement string `json:"statement"`
}

// TestNodePageFeature pins the feature page end to end: key-addressed (the
// slash in the canonical key survives routing), the untouched node view, the
// explicit ref, the owning epic, the ledger tasks, both dependency
// directions, the blocking question, and the history newest first with stable
// timestamps.
func TestNodePageFeature(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))

	require.Equal(t, "feature", got.Node.Noun)
	require.Equal(t, "F-000001", got.Node.ID)
	require.Equal(t, "form-engine/schema-engine", got.Node.Key)
	require.Equal(t, "Schema engine", got.Node.Title)
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Ref)

	require.NotNil(t, got.Relationships.Epic)
	require.Equal(t, "form-engine (E-000001)", got.Relationships.Epic.Ref)
	require.Equal(t, "Form Engine", got.Relationships.Epic.Title)

	require.Len(t, got.Relationships.Tasks, 1)
	require.Equal(t, "T-000001", got.Relationships.Tasks[0].ID)
	require.Equal(t, "form-engine/schema-engine/parse-the-schema (T-000001)", got.Relationships.Tasks[0].Ref)
	require.Equal(t, "Parse the schema", got.Relationships.Tasks[0].Title)

	require.Empty(t, got.Relationships.DependsOn)
	require.Len(t, got.Relationships.DependedOnBy, 1)
	require.Equal(t, "form-engine/form-loading (F-000002)", got.Relationships.DependedOnBy[0].Ref)

	require.Len(t, got.Relationships.BlockedByQuestions, 1)
	require.Equal(t, "Q-000001", got.Relationships.BlockedByQuestions[0].ID)
	require.Equal(t, "Schema dialect", got.Relationships.BlockedByQuestions[0].Title)
	require.Equal(t, "Which schema dialect?", got.Relationships.BlockedByQuestions[0].Statement)

	// History: the feature's own add, with the stable timestamp and actor.
	require.Len(t, got.History, 1)
	require.Equal(t, "add.feature", got.History[0].Type)
	for _, h := range got.History {
		require.Equal(t, "2026-07-01T10:00:00Z", h.At)
		require.Equal(t, "jc", h.Contributor)
	}
}

// TestNodePageFeatureDependsOnBothDirections pins the dependent feature's
// side: its depends_on resolves to the dependency's full link row.
func TestNodePageFeatureDependsOnBothDirections(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/form-loading", &got))

	require.Equal(t, []string{"F-000001"}, got.Node.DependsOn)
	require.Len(t, got.Relationships.DependsOn, 1)
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Relationships.DependsOn[0].Ref)
	require.Equal(t, "Schema engine", got.Relationships.DependsOn[0].Title)
	require.Empty(t, got.Relationships.DependedOnBy)
}

// orderedEpicRoot lays one epic and two features (E-000001, F-000001
// schema-engine, F-000002 form-loading, the identical minting nodeFixture
// produces), then records the epic's feature order the reverse of their
// minted id order, all through the same deps/mint the add steps use so the
// event stream mints continuously (a second, freshly constructed deps for a
// later mutation would restart the mint counter and collide with paths the
// earlier steps already wrote, tripping store's stale-write guard).
func orderedEpicRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	step := func(fn func(tree *store.Tree) error) {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		require.NoError(t, fn(tree))
	}
	step(func(tree *store.Tree) error {
		_, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "form-engine/form-loading", Title: "Form loading", ChangeType: "create",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.SetOrder(tree, deps, mutate.SetOrderInput{
			Noun:  store.NounFeature,
			Nodes: []string{"F-000002", "F-000001"},
		})
		return err
	})
	return root
}

// TestNodePageCarriesRecordedFeatureOrder pins Task 8 at the wire: once
// `tiller order` has recorded an epic's feature sequence, the node page
// carries it as node.order (bare ids, in the recorded sequence) and
// node.order_refs (the same targets as human references), untouched by
// Task 6's own DependencyOrder wiring, which rides Relationships.Features
// instead.
func TestNodePageCarriesRecordedFeatureOrder(t *testing.T) {
	srv := httptest.NewServer(serve.NodeHandler(orderedEpicRoot(t)))
	t.Cleanup(srv.Close)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/epic/form-engine", &got))
	require.Equal(t, []string{"F-000002", "F-000001"}, got.Node.Order,
		"the recorded sequence rides verbatim, not id order")
	require.Equal(t, []string{
		"form-engine/form-loading (F-000002)",
		"form-engine/schema-engine (F-000001)",
	}, got.Node.OrderRefs)
}

// TestNodePageOmitsOrderWithoutOne pins the additive, omitempty guarantee at
// the wire: a node that never ran `tiller order` carries neither key at all,
// not an empty array.
func TestNodePageOmitsOrderWithoutOne(t *testing.T) {
	srv := nodeServer(t)

	status, top := rawPage(t, srv, "/api/node/feature/form-engine/schema-engine")
	require.Equal(t, 200, status)
	var node map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["node"], &node))
	_, hasOrder := node["order"]
	_, hasOrderRefs := node["order_refs"]
	require.False(t, hasOrder, "order must be absent, not an empty array")
	require.False(t, hasOrderRefs, "order_refs must be absent, not an empty array")
}

// TestNodePageTask pins the task page: addressing by id and by the canonical
// three-segment key, the task's own reference, and the climb to feature and epic.
func TestNodePageTask(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/T-000001", &got))

	require.Equal(t, "task", got.Node.Noun)
	require.Equal(t, "Parse the schema", got.Node.Title)
	require.Equal(t, "form-engine/schema-engine/parse-the-schema", got.Node.Key)
	require.Equal(t, "form-engine/schema-engine/parse-the-schema (T-000001)", got.Ref,
		"a task renders its own canonical key path")

	var byKey nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/form-engine/schema-engine/parse-the-schema", &byKey),
		"a task resolves by its canonical key at the HTTP surface too")
	require.Equal(t, "T-000001", byKey.Node.ID)

	require.NotNil(t, got.Relationships.Feature)
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Relationships.Feature.Ref)
	require.NotNil(t, got.Relationships.Epic)
	require.Equal(t, "form-engine (E-000001)", got.Relationships.Epic.Ref)

	require.Len(t, got.History, 1)
	require.Equal(t, "add.task", got.History[0].Type)
}

// TestNodePageQuestion pins the question page: the bare-id reference, the
// statement, and the blocks targets resolved to link rows.
func TestNodePageQuestion(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/question/Q-000001", &got))

	require.Equal(t, "Q-000001", got.Ref)
	require.Equal(t, "Which schema dialect?", got.Node.Statement)
	require.Equal(t, "Schema dialect", got.Node.Title)
	require.Len(t, got.Relationships.Blocks, 1)
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Relationships.Blocks[0].Ref)
}

func TestQuestionRelatedRowCarriesTitleAndFullStatement(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
	require.Len(t, got.Relationships.BlockedByQuestions, 1)
	row := got.Relationships.BlockedByQuestions[0]
	require.Equal(t, "Schema dialect", row.Title)
	require.Equal(t, "Which schema dialect?", row.Statement)
}

// TestNodePageByID pins id addressing for a keyed noun: the same page the key
// serves.
func TestNodePageByID(t *testing.T) {
	srv := nodeServer(t)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/F-000001", &got))
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Ref)
}

func TestNodeHandlerServesBugByIDAndKey(t *testing.T) {
	root := nodeShelfFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	for _, addr := range []string{"B-aaaaaa", "bugs/blank-on-reload"} {
		var got nodePagePayload
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/bug/"+addr, &got))
		require.Equal(t, "bug", got.Node.Noun)
		require.Equal(t, "Blank on reload", got.Node.Title)
		require.NotNil(t, got.Prose)
		require.Contains(t, got.Prose.Content, "Expected")
		require.NotNil(t, got.Relationships.AttachedTo)
		require.Equal(t, "F-cccccc", got.Relationships.AttachedTo.ID)
		require.NotEmpty(t, got.History)
	}
}

func TestNodeHandlerServesChoreAndKeepsEvalNull(t *testing.T) {
	root := nodeShelfFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	for _, addr := range []string{"CH-bbbbbb", "chores/bump-go"} {
		var got struct {
			Node struct {
				Noun  string `json:"noun"`
				Title string `json:"title"`
			} `json:"node"`
			Prose *struct {
				Content string `json:"content"`
			} `json:"prose"`
			Eval any `json:"eval"`
		}
		require.Equal(t, http.StatusOK, getJSON(t, srv, "/api/node/chore/"+addr, &got))
		require.Equal(t, "chore", got.Node.Noun)
		require.Equal(t, "Bump Go", got.Node.Title)
		require.NotNil(t, got.Prose)
		require.Contains(t, got.Prose.Content, "Bump Go")
		require.Nil(t, got.Eval)
	}
}

// TestNodePageNotFound pins the 404 set: an unknown noun, an unresolved
// address, a noun mismatch (the checksum the redundant noun exists for), and a
// path with no address at all. Every answer is the JSON error shape.
func TestNodePageNotFound(t *testing.T) {
	srv := nodeServer(t)

	for _, path := range []string{
		"/api/node/widget/F-000001",  // unknown noun
		"/api/node/feature/nope",     // unresolved key
		"/api/node/feature/F-zzzzzz", // unresolved id
		"/api/node/epic/F-000001",    // noun mismatch
		"/api/node/feature",          // no address
		"/api/node/",                 // nothing at all
	} {
		var got struct {
			Error string `json:"error"`
		}
		resp, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode, "GET %s", path)
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json", "GET %s", path)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		resp.Body.Close()
		require.NotEmpty(t, got.Error, "GET %s carries an error message", path)
	}
}

// TestNodePageRefusesWrites pins the read-only posture on this handler too.
func TestNodePageRefusesWrites(t *testing.T) {
	srv := nodeServer(t)

	resp, err := srv.Client().Post(srv.URL+"/api/node/feature/F-000001", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
}

// nodeProsePayload decodes the page's prose pointer, the field the serve
// handler fills from disk after the view hands it a path.
type nodeProsePayload struct {
	Ref   string `json:"ref"`
	Prose *struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Stub    bool   `json:"stub"`
	} `json:"prose"`
}

// TestNodePageProseContentRoundtrips pins the prose wiring: the view hands the
// handler a repo-relative path, the handler reads the file, and its content
// rides the page verbatim.
func TestNodePageProseContentRoundtrips(t *testing.T) {
	root := nodeFixture(t)
	prose := "# Schema engine\n\nThe canonical spec prose for this feature.\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine", "feature.md"),
		[]byte(prose), 0o644))

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got nodeProsePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
	require.NotNil(t, got.Prose, "a feature page carries a prose pointer")
	require.Equal(t, ".tiller/epics/form-engine/features/schema-engine/feature.md", got.Prose.Path)
	require.Equal(t, prose, got.Prose.Content, "the handler fills the content from disk")
	require.False(t, got.Prose.Stub, "written prose is not the add stub")
}

// TestNodePageProseFlagsTheAddStub pins finding 3's read model: a feature whose
// feature.md is still exactly what `tiller add feature` generated arrives
// flagged, so the console can collapse it to one line naming the move instead
// of rendering generated boilerplate as if it were a spec. The fixture writes
// no prose of its own here, so the file under test IS the real generated stub.
func TestNodePageProseFlagsTheAddStub(t *testing.T) {
	root := nodeFixture(t)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var feature nodeProsePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &feature))
	require.NotNil(t, feature.Prose)
	require.True(t, feature.Prose.Stub, "an untouched feature.md is the add stub")
	require.Contains(t, feature.Prose.Content, "Draft feature created by",
		"the content still rides the page whole; only the flag says what it is")

	var epic nodeProsePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/epic/form-engine", &epic))
	require.NotNil(t, epic.Prose)
	require.True(t, epic.Prose.Stub, "an untouched epic.md is the add stub")
}

// TestNodePageProseStubIsFalseForAMissingFile keeps the two gaps distinct: a
// prose file that is not there is a missing file, not an unwritten spec, and
// the console has a different honest line for each.
func TestNodePageProseStubIsFalseForAMissingFile(t *testing.T) {
	root := nodeFixture(t)
	require.NoError(t, os.Remove(
		filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine", "feature.md")))

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got nodeProsePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
	require.NotNil(t, got.Prose)
	require.False(t, got.Prose.Stub, "a missing file is not a stub claim")
	require.Equal(t, "", got.Prose.Content)
}

// TestNodePageProseMissingFileKeepsPointer pins the honest-pointer degradation:
// with the prose file gone, the path stays and the content is empty rather than
// the page failing, the same way a dangling relationship keeps a bare id.
func TestNodePageProseMissingFileKeepsPointer(t *testing.T) {
	root := nodeFixture(t)
	require.NoError(t, os.Remove(
		filepath.Join(root, ".tiller", "epics", "form-engine", "features", "schema-engine", "feature.md")))

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got nodeProsePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
	require.NotNil(t, got.Prose, "the pointer stays even with the file gone")
	require.Equal(t, ".tiller/epics/form-engine/features/schema-engine/feature.md", got.Prose.Path)
	require.Equal(t, "", got.Prose.Content, "a missing prose file renders an honest empty pointer")
}

// TestNodePageSurvivesABrokenEventFile pins the degradation choice: a
// malformed event file empties the history rather than taking the page down.
func TestNodePageSurvivesABrokenEventFile(t *testing.T) {
	root := nodeFixture(t)
	broken := filepath.Join(root, ".tiller", "events", "2026", "07", "01", "EV-broken.yml")
	require.NoError(t, os.WriteFile(broken, []byte(":\tnot yaml"), 0o644))

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/F-000001", &got))
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Ref)
	require.NotNil(t, got.History, "history stays an array")
	require.Empty(t, got.History, "a broken stream serves an empty timeline, not a 500")
}

// ---- eval contracts and PR links (Task 10) --------------------------------

// validFeatureEval is a well-formed feature.eval.md: two criteria, an
// introduce/command required criterion and a preserve/test criterion.
const validFeatureEval = `---
node: F-000001
criteria:
  - id: C1
    statement: The parser accepts a v2 schema document
    polarity: introduce
    binding: { type: command, run: "go test ./internal/schema/ -run TestV2" }
    required: true
  - id: C2
    statement: Existing v1 documents still parse unchanged
    polarity: preserve
    binding: { type: test, ref: "internal/schema/v1_test.go" }
---

# Acceptance
`

// brokenFeatureEval opens a frontmatter block but its YAML never terminates the
// flow sequence, so it parses as YAML garbage: LoadEvalFile fails before any
// contract validation runs.
const brokenFeatureEval = `---
node: F-000001
criteria: [unterminated
---

# Acceptance
`

// evalContractPayload decodes the page's eval slot in its success shape: the
// path it was read from, the node it grounds, and its criteria in file order.
type evalContractPayload struct {
	Path     string `json:"path"`
	Node     string `json:"node"`
	Criteria []struct {
		ID        string `json:"id"`
		Statement string `json:"statement"`
		Polarity  string `json:"polarity"`
		Required  bool   `json:"required"`
		Binding   struct {
			Type string `json:"type"`
			Ref  string `json:"ref"`
			Run  string `json:"run"`
		} `json:"binding"`
	} `json:"criteria"`
}

// writeUnder writes rel (a repo-relative, forward-slash path) under root,
// creating parents, so a fixture can lay an eval file beside a node's folder.
func writeUnder(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// rawPage fetches a node page and returns the status plus the top-level object
// as raw fields, so a test can assert a key's presence or absence (pr_url,
// eval) rather than a decoded zero value.
func rawPage(t *testing.T, srv *httptest.Server, path string) (int, map[string]json.RawMessage) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return resp.StatusCode, nil
	}
	var top map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&top))
	return resp.StatusCode, top
}

// gitRun runs a git command in dir and fails the test on a non-zero exit.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// setOrigin points the repo's origin remote at url, or removes it when url is
// empty (get-url is a local config read, so the URL never needs to resolve).
func setOrigin(t *testing.T, root, url string) {
	t.Helper()
	rm := exec.Command("git", "remote", "remove", "origin")
	rm.Dir = root
	_ = rm.Run() // there may be no origin to remove yet
	if url != "" {
		gitRun(t, root, "remote", "add", "origin", url)
	}
}

// setTaskPR records a PR number on one task by round-trip through the store
// type, so store.Load reads back a task that carries a PR the handler can derive
// a link from. A PR always represents a task (artifact-schema.md, "A PR always
// represents a task"), so the task is the only node that can carry one.
func setTaskPR(t *testing.T, root, featureKey, taskKey string, pr int) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", featureKey, taskKey), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	tk.Status = "in_review"
	tk.Started = true
	tk.Owner = "jc"
	tk.PR = &pr
	out, err := yaml.Marshal(&tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// setFeatureBranch records a recorded feature branch by round-trip through
// the store type, the identical write-then-reload pattern setTaskPR already
// uses for a task, so store.Load reads back a feature whose Branch
// review.ForTask's own feature lookup (internal/core/review) can resolve.
func setFeatureBranch(t *testing.T, root, featureKey, branch string) {
	t.Helper()
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", featureKey, "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var f store.Feature
	require.NoError(t, yaml.Unmarshal(data, &f))
	f.Branch = branch
	out, err := yaml.Marshal(&f)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func setTaskBranch(t *testing.T, root, featureKey, taskKey, branch string) {
	t.Helper()
	path := filepath.Join(root, store.TaskRelDir("form-engine", featureKey, taskKey), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var task store.Task
	require.NoError(t, yaml.Unmarshal(data, &task))
	task.Branch = branch
	data, err = yaml.Marshal(&task)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// gitOut runs a git command in dir and returns its trimmed stdout, failing
// the test on a non-zero exit; gitRun's own sibling for the calls where a
// test needs the output itself (the state commit's own short sha, here).
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// reversedSiblingRoot lays one epic and one feature, then three tasks whose
// local key order is the exact reverse of their minted id order (alfa is
// T-000003, charlie is T-000001), and marks the last sibling (T-000001)
// submitted offline with a branch-ref review target. It pins two HTTP-surface
// guarantees at once: the feature page lists tasks in sibling order, not id
// order, and a task page renders the offline review_ref.
func reversedSiblingRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	step := func(fn func(*store.Tree) error) {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		require.NoError(t, fn(tree))
	}
	step(func(tree *store.Tree) error {
		_, err := mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
			Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create",
		})
		return err
	})
	// Ids mint in add order (T-000001..T-000003) while the local keys run the
	// other way, so key order and id order disagree.
	for _, tk := range []struct{ local, title string }{
		{"charlie", "Wire the cache"},
		{"bravo", "Build the loader"},
		{"alfa", "Parse the schema"},
	} {
		tk := tk
		step(func(tree *store.Tree) error {
			_, err := mutate.AddTask(tree, deps, mutate.TaskInput{
				Key: "form-engine/schema-engine/" + tk.local, Title: tk.title,
			})
			return err
		})
	}

	// The last sibling by key (charlie, minted T-000001) is submitted offline.
	path := filepath.Join(root, store.TaskRelDir("form-engine", "schema-engine", "charlie"), store.TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var last store.Task
	require.NoError(t, yaml.Unmarshal(data, &last))
	last.Status = "in_review"
	last.Started = true
	last.Owner = "jc"
	last.ReviewRef = &store.ReviewRef{
		Branch: "tiller/form-engine/schema-engine",
		Head:   store.RevisionRef{Kind: store.KindGitCommit, Value: "git:e5f9d28"},
	}
	out, err := yaml.Marshal(last)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return root
}

// TestNodePagePRURL pins the derived GitHub PR link (design verification point
// 7): both qualifying origin forms, with and without the .git suffix, produce
// the exact PR page; a gitlab origin, a filesystem-path origin, and no origin
// each omit pr_url (absent, never null) while pr still renders, and the page
// never 500s over any origin shape.
func TestNodePagePRURL(t *testing.T) {
	root := nodeFixture(t)
	setTaskPR(t, root, "schema-engine", "parse-the-schema", 42)
	gitRun(t, root, "init")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	const reviewed = "/api/node/task/T-000001"

	cases := []struct {
		name    string
		origin  string // "" removes the origin entirely
		wantURL string // "" means pr_url must be absent
	}{
		{"https github", "https://github.com/madeintandem/tiller", "https://github.com/madeintandem/tiller/pull/42"},
		{"https github dot-git", "https://github.com/madeintandem/tiller.git", "https://github.com/madeintandem/tiller/pull/42"},
		{"ssh github", "git@github.com:madeintandem/tiller", "https://github.com/madeintandem/tiller/pull/42"},
		{"ssh github dot-git", "git@github.com:madeintandem/tiller.git", "https://github.com/madeintandem/tiller/pull/42"},
		{"gitlab https", "https://gitlab.com/madeintandem/tiller.git", ""},
		{"filesystem path", "/srv/git/tiller.git", ""},
		{"no origin", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setOrigin(t, root, c.origin)
			status, top := rawPage(t, srv, reviewed)
			require.Equal(t, 200, status, "the page never 500s over any origin shape")

			var node map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(top["node"], &node))
			require.Contains(t, node, "pr", "pr renders regardless of the origin")
			require.JSONEq(t, "42", string(node["pr"]))

			if c.wantURL == "" {
				_, ok := node["pr_url"]
				require.False(t, ok, "pr_url is absent (never null) for %s", c.name)
				return
			}
			var url string
			require.NoError(t, json.Unmarshal(node["pr_url"], &url))
			require.Equal(t, c.wantURL, url)
		})
	}
}

// TestNodePageEvalContract pins the success shape (design verification point
// 6): a feature with a well-formed feature.eval.md renders its full contract,
// the criteria count and a criterion's binding intact.
func TestNodePageEvalContract(t *testing.T) {
	root := nodeFixture(t)
	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md", validFeatureEval)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Eval evalContractPayload `json:"eval"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))
	require.Equal(t, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md", got.Eval.Path)
	require.Equal(t, "F-000001", got.Eval.Node)
	require.Len(t, got.Eval.Criteria, 2)

	c1 := got.Eval.Criteria[0]
	require.Equal(t, "C1", c1.ID)
	require.Equal(t, "introduce", c1.Polarity)
	require.True(t, c1.Required)
	require.Equal(t, "command", c1.Binding.Type)
	require.Equal(t, "go test ./internal/schema/ -run TestV2", c1.Binding.Run)
}

// TestNodePageEpicCarriesNoEvalContract pins the epic page's eval slot as
// empty, and stays empty even with a stray epic.eval.md on disk. An epic has no
// acceptance contract of its own: its bar is the sum of its features'
// contracts, each enforced at feature grain (artifact-schema.md, Epics), so
// gen.EvalPath answers no path for an epic and the page has nothing to read.
func TestNodePageEpicCarriesNoEvalContract(t *testing.T) {
	root := nodeFixture(t)
	writeUnder(t, root, ".tiller/epics/form-engine/epic.eval.md", `---
node: E-000001
criteria:
  - id: EC1
    statement: A leftover contract from before the epic grain was removed
    polarity: introduce
    binding: { type: command, run: "true" }
    required: true
---
`)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Eval evalContractPayload `json:"eval"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/epic/form-engine", &got))
	require.Empty(t, got.Eval.Path, "an epic page has no eval slot to fill")
	require.Empty(t, got.Eval.Criteria)
}

// TestNodePageEvalBrokenFrontmatterPreservesPage pins the error variant and the
// degradation choice: a feature whose eval file is malformed renders exactly
// {path, error} in the eval slot, with no node or criteria, while prose,
// relationships, and history still render and the page serves 200.
func TestNodePageEvalBrokenFrontmatterPreservesPage(t *testing.T) {
	root := nodeFixture(t)
	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md", brokenFeatureEval)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Ref   string `json:"ref"`
		Prose *struct {
			Path string `json:"path"`
		} `json:"prose"`
		Relationships struct {
			Epic  *relatedPayload  `json:"epic"`
			Tasks []relatedPayload `json:"tasks"`
		} `json:"relationships"`
		History []struct {
			Type string `json:"type"`
		} `json:"history"`
		Eval json.RawMessage `json:"eval"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))

	// The eval slot is exactly {path, error}.
	var eval map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got.Eval, &eval))
	require.Contains(t, eval, "path")
	require.Contains(t, eval, "error")
	require.NotContains(t, eval, "node", "a broken contract carries no node")
	require.NotContains(t, eval, "criteria", "a broken contract carries no criteria")
	var evalPath, evalErr string
	require.NoError(t, json.Unmarshal(eval["path"], &evalPath))
	require.NoError(t, json.Unmarshal(eval["error"], &evalErr))
	require.Equal(t, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md", evalPath)
	require.NotEmpty(t, evalErr, "the error names why the contract failed to load")

	// The rest of the page is untouched.
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Ref)
	require.NotNil(t, got.Prose, "prose still renders")
	require.NotNil(t, got.Relationships.Epic, "relationships still render")
	require.Len(t, got.Relationships.Tasks, 1)
	require.NotEmpty(t, got.History, "history still renders")
}

// TestNodePageEvalNullWithoutContract pins the null state: a feature with no
// eval file, and a task (a noun that never carries a contract), both render
// "eval": null so a client tells "no contract" (null) from an error ({path,
// error}) by the value while the key is always present.
func TestNodePageEvalNullWithoutContract(t *testing.T) {
	srv := nodeServer(t)

	for _, path := range []string{
		"/api/node/feature/form-engine/form-loading", // a feature with no eval file
		"/api/node/task/T-000001",                    // a noun that never carries one
	} {
		status, top := rawPage(t, srv, path)
		require.Equal(t, 200, status, "GET %s", path)
		require.Contains(t, top, "eval", "the eval key is always present: %s", path)
		require.JSONEq(t, "null", string(top["eval"]), "no contract renders null: %s", path)
	}
}

// TestNodePageSiblingOrderOverHTTP pins that a feature page lists its tasks in
// sibling order, not id order, at the HTTP surface.
func TestNodePageSiblingOrderOverHTTP(t *testing.T) {
	srv := httptest.NewServer(serve.NodeHandler(reversedSiblingRoot(t)))
	t.Cleanup(srv.Close)

	var got nodePagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/form-engine/schema-engine", &got))

	ids := make([]string, len(got.Relationships.Tasks))
	for i, tk := range got.Relationships.Tasks {
		ids[i] = tk.ID
	}
	require.Equal(t, []string{"T-000003", "T-000002", "T-000001"}, ids,
		"the feature page lists tasks in key order, the reverse of id order here")
}

// TestNodePageOfflineTaskReviewRef pins that a task submitted offline carries a
// review_ref with its branch and the git:-prefixed head, and never a fake PR.
func TestNodePageOfflineTaskReviewRef(t *testing.T) {
	srv := httptest.NewServer(serve.NodeHandler(reversedSiblingRoot(t)))
	t.Cleanup(srv.Close)

	var got struct {
		Node struct {
			PR        *int `json:"pr"`
			ReviewRef *struct {
				Branch string `json:"branch"`
				Head   string `json:"head"`
			} `json:"review_ref"`
		} `json:"node"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/T-000001", &got))
	require.Nil(t, got.Node.PR, "an offline submit records a branch ref, never a fake PR")
	require.NotNil(t, got.Node.ReviewRef)
	require.Equal(t, "tiller/form-engine/schema-engine", got.Node.ReviewRef.Branch)
	require.Equal(t, "git:e5f9d28", got.Node.ReviewRef.Head, "the head is the git:-prefixed compact token")
}

// taskProseRel is the fixture task's own implementation spec, the file a task
// page now renders as its body (artifact-schema.md, Tasks: a task is a folder
// node whose task.md is the implementation spec).
const taskProseRel = ".tiller/epics/form-engine/features/schema-engine/tasks/parse-the-schema/task.md"

// writtenTaskSpec is an implementation spec somebody actually wrote: the add
// stub's sentinel paragraph is gone and the headings carry bodies, which is what
// separates a written spec from a scaffold on both sides of the seam
// (mutate.IsProseUnwritten).
const writtenTaskSpec = `# Parse the schema

## Approach

Walk the schema document once, collecting field descriptors as we go.

## Surfaces

internal/schema/parse.go

## Tests

A v2 fixture document parses into the expected descriptor set.

## Edge cases

An empty document is an empty descriptor set, never an error.
`

// TestNodePageTaskCarriesItsImplementationSpec pins the task page's body
// (ui-surface.md, "Node detail": a task shows its implementation spec, its gates
// and evidence, and its PR). A task is a folder node now, so its prose slot is
// its own task.md rather than the null the ledger item used to serve, and its
// key is the canonical three-segment path the console renders.
func TestNodePageTaskCarriesItsImplementationSpec(t *testing.T) {
	root := nodeFixture(t)
	writeUnder(t, root, taskProseRel, writtenTaskSpec)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Node struct {
			Key string `json:"key"`
		} `json:"node"`
		Prose *struct {
			Path    string `json:"path"`
			Content string `json:"content"`
			Stub    bool   `json:"stub"`
		} `json:"prose"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/T-000001", &got))
	require.Equal(t, "form-engine/schema-engine/parse-the-schema", got.Node.Key)
	require.NotNil(t, got.Prose, "a task page carries its own prose file, not null")
	require.Equal(t, taskProseRel, got.Prose.Path)
	require.Contains(t, got.Prose.Content, "## Approach")
	require.Contains(t, got.Prose.Content, "Walk the schema document once")
	require.False(t, got.Prose.Stub, "a written implementation spec is not a stub")
}

// TestNodePageUnwrittenTaskSpecReadsAsAStub pins the other half of the same
// slot: the task.md `tiller add task` generated carries the sentinel and no body
// under its headings, so the page marks it a stub and the console collapses it
// to one line rather than typesetting scaffolding as a spec.
func TestNodePageUnwrittenTaskSpecReadsAsAStub(t *testing.T) {
	srv := nodeServer(t)

	var got struct {
		Prose *struct {
			Path string `json:"path"`
			Stub bool   `json:"stub"`
		} `json:"prose"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/T-000001", &got))
	require.NotNil(t, got.Prose)
	require.Equal(t, taskProseRel, got.Prose.Path)
	require.True(t, got.Prose.Stub, "the generated task.md is a stub until somebody writes it")
}

// approvedTaskRoot lays a real git repo whose one feature has passed its plan
// gate, so both pins the active-basis rule compares are recorded: the feature's
// feature.md and feature.eval.md, and the task's own task.md
// (artifact-schema.md, Tasks: approving pins the task.md blob in
// accepted_against). It returns the repo root.
func approvedTaskRoot(t *testing.T) string {
	t.Helper()
	root := nodeFixture(t)
	testgit.Init(t, root)
	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.md",
		"# Schema engine\n\n## Summary\n\nParse and validate schema documents.\n")
	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.eval.md", validFeatureEval)
	writeUnder(t, root, taskProseRel, writtenTaskSpec)
	// approve reads HEAD for the commit anchor now (artifact-schema.md,
	// "Accepted basis"), and HEAD cannot resolve before a first commit exists.
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "lay tree")

	mint := sequentialMint()
	for range 5 {
		mint("EV")
	}
	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: mint}
	// nodeFixture's open question names the feature, and the plan gate refuses
	// while one holds the plan, so answer it before approving.
	{
		tree, err := store.Load(root)
		require.NoError(t, err)
		_, err = mutate.AnswerQuestion(tree, deps, vcs.New(root), mutate.AnswerInput{Node: "Q-000001", Answer: "JSON Schema 2020-12"})
		require.NoError(t, err)
	}
	for _, verb := range []string{"spec", "approve"} {
		tree, err := store.Load(root)
		require.NoError(t, err)
		_, err = mutate.Transition(tree, deps, tree.Features["F-000001"], mutate.TransitionInput{Verb: verb})
		require.NoErrorf(t, err, "%s feature", verb)
	}
	return root
}

// TestNodePageReportsActiveBasisDrift pins the reporting half of the
// active-basis rule at the console's node page. cli-surface.md ("Specifying a
// feature") names serve alongside status and drift as a reporter while start,
// check, and submit refuse; the refusals landed in task 7 and the CLI's report
// in task 9, and this is the console's.
//
// The bar is agreement with the refusal, not the mere presence of a field: each
// leg reads the page and then runs the guard the verbs sit behind, so a page
// that called a node clean where start would refuse fails here.
func TestNodePageReportsActiveBasisDrift(t *testing.T) {
	root := approvedTaskRoot(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	type basisPayload struct {
		Drifted []string `json:"drifted"`
		Missing []string `json:"missing"`
		Noun    string   `json:"noun"`
		ID      string   `json:"id"`
		Ref     string   `json:"ref"`
		Command string   `json:"command"`
	}
	read := func(path string) *basisPayload {
		t.Helper()
		var got struct {
			ActiveBasis *basisPayload `json:"active_basis"`
		}
		require.Equal(t, 200, getJSON(t, srv, path, &got))
		return got.ActiveBasis
	}
	// guardRefuses reports what the verbs' own predicate says about a node, the
	// answer the page must not contradict.
	guardRefuses := func(id string) bool {
		t.Helper()
		tree, err := store.Load(root)
		require.NoError(t, err)
		n, ok := tree.Tasks[id]
		require.True(t, ok)
		var ab *mutate.ActiveBasisError
		return errors.As(mutate.BasisDrift(tree, n, vcs.New(root)), &ab)
	}

	require.Nil(t, read("/api/node/task/T-000001"), "nothing drifted, so the page says nothing")
	require.Nil(t, read("/api/node/feature/form-engine/schema-engine"))
	require.False(t, guardRefuses("T-000001"))

	// A bare working-tree edit of the task's own implementation spec, no commit.
	writeUnder(t, root, taskProseRel, writtenTaskSpec+"\nRevised after approval.\n")

	got := read("/api/node/task/T-000001")
	require.NotNil(t, got, "the task page reports the drift its own pin took")
	require.Equal(t, []string{store.TaskProseFile}, got.Drifted)
	require.Empty(t, got.Missing, "the file changed, it did not go away")
	require.Equal(t, "task", got.Noun, "the task's own pin is the one that moved")
	require.Equal(t, "T-000001", got.ID)
	require.Equal(t, "form-engine/schema-engine/parse-the-schema (T-000001)", got.Ref)
	require.Equal(t,
		"tiller approve task form-engine/schema-engine/parse-the-schema --refresh-basis",
		got.Command, "the move is runnable as printed")
	require.True(t, guardRefuses("T-000001"), "the page agrees with the guard")

	// The feature above it pinned different files, and its own page says so.
	require.Nil(t, read("/api/node/feature/form-engine/schema-engine"),
		"a task's own spec edit leaves the feature's pins where they were")

	// A feature's spec edit reaches the TASK's page, because a task's guard
	// compares its parent's pins before its own (mutate.guardTask). The report
	// there names the FEATURE, since refreshing the task's pin would not clear it.
	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.md",
		"# Schema engine\n\n## Summary\n\nScope changed since acceptance.\n")
	got = read("/api/node/task/T-000001")
	require.NotNil(t, got)
	require.Equal(t, "feature", got.Noun, "the pin that moved belongs to the parent")
	require.Equal(t, "F-000001", got.ID)
	require.Contains(t, got.Drifted, "feature.md")
	require.Equal(t,
		"tiller approve feature form-engine/schema-engine --refresh-basis",
		got.Command, "the remedy addresses the grain that owns the pin")
}

// TestNodePageBasisIsAbsentWhereThereIsNoPin covers the nouns that carry no
// accepted basis at all: an epic never has one and a question is not a work
// node, which is a different fact from a pin that matches and renders the same
// silence either way.
func TestNodePageBasisIsAbsentWhereThereIsNoPin(t *testing.T) {
	srv := nodeServer(t)

	for _, path := range []string{"/api/node/epic/form-engine", "/api/node/question/Q-000001"} {
		status, top := rawPage(t, srv, path)
		require.Equal(t, 200, status, "GET %s", path)
		_, ok := top["active_basis"]
		require.Falsef(t, ok, "active_basis is absent, never null: %s", path)
	}
}

// ---- Task 12: the node page's per-node drift badges -----------------------

// enableDriftChecks rewrites root's drift.checks in place, preserving every
// other manifest field: the node-page twin of
// internal/core/drift/drift_test.go's own setManifestChecks, a different
// test package so rebuilt here rather than imported. serveManifest (this
// file's own fixture manifest) carries no drift: block at all, which
// enabledSet reads as every marker disabled (drift.go's own doc comment), so
// a test exercising a badge has to name the marker it wants live.
func enableDriftChecks(t *testing.T, root string, checks ...string) {
	t.Helper()
	m, err := store.LoadManifest(root)
	require.NoError(t, err)
	m.Drift.Checks = checks
	data, err := yaml.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), data, 0o644))
}

// driftBadgePayload decodes one entry of the node page's drift slot
// (views.DriftBadgeView).
type driftBadgePayload struct {
	Marker   string `json:"marker"`
	Refusing bool   `json:"refusing"`
}

// doneFeatureWithVanishedSurfaceRoot lays one epic and one feature, hand-set
// done with a completion record naming a surface file that was never
// written to disk at all: the cheapest fixture that reaches "a done feature
// whose recorded surface vanished" (mirroring
// internal/core/drift/drift_test.go's own setFeatureStatus idiom, a
// different test package so rebuilt here rather than imported). The real
// spec/approve/start/merge/submit/sync arc that actually WRITES
// verified_against is already pinned in mutate/sync_test.go; this fixture's
// own business is only whether the served page's badge agrees with what
// drift.ForNode finds for a done feature with a vanished surface, not a
// second proof of how a feature reaches done. Missing the recorded
// inventory entry is caught before drift.go's own staleSpecSurfacesLeg ever
// calls TreeHash (the vanish rule's own ordering, drift.go's doc comment on
// that function), so this fixture needs no git repo at all: the file simply
// never exists in the working tree.
func doneFeatureWithVanishedSurfaceRoot(t *testing.T) (root, featureID string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	tree, err := store.Load(root)
	require.NoError(t, err)
	_, err = mutate.AddEpic(tree, deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)

	tree, err = store.Load(root)
	require.NoError(t, err)
	fRes, err := mutate.AddFeature(tree, deps, mutate.FeatureInput{
		Key: "form-engine/feat-done", Title: "Feat done", ChangeType: "create",
	})
	require.NoError(t, err)
	featureID = fRes.ID

	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "feat-done", "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Feature
	require.NoError(t, yaml.Unmarshal(data, &doc))
	doc.Status = "done"
	doc.VerifiedAgainst = store.VerifiedAgainst{
		Surfaces: []store.SurfaceFingerprint{
			{Path: "app/calc", Inventory: []string{"app/calc/zone.rb"}, Value: "git:0000000000000000000000000000000000000000"},
		},
	}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))

	return root, featureID
}

// TestNodePageDriftBadgeStaleSpecNeverRefuses pins the first
// example: a done feature whose recorded surface file is gone carries a
// stale_spec badge, and stale_spec never refuses a verb (artifact-schema.md
// names only active_basis as "the one marker whose condition also refuses
// verbs"), so the badge's own refusing bit reads false regardless of the drift.
func TestNodePageDriftBadgeStaleSpecNeverRefuses(t *testing.T) {
	root, featureID := doneFeatureWithVanishedSurfaceRoot(t)
	enableDriftChecks(t, root, "stale_spec")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Drift []driftBadgePayload `json:"drift"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/"+featureID, &got))
	require.Equal(t, []driftBadgePayload{{Marker: "stale_spec", Refusing: false}}, got.Drift)
}

// TestNodePageDriftBadgeActiveBasisRefuses pins the second
// example: an approved feature whose feature.md is edited after approval
// carries an active_basis badge, and active_basis always refuses (start,
// check, and submit all refuse on it), so the badge's refusing bit reads
// true.
func TestNodePageDriftBadgeActiveBasisRefuses(t *testing.T) {
	root := approvedTaskRoot(t)
	enableDriftChecks(t, root, "active_basis")

	writeUnder(t, root, ".tiller/epics/form-engine/features/schema-engine/feature.md",
		"# Schema engine\n\n## Summary\n\nEdited after approval.\n")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Drift []driftBadgePayload `json:"drift"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/F-000001", &got))
	require.Equal(t, []driftBadgePayload{{Marker: "active_basis", Refusing: true}}, got.Drift)
}

// TestNodePageDriftAbsentWithNoBadges pins the omitempty contract:
// a page with nothing to report carries
// no "drift" key at all, never an empty array, the same absent-not-null
// convention active_basis already keeps
// (TestNodePageBasisIsAbsentWhereThereIsNoPin, one section up). nodeServer's
// own fixture manifest (serveManifest) carries no drift: block, so every
// marker is disabled and there is nothing for any node to carry.
func TestNodePageDriftAbsentWithNoBadges(t *testing.T) {
	srv := nodeServer(t)

	status, top := rawPage(t, srv, "/api/node/feature/F-000001")
	require.Equal(t, 200, status)
	_, ok := top["drift"]
	require.False(t, ok, "drift is absent, never an empty array, when nothing was found")
}

// TestNodePageSurvivesABadDriftChecksManifest pins the Important
// finding: an unknown drift.checks entry (a manifest typo, drift.enabledSet's
// own misuse refusal) must not take the WHOLE node page down. Before this fix
// drift.ForNode's error rode the handler's return path unwrapped, so a single
// typo in the manifest turned every node page dark; the precedent this fix
// follows already sits a few lines up in the same handler, the events arm's
// own comment: "A broken event file must not take the node page down: the
// node's state renders from its own files and the history is a side dish, so
// the page serves with an empty timeline instead of a 500." A Drifted chip is
// at least as much a side dish as the history timeline is, and /api/drift's
// own loud-failure posture does not transfer here: there the sweep's report
// IS the whole payload, so failing loud costs nothing else, but the node page
// carries prose, relationships, gates, and eval that drift.ForNode never even
// touches.
func TestNodePageSurvivesABadDriftChecksManifest(t *testing.T) {
	root := nodeFixture(t)
	enableDriftChecks(t, root, "stale_specc") // typo: an unknown check name

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		Ref   string              `json:"ref"`
		Drift []driftBadgePayload `json:"drift"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/feature/F-000001", &got),
		"a broken drift.checks entry must not take the whole node page down")
	require.Equal(t, "form-engine/schema-engine (F-000001)", got.Ref, "the rest of the payload still renders")
	require.Empty(t, got.Drift, "no badges can be computed, so the page degrades to none rather than 500ing")
}

// unpushedReviewFields is the client's-eye view of the unpushed_review
// object, enough to assert commit, branch, and upstream together.
type unpushedReviewFields struct {
	Commit   string `json:"commit"`
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
}

// unpushedReviewFixture lays the node fixture under real git the way the
// review-state warning's own field case does (
// TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed's own fixture,
// internal/cli/lifecycle_cmds_test.go): an initial commit, a bare remote, and
// the fixture task's feature branch checked out from it, so a caller can push
// it (or not) and add a further local commit (or not) to drive every arm of
// review.ForTask.
func unpushedReviewFixture(t *testing.T) (root, branch string) {
	t.Helper()
	root = nodeFixture(t)
	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "initial")

	bare := t.TempDir()
	gitRun(t, bare, "init", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)

	branch = "tiller/form-engine/schema-engine/parse-the-schema"
	gitRun(t, root, "checkout", "-b", branch)
	return root, branch
}

// TestNodePageUnpushedReviewOnAnUnpushedStateCommit pins the positive case
// (Task 15's server addition, plan item 5): a task in review with a recorded
// PR, whose parent feature's branch is pushed but then holds a further local
// commit its remote-tracking counterpart does not, serves
// unpushed_review {commit, branch, upstream} on its node page, the identical
// object `tiller status task` renders for the same fixture shape
// (TestSubmitConfirmationWarnsWhenTheStateCommitIsUnpushed).
func TestNodePageUnpushedReviewOnAnUnpushedStateCommit(t *testing.T) {
	root, branch := unpushedReviewFixture(t)
	gitRun(t, root, "push", "-u", "origin", branch)

	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state\n"), 0o644))
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "state commit")
	head := gitOut(t, root, "rev-parse", "--short", "HEAD")

	setTaskBranch(t, root, "schema-engine", "parse-the-schema", branch)
	setTaskPR(t, root, "schema-engine", "parse-the-schema", 41)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got struct {
		UnpushedReview *unpushedReviewFields `json:"unpushed_review"`
	}
	require.Equal(t, 200, getJSON(t, srv, "/api/node/task/T-000001", &got))
	require.NotNil(t, got.UnpushedReview, "the state commit sits unpushed on the branch")
	require.Equal(t, head, got.UnpushedReview.Commit)
	require.Equal(t, branch, got.UnpushedReview.Branch)
	require.Equal(t, "origin/"+branch, got.UnpushedReview.Upstream)
}

// TestNodePageUnpushedReviewOmittedWhenPushed pins the all-clear pair the
// brief's fixture pair names: the identical setup, but with nothing
// advancing past the push, so the local tip is already on the remote and
// unpushed_review is absent (never null) from the page.
func TestNodePageUnpushedReviewOmittedWhenPushed(t *testing.T) {
	root, branch := unpushedReviewFixture(t)
	gitRun(t, root, "push", "-u", "origin", branch)

	setTaskBranch(t, root, "schema-engine", "parse-the-schema", branch)
	setTaskPR(t, root, "schema-engine", "parse-the-schema", 41)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	status, top := rawPage(t, srv, "/api/node/task/T-000001")
	require.Equal(t, 200, status)
	_, ok := top["unpushed_review"]
	require.False(t, ok, "a fully pushed branch carries nothing to warn about")
}

// TestNodePageUnpushedReviewOmittedWithNoUpstream pins the offline path: the
// feature's recorded branch exists locally but was never pushed, so
// UnpushedTip's own resolveUpstream finds no remote counterpart
// (ok=false, no error, the deliberate "nothing to compare" signal
// UnpushedTip's own doc comment names) and unpushed_review is absent.
func TestNodePageUnpushedReviewOmittedWithNoUpstream(t *testing.T) {
	root, branch := unpushedReviewFixture(t)
	// Deliberately never pushed: the branch has no remote counterpart at all.

	setTaskBranch(t, root, "schema-engine", "parse-the-schema", branch)
	setTaskPR(t, root, "schema-engine", "parse-the-schema", 41)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	status, top := rawPage(t, srv, "/api/node/task/T-000001")
	require.Equal(t, 200, status)
	_, ok := top["unpushed_review"]
	require.False(t, ok, "a branch with no remote counterpart has nothing to compare against")
}

// TestNodePageUnpushedReviewOmittedWithNoRecordedBranch pins the third
// omission condition review.ForTask's own doc comment names (alongside no
// upstream and already-pushed): a feature that has not yet recorded a
// branch at all leaves f.Branch empty, so there is nothing for UnpushedTip
// to even ask about and unpushed_review is absent. This runs under a fully
// working git repo (unpushedReviewFixture, the same fixture the other three
// unpushed_review tests share) rather than a git-less root, deliberately: it
// isolates this omission from TestNodePageSurvivesAnUnpushedReviewVcsError's
// own git-less case, proving the absence traces to the unrecorded branch
// specifically and not to git being unreachable.
func TestNodePageUnpushedReviewOmittedWithNoRecordedBranch(t *testing.T) {
	root, _ := unpushedReviewFixture(t)
	// Deliberately no setFeatureBranch call: the feature's Branch stays "".

	setTaskPR(t, root, "schema-engine", "parse-the-schema", 41)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	status, top := rawPage(t, srv, "/api/node/task/T-000001")
	require.Equal(t, 200, status)
	_, ok := top["unpushed_review"]
	require.False(t, ok, "a feature with no recorded branch has nothing to compare against")
}

// TestNodePageSurvivesAnUnpushedReviewVcsError pins the degrade-silently
// posture every caller takes (review.ForTask's own doc comment,
// internal/core/review/review.go: "every caller of this function is a
// report, not a gate"): a fixture root with no git repository at all cannot answer
// UnpushedTip's git reads no matter what it asks, and the node page must
// still serve 200 with unpushed_review simply absent rather than a 500, the
// identical posture basisPayload and driftViews already take for their own
// git reads on this same page.
func TestNodePageSurvivesAnUnpushedReviewVcsError(t *testing.T) {
	root := nodeFixture(t) // no git init at all: every git read fails

	setTaskBranch(t, root, "schema-engine", "parse-the-schema", "tiller/form-engine/schema-engine/parse-the-schema")
	setTaskPR(t, root, "schema-engine", "parse-the-schema", 41)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	status, top := rawPage(t, srv, "/api/node/task/T-000001")
	require.Equal(t, 200, status, "a vcs failure must not take the whole node page down")
	_, ok := top["unpushed_review"]
	require.False(t, ok, "nothing can be reported when git itself cannot answer")

	var node map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["node"], &node))
	require.Contains(t, node, "id", "the rest of the page still renders")
}

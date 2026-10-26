package serve_test

// These tests pin the amendment's own node page: the noun
// joins /api/node's validated set, its prose narrative reads from disk the
// same way every other node's does, and its staged operations' preconditions
// carry LIVE validity (valid/stale/unknown) computed server-side through the
// identical mutate.EvaluateAmendmentPreconditions helper `tiller show
// amendment` already renders, single-sourced rather than a second
// comparison. Plus /api/ls?noun=amendment, and the degrade-on-error posture
// for a batch that cannot resolve against the current tree.
//
// amendmentFixture builds its own small tree and its own git repo rather
// than reusing node_test.go's shared nodeFixture: opening an amendment reads
// HEAD for its own base_revision (mutate.AddAmendment), so git has to exist,
// with a first commit, before the first mutate call runs (the same ordering
// internal/core/mutate/add_test.go's writeOwnedRepo doc comment states), and
// nodeFixture is shared by every other serve test, whose deterministic ids a
// new amendment step would shift for tests that have nothing to do with
// amendments.

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// amendmentFixture lays one epic, one feature (form-engine/schema-engine,
// freshly added and so status draft), and one proposed amendment staging two
// operations: a revise on the feature carrying one precondition that holds
// right now (target_status: draft, VALID) and one that does not
// (key_absent: the feature's own key, which the feature itself holds, so
// STALE), plus an add-task naming a brand-new key with a target_status
// precondition that cannot compare against anything yet (UNKNOWN, since the
// staged key has no live node until approve materializes it). That is every
// LIVE validity state the binding names, in one fixture.
func amendmentFixture(t *testing.T) (root, amendmentID, featureID string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))
	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "init")

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
		res, err := mutate.AddAmendment(tree, deps, mutate.AmendmentInput{
			Key: "iphone-only", Title: "Remove native iPad support from v1",
			TriggeredBy:  []string{"design/solution-design/09-audit-engine.md"},
			AmendsDesign: []string{"design/solution-design/03-platform-scope.md"},
		})
		amendmentID = res.ID
		return err
	})

	tree, err := store.Load(root)
	require.NoError(t, err)
	for id := range tree.Features {
		featureID = id
	}
	require.NotEmpty(t, featureID)

	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID,
			Preconditions: map[string]string{
				"target_status": "draft",
				"key_absent":    "form-engine/schema-engine",
			},
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpAddTask, Key: "form-engine/schema-engine/extra-task", Title: "Extra task",
			Preconditions: map[string]string{"target_status": "done"},
		})
		return err
	})

	return root, amendmentID, featureID
}

func amendmentServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	root, amendmentID, featureID := amendmentFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	return srv, amendmentID, featureID
}

// amendmentPagePayload is the client's-eye view of the amendment block, the
// shape the console's api.ts mirrors.
type amendmentPagePayload struct {
	Node struct {
		Noun   string `json:"noun"`
		ID     string `json:"id"`
		Key    string `json:"key"`
		Title  string `json:"title"`
		Status string `json:"status"`
	} `json:"node"`
	Ref   string `json:"ref"`
	Prose *struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Stub    bool   `json:"stub"`
	} `json:"prose"`
	Amendment *struct {
		Operations []struct {
			Index         int                  `json:"index"`
			Op            string               `json:"op"`
			Target        *amendRelatedPayload `json:"target"`
			By            *amendRelatedPayload `json:"by"`
			Key           string               `json:"key"`
			Title         string               `json:"title"`
			ChangeType    string               `json:"change_type"`
			Reason        string               `json:"reason"`
			SourceMissing bool                 `json:"source_missing"`
			ShapeError    string               `json:"shape_error"`
			Preconditions []struct {
				Name     string `json:"name"`
				Expected string `json:"expected"`
				Live     *struct {
					Status string `json:"status"`
					Actual string `json:"actual"`
					Note   string `json:"note"`
				} `json:"live"`
			} `json:"preconditions"`
		} `json:"operations"`
		TriggeredBy  []string `json:"triggered_by"`
		AmendsDesign []string `json:"amends_design"`
		Source       *struct {
			Path     string `json:"path"`
			Revision string `json:"revision"`
			Live     *struct {
				Status string `json:"status"`
				Actual string `json:"actual"`
				Note   string `json:"note"`
			} `json:"live"`
		} `json:"source"`
		ValidityError string `json:"validity_error"`
	} `json:"amendment"`
	History []struct {
		Type string `json:"type"`
	} `json:"history"`
}

type amendRelatedPayload struct {
	Noun   string `json:"noun"`
	ID     string `json:"id"`
	Key    string `json:"key"`
	Ref    string `json:"ref"`
	Status string `json:"status"`
}

// TestNodePageAmendment pins the whole shape end to end: the node identity,
// prose (an amendment's narrative .md, mutate's own stub content since this
// fixture never overwrites it), the two staged operations with their targets
// resolved, and every LIVE validity state (valid, stale, unknown) the
// fixture's own preconditions are built to exercise.
func TestNodePageAmendment(t *testing.T) {
	srv, amendmentID, featureID := amendmentServer(t)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got))

	require.Equal(t, "amendment", got.Node.Noun)
	require.Equal(t, amendmentID, got.Node.ID)
	require.Equal(t, "iphone-only", got.Node.Key)
	require.Equal(t, "Remove native iPad support from v1", got.Node.Title)
	require.Equal(t, "proposed", got.Node.Status)

	// The prose slot fills the same way every other node's does: the handler
	// reads gen.ProsePath's file off disk. mutate.AddAmendment writes a stub
	// narrative at open, so the content is non-empty and the path names the
	// flat .md pair (gen.ProsePath's own *store.Amendment case).
	require.NotNil(t, got.Prose)
	require.Equal(t, ".tiller/amendments/"+amendmentID+"-iphone-only.md", got.Prose.Path)
	require.NotEmpty(t, got.Prose.Content, "the add-time narrative stub is real content, not empty")

	require.NotNil(t, got.Amendment)
	require.Equal(t, []string{"design/solution-design/09-audit-engine.md"}, got.Amendment.TriggeredBy)
	require.Equal(t, []string{"design/solution-design/03-platform-scope.md"}, got.Amendment.AmendsDesign)
	require.Len(t, got.Amendment.Operations, 2)

	revise := got.Amendment.Operations[0]
	require.Equal(t, 0, revise.Index)
	require.Equal(t, "revise", revise.Op)
	require.NotNil(t, revise.Target)
	require.Equal(t, featureID, revise.Target.ID)
	require.Equal(t, "feature", revise.Target.Noun)
	require.Equal(t, "form-engine/schema-engine (F-000001)", revise.Target.Ref)
	require.Len(t, revise.Preconditions, 2)

	byName := map[string]struct {
		Name     string
		Expected string
		Status   string
		Actual   string
	}{}
	for _, pc := range revise.Preconditions {
		row := byName[pc.Name]
		row.Name, row.Expected = pc.Name, pc.Expected
		if pc.Live != nil {
			row.Status, row.Actual = pc.Live.Status, pc.Live.Actual
		}
		byName[pc.Name] = row
	}
	require.Equal(t, "valid", byName["target_status"].Status, "the feature really is draft right now")
	require.Equal(t, "draft", byName["target_status"].Actual)
	require.Equal(t, "stale", byName["key_absent"].Status, "the feature itself already holds that key")

	addTask := got.Amendment.Operations[1]
	require.Equal(t, 1, addTask.Index)
	require.Equal(t, "add-task", addTask.Op)
	require.Nil(t, addTask.Target, "an add-task op names no live target, only its own staged key")
	require.Equal(t, "form-engine/schema-engine/extra-task", addTask.Key)
	require.Equal(t, "Extra task", addTask.Title)
	require.Len(t, addTask.Preconditions, 1)
	require.Equal(t, "unknown", addTask.Preconditions[0].Live.Status,
		"the staged key has no live node to compare a status against yet")
	require.Empty(t, addTask.Preconditions[0].Live.Actual, "unknown carries nothing to have compared")

	// #57: a clean batch (no Source at all here, so nothing to blank; no
	// duplicate staged keys; the batch resolves) carries none of the three
	// new hand-edit-defect signals.
	require.Empty(t, got.Amendment.ValidityError, "a batch that resolves names no validity error")
	require.False(t, revise.SourceMissing)
	require.Empty(t, revise.ShapeError)
	require.False(t, addTask.SourceMissing)
	require.Empty(t, addTask.ShapeError)

	// History still renders, unrelated to the amendment-specific fill above.
	require.NotEmpty(t, got.History)
}

// TestNodePageAmendmentByKey pins key addressing: an amendment's key has no
// slash the way a feature's canonical key does, but it still round-trips
// through the same /api/node/<noun>/<address> path every other noun uses.
func TestNodePageAmendmentByKey(t *testing.T) {
	srv, amendmentID, _ := amendmentServer(t)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/iphone-only", &got))
	require.Equal(t, amendmentID, got.Node.ID)
}

// TestNodePageAmendmentDegradesOnEvaluatorError pins the binding's own
// promise: "an evaluator error must not 500 the page." A hand-edited
// operation naming a target that has since vanished cannot even resolve
// (mutate.EvaluateAmendmentPreconditions returns an error rather than a
// per-precondition report), which must not take the rest of the page down
// with it, the same side-dish posture active_basis and the Drifted chip
// already keep on this page (their own doc comments in node.go).
func TestNodePageAmendmentDegradesOnEvaluatorError(t *testing.T) {
	root, amendmentID, _ := amendmentFixture(t)
	corruptAmendmentTarget(t, root, amendmentID, "F-vanished")

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got),
		"a batch that cannot resolve still serves the page, not a 500")
	require.NotNil(t, got.Amendment)
	require.Len(t, got.Amendment.Operations, 2, "the structural shape still renders")
	for _, op := range got.Amendment.Operations {
		for _, pc := range op.Preconditions {
			require.Nil(t, pc.Live, "every Live slot stays nil rather than a fabricated read")
		}
	}
	require.NotEmpty(t, got.Amendment.ValidityError,
		"#57: the degrade used to be silent; now the one reason live validity could not run is named on the page")
}

// corruptAmendmentTarget hand-lays a vanished target onto the amendment's
// first operation, the same way mutate's own writeAmendmentOps test helper
// does for its package's tests (internal/core/mutate/amendment_approve_test.go):
// staging validates what it writes, so this shape only a hand edit can
// produce.
func corruptAmendmentTarget(t *testing.T, root, amendmentID, vanishedID string) {
	t.Helper()
	path := amendmentDocPath(t, root, amendmentID)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Operations)
	doc.Operations[0].Target = vanishedID
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// amendmentDocPath finds the on-disk yml for amendmentID under root's
// .tiller/amendments/ directory (the store's own <id>-<key>.yml naming),
// globbed rather than hardcoded to one key: corruptAmendmentTarget's own
// fixture always opens under "iphone-only", but amendmentHandEditFixture
// reuses amendmentSourceFixture's "review-intake" key instead, so a
// single fixed suffix would silently miss it.
func amendmentDocPath(t *testing.T, root, amendmentID string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".tiller", "amendments", amendmentID+"-*.yml"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "exactly one amendment yml for %s", amendmentID)
	return matches[0]
}

// blankOpSourceOnDisk hand-blanks one staged operation's own Source
// citation, the shape only a hand edit can produce (StageOperation itself
// requires a non-blank Source on every op of an intake-minted amendment):
// the same yaml-read-modify-write corruptAmendmentTarget performs above, for
// the sibling hand-edit defect class #57 also covers.
func blankOpSourceOnDisk(t *testing.T, root, amendmentID string, index int) {
	t.Helper()
	path := amendmentDocPath(t, root, amendmentID)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc store.Amendment
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Greater(t, len(doc.Operations), index)
	doc.Operations[index].Source = ""
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// amendmentSourceFixture is amendmentFixture's twin for Task 5's own
// addition: an intake-minted amendment (mutate.AddAmendment's Source field)
// citing a REAL feedback artifact landed under .tiller/feedback/, so its
// live read (mutate.EvaluateAmendmentSource, node.go's fillAmendmentValidity)
// has something real to re-hash against, the same discipline
// internal/core/mutate's own sourceFixture keeps one package over
// (amendment_approve_test.go).
func amendmentSourceFixture(t *testing.T, body string) (root, amendmentID, relPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))
	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "init")

	relPath = filepath.Join(".tiller", "feedback", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "feedback"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte(body), 0o644))

	hash, err := vcs.New(root).WorktreeBlobHash(relPath)
	require.NoError(t, err)

	deps := mutate.Deps{Contributor: "jc", Clock: fixedClock{}, Mint: sequentialMint()}
	tree, err := store.Load(root)
	require.NoError(t, err)
	res, err := mutate.AddAmendment(tree, deps, mutate.AmendmentInput{
		Key: "review-intake", Title: "Intake: review.md",
		Source: &store.AmendmentSource{Path: relPath, Revision: "git:" + hash},
	})
	require.NoError(t, err)
	return root, res.ID, relPath
}

// TestNodePageAmendmentSourceLiveValid pins the fresh-artifact side of the
// honesty arc: an intake-minted amendment whose document
// has not moved reads Source.Live.Status "valid", Actual matching the
// recorded revision.
func TestNodePageAmendmentSourceLiveValid(t *testing.T) {
	root, amendmentID, _ := amendmentSourceFixture(t, "the review body\n")
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got))

	require.NotNil(t, got.Amendment)
	require.NotNil(t, got.Amendment.Source)
	require.Equal(t, ".tiller/feedback/review.md", got.Amendment.Source.Path)
	require.NotNil(t, got.Amendment.Source.Live)
	require.Equal(t, "valid", got.Amendment.Source.Live.Status)
	require.Equal(t, got.Amendment.Source.Revision, got.Amendment.Source.Live.Actual)
}

// TestNodePageAmendmentSourceLiveStaleOnAnEdit pins the middle of the
// honesty arc: `show amendment` (and this page) says stale once the
// feedback document changes underneath an already-staged amendment, ahead
// of approve ever refusing.
func TestNodePageAmendmentSourceLiveStaleOnAnEdit(t *testing.T) {
	root, amendmentID, relPath := amendmentSourceFixture(t, "the review body\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("edited\n"), 0o644))
	live, err := vcs.New(root).WorktreeBlobHash(relPath)
	require.NoError(t, err)

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got))

	require.NotNil(t, got.Amendment.Source)
	require.NotNil(t, got.Amendment.Source.Live)
	require.Equal(t, "stale", got.Amendment.Source.Live.Status)
	require.Equal(t, "git:"+live, got.Amendment.Source.Live.Actual)
}

// TestNodePageNonIntakeAmendmentCarriesNoSourceBlock pins the hand-opened
// case one grain up from TestNodePageAmendmentDegradesOnEvaluatorError: no
// Source at all, so the page's own Source key is absent (never a null live
// read for a fact that does not exist), the same honest-absence contract
// mutate.EvaluateAmendmentSource's own doc comment states.
func TestNodePageNonIntakeAmendmentCarriesNoSourceBlock(t *testing.T) {
	srv, amendmentID, _ := amendmentServer(t)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got))
	require.NotNil(t, got.Amendment)
	require.Nil(t, got.Amendment.Source, "amendmentFixture opens its amendment by hand, no Source")
}

// --- #57: source_missing, shape_error, and validity_error together --------

// amendmentHandEditFixture lays an intake-minted amendment (a real
// fingerprinted Source, the same feedback-artifact setup amendmentSourceFixture
// uses) staging three operations that between them trip every one of #57's
// console-side pre-flight signals: a revise carrying a staged precondition,
// later hand-blanked (source_missing) and hand-vanished (the resolution
// failure fillAmendmentValidity now names on ValidityError), and two
// add-epic operations staging the SAME key (shape_error:
// duplicate_staged_key on the second). One fixture exercising all three new
// fields at once, the same combined posture the CLI's own
// TestShowAmendmentRendersEveryHandEditDefectAlongsideTheResolutionFailure
// pins for `show amendment --json` (internal/cli/amendment_cmds_test.go).
//
// Built inline rather than composed from amendmentSourceFixture: every
// mutate call here has to share ONE deps (one sequentialMint counter),
// because the counter is per-prefix across the WHOLE call sequence
// (node_test.go's own doc comment) - a second deps instance restarting the
// counter would re-mint an event id the first deps already claimed on disk,
// tripping the store's own optimistic-lock conflict rather than the
// hand-edit defects this fixture means to set up.
func amendmentHandEditFixture(t *testing.T) (root, amendmentID string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	relPath := filepath.Join(".tiller", "feedback", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "feedback"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("the review body\n"), 0o644))

	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "init")

	hash, err := vcs.New(root).WorktreeBlobHash(relPath)
	require.NoError(t, err)

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
		res, err := mutate.AddAmendment(tree, deps, mutate.AmendmentInput{
			Key: "review-intake", Title: "Intake: review.md",
			Source: &store.AmendmentSource{Path: relPath, Revision: "git:" + hash},
		})
		amendmentID = res.ID
		return err
	})

	tree, err := store.Load(root)
	require.NoError(t, err)
	var featureID string
	for id := range tree.Features {
		featureID = id
	}
	require.NotEmpty(t, featureID)

	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID, Source: "docs/feedback/review.md#L1-2",
			Preconditions: map[string]string{"target_status": "draft"},
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening one", Source: "docs/feedback/review.md#L3-4",
		})
		return err
	})
	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpAddEpic, Key: "hardening", Title: "Hardening two", Source: "docs/feedback/review.md#L5-6",
		})
		return err
	})

	blankOpSourceOnDisk(t, root, amendmentID, 0)
	corruptAmendmentTarget(t, root, amendmentID, "F-vanished")

	return root, amendmentID
}

// TestNodePageAmendmentRendersHandEditDefectsAlongsideValidityError is the
// full pre-flight #57 asks the console for: an amendment carrying a blanked
// op source AND a duplicate staged key, on top of a vanished target, renders
// every per-op signal AND page.Amendment.ValidityError from the one page
// fetch, none of the four silencing any other - the same claim the CLI's own
// combined test pins for `show amendment --json`.
func TestNodePageAmendmentRendersHandEditDefectsAlongsideValidityError(t *testing.T) {
	root, amendmentID := amendmentHandEditFixture(t)
	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)

	var got amendmentPagePayload
	require.Equal(t, 200, getJSON(t, srv, "/api/node/amendment/"+amendmentID, &got),
		"a batch that cannot resolve, and carries hand-edit defects, still serves the page")

	require.NotNil(t, got.Amendment)
	require.NotEmpty(t, got.Amendment.ValidityError,
		"the resolution failure is named on the page, not silent")
	require.Len(t, got.Amendment.Operations, 3)

	op0, op1, op2 := got.Amendment.Operations[0], got.Amendment.Operations[1], got.Amendment.Operations[2]
	require.True(t, op0.SourceMissing,
		"the op-source evaluator is doc-only and needs no resolution, so it still fires even though the batch cannot resolve")
	require.Empty(t, op0.ShapeError, "a revise is not a shape defect either predicate covers")
	require.Len(t, op0.Preconditions, 1)
	require.Nil(t, op0.Preconditions[0].Live, "the batch cannot resolve, so the Live slot stays nil rather than a fabricated read")

	require.Empty(t, op1.ShapeError, "the first operation to stage the key is clean")
	require.False(t, op1.SourceMissing)
	require.Equal(t, "duplicate_staged_key", op2.ShapeError,
		"the shape evaluator is doc-only too and still flags the duplicate even though the batch cannot resolve")
	require.False(t, op2.SourceMissing)
}

// amendmentSourceReadableFixture lays an intake-minted amendment exactly
// like amendmentSourceFixture's own real-artifact setup, but ALSO stages one
// revise operation carrying a target_status precondition that holds against
// the live tree right now, so
// TestNodePageAmendmentSourceErrorAloneStillFillsPreconditionLive has
// something real to prove stays filled even when Source's own read fails
// alone. Built inline rather than composed from amendmentSourceFixture, the
// same reason amendmentHandEditFixture states for its own inline build: one
// shared deps for the whole call sequence, so a second sequentialMint
// instance never re-mints an event id the first already claimed on disk.
func amendmentSourceReadableFixture(t *testing.T) (root, amendmentID, relPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(serveManifest), 0o644))

	relPath = filepath.Join(".tiller", "feedback", "review.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "feedback"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, relPath), []byte("the review body\n"), 0o644))

	testgit.Init(t, root)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-m", "init")

	hash, err := vcs.New(root).WorktreeBlobHash(relPath)
	require.NoError(t, err)

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
		res, err := mutate.AddAmendment(tree, deps, mutate.AmendmentInput{
			Key: "review-intake", Title: "Intake: review.md",
			Source: &store.AmendmentSource{Path: relPath, Revision: "git:" + hash},
		})
		amendmentID = res.ID
		return err
	})

	tree, err := store.Load(root)
	require.NoError(t, err)
	var featureID string
	for id := range tree.Features {
		featureID = id
	}
	require.NotEmpty(t, featureID)

	step(func(tree *store.Tree) error {
		_, err := mutate.StageOperation(tree, deps, tree.Amendments[amendmentID], mutate.StageInput{
			Op: store.AmendmentOpRevise, Target: featureID, Source: "docs/feedback/review.md#L1-2",
			Preconditions: map[string]string{"target_status": "draft"},
		})
		return err
	})

	return root, amendmentID, relPath
}

func TestNodePageAmendmentSourceErrorAloneStillFillsPreconditionLive(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permission bits, so this repro needs a non-root process")
	}

	root, amendmentID, relPath := amendmentSourceReadableFixture(t)
	sourcePath := filepath.Join(root, relPath)
	require.NoError(t, os.Chmod(sourcePath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(sourcePath, 0o644) })

	srv := httptest.NewServer(serve.NodeHandler(root))
	t.Cleanup(srv.Close)
	response, err := srv.Client().Get(srv.URL + "/api/node/amendment/" + amendmentID)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)

	var got views.NodePage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
	require.Equal(t, amendmentID, got.Node.ID)
	require.False(t, got.Backlinks.Complete)
	require.NotNil(t, got.Amendment)
	require.NotNil(t, got.Amendment.Source)
	require.Nil(t, got.Amendment.Source.Live)
	require.Contains(t, got.Amendment.ValidityError, relPath)
	require.Len(t, got.Amendment.Operations, 1)
	require.Len(t, got.Amendment.Operations[0].Preconditions, 1)
	require.NotNil(t, got.Amendment.Operations[0].Preconditions[0].Live)
	var corpusWarning bool
	for _, diagnostic := range got.Backlinks.Diagnostics {
		if diagnostic.Code == "corpus_unreadable" {
			corpusWarning = true
			require.Contains(t, diagnostic.Message, relPath)
		}
	}
	require.True(t, corpusWarning)
}

// TestListAmendments pins /api/ls?noun=amendment(s): the row shape and both
// noun spellings resolving to the same class, the identical singular/plural
// convention every other noun already has on this endpoint.
func TestListAmendments(t *testing.T) {
	root, amendmentID, _ := amendmentFixture(t)
	srv := httptest.NewServer(serve.ListHandler(root))
	t.Cleanup(srv.Close)

	for _, noun := range []string{"amendment", "amendments"} {
		resp, err := srv.Client().Get(srv.URL + "/api/ls?noun=" + noun)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, 200, resp.StatusCode, "noun=%s", noun)

		var got struct {
			Noun  string `json:"noun"`
			Count int    `json:"count"`
			Nodes []struct {
				ID     string `json:"id"`
				Key    string `json:"key"`
				Title  string `json:"title"`
				Status string `json:"status"`
				Ref    string `json:"ref"`
			} `json:"nodes"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		require.Equal(t, "amendment", got.Noun, "noun=%s always echoes singular", noun)
		require.Equal(t, 1, got.Count, "noun=%s", noun)
		require.Len(t, got.Nodes, 1)
		require.Equal(t, amendmentID, got.Nodes[0].ID)
		require.Equal(t, "iphone-only", got.Nodes[0].Key)
		require.Equal(t, "Remove native iPad support from v1", got.Nodes[0].Title)
		require.Equal(t, "proposed", got.Nodes[0].Status)
	}
}

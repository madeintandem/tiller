package store

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureTreeRoot is the read-only valid .tiller/ tree under testdata.
const fixtureTreeRoot = "testdata/tree"

func TestLoad_FixtureTree(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	require.NotNil(t, tree.Manifest)
	assert.Equal(t, "acme", tree.Manifest.Project.Key)

	require.Len(t, tree.Epics, 1)
	require.Len(t, tree.Features, 1)
	require.Len(t, tree.Tasks, 1)
	require.Len(t, tree.Questions, 1)
	require.Len(t, tree.Amendments, 1)
}

// The addressable-class model: the folder-backed nodes carry a key and the one
// ledger item, question, does not (artifact-schema.md, References and IDs).
func TestLoad_KeyClass(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	feat := tree.Features["F-79dh3q"]
	require.NotNil(t, feat)
	require.NotNil(t, feat.Key, "a feature's key is non-nil")
	assert.Equal(t, "form-engine/certification-form-loading", *feat.Key)

	task := tree.Tasks["T-k3p9w6"]
	require.NotNil(t, task)
	require.NotNil(t, task.Key, "a task's key is non-nil")

	question := tree.Questions["Q-h2n7m5"]
	require.NotNil(t, question)
	assert.Nil(t, question.Key, "a question's key is nil")

	// And the same through the Node interface.
	var fn Node = feat
	var tn Node = task
	var qn Node = question
	require.NotNil(t, fn.NodeKey())
	require.NotNil(t, tn.NodeKey())
	assert.Nil(t, qn.NodeKey())
	assert.Equal(t, NounFeature, fn.Noun())
	assert.Equal(t, NounTask, tn.Noun())
	assert.Equal(t, NounQuestion, qn.Noun())
}

// Loaded nodes carry their parent linkage set from the directory layout, which
// is not part of the on-disk node shape.
func TestLoad_ParentLinkage(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	assert.Equal(t, "form-engine", tree.Features["F-79dh3q"].EpicKey)
	assert.Equal(t, "F-79dh3q", tree.Tasks["T-k3p9w6"].FeatureID)
}

// The stored gate block round-trips on load: the needs_attestation criterion and
// the AT- attestation survive (artifact-schema.md, The stored gate block).
func TestLoad_FeatureGatesBlock(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)
	feat := tree.Features["F-79dh3q"]
	require.NotNil(t, feat)

	build := feat.Gates["build"]
	assert.True(t, build.Required)
	assert.Equal(t, GateStatusPass, build.Status)
	assert.Equal(t, KindGitCommit, build.Against.Kind)
	require.Len(t, build.Evidence, 1)
	assert.Equal(t, "command_output", build.Evidence[0].Kind)

	evals := feat.Gates["evals"]
	assert.Equal(t, GateStatusPending, evals.Status)
	c3 := evals.Criteria["C3"]
	assert.Equal(t, CriterionStatusNeedsAttestation, c3.Status)
	require.Len(t, c3.Evidence, 1)
	assert.Equal(t, "rubric_output", c3.Evidence[0].Kind)

	require.Len(t, feat.Attestations, 1)
	at := feat.Attestations[0]
	assert.Equal(t, "AT-4m9x2q", at.ID)
	assert.Equal(t, "criterion", at.Target.Kind)
	assert.Equal(t, "C3", at.Target.ID)
	assert.Equal(t, "jc", at.By)
	assert.NotNil(t, feat.PolicyViolations) // present as an empty list on disk

	// accepted_against carries RevisionRefs for the active-work basis.
	assert.Equal(t, KindGitBlob, feat.AcceptedAgainst.Spec.Kind)
	assert.Equal(t, "git:31fd9c2", feat.AcceptedAgainst.Spec.Value)
}

// A loaded feature records whether its feature.eval.md acceptance contract
// exists on disk, set from the directory layout the same way as the parent
// linkage. An epic has no such field, and no such file: acceptance lives at
// feature grain (artifact-schema.md, Epics).
func TestLoad_EvalPresence(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	require.Len(t, tree.Epics, 1)
	assert.True(t, tree.Features["F-79dh3q"].HasEval, "the fixture feature has a feature.eval.md")
}

// amendmentFixtureID names the one amendment under testdata/tree's
// .tiller/amendments/, the same worked example task-1-brief.md pins verbatim.
const amendmentFixtureID = "A-d6w3f9"

// An amendment is a flat file pair under .tiller/amendments/, not a
// folder-backed node like the epic/feature/task tree above, but it still loads
// into the tree and answers the same Node view (artifact-schema.md,
// Amendments).
func TestLoad_Amendments(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	require.Len(t, tree.Amendments, 1)
	a := tree.Amendments[amendmentFixtureID]
	require.NotNil(t, a)
	require.NotNil(t, a.Key)
	assert.Equal(t, "iphone-only", *a.Key)
	assert.Equal(t, AmendmentStatusProposed, a.Status)
	assert.Equal(t, "git:78d2166", a.BaseRevision)
	require.Len(t, a.Operations, 5)
	assert.Equal(t, AmendmentOpRevise, a.Operations[0].Op)

	var n Node = a
	assert.Equal(t, amendmentFixtureID, n.NodeID())
	assert.Equal(t, NounAmendment, n.Noun())
	assert.Equal(t, AmendmentStatusProposed, n.NodeStatus())
}

// A project with no amendments yet has no amendments/ directory at all, and
// that absence is an empty map, never an error, the same lazy-subtree rule
// loadQuestions already follows for open-questions.yml.
func TestLoad_AmendmentsDirAbsentIsEmptyMap(t *testing.T) {
	tree, err := Load(siblingFixtureRoot)
	require.NoError(t, err)
	require.NotNil(t, tree.Amendments, "an absent directory is an empty map, not a nil one left unset")
	assert.Len(t, tree.Amendments, 0)
}

// A malformed amendment file surfaces the same load error shape every other
// node's malformed YAML does: a parse error naming the path.
func TestLoad_AmendmentMalformedYAMLIsRefused(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(root, ".tiller", "amendments", "A-d6w3f9-iphone-only.yml")
	require.NoError(t, os.WriteFile(path, []byte("id: [this is not a mapping\n"), 0o644))

	_, err := Load(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

// Two amendment files claiming the same id is the same registry refusal a
// duplicated epic, feature, task, or question id gets: one id names one node.
func TestLoad_DuplicateAmendmentIDIsRefused(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	src := filepath.Join(root, ".tiller", "amendments", "A-d6w3f9-iphone-only.yml")
	dup := filepath.Join(root, ".tiller", "amendments", "A-d6w3f9-second.yml")
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dup, data, 0o644))

	_, err = Load(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), NounAmendment)
	assert.Contains(t, err.Error(), amendmentFixtureID)
}

func TestLoad_Question(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	q := tree.Questions["Q-h2n7m5"]
	require.NotNil(t, q)
	assert.Nil(t, q.Key, "a question is a ledger item with no key")
	assert.Equal(t, "open", q.Status)
	assert.Equal(t, "2026-08-15", q.Due.Date)
	assert.Equal(t, "end of week 6", q.Due.Hint)
	assert.Equal(t, []string{"E-j8m4q2"}, q.Blocks)
}

// siblingFixtureRoot is a tree whose one feature holds three task folders whose
// local key order (alpha-first, bravo-second, charlie-third) is the exact
// reverse of their id sort (T-aaaaaa, T-mmmmmm, T-zzzzzz), so an id-keyed map
// alone could never reproduce the sibling order.
const siblingFixtureRoot = "testdata/sibling-tree"

// Tree carries a per-feature grouping beside the id-keyed map, in local key
// order, aliasing the same *Task values Tasks resolves by id.
func TestLoad_FeatureTasks(t *testing.T) {
	tree, err := Load(siblingFixtureRoot)
	require.NoError(t, err)

	const featureID = "F-q3v8n5"
	siblings := tree.FeatureTasks[featureID]
	require.Len(t, siblings, 3)

	ids := make([]string, len(siblings))
	keys := make([]string, len(siblings))
	for i, task := range siblings {
		ids[i] = task.ID
		require.NotNil(t, task.Key)
		keys[i] = *task.Key
	}
	assert.Equal(t, []string{
		"sibling-epic/sibling-feature/alpha-first",
		"sibling-epic/sibling-feature/bravo-second",
		"sibling-epic/sibling-feature/charlie-third",
	}, keys, "siblings group in local key order")
	assert.Equal(t, []string{"T-zzzzzz", "T-mmmmmm", "T-aaaaaa"}, ids,
		"which is not id order")

	// Tasks and FeatureTasks must point at the same *Task values, not copies.
	for i, id := range ids {
		assert.Same(t, tree.Tasks[id], siblings[i])
	}

	// A task carries the orthogonal blocked flag the two grains above it do
	// (artifact-schema.md, Tasks), so it has to be able to load true.
	assert.True(t, tree.Tasks["T-mmmmmm"].Blocked, "the fixture task records blocked: true")
	assert.False(t, tree.Tasks["T-zzzzzz"].Blocked)
}

// A task is a folder node like every other grain: tasks/<task-key>/task.md holds
// the implementation spec and task.yml holds the state, and the canonical key is
// the three-segment <epic-key>/<feature-key>/<task-key> (artifact-schema.md,
// Tasks; Keys and paths).
func TestLoadReadsFolderBackedTasks(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	task := tree.Tasks["T-k3p9w6"]
	require.NotNil(t, task)
	require.NotNil(t, task.Key, "a task is a keyed, folder-backed node")
	assert.Equal(t, "form-engine/certification-form-loading/schema-transcription", *task.Key)
	assert.Equal(t, "Transcribe CERT-LTI-02-A (2025) into JSON Schema with computed-field bindings", task.Title)
	assert.Equal(t, "F-79dh3q", task.FeatureID, "the parent link comes from the folder path")
}

// The two fields the stage gates ahead will read: depends_on, the recorded
// sibling sequence folders cannot carry (artifact-schema.md, Tasks: Ordering),
// and accepted_against, the task.md blob pinned at approve (Tasks: States).
// Nothing in this build writes either one, so this pins that they survive a load
// rather than being dropped on the floor.
func TestLoad_TaskDependsOnAndAcceptedBasis(t *testing.T) {
	tree, err := Load(fixtureTreeRoot)
	require.NoError(t, err)

	task := tree.Tasks["T-k3p9w6"]
	require.NotNil(t, task)
	assert.Equal(t, []string{"T-9wq4m1"}, task.DependsOn)

	require.NotNil(t, task.AcceptedAgainst, "the pinned basis is a pointer: an unapproved task has none")
	assert.Equal(t, KindGitBlob, task.AcceptedAgainst.Spec.Kind)
	assert.Equal(t, "git:5c1e8a9", task.AcceptedAgainst.Spec.Value)
	assert.Zero(t, task.AcceptedAgainst.Eval, "the fixture pins a spec leg alone, the shape the schema shows")
}

// copyTree materializes a testdata tree into a writable temp dir so a test can
// perturb the layout without touching the read-only fixture.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	require.NoError(t, filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}))
	return dst
}

func TestLoadRefusesInvalidEpicReviewGrain(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("status:"), []byte("review_grain: sideways\nstatus:"), 1)
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err = Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), path)
	require.Contains(t, err.Error(), `epic review_grain "sideways"`)
	require.Contains(t, err.Error(), "task, feature, or epic")
}

func TestLoadAllowsExplicitEpicGrainOnMovedFeature(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("status:"), []byte("review_grain: epic\nstatus:"), 1)
	require.NoError(t, os.WriteFile(path, data, 0644))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, "epic", tree.Features["F-79dh3q"].ReviewGrain)
}

// tasksDirOf is the fixture feature's tasks/ folder inside a copied tree.
func tasksDirOf(root string) string {
	return filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "tasks")
}

// A directory under tasks/ is a claim that a node lives there, so a missing
// task.yml fails the load, the same refusal a feature folder
// with no feature.yml already makes. Skipping it would drop the node from every
// surface with nothing to say it had been dropped.
func TestLoad_TaskFolderWithNoStateFileIsRefused(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(tasksDirOf(root), "half-written"), 0o755))

	_, err := Load(root)
	require.Error(t, err)
}

// The loader reads state, not prose: no grain's .md is read here, and a task
// whose implementation spec has not been written yet still loads. Whether that
// spec is written is a gate's question, asked over content the gate reads
// itself.
func TestLoad_TaskFolderWithNoProseStillLoads(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.Remove(filepath.Join(tasksDirOf(root), "schema-transcription", TaskProseFile)))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Contains(t, tree.Tasks, "T-k3p9w6")
}

// A node whose recorded key does not end in the folder it was found in is
// refused, because the loader identifies it by the folder while every path
// derived for it afterwards is rebuilt from the key. Hand-moving a task folder
// is the way this happens today: there is no rename verb at task grain.
func TestLoad_TaskFolderDisagreeingWithItsKeyIsRefused(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.Rename(
		filepath.Join(tasksDirOf(root), "schema-transcription"),
		filepath.Join(tasksDirOf(root), "moved-elsewhere")))

	_, err := Load(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "form-engine/certification-form-loading/schema-transcription", "the error names the recorded key")
	assert.Contains(t, err.Error(), "moved-elsewhere", "and the folder it disagrees with")
}

// The same rule at the two grains above, so one answer covers every
// folder-backed node rather than the newest one only.
func TestLoad_FeatureAndEpicFoldersDisagreeingWithTheirKeysAreRefused(t *testing.T) {
	featureRoot := copyTree(t, fixtureTreeRoot)
	features := filepath.Join(featureRoot, ".tiller", "epics", "form-engine", "features")
	require.NoError(t, os.Rename(filepath.Join(features, "certification-form-loading"), filepath.Join(features, "moved-elsewhere")))
	_, err := Load(featureRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "form-engine/certification-form-loading")

	epicRoot := copyTree(t, fixtureTreeRoot)
	epics := filepath.Join(epicRoot, ".tiller", "epics")
	require.NoError(t, os.Rename(filepath.Join(epics, "form-engine"), filepath.Join(epics, "moved-elsewhere")))
	_, err = Load(epicRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "form-engine")
}

func TestLoad_TwoSourcesClaimingOneIDAreRefused(t *testing.T) {
	tests := []struct {
		noun    string
		nodeID  string
		arrange func(t *testing.T, root string) (string, string)
	}{
		{
			noun:   NounEpic,
			nodeID: "E-j8m4q2",
			arrange: func(t *testing.T, root string) (string, string) {
				first := filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
				second := filepath.Join(root, ".tiller", "epics", "second-epic", "epic.yml")
				data, err := os.ReadFile(first)
				require.NoError(t, err)
				data = []byte(strings.Replace(string(data), "key: form-engine\n", "key: second-epic\n", 1))
				require.NoError(t, os.MkdirAll(filepath.Dir(second), 0o755))
				require.NoError(t, os.WriteFile(second, data, 0o644))
				return first, second
			},
		},
		{
			noun:   NounFeature,
			nodeID: "F-79dh3q",
			arrange: func(t *testing.T, root string) (string, string) {
				first := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
				second := filepath.Join(root, ".tiller", "epics", "form-engine", "features", "second-feature", "feature.yml")
				data, err := os.ReadFile(first)
				require.NoError(t, err)
				data = []byte(strings.Replace(string(data),
					"key: form-engine/certification-form-loading\n", "key: form-engine/second-feature\n", 1))
				require.NoError(t, os.MkdirAll(filepath.Dir(second), 0o755))
				require.NoError(t, os.WriteFile(second, data, 0o644))
				return first, second
			},
		},
		{
			noun:   NounTask,
			nodeID: "T-k3p9w6",
			arrange: func(t *testing.T, root string) (string, string) {
				first := filepath.Join(tasksDirOf(root), "schema-transcription", TaskStateFile)
				second := filepath.Join(tasksDirOf(root), "render-the-form", TaskStateFile)
				require.NoError(t, os.MkdirAll(filepath.Dir(second), 0o755))
				require.NoError(t, os.WriteFile(second, []byte(
					"id: T-k3p9w6\n"+
						"key: form-engine/certification-form-loading/render-the-form\n"+
						"title: Render the form\n"+
						"status: ready\n"+
						"blocked: false\n"+
						"owner: jc\n"), 0o644))
				return first, second
			},
		},
		{
			noun:   NounQuestion,
			nodeID: "Q-h2n7m5",
			arrange: func(t *testing.T, root string) (string, string) {
				path := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				data = append(data, []byte(
					"  - id: Q-h2n7m5\n"+
						"    statement: Is this the second claim?\n"+
						"    owner: jc\n"+
						"    status: open\n")...)
				require.NoError(t, os.WriteFile(path, data, 0o644))
				return path + "[0]", path + "[1]"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.noun, func(t *testing.T) {
			root := copyTree(t, fixtureTreeRoot)
			first, second := tt.arrange(t, root)

			tree, err := Load(root)
			require.Error(t, err)
			assert.Nil(t, tree, "a registry error must not return a partial tree")
			assert.Contains(t, err.Error(), tt.noun)
			assert.Contains(t, err.Error(), tt.nodeID)
			assert.Contains(t, err.Error(), first)
			assert.Contains(t, err.Error(), second)
		})
	}
}

func TestLoad_InvalidOrWrongNounIDsAreRefused(t *testing.T) {
	tests := []struct {
		noun       string
		id         string
		validID    string
		sourcePath func(root string) string
	}{
		{
			noun:    NounEpic,
			id:      "not-an-id",
			validID: "E-j8m4q2",
			sourcePath: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "form-engine", "epic.yml")
			},
		},
		{
			noun:    NounFeature,
			id:      "E-aaaaaa",
			validID: "F-79dh3q",
			sourcePath: func(root string) string {
				return filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "feature.yml")
			},
		},
		{
			noun:    NounTask,
			id:      "F-aaaaaa",
			validID: "T-k3p9w6",
			sourcePath: func(root string) string {
				return filepath.Join(tasksDirOf(root), "schema-transcription", TaskStateFile)
			},
		},
		{
			noun:    NounQuestion,
			id:      "Q-AAAAAA",
			validID: "Q-h2n7m5",
			sourcePath: func(root string) string {
				return filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.noun, func(t *testing.T) {
			root := copyTree(t, fixtureTreeRoot)
			path := tt.sourcePath(root)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			corrupt := strings.Replace(string(data), "id: "+tt.validID, "id: "+tt.id, 1)
			require.NotEqual(t, string(data), corrupt)
			require.NoError(t, os.WriteFile(path, []byte(corrupt), 0o644))

			source := path
			if tt.noun == NounQuestion {
				source += "[0]"
			}
			tree, err := Load(root)
			require.Error(t, err)
			assert.Nil(t, tree, "an invalid id must not return a partial tree")
			assert.Contains(t, err.Error(), tt.noun)
			assert.Contains(t, err.Error(), tt.id)
			assert.Contains(t, err.Error(), source)
		})
	}
}

// A node that records no key at all is a different fact from one that records a
// disagreeing key: nothing derives a path for it, so it loads and simply does
// not resolve by key.
func TestLoad_NodeWithNoRecordedKeyStillLoads(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(tasksDirOf(root), "schema-transcription", TaskStateFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	stripped := strings.ReplaceAll(string(data),
		"key: form-engine/certification-form-loading/schema-transcription\n", "")
	require.NotEqual(t, string(data), stripped, "the fixture carried a key to strip")
	require.NoError(t, os.WriteFile(path, []byte(stripped), 0o644))

	tree, err := Load(root)
	require.NoError(t, err)
	task := tree.Tasks["T-k3p9w6"]
	require.NotNil(t, task)
	assert.Nil(t, task.Key)
}

// A stray file under tasks/ is not a node position and is skipped, matching how
// the epics/ and features/ walks skip non-directory entries.
func TestLoad_StrayFileUnderTasksIsSkipped(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	require.NoError(t, os.WriteFile(filepath.Join(tasksDirOf(root), "README.md"), []byte("notes\n"), 0o644))

	tree, err := Load(root)
	require.NoError(t, err)
	assert.Len(t, tree.Tasks, 1)
}

func TestLoad_NotARepo(t *testing.T) {
	_, err := Load(t.TempDir())
	require.Error(t, err)
	var ee *EnvError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, ReasonNotARepo, ee.Reason)
	assert.Equal(t, 3, ee.ExitCode())
}

func TestLoadRefusesWhileAMutationIsLocked(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	tree, err := Load(root)
	require.NoError(t, err)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- tree.withLock(func(*journal) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	_, err = Load(root)
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonLocked, env.Reason)

	close(release)
	require.NoError(t, <-done)
}

func TestStableGenerationComparison(t *testing.T) {
	require.NoError(t, stableGeneration("same", "same"))

	err := stableGeneration("before", "after")
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
}

func TestStableGenerationWithLockAdvancesMarker(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	tree, err := Load(root)
	require.NoError(t, err)
	before := tree.generation

	require.NoError(t, tree.withLock(func(*journal) error { return nil }))

	after, err := readableGeneration(root)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestLoadPrefersOverlappingMutationStateToTreeReadError(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)

	_, err := loadStable(root, func(string) (*Tree, error) {
		lockPath := filepath.Join(root, ".tiller", cacheDirName, lockFileName)
		require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o755))
		require.NoError(t, os.WriteFile(lockPath, []byte("held"), 0o600))
		return nil, fs.ErrNotExist
	})
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonLocked, env.Reason)
}

func TestLoadedTreeStabilityRecheckDetectsLaterGeneration(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	tree, err := Load(root)
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Dir(tree.generationPath()), 0o755))
	require.NoError(t, os.WriteFile(tree.generationPath(), []byte("new-generation"), 0o600))

	err = tree.CheckStable()
	require.Error(t, err)
	var env *EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonMutationStale, env.Reason)
}

func TestLoadIdentityPreservingMovedKeysKeepStableParentLinks(t *testing.T) {
	root := writeManifestFixture(t, "schema_version: 12\nschema_epoch: initial\nproject:\n  name: X\n  key: x\n  kind: greenfield\ncontributors: []\n")
	epicDir := filepath.Join(root, ".tiller", "epics", "platform-foundation")
	featureDir := filepath.Join(epicDir, "features", "application")
	taskDir := filepath.Join(featureDir, "tasks", "bootstrap")
	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(epicDir, "epic.yml"), []byte(
		"id: E-found1\nkey: platform-foundation\nstatus: draft\nblocked: false\norder: [F-app001]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(featureDir, "feature.yml"), []byte(
		"id: F-app001\nkey: platform-foundation/application\nstatus: draft\nblocked: false\norder: [T-scaf01]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "task.yml"), []byte(
		"id: T-scaf01\nkey: platform-foundation/application/bootstrap\nstatus: proposed\nblocked: false\n"), 0o644))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, "platform-foundation", *tree.Epics["E-found1"].Key)
	require.Equal(t, []string{"F-app001"}, tree.Epics["E-found1"].Order)
	require.Equal(t, "platform-foundation/application", *tree.Features["F-app001"].Key)
	require.Equal(t, "platform-foundation", tree.Features["F-app001"].EpicKey)
	require.Equal(t, "platform-foundation/application/bootstrap", *tree.Tasks["T-scaf01"].Key)
	require.Equal(t, "F-app001", tree.Tasks["T-scaf01"].FeatureID)
}

// TestLoad_NullQuestionEntryIsAnError confirms a null entry anywhere in the
// questions ledger is a normal parse error naming the ledger path and the entry
// position, never a nil dereference, and that no partial tree is returned.
func TestLoad_NullQuestionEntryIsAnError(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"first": func(s string) string { return strings.Replace(s, "questions:\n", "questions:\n  - null\n", 1) },
		"last":  func(s string) string { return s + "\n  - null\n" },
		"bare":  func(s string) string { return strings.Replace(s, "questions:\n", "questions:\n  -\n", 1) },
		"middle": func(s string) string {
			return s + "\n  - null\n  - id: Q-a1b2c3\n    statement: x\n    owner: jc\n    status: open\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := copyTree(t, fixtureTreeRoot)
			path := filepath.Join(root, ".tiller", "decisions", "open-questions.yml")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(edit(string(data))), 0o644))

			var tree *Tree
			require.NotPanics(t, func() { tree, err = Load(root) })
			require.Error(t, err)
			require.Nil(t, tree)
			require.Contains(t, err.Error(), path)
			require.Contains(t, err.Error(), "null")
		})
	}
}

func TestLoadRefusesInvalidTaskStartedGrain(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(tasksDirOf(root), "schema-transcription", "task.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("status:"), []byte("started_grain: sideways\nstatus:"), 1)
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err = Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), path)
	require.Contains(t, err.Error(), `task started_grain "sideways"`)
	require.Contains(t, err.Error(), "task, feature, or epic")
}

func TestLoadReadsTaskStartedGrain(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	path := filepath.Join(tasksDirOf(root), "schema-transcription", "task.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("status:"), []byte("started_grain: task\nstatus:"), 1)
	require.NoError(t, os.WriteFile(path, data, 0644))

	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, "task", tree.Tasks["T-k3p9w6"].StartedGrain)
}

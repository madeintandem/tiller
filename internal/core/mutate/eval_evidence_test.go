package mutate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/stretchr/testify/require"
)

// These tests pin retained rubric evidence (artifact-schema.md, "The rubric
// handoff"): a recorded rubric output is kept under a name derived from its
// bytes, so a reference an attestation or an earlier criterion state holds
// keeps naming the bytes it named when it was written. Neither a regrade
// written to the packet's output path, nor a later recording, nor a recording
// the store refuses as stale may change them.

// rubricFixture is one started task under a feature whose contract carries a
// rubric criterion (C2), the smallest tree a rubric recording runs against.
type rubricFixture struct {
	judgedFixture
	task string
}

func layRubricTask(t *testing.T) rubricFixture {
	t.Helper()
	root := writeGatedRepo(t, "true", "true")
	seq := 0
	f := rubricFixture{judgedFixture: judgedFixture{root: root, deps: amendDepsCounting(root, &seq)}}
	_, err := mutate.AddEpic(load(t, root), f.deps, mutate.EpicInput{Key: "app", Title: "App"})
	require.NoError(t, err)
	driveWith(t, root, f.deps, store.NounEpic, "app", mutate.TransitionInput{Verb: "approve"})
	_, tasks := layJudgedFeature(t, &f.judgedFixture, "accept", "live")
	f.task = tasks[0]
	driveWith(t, root, f.deps, store.NounTask, f.task, mutate.TransitionInput{Verb: "start"})
	return f
}

func rubricJSON(verdict, rationale string) []byte {
	return []byte(fmt.Sprintf(
		`{"schema": "tiller.rubric_output.v1", "criterion": "C2", "verdict": %q, "rationale": %q}`,
		verdict, rationale))
}

// record runs one rubric recording of the file at path against tree, which the
// caller loads so a test can hold a tree across another mutation.
func (f rubricFixture) record(tree *store.Tree, path string) error {
	_, err := mutate.Eval(tree, f.deps, vcs.New(f.root), tree.Tasks[f.task],
		mutate.EvalInput{Criterion: "C2", RecordRubricOutput: path})
	return err
}

// recordBytes writes body to a file outside the checkout and records it.
func (f rubricFixture) recordBytes(t *testing.T, body []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rubric.json")
	require.NoError(t, os.WriteFile(path, body, 0o644))
	require.NoError(t, f.record(load(t, f.root), path))
}

func (f rubricFixture) attestRubric(t *testing.T) store.Attestation {
	t.Helper()
	tree := load(t, f.root)
	_, err := mutate.Attest(tree, f.deps, tree.Tasks[f.task], mutate.AttestInput{Criterion: "C2", Status: store.GateStatusPass})
	require.NoError(t, err)
	attestations := load(t, f.root).Tasks[f.task].Attestations
	require.NotEmpty(t, attestations)
	return attestations[len(attestations)-1]
}

func (f rubricFixture) rubricCriterion(t *testing.T) store.Criterion {
	t.Helper()
	return load(t, f.root).Tasks[f.task].Gates["evals"].Criteria["C2"]
}

func (f rubricFixture) evidence(t *testing.T, ref string) string {
	t.Helper()
	require.NotEmpty(t, ref)
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(ref)))
	require.NoError(t, err)
	return string(data)
}

// outputPath is the path the rubric packet tells the agent to write C2's
// graded output to.
func (f rubricFixture) outputPath(t *testing.T) string {
	t.Helper()
	tree := load(t, f.root)
	res, err := mutate.Eval(tree, f.deps, vcs.New(f.root), tree.Tasks[f.task], mutate.EvalInput{EmitRubrics: true})
	require.NoError(t, err)
	require.Len(t, res.Packet.Criteria, 1)
	return filepath.Join(f.root, filepath.FromSlash(res.Packet.Criteria[0].OutputPath))
}

// A regrade keeps the evidence a human already attested. The agent's regrade
// reaches Tiller either through the packet's output path (the documented
// handoff) or through a file of its own; in both, the attestation's
// evidence_ref still names the first grade's bytes and the criterion points at
// the new grade.
func TestRubricRegradeKeepsAttestedEvidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		path func(t *testing.T, f rubricFixture) string
	}{
		{"through the packet's output path", func(t *testing.T, f rubricFixture) string { return f.outputPath(t) }},
		{"through a file of the agent's own", func(t *testing.T, f rubricFixture) string {
			return filepath.Join(t.TempDir(), "rubric.json")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := layRubricTask(t)
			path := tc.path(t, f)
			first := rubricJSON("pass", "Reads naturally.")
			second := rubricJSON("fail", "Reads stiffly after the rewrite.")

			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, first, 0o644))
			require.NoError(t, f.record(load(t, f.root), path))
			attested := f.attestRubric(t)
			require.Equal(t, string(first), f.evidence(t, attested.EvidenceRef))

			require.NoError(t, os.WriteFile(path, second, 0o644))
			require.Equal(t, string(first), f.evidence(t, attested.EvidenceRef),
				"writing the regrade where the packet says must not change the attested evidence")
			require.NoError(t, f.record(load(t, f.root), path))

			regraded := f.rubricCriterion(t)
			require.Equal(t, store.CriterionStatusFail, regraded.Status)
			require.Len(t, regraded.Evidence, 1)
			require.NotEqual(t, attested.EvidenceRef, regraded.Evidence[0].Ref, "the regrade is a new artifact")
			require.Equal(t, string(second), f.evidence(t, regraded.Evidence[0].Ref))

			history := load(t, f.root).Tasks[f.task].Attestations
			require.Equal(t, attested, history[len(history)-1], "the attestation is history, not rewritten")
			require.Equal(t, string(first), f.evidence(t, attested.EvidenceRef),
				"the attestation still names the grade the human reviewed")
		})
	}
}

// Two recordings of one criterion overlap: both load the tree, one commits, and
// the store refuses the other as stale. The refused recording changes nothing a
// reference names: the winner's status and evidence, the earlier attestation's
// evidence, the task state, and the event stream all stand.
func TestRefusedRubricRecordingLeavesReferencedEvidenceUnchanged(t *testing.T) {
	t.Parallel()
	f := layRubricTask(t)
	first := rubricJSON("pass", "Reads naturally.")
	second := rubricJSON("pass", "Still reads naturally after the edit.")
	third := rubricJSON("fail", "A grade the store refuses.")

	f.recordBytes(t, first)
	attested := f.attestRubric(t)

	loser := load(t, f.root)
	f.recordBytes(t, second)
	winner := f.rubricCriterion(t)
	require.Len(t, winner.Evidence, 1)
	state := f.taskBytes(t, f.task)
	recorded, err := events.List(load(t, f.root))
	require.NoError(t, err)

	late := filepath.Join(t.TempDir(), "rubric.json")
	require.NoError(t, os.WriteFile(late, third, 0o644))
	err = f.record(loser, late)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, store.ReasonMutationStale, env.Reason)

	require.Equal(t, state, f.taskBytes(t, f.task), "a refused recording leaves status and attestations as they were")
	after, err := events.List(load(t, f.root))
	require.NoError(t, err)
	require.Len(t, after, len(recorded), "a refused recording appends no event")
	require.Equal(t, string(second), f.evidence(t, winner.Evidence[0].Ref),
		"the accepted recording's evidence keeps its bytes")
	require.Equal(t, string(first), f.evidence(t, attested.EvidenceRef),
		"the attested evidence keeps its bytes")
}

// A rubric output Tiller refuses records nothing and retains no evidence.
func TestInvalidRubricOutputRetainsNoEvidence(t *testing.T) {
	t.Parallel()
	f := layRubricTask(t)
	state := f.taskBytes(t, f.task)
	path := filepath.Join(t.TempDir(), "rubric.json")
	require.NoError(t, os.WriteFile(path, rubricJSON("maybe", "Not a verdict."), 0o644))

	require.Error(t, f.record(load(t, f.root), path))

	require.Equal(t, state, f.taskBytes(t, f.task))
	require.NoDirExists(t, filepath.Join(f.root, "artifacts", "tiller", "evals"))
}

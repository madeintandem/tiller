package gates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// fixedTime is the injected instant the packet golden is stamped with, so the
// generated_at field is deterministic.
var fixedTime = time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)

// sampleEval is an eval file with one criterion of each binding kind, the fixture
// the packet and criteria-evaluation tests run against.
func sampleEval() *store.EvalFile {
	return &store.EvalFile{
		Node: "F-aaaaaa",
		Criteria: []store.EvalCriterion{
			{ID: "C1", Statement: "The form validates against its schema", Polarity: "introduce",
				Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/form_spec.rb"}}, Required: true},
			{ID: "C2", Statement: "Renders to a single PDF", Polarity: "introduce",
				Binding: store.EvalBinding{Type: store.BindingCommand, Run: "true"}, Required: true},
			{ID: "C3", Statement: "A reviewer confirms the PDF matches the source layout", Polarity: "introduce",
				Binding: store.EvalBinding{Type: store.BindingManual}, Required: true},
			{ID: "C4", Statement: "The generated copy reads naturally for a homeowner", Polarity: "introduce",
				Binding: store.EvalBinding{Type: store.BindingEval, Run: "Grade whether the copy reads naturally."}, Required: true},
		},
	}
}

// TestBuildPacketGolden pins the rubric packet shape: only eval-binding criteria,
// each carrying its rubric guidance and the output path the agent writes back to.
func TestBuildPacketGolden(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "rubric_packet.json"))
	require.NoError(t, err)

	packet := BuildPacket(store.NounFeature, "F-aaaaaa", sampleEval(), fixedTime)
	got, err := json.MarshalIndent(packet, "", "  ")
	require.NoError(t, err)
	require.Equal(t, string(golden), string(got)+"\n")
}

// TestBuildPacketEmptyIsArray confirms a node with no eval criteria yields an empty
// JSON array, not null, so the consumer always parses a list.
func TestBuildPacketEmptyIsArray(t *testing.T) {
	packet := BuildPacket(store.NounFeature, "F-bbbbbb", &store.EvalFile{Node: "F-bbbbbb"}, fixedTime)
	b, err := json.Marshal(packet)
	require.NoError(t, err)
	require.Contains(t, string(b), `"criteria":[]`)
}

// TestEvaluateCriteria runs the mechanical bindings and classifies the rest: a
// command binding runs, a test binding runs via the suite runner, a required manual
// criterion needs attestation, and an eval criterion is pending until graded.
func TestEvaluateCriteria(t *testing.T) {
	root := t.TempDir()
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"tests": {Kind: "command", Run: "true"},
	}}}

	criteria, err := EvaluateCriteria(sampleEval(), nil, m, root, CriteriaRun{Against: "abc1234", At: fixedTime})
	require.NoError(t, err)

	require.Equal(t, store.GateStatusPass, criteria["C1"].Status, "test binding runs via the suite runner")
	require.Equal(t, store.GateStatusPass, criteria["C2"].Status, "command binding runs")
	require.Equal(t, store.CriterionStatusNeedsAttestation, criteria["C3"].Status, "required manual awaits a human")
	require.Equal(t, store.CriterionStatusPending, criteria["C4"].Status, "eval awaits a rubric grade")

	require.Equal(t, store.GateStatusPending, GateStatusFromCriteria(sampleEval(), criteria),
		"a needs_attestation/pending required criterion keeps the gate pending")
	require.False(t, RequiredFailed(sampleEval(), criteria), "nothing failed")
}

// TestEvaluateCriteriaPreservesHumanGrade confirms a re-evaluation does not
// downgrade a human-attested pass or a recorded needs_attestation.
func TestEvaluateCriteriaPreservesHumanGrade(t *testing.T) {
	root := t.TempDir()
	prior := map[string]store.Criterion{
		"C3": {Status: store.CriterionStatusPass},             // human attested
		"C4": {Status: store.CriterionStatusNeedsAttestation}, // rubric recorded
	}
	criteria, err := EvaluateCriteria(sampleEval(), prior, nil, root, CriteriaRun{Against: "abc1234", At: fixedTime})
	require.NoError(t, err)
	require.Equal(t, store.CriterionStatusPass, criteria["C3"].Status)
	require.Equal(t, store.CriterionStatusNeedsAttestation, criteria["C4"].Status)
}

// TestAllRequiredSatisfied is the honest-passed contract: a run is satisfied only
// when every required criterion is an explicit pass. A pending, needs_attestation,
// or failed required criterion leaves it unsatisfied, while a non-required
// criterion in any state does not hold it back. A nil contract is vacuously
// satisfied (no required criteria), the clean no-op an unspecced feature gets.
func TestAllRequiredSatisfied(t *testing.T) {
	require.True(t, AllRequiredSatisfied(nil, nil), "no contract, no required criteria, vacuously satisfied")

	pass := map[string]store.Criterion{
		"C1": {Status: store.CriterionStatusPass},
		"C2": {Status: store.CriterionStatusPass},
		"C3": {Status: store.CriterionStatusPass},
		"C4": {Status: store.CriterionStatusPass},
	}
	require.True(t, AllRequiredSatisfied(sampleEval(), pass), "every required criterion passed")

	for _, st := range []string{
		store.CriterionStatusPending,
		store.CriterionStatusNeedsAttestation,
		store.CriterionStatusFail,
	} {
		crit := map[string]store.Criterion{
			"C1": {Status: store.CriterionStatusPass},
			"C2": {Status: store.CriterionStatusPass},
			"C3": {Status: st}, // a required criterion not yet a pass
			"C4": {Status: store.CriterionStatusPass},
		}
		require.Falsef(t, AllRequiredSatisfied(sampleEval(), crit),
			"a required criterion in %q is not satisfied", st)
	}

	// A non-required criterion that is not a pass does not hold the run back.
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{
		{ID: "R", Binding: store.EvalBinding{Type: store.BindingManual}, Required: true},
		{ID: "O", Binding: store.EvalBinding{Type: store.BindingManual}, Required: false},
	}}
	crit := map[string]store.Criterion{
		"R": {Status: store.CriterionStatusPass},
		"O": {Status: store.CriterionStatusPending},
	}
	require.True(t, AllRequiredSatisfied(ef, crit), "an optional criterion does not gate the run")
}

// TestRecordedCriterionStatus pins the rule that a pass verdict is never a pass: an
// agent-graded pass lands as needs_attestation, a fail is recorded red.
func TestRecordedCriterionStatus(t *testing.T) {
	require.Equal(t, store.CriterionStatusNeedsAttestation, RecordedCriterionStatus(RubricVerdictPass))
	require.Equal(t, store.CriterionStatusFail, RecordedCriterionStatus(RubricVerdictFail))
}

// TestValidateRubricOutput exercises the validation arms: schema, node, verdict,
// and the criterion-must-be-an-eval-binding rule.
func TestValidateRubricOutput(t *testing.T) {
	ef := sampleEval()

	good := RubricOutput{Schema: RubricOutputSchema, Node: "F-aaaaaa", Criterion: "C4", Verdict: "pass"}
	require.NoError(t, ValidateRubricOutput(good, "F-aaaaaa", ef))

	wrongSchema := good
	wrongSchema.Schema = "nope"
	require.Error(t, ValidateRubricOutput(wrongSchema, "F-aaaaaa", ef))

	wrongNode := good
	wrongNode.Node = "F-zzzzzz"
	require.Error(t, ValidateRubricOutput(wrongNode, "F-aaaaaa", ef))

	badVerdict := good
	badVerdict.Verdict = "maybe"
	require.Error(t, ValidateRubricOutput(badVerdict, "F-aaaaaa", ef))

	notEval := good
	notEval.Criterion = "C2" // a command binding, not eval
	require.Error(t, ValidateRubricOutput(notEval, "F-aaaaaa", ef))

	unknown := good
	unknown.Criterion = "C9"
	require.Error(t, ValidateRubricOutput(unknown, "F-aaaaaa", ef))
}

// TestParseRubricOutputRejectsGarbage confirms malformed JSON is a typed misuse.
func TestParseRubricOutputRejectsGarbage(t *testing.T) {
	_, err := ParseRubricOutput([]byte("{not json"))
	require.Error(t, err)
	var roErr *RubricOutputError
	require.ErrorAs(t, err, &roErr)
	require.Equal(t, 2, roErr.ExitCode())
}

func TestEvaluateCriteriaStopsWithoutCommandResultWhenEvidenceCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o755)) })
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{{
		ID:       "C1",
		Binding:  store.EvalBinding{Type: store.BindingCommand, Run: "true"},
		Required: true,
	}}}

	criteria, err := EvaluateCriteria(ef, nil, nil, root, CriteriaRun{Against: "abc1234", At: fixedTime})

	require.Error(t, err)
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonEvidenceUnwritable, env.Reason)
	require.NotContains(t, criteria, "C1")
}

// A test binding with several refs composes all of them onto the project tests
// runner, in order, and a run still wins when present.
func TestCriterionCommandComposesEveryRef(t *testing.T) {
	multi := store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"./a", "./b"}}
	require.Equal(t, "go test ./a ./b", criterionCommand(multi, "go test"))

	single := store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"./a"}}
	require.Equal(t, "go test ./a", criterionCommand(single, "go test"))

	withRun := store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"./a", "./b"}, Run: "make check"}
	require.Equal(t, "make check", criterionCommand(withRun, "go test"))

	require.Empty(t, criterionCommand(multi, ""), "no project runner, nothing runnable")
}

// With no runner Tiller records every ref as its own pending test evidence.
func TestRefEvidenceHasOneRowPerRef(t *testing.T) {
	multi := store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"./a", "./b"}}
	require.Equal(t, []store.Evidence{{Kind: "test", Ref: "./a"}, {Kind: "test", Ref: "./b"}}, refEvidence(multi))

	require.Nil(t, refEvidence(store.EvalBinding{Type: store.BindingTest, Run: "make check"}))
}

// countingCriterion is an eval file with one command criterion that appends a
// line to a counter file per run, plus the number of runs so far.
func countingCriterion(t *testing.T) (*store.EvalFile, func() int) {
	t.Helper()
	counter := filepath.Join(t.TempDir(), "runs")
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{{
		ID: "C1", Binding: store.EvalBinding{Type: store.BindingCommand, Run: "echo run >> " + counter}, Required: true,
	}}}
	return ef, func() int {
		data, err := os.ReadFile(counter)
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
		return strings.Count(string(data), "run\n")
	}
}

func TestEvaluateCriteriaReusesPassedCommand(t *testing.T) {
	root := t.TempDir()
	ef, runs := countingCriterion(t)
	cmd := ef.Criteria[0].Binding.Run
	evidence := []store.Evidence{{Kind: "command_output", Ref: "artifacts/tiller/gates/eval-C1-abc1234.txt"}}
	prior := map[string]store.Criterion{"C1": {
		Status: store.CriterionStatusPass, Code: "sha256:x", Config: ConfigHash("command", cmd), Evidence: evidence,
	}}
	at := fixedTime.Add(time.Hour)

	criteria, err := EvaluateCriteria(ef, prior, nil, root, CriteriaRun{Against: "abc1234", Code: "sha256:x", Reuse: true, At: at})
	require.NoError(t, err)

	require.Equal(t, 0, runs(), "the passed criterion is reused, not run")
	got := criteria["C1"]
	require.Equal(t, store.CriterionStatusPass, got.Status)
	require.Equal(t, at, got.ReusedAt)
	require.Equal(t, "sha256:x", got.Code)
	require.Equal(t, prior["C1"].Config, got.Config)
	require.Equal(t, evidence, got.Evidence, "the original evidence stands")
}

func TestEvaluateCriteriaReusesPassedTestBinding(t *testing.T) {
	root := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs")
	runner := "echo run >> " + counter + "; echo"
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"tests": {Kind: "command", Run: runner},
	}}}
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{{
		ID:      "C1",
		Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb"}}, Required: true,
	}}}
	prior := map[string]store.Criterion{"C1": {
		Status: store.CriterionStatusPass, Code: "sha256:x", Config: ConfigHash("command", runner+" spec/a_spec.rb"),
	}}

	criteria, err := EvaluateCriteria(ef, prior, m, root, CriteriaRun{Code: "sha256:x", Reuse: true, At: fixedTime})
	require.NoError(t, err)
	require.Equal(t, fixedTime, criteria["C1"].ReusedAt, "the resolved test command is what is compared")
	_, statErr := os.Stat(counter)
	require.True(t, os.IsNotExist(statErr), "nothing ran")
}

// TestCriteriaCurrent pins the held-evals half of the reuse key: every
// runnable criterion recorded pass must be keyed to the current code identity
// and the current resolved command, so a moved tests runner retires a test
// binding's pass even on unchanged code.
func TestCriteriaCurrent(t *testing.T) {
	runner := "go test ./..."
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"tests": {Kind: "command", Run: runner},
	}}}
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{
		{ID: "C1", Binding: store.EvalBinding{Type: store.BindingTest, Ref: store.BindingRefs{"spec/a_spec.rb"}}, Required: true},
		{ID: "C2", Binding: store.EvalBinding{Type: store.BindingCommand, Run: "make check"}},
		{ID: "C3", Binding: store.EvalBinding{Type: store.BindingManual}},
	}}
	keyed := func(code, cmd string) store.Criterion {
		return store.Criterion{Status: store.CriterionStatusPass, Code: code, Config: ConfigHash("command", cmd)}
	}
	current := map[string]store.Criterion{
		"C1": keyed("sha256:x", runner+" spec/a_spec.rb"),
		"C2": {Status: store.CriterionStatusFail, Code: "sha256:old"},
		"C3": {Status: store.CriterionStatusPass},
	}
	require.True(t, CriteriaCurrent(ef, current, m, "sha256:x"), "a non-pass and a manual criterion do not need keys")
	require.True(t, CriteriaCurrent(nil, nil, m, "sha256:x"), "no contract has nothing to retire")

	for name, criteria := range map[string]map[string]store.Criterion{
		"moved runner":   {"C1": keyed("sha256:x", "go test -race ./... spec/a_spec.rb")},
		"changed code":   {"C1": keyed("sha256:y", runner+" spec/a_spec.rb")},
		"legacy record":  {"C1": {Status: store.CriterionStatusPass}},
		"command passed": {"C1": current["C1"], "C2": keyed("sha256:x", "make lint")},
	} {
		require.False(t, CriteriaCurrent(ef, criteria, m, "sha256:x"), name)
	}
	require.False(t, CriteriaCurrent(ef, current, m, ""), "an empty identity never matches")
}

func TestEvaluateCriteriaRerunsOnChange(t *testing.T) {
	pass := func(code, config string) store.Criterion {
		return store.Criterion{Status: store.CriterionStatusPass, Code: code, Config: config}
	}
	for _, tc := range []struct {
		name  string
		prior func(cmd string) store.Criterion
		run   CriteriaRun
	}{
		{"different code",
			func(cmd string) store.Criterion { return pass("sha256:old", ConfigHash("command", cmd)) },
			CriteriaRun{Code: "sha256:new", Reuse: true}},
		{"changed binding run",
			func(string) store.Criterion { return pass("sha256:x", ConfigHash("command", "something else")) },
			CriteriaRun{Code: "sha256:x", Reuse: true}},
		{"prior fail",
			func(cmd string) store.Criterion {
				c := pass("sha256:x", ConfigHash("command", cmd))
				c.Status = store.CriterionStatusFail
				return c
			},
			CriteriaRun{Code: "sha256:x", Reuse: true}},
		{"reuse off",
			func(cmd string) store.Criterion { return pass("sha256:x", ConfigHash("command", cmd)) },
			CriteriaRun{Code: "sha256:x", Reuse: false}},
		{"empty code",
			func(cmd string) store.Criterion { return pass("", ConfigHash("command", cmd)) },
			CriteriaRun{Code: "", Reuse: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ef, runs := countingCriterion(t)
			cmd := ef.Criteria[0].Binding.Run
			prior := map[string]store.Criterion{"C1": tc.prior(cmd)}
			tc.run.Against, tc.run.At = "abc1234", fixedTime

			criteria, err := EvaluateCriteria(ef, prior, nil, t.TempDir(), tc.run)
			require.NoError(t, err)

			require.Equal(t, 1, runs(), "the command runs")
			got := criteria["C1"]
			require.Equal(t, store.CriterionStatusPass, got.Status)
			require.Equal(t, tc.run.Code, got.Code)
			require.Equal(t, ConfigHash("command", cmd), got.Config)
			require.True(t, got.ReusedAt.IsZero())
		})
	}
}

func TestEvaluateCriteriaLeavesHumanCriteria(t *testing.T) {
	ef := &store.EvalFile{Criteria: []store.EvalCriterion{
		{ID: "C3", Binding: store.EvalBinding{Type: store.BindingManual}, Required: true},
		{ID: "C4", Binding: store.EvalBinding{Type: store.BindingEval, Run: "Grade it."}, Required: true},
	}}
	prior := map[string]store.Criterion{
		"C3": {Status: store.CriterionStatusPass},
		"C4": {Status: store.CriterionStatusNeedsAttestation},
	}

	criteria, err := EvaluateCriteria(ef, prior, nil, t.TempDir(), CriteriaRun{Code: "sha256:x", Reuse: true, At: fixedTime})
	require.NoError(t, err)

	require.Equal(t, prior, criteria, "human criteria carry forward unchanged")
}

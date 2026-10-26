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

// These tests pin gate eligibility at the boundaries that consume recorded
// results: submit reads the latest applicable required results rather than
// trusting `status: check`, attest refuses to overwrite a declared command
// gate, and check carries a settled required attestation forward instead of
// resetting it to pending.

// toggledManifestTmpl binds the tests gate to a flag file outside the repo,
// so one fixture can pass a check, fail a recheck, and pass again without
// touching the working tree. Because the flag is outside the repo, the code
// identity never sees it flip, so the gate declares `reuse: false`: the case
// the gate-reuse design keeps for a result that depends on something outside
// the repository. signoff is a required attestation gate on the task grain,
// which every shelf node inherits.
const toggledManifestTmpl = `schema_version: 12
schema_epoch: initial
project:
  name: Test Project
  key: test
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
gates:
  required:
    build: { kind: command, run: "true", on: [feature, task] }
    tests: { kind: command, run: "test -f %s", on: [feature, task], reuse: false }
%s`

const evalsOnTaskLine = `    evals: { kind: command, run: "tiller eval ${node.noun} ${node.id}", on: [feature, task] }
`

const signoffLine = `    signoff: { kind: attestation, on: [task] }
`

// gatedUnit is one execution unit laid on a toggled repo: its noun, the
// reference the verbs take, its id, and the flag file its tests gate reads.
type gatedUnit struct {
	root string
	noun string
	ref  string
	id   string
	flag string
}

func (u gatedUnit) green(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(u.flag, []byte("ok\n"), 0o644))
}

func (u gatedUnit) red(t *testing.T) {
	t.Helper()
	require.NoError(t, os.Remove(u.flag))
}

func (u gatedUnit) node(t *testing.T) store.Node {
	t.Helper()
	return resolveNode(t, load(t, u.root), u.noun, u.ref)
}

func (u gatedUnit) gates(t *testing.T) map[string]store.Gate {
	t.Helper()
	switch n := u.node(t).(type) {
	case *store.Task:
		return n.Gates
	default:
		w, ok := store.AsWorkNode(n)
		require.True(t, ok)
		return w.Body().Gates
	}
}

func (u gatedUnit) check(t *testing.T) mutate.CheckResult {
	t.Helper()
	tree := load(t, u.root)
	res, err := mutate.Check(tree, testDeps(), vcs.New(u.root), resolveNode(t, tree, u.noun, u.ref), mutate.CheckInput{})
	require.NoError(t, err)
	return res
}

func (u gatedUnit) submit(t *testing.T) error {
	t.Helper()
	pr := 7
	return tryTransition(t, u.root, testDeps(), u.noun, u.ref, mutate.TransitionInput{Verb: "submit", PR: &pr})
}

func (u gatedUnit) attestGate(t *testing.T, gate, status string) (mutate.AttestResult, error) {
	t.Helper()
	tree := load(t, u.root)
	return mutate.Attest(tree, testDeps(), resolveNode(t, tree, u.noun, u.ref), mutate.AttestInput{Gate: gate, Status: status})
}

// layToggledUnit lays one started execution unit of the given noun on a repo
// whose tests gate reads a flag file, with extra manifest gate lines
// appended. A task runs under an approved feature; a shelf node stands alone.
// Every unit is checked out on its recorded branch so submission observes
// the same review identity as ordinary execution.
func layToggledUnit(t *testing.T, noun, extraGates string) gatedUnit {
	t.Helper()
	flag := filepath.Join(t.TempDir(), "tests-green")
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	writeManifest(t, root, fmt.Sprintf(toggledManifestTmpl, flag, extraGates))
	gitInitM(t, root)
	gitCommitAllM(t, root, "init")

	u := gatedUnit{root: root, noun: noun, flag: flag}
	if noun == store.NounTask {
		u.id = driveGatedTask(t, root)
		u.ref = u.id
		gitRunM(t, root, "checkout", "-b", load(t, root).Tasks[u.id].Branch)
		return u
	}

	var prefix string
	for _, s := range store.Shelves() {
		if s.Noun == noun {
			prefix = s.IDPrefix
		}
	}
	require.NotEmpty(t, prefix, "unknown shelf noun %s", noun)
	deps := testDeps()
	deps.Mint = func(p string) string {
		if p == prefix {
			return prefix + "-aaaaaa"
		}
		return fixedMint(p)
	}
	_, err := mutate.AddWorkItem(load(t, root), deps, mutate.WorkItemInput{Noun: noun, Key: "alpha", Title: "A"})
	require.NoError(t, err)
	prose := filepath.Join(root, store.ShelfRelDir(noun, "alpha"), store.ShelfProseFile(noun))
	require.NoError(t, os.WriteFile(prose, []byte("# A\n\n## Reproduction\n\nx\n"), 0o644))
	gitCommitAllM(t, root, "lay "+noun)
	driveTo(t, root, noun, "alpha", mutate.TransitionInput{Verb: "approve"})
	_, err = mutate.ClaimWork(load(t, root), testDeps(), noun, "alpha", "")
	require.NoError(t, err)
	started := driveTo(t, root, noun, "alpha", mutate.TransitionInput{Verb: "start"})
	gitRunM(t, root, "checkout", "-b", started.Branch)
	u.id, u.ref = prefix+"-aaaaaa", "alpha"
	return u
}

func eventCount(t *testing.T, root string) int {
	t.Helper()
	evs, err := events.List(load(t, root))
	require.NoError(t, err)
	return len(evs)
}

var executionUnitNouns = []string{store.NounTask, store.NounBug, store.NounChore, store.NounTweak}

// Defect 01, submission after a failed recheck: a unit that reached check on
// a green run and then failed a recheck stays at check with its failure on
// record, and submit refuses on that latest required result without writing
// anything. A later passing check restores submission, and a failing optional
// gate never blocks it.
func TestSubmitRefusesAfterAFailedRecheck(t *testing.T) {
	t.Parallel()
	for _, noun := range executionUnitNouns {
		t.Run(noun, func(t *testing.T) {
			u := layToggledUnit(t, noun, "")

			u.green(t)
			first := u.check(t)
			require.True(t, first.Passed)
			require.Equal(t, "check", first.Status)

			u.red(t)
			recheck := u.check(t)
			require.False(t, recheck.Passed)
			require.Equal(t, "check", recheck.Status, "a failed recheck adds no lifecycle state")
			failed := u.gates(t)["tests"]
			require.Equal(t, store.GateStatusFail, failed.Status, "the failed recheck is recorded")
			require.NotEmpty(t, failed.Evidence, "the failure keeps its evidence")

			before := snapshotDir(t, u.root)
			err := u.submit(t)
			var notGreen *mutate.RequiredGatesNotGreenError
			require.ErrorAs(t, err, &notGreen)
			require.Equal(t, noun, notGreen.Noun)
			require.Equal(t, u.id, notGreen.NodeID)
			require.Equal(t, []string{"tests"}, notGreen.Missing)
			requireNoWrites(t, before, snapshotDir(t, u.root))

			_, err = u.attestGate(t, "code_review", store.GateStatusFail)
			require.NoError(t, err, "an optional review verdict is recorded")
			u.green(t)
			require.True(t, u.check(t).Passed)
			require.NoError(t, u.submit(t), "a passing recheck restores submission; an optional failure stays advisory")
			require.Equal(t, "in_review", u.node(t).NodeStatus())
		})
	}
}

// A blocked `check --gate` recheck on new code: build runs and passes, tests
// sits outside the set and cannot be reused (it declares `reuse: false`, and
// the code moved anyway), so its old pass is marked stale rather than left on
// record as a pass over code it never ran against. Submit then refuses on
// tests exactly as it would after a failed recheck.
func TestSubmitRefusesAfterABlockedGateRecheck(t *testing.T) {
	t.Parallel()
	for _, noun := range executionUnitNouns {
		t.Run(noun, func(t *testing.T) {
			u := layToggledUnit(t, noun, "")

			u.green(t)
			require.True(t, u.check(t).Passed)
			passed := u.gates(t)["tests"]
			require.Equal(t, store.GateStatusPass, passed.Status)

			require.NoError(t, os.WriteFile(filepath.Join(u.root, "new.go"), []byte("package main\n"), 0o644))
			u.red(t)
			tree := load(t, u.root)
			recheck, err := mutate.Check(tree, testDeps(), vcs.New(u.root), resolveNode(t, tree, u.noun, u.ref), mutate.CheckInput{Gates: []string{"build"}})
			require.NoError(t, err)
			require.False(t, recheck.Passed)
			require.Equal(t, "check", recheck.Status)

			staled := u.gates(t)["tests"]
			require.Equal(t, store.GateStatusStale, staled.Status, "an excluded gate that is not current is marked stale")
			require.Equal(t, passed.Evidence, staled.Evidence, "staling keeps the old run's evidence")
			require.Equal(t, passed.RunAt, staled.RunAt, "staling keeps the old run's provenance")
			require.Equal(t, passed.Code, staled.Code)

			err = u.submit(t)
			var notGreen *mutate.RequiredGatesNotGreenError
			require.ErrorAs(t, err, &notGreen)
			require.Equal(t, []string{"tests"}, notGreen.Missing)
		})
	}
}

// Defect 08, command gate can be overwritten by attestation: a gate the
// manifest declares as command-backed is evidence check records from a run,
// never a human verdict, so attest refuses it and leaves the recorded failure,
// its kind, and the event ledger exactly as they were.
func TestAttestRefusesADeclaredCommandGate(t *testing.T) {
	t.Parallel()
	for _, noun := range []string{store.NounTask, store.NounBug} {
		for _, gate := range []string{"build", "tests"} {
			t.Run(noun+"/"+gate, func(t *testing.T) {
				u := layToggledUnit(t, noun, "")
				require.False(t, u.check(t).Passed, "the tests flag is absent, so tests fails")

				before := snapshotDir(t, u.root)
				events := eventCount(t, u.root)
				_, err := u.attestGate(t, gate, store.GateStatusPass)
				require.Error(t, err)
				var reasoned interface{ Reason() string }
				require.ErrorAs(t, err, &reasoned)
				require.Equal(t, "command_gate", reasoned.Reason())
				var exit interface{ ExitCode() int }
				require.ErrorAs(t, err, &exit)
				require.Equal(t, 2, exit.ExitCode())
				requireNoWrites(t, before, snapshotDir(t, u.root))
				require.Equal(t, events, eventCount(t, u.root))
				require.Equal(t, store.GateKindCommand, u.gates(t)[gate].Kind)
			})
		}
	}
	t.Run("task/evals", func(t *testing.T) {
		u := layToggledUnit(t, store.NounTask, evalsOnTaskLine)
		before := snapshotDir(t, u.root)
		_, err := u.attestGate(t, "evals", store.GateStatusPass)
		require.Error(t, err, "the evals gate is derived from its criteria")
		requireNoWrites(t, before, snapshotDir(t, u.root))
	})
}

// The other two attest arms keep their behavior: a declared attestation gate
// takes the verdict, and an undeclared gate is recorded with its warning.
func TestAttestStillRecordsAttestationAndUndeclaredGates(t *testing.T) {
	t.Parallel()
	u := layToggledUnit(t, store.NounTask, signoffLine)
	res, err := u.attestGate(t, "signoff", store.GateStatusPass)
	require.NoError(t, err)
	require.Empty(t, res.Warnings)
	require.Equal(t, store.GateKindAttestation, u.gates(t)["signoff"].Kind)
	require.Equal(t, store.GateStatusPass, u.gates(t)["signoff"].Status)

	res, err = u.attestGate(t, "code-review", store.GateStatusPass)
	require.NoError(t, err)
	require.Len(t, res.Warnings, 1, "an undeclared gate is recorded with a warning")
}

// Defect 09, check resets required attestation to pending: a settled required
// attestation survives check with its verdict and provenance and lets the unit
// progress once the mechanical gates pass. A failing one stays blocking.
func TestCheckPreservesASettledRequiredAttestation(t *testing.T) {
	t.Parallel()
	for _, noun := range executionUnitNouns {
		t.Run(noun, func(t *testing.T) {
			u := layToggledUnit(t, noun, signoffLine)
			u.green(t)

			_, err := u.attestGate(t, "signoff", store.GateStatusFail)
			require.NoError(t, err)
			res := u.check(t)
			require.False(t, res.Passed, "a failed required attestation blocks")
			require.Equal(t, store.GateStatusFail, u.gates(t)["signoff"].Status)

			_, err = u.attestGate(t, "signoff", store.GateStatusPass)
			require.NoError(t, err)
			attested := u.gates(t)["signoff"]

			res = u.check(t)
			require.True(t, res.Passed, "a passing required attestation plus green mechanical gates passes")
			require.Equal(t, "check", res.Status)
			kept := u.gates(t)["signoff"]
			require.Equal(t, attested, kept, "check keeps the human verdict and its provenance")
			for _, g := range res.Gates {
				if g.Name == "signoff" {
					require.Equal(t, store.GateStatusPass, g.Status)
				}
			}
			require.NoError(t, u.submit(t))
		})
	}
}

// A required attestation that has not been given, or whose verdict went stale
// on resume, still blocks check.
func TestCheckKeepsPendingAndStaleRequiredAttestationBlocking(t *testing.T) {
	t.Parallel()
	u := layToggledUnit(t, store.NounBug, signoffLine)
	u.green(t)
	res := u.check(t)
	require.False(t, res.Passed)
	require.Equal(t, store.GateStatusPending, u.gates(t)["signoff"].Status)

	_, err := u.attestGate(t, "signoff", store.GateStatusPass)
	require.NoError(t, err)
	require.True(t, u.check(t).Passed)
	require.NoError(t, u.submit(t))
	driveTo(t, u.root, u.noun, u.ref, mutate.TransitionInput{Verb: "resume", Reason: "changes requested"})
	require.Equal(t, store.GateStatusStale, u.gates(t)["signoff"].Status)

	res = u.check(t)
	require.False(t, res.Passed, "a stale attestation is not a current verdict")
	require.Equal(t, "in_progress", res.Status)
	require.Equal(t, store.GateStatusStale, u.gates(t)["signoff"].Status)
}

// A verdict recorded under a different declared kind, or for a gate the
// manifest no longer scopes to the grain, is not inherited.
func TestCheckDoesNotInheritAnIncompatibleGateVerdict(t *testing.T) {
	t.Parallel()
	t.Run("kind changed from command to attestation", func(t *testing.T) {
		u := layToggledUnit(t, store.NounBug, `    signoff: { kind: command, run: "true", on: [task] }
`)
		u.green(t)
		require.True(t, u.check(t).Passed)
		require.Equal(t, store.GateKindCommand, u.gates(t)["signoff"].Kind)

		writeManifest(t, u.root, fmt.Sprintf(toggledManifestTmpl, u.flag, signoffLine))
		res := u.check(t)
		require.False(t, res.Passed, "a command pass is not a human sign-off")
		require.Equal(t, store.GateStatusPending, u.gates(t)["signoff"].Status)
		require.Equal(t, store.GateKindAttestation, u.gates(t)["signoff"].Kind)
	})
	t.Run("rescoped away from the grain", func(t *testing.T) {
		u := layToggledUnit(t, store.NounBug, signoffLine)
		u.green(t)
		_, err := u.attestGate(t, "signoff", store.GateStatusFail)
		require.NoError(t, err)
		writeManifest(t, u.root, fmt.Sprintf(toggledManifestTmpl, u.flag, `    signoff: { kind: attestation, on: [feature] }
`))
		res := u.check(t)
		require.True(t, res.Passed, "a gate the grain no longer carries neither blocks nor survives")
		require.NotContains(t, u.gates(t), "signoff")
	})
}

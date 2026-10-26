package cli

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// execCLI builds a fresh root command, runs it with the given args while
// capturing stdout and stderr, and returns them plus the process exit code in
// the Tiller scheme. It mirrors Run, the production entrypoint, so the tests
// exercise the same exit-code mapping.
func execCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			return out.String(), errb.String(), ee.code
		}
		// A raw cobra/pflag error (e.g. a required flag never set) never passes
		// through the envelope writers, so nothing has reached errb yet. Run
		// prints it as "tiller: <err>" before returning 2; mirroring that write
		// here is what makes this helper's own doc comment true.
		fmt.Fprintln(root.ErrOrStderr(), "tiller:", err)
		return out.String(), errb.String(), 2
	}
	return out.String(), errb.String(), 0
}

// TestClassifyMapsTypedErrors pins the error-to-envelope contract that later
// tasks (status<node>, lifecycle verbs) consume: a store EnvError keeps its code
// and reason; the resolver's typed errors all map to exit 2 with stable reasons,
// and an ambiguous key carries its candidate ids.
func TestClassifyMapsTypedErrors(t *testing.T) {
	body := classify(&store.EnvError{Code: 3, Reason: store.ReasonNotARepo, Msg: "not a repo"})
	require.Equal(t, 3, body.Code)
	require.Equal(t, store.ReasonNotARepo, body.Reason)

	e1 := &store.Epic{ID: "E-aaaaaa"}
	e2 := &store.Epic{ID: "E-bbbbbb"}
	amb := &resolve.ErrAmbiguousKey{Noun: "epic", Key: "k", Candidates: []store.Node{e1, e2}}
	body = classify(amb)
	require.Equal(t, 2, body.Code)
	require.Equal(t, reasonAmbiguousKey, body.Reason)
	require.Equal(t, []string{"E-aaaaaa", "E-bbbbbb"}, body.Candidates)

	require.Equal(t, 2, classify(&resolve.ErrNounMismatch{}).Code)
	require.Equal(t, reasonNounMismatch, classify(&resolve.ErrNounMismatch{}).Reason)
	require.Equal(t, 2, classify(&resolve.ErrNotFound{}).Code)
	require.Equal(t, reasonNotFound, classify(&resolve.ErrNotFound{}).Reason)
}

// TestClassifyPrefersATopLevelReasonerOverAWrappedResolveError pins the
// fix: mutate's parentUnknownErr (an add-feature op whose
// epic segment is neither live nor staged earlier on this amendment)
// answers its own Reason()/ExitCode(), but also carries the live resolver's
// bare *resolve.ErrNotFound through Unwrap so a Go-level errors.As can still
// find it (the shape a genuinely unknown parent has always refused as).
// classify's ErrNotFound branch used to run before the generic
// ExitCoder/reasoner branch, so it matched first via that Unwrap and printed
// only the bare "no epic found for ..." — dropping the same-batch half of
// the message on exactly the surface (--json) the amend skill drives, with
// the exit code and reason unchanged, so the regression was silent. This
// drives the real CLI path end to end (not just classify() in isolation) so
// it proves the fix at the surface the finding named.
func TestClassifyPrefersATopLevelReasonerOverAWrappedResolveError(t *testing.T) {
	layAmendmentTree(t)
	aid := openAmendmentCLI(t, "iphone-only", "Remove native iPad support")

	out, _, code := execCLI(t, "amend", aid, "--add-feature", "no-such-epic/x",
		"--title", "X", "--change-type", "create", "--contributor", "jc", "--json")
	require.Equal(t, 2, code, out)
	require.Contains(t, out, `"reason": "not_found"`)
	require.Contains(t, out, "no epic in the tree", "the live-tree half of the message must still reach --json")
	require.Contains(t, out, "staged earlier on this amendment", "the same-batch half must reach --json, not just Reason()")
	require.NotContains(t, out, "no epic found for",
		"the bare *resolve.ErrNotFound text must not win over usageError's own fuller message")
}

// TestWriteSuccessEnvelopeShape confirms the success envelope is stable: ok true,
// the command name, the data block, and no error block.
func TestWriteSuccessEnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteSuccess(&buf, "demo", map[string]int{"n": 1}))
	got := buf.String()
	require.Contains(t, got, "\"ok\": true")
	require.Contains(t, got, "\"command\": \"demo\"")
	require.Contains(t, got, "\"data\"")
	require.NotContains(t, got, "\"error\"")
	require.True(t, got[len(got)-1] == '\n', "envelope ends with a newline")
}

// TestMutatingEnvelopeCarriesTheActorField pins the design's generic rule
// (docs/spec/cli-surface.md): a mutating command's --json envelope always
// carries a top-level additive "actor" field naming who
// resolveActingContributor picked and which source won, via the same map-merge
// mechanism withReadout uses for "readout". "add epic" stands in as the
// representative mutating command: every addContributorFlag verb funnels
// through the same emitReport/emitSuccess tail this exercises.
func TestMutatingEnvelopeCarriesTheActorField(t *testing.T) {
	initOwnedRepo(t)
	out, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	data := jsonData(t, out)
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "actor rides the data block additively")
	require.Equal(t, "jc", actorField["id"])
	require.Equal(t, "--contributor", actorField["source"])
}

// TestReadCommandsCarryNoActorField confirms the design's other half: a
// read-only command (show has no --contributor flag registered) gains nothing.
func TestReadCommandsCarryNoActorField(t *testing.T) {
	initOwnedRepo(t)
	_, _, code := execCLI(t, "add", "epic", "form-engine", "--title", "Form Engine", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "show", "epic", "form-engine", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, out, `"actor"`)
}

func TestContributorReadFilterCarriesNoActor(t *testing.T) {
	initOwnedRepo(t)
	out, _, code := execCLI(t, "ls", "features", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	require.NotContains(t, jsonData(t, out), "actor")
}

// TestBlockedMutatingEnvelopeStillCarriesTheActorField confirms the actor
// field is "ALWAYS present regardless of source" (design doc) on a blocked
// (ok:false) business answer too, not only a success: an illegal transition
// is a mutating command's report the same as a success is, and owner
// resolution has already succeeded by the time this report is reached (a
// genuine owner failure would have refused through emitFailure's error
// envelope instead, which this design leaves untouched).
func TestBlockedMutatingEnvelopeStillCarriesTheActorField(t *testing.T) {
	laySpeccableFeature(t)
	out, _, code := execCLI(t, "start", "task", cliTaskKey, "--contributor", "jc", "--json")
	require.Equal(t, 1, code, "an illegal transition is a blocked business answer, not a crash")
	require.Contains(t, out, `"ok": false`)
	data := jsonData(t, out)
	actorField, ok := data["actor"].(map[string]any)
	require.True(t, ok, "the actor field rides a blocked report too")
	require.Equal(t, "jc", actorField["id"])
	require.Equal(t, "--contributor", actorField["source"])
}

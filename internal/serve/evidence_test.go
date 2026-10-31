package serve_test

// These tests pin GET /api/evidence (ui-surface.md; plan 2026-07-23
// console-chart-table pass, Task A3, scope as amended in 67760d0): the
// acceptance instrument's evidence link (Task C3) resolves straight to this
// endpoint, so it must serve a recorded gate-evidence file's bytes verbatim
// as inert text, refuse before ever touching disk a ref that could walk
// outside the project tree or into a dot-prefixed path (.git/, .tiller/, any
// dotfile), 404 a well-formed ref no node records or that names no file on
// disk, and 413 a recorded file over the read cap. The
// fixture mirrors the real on-disk shape gates.RunGate writes
// (internal/core/gates/run.go, evidenceRef/againstToken):
// artifacts/tiller/gates/<name>-sha256-<short>.txt, a sibling of .tiller/,
// not inside it. Every request here rides the real console Handler, mux and
// all, so a dropped route registration fails these tests instead of passing
// them against a handler production never reaches.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/stretchr/testify/require"
)

const evidenceTestManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Evidence Serve Test
  key: evidenceserve
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
`

// evidenceFixture lays a bare owned repo whose one ledger chore records each
// of recorded as gate evidence, and returns a server over the real console
// Handler plus the root. The endpoint serves only refs the loaded tree
// records, so a test names every ref it expects the endpoint to consider.
// Unlike search_test.go and list_test.go these tests deliberately do not
// mount the handler alone: the review of the first cut found every evidence
// test green with the route registration removable, so the whole battery
// goes through serve.Handler and a dropped or typo'd HandleFunc line answers
// the catch-all's JSON 404 here instead of passing silently.
func evidenceFixture(t *testing.T, recorded ...string) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(evidenceTestManifest), 0o644))
	if len(recorded) > 0 {
		writeLedgerChore(t, root, gateEvidenceYAML(recorded...))
	}

	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Tiller</title>")}}
	srv := httptest.NewServer(serve.Handler(root, assets, "test", serve.DefaultAuthority()))
	t.Cleanup(srv.Close)
	return srv, root
}

// writeLedgerChore records one ready chore whose chore.yml carries extra, the
// ledger fields a test needs the console to find (gate evidence, a source
// citation). The evidence and prose endpoints serve only what the loaded tree
// records, so a fixture file is reachable exactly when a record points at it.
func writeLedgerChore(t *testing.T, root, extra string) {
	t.Helper()
	writeFixtureFile(t, root, ".tiller/chores/ledger-chore/chore.md", []byte("# Ledger chore\n"))
	writeFixtureFile(t, root, ".tiller/chores/ledger-chore/chore.yml", []byte(`id: CH-cccccc
key: chores/ledger-chore
title: Ledger chore
status: ready
blocked: false
owner: jc
proposed_by: jc
`+extra))
	writeFixtureFile(t, root, ".tiller/chores/order.yml", []byte("order: [CH-cccccc]\n"))
}

// gateEvidenceYAML is a gates block whose build gate records each ref as
// command output, the shape gates.RunGate writes.
func gateEvidenceYAML(refs ...string) string {
	var b strings.Builder
	b.WriteString("gates:\n  build:\n    required: true\n    status: pass\n    evidence:\n")
	for _, ref := range refs {
		fmt.Fprintf(&b, "      - { kind: command_output, ref: %q }\n", ref)
	}
	return b.String()
}

// writeEvidenceFile lays a recorded gate-evidence file at the exact shape
// production writes: artifacts/tiller/gates/<name>-<against-token>.txt, the
// against token sha256-prefixed the way a dirty working tree's gate run
// records it. Returns the ref (repo-relative, forward-slashed, exactly what a
// GateView.Evidence[].Ref carries) and the content written.
func writeEvidenceFile(t *testing.T, root string) (ref, content string) {
	t.Helper()
	ref = recordedEvidenceRef
	content = "go build ./...\nok\n"
	full := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	return ref, content
}

// recordedEvidenceRef is the ref writeEvidenceFile lays, at the exact shape
// production records.
const recordedEvidenceRef = "artifacts/tiller/gates/build-sha256-d938b60e00c3.txt"

func evidenceURL(srv *httptest.Server, ref string) string {
	return srv.URL + "/api/evidence?" + url.Values{"ref": {ref}}.Encode()
}

// TestEvidenceEndpointServesTheRecordedFile pins the happy path: the exact
// bytes of a fixture artifact file at the real recorded path shape, answered
// as inert text, never JSON and never anything a browser could sniff into
// HTML, since evidence is captured command output (Task C3 opens this URL
// directly in a new tab).
func TestEvidenceEndpointServesTheRecordedFile(t *testing.T) {
	srv, root := evidenceFixture(t, recordedEvidenceRef)
	ref, content := writeEvidenceFile(t, root)

	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, string(body))
}

// TestEvidenceEndpointRejectsAnUnsafeRef pins the traversal refusal: an empty
// ref, an absolute path, and any ref whose cleaned form climbs outside the
// project root via .. are each a 400, the read API's misuse status, decided
// before the path ever reaches disk. The unsafe refs are recorded in the
// ledger on purpose: a record never makes an unsafe ref servable.
func TestEvidenceEndpointRejectsAnUnsafeRef(t *testing.T) {
	srv, _ := evidenceFixture(t, "/etc/passwd", "../../../etc/passwd", "artifacts/tiller/gates/../../../../../../etc/passwd")

	cases := []struct {
		name string
		ref  string
	}{
		{"empty ref", ""},
		{"absolute path", "/etc/passwd"},
		{"dot-dot traversal", "../../../etc/passwd"},
		{"embedded dot-dot", "artifacts/tiller/gates/../../../../../../etc/passwd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, err := srv.Client().Get(evidenceURL(srv, c.ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode, "ref=%q", c.ref)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestEvidenceEndpointDeniesDotSegments pins the hidden-path refusal (the
// as-built scope, plan amendment 67760d0): a ref naming any dot-prefixed
// path segment is a 400 even though the file exists, because no legitimate
// evidence ref enters a hidden path (evidence lives under artifacts/tiller/
// and test refs name bare repo paths), while .git/ and .tiller/ hold repo
// internals and the gitignored identity file that a pass-1 local server
// with no origin or auth checks must never serve. The files are written
// first deliberately: this test going red would mean the endpoint actually
// leaked them, not that they happened to be absent. They are recorded in
// the ledger too, so the dot rule is proven to hold for a recorded ref.
func TestEvidenceEndpointDeniesDotSegments(t *testing.T) {
	srv, root := evidenceFixture(t, ".git/config", ".tiller/identity.yml")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"),
		[]byte("[core]\n\trepositoryformatversion = 0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "identity.yml"),
		[]byte("owner: jc\n"), 0o644))

	for _, ref := range []string{".git/config", ".tiller/identity.yml"} {
		t.Run(ref, func(t *testing.T) {
			resp, err := srv.Client().Get(evidenceURL(srv, ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode, "ref=%q names a hidden path and must refuse, file present or not", ref)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestEvidenceEndpointServesBareTestRefs pins the other half of the scope
// rule: a criterion's test binding legitimately records a bare repo path as
// its evidence ref (store.EvalBinding.Ref via refEvidence, e.g. a spec
// file), nothing under artifacts/tiller/ at all, so the dot-segment deny
// must not narrow the legitimate ref space to the artifacts tree.
func TestEvidenceEndpointServesBareTestRefs(t *testing.T) {
	const ref = "spec/form_definitions/form_a_100_spec.rb"
	srv, root := evidenceFixture(t, ref)
	const content = "RSpec.describe \"FORM A-100\" do\nend\n"
	full := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))

	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, string(body))
}

// TestEvidenceEndpointMissingFileIs404 pins the not-found case: a
// well-formed ref in the right shape that simply names no file on disk is a
// 404 naming the ref, not a 400 (the request itself was not malformed) and
// not a 500 (a missing evidence file is an ordinary, expected shape, not an
// environment failure). Asserting the error message names the ref, not just
// the status, so a stub that answered 404 to everything regardless of
// whether it looked at ref could not pass this for the wrong reason.
func TestEvidenceEndpointMissingFileIs404(t *testing.T) {
	const ref = "artifacts/tiller/gates/does-not-exist-sha256-deadbeefcafe.txt"
	srv, _ := evidenceFixture(t, ref)

	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 404, resp.StatusCode)

	var got struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Contains(t, got.Error, ref)
}

// TestEvidenceEndpointRejectsEscapingSymlink closes the canonical-path gap in
// the lexical traversal checks. Both a symlink at the requested leaf and one
// in a parent segment are unsafe refs, even though their visible path remains
// dot-free and repo-relative.
func TestEvidenceEndpointRejectsEscapingSymlink(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		link func(t *testing.T, root, outside string)
	}{
		{
			name: "leaf",
			ref:  "artifacts/tiller/gates/escape.txt",
			link: func(t *testing.T, root, outside string) {
				t.Helper()
				dir := filepath.Join(root, "artifacts", "tiller", "gates")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")))
			},
		},
		{
			name: "parent",
			ref:  "artifacts/tiller/escaped-gates/secret.txt",
			link: func(t *testing.T, root, outside string) {
				t.Helper()
				dir := filepath.Join(root, "artifacts", "tiller")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escaped-gates")))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, root := evidenceFixture(t, tc.ref)
			outside := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not evidence\n"), 0o644))
			tc.link(t, root, outside)

			resp, err := srv.Client().Get(evidenceURL(srv, tc.ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
		})
	}
}

// TestEvidenceEndpointRefusesInternalSymlink pins one confinement policy with
// the store: a recorded ref is served only as the regular file it names, never
// through a symlink, even one whose target stays inside the repo. The ledger
// records a committed alias to a gitignored secret that exists only on the
// host; following it would serve the secret under the recorded name.
func TestEvidenceEndpointRefusesInternalSymlink(t *testing.T) {
	const ref = "artifacts/tiller/gates/alias.txt"
	srv, root := evidenceFixture(t, ref)
	writeFixtureFile(t, root, "config/master.key", []byte("super-secret\n"))
	dir := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Symlink("../../../config/master.key", filepath.Join(root, filepath.FromSlash(ref))))

	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 400, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "super-secret")
}

// TestEvidenceEndpointRefusesInternalSymlinkedParent applies the same rule to
// a parent segment: a directory alias inside the repo is refused rather than
// followed, whether or not the file below it exists.
func TestEvidenceEndpointRefusesInternalSymlinkedParent(t *testing.T) {
	const present = "artifacts/tiller/current/present.txt"
	const deleted = "artifacts/tiller/current/deleted.txt"
	srv, root := evidenceFixture(t, present, deleted)
	writeFixtureFile(t, root, "artifacts/tiller/gates/present.txt", []byte("aliased evidence\n"))
	require.NoError(t, os.Symlink("gates", filepath.Join(root, "artifacts", "tiller", "current")))

	for _, ref := range []string{present, deleted} {
		t.Run(ref, func(t *testing.T) {
			resp, err := srv.Client().Get(evidenceURL(srv, ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, 400, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "aliased evidence")
		})
	}
}

// TestEvidenceEndpointRejectsHiddenSymlinkTarget applies the established
// dot-segment policy to the canonical target, not only the visible alias.
func TestEvidenceEndpointRejectsHiddenSymlinkTarget(t *testing.T) {
	const ref = "artifacts/tiller/gates/alias.txt"
	srv, root := evidenceFixture(t, ref)
	hidden := filepath.Join(root, ".git")
	require.NoError(t, os.MkdirAll(hidden, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hidden, "config"), []byte("secret\n"), 0o644))
	aliasDir := filepath.Join(root, "artifacts", "tiller", "gates")
	require.NoError(t, os.MkdirAll(aliasDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(hidden, "config"), filepath.Join(root, filepath.FromSlash(ref))))

	resp, err := srv.Client().Get(evidenceURL(srv, ref))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 400, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

// TestEvidenceEndpointRefusesFIFOWithoutReading pins the shared confined read
// at evidence's raw-text surface too. The timeout fallback releases a regressed
// blocking open before failing so the suite itself cannot hang.
func TestEvidenceEndpointRefusesFIFOWithoutReading(t *testing.T) {
	const ref = "artifacts/tiller/gates/pipe.txt"
	_, root := evidenceFixture(t, ref)
	full := filepath.Join(root, filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, syscall.Mkfifo(full, 0o600))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/evidence?"+url.Values{"ref": {ref}}.Encode(), nil)
	done := make(chan struct{})
	go func() {
		serve.EvidenceHandler(root).ServeHTTP(recorder, request)
		close(done)
	}()

	select {
	case <-done:
		require.Equal(t, 500, recorder.Code)
	case <-time.After(time.Second):
		writer, err := os.OpenFile(full, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = writer.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
		t.Fatal("evidence handler blocked opening a FIFO instead of refusing it as non-regular")
	}
}

// TestEvidenceEndpointRefusesWrites pins the pass-1 posture on this handler
// too: non-GET methods refuse with 405 and the Allow header, the same shell
// every other read endpoint answers through.
func TestEvidenceEndpointRefusesWrites(t *testing.T) {
	srv, _ := evidenceFixture(t)

	resp, err := srv.Client().Post(srv.URL+"/api/evidence?ref=x", "text/plain", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 405, resp.StatusCode)
	require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
}

// TestEvidenceEndpointServesOnlyRecordedRefs pins the network-safe scope: the
// endpoint serves a ref only when some node in the loaded tree records it. A
// gitignored secret and an unrecorded file beside real evidence both exist on
// disk, are well-formed refs, and must answer 404 without their bytes, while
// the recorded file right next to them still serves. Removing the record
// check turns the first two cases into 200s carrying the secret.
func TestEvidenceEndpointServesOnlyRecordedRefs(t *testing.T) {
	srv, root := evidenceFixture(t, recordedEvidenceRef)
	_, content := writeEvidenceFile(t, root)
	writeFixtureFile(t, root, "config/master.key", []byte("super-secret-key\n"))
	writeFixtureFile(t, root, ".gitignore", []byte("config/master.key\n"))
	writeFixtureFile(t, root, "artifacts/tiller/gates/unrecorded-sha256-000000000000.txt", []byte("super-secret-log\n"))

	for _, ref := range []string{"config/master.key", "artifacts/tiller/gates/unrecorded-sha256-000000000000.txt"} {
		t.Run(ref, func(t *testing.T) {
			resp, err := srv.Client().Get(evidenceURL(srv, ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusNotFound, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "super-secret")
		})
	}

	resp, err := srv.Client().Get(evidenceURL(srv, recordedEvidenceRef))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, string(body))
}

// TestEvidenceEndpointServesEveryRecordedEvidenceField walks each place the
// ledger records an evidence pointer: gate evidence, per-criterion evidence,
// node-level evidence, an attestation's evidence_ref, and a policy
// violation's evidence_ref. Each one alone makes its ref servable.
func TestEvidenceEndpointServesEveryRecordedEvidenceField(t *testing.T) {
	cases := []struct {
		name  string
		ref   string
		extra string
	}{
		{"gate evidence", "artifacts/tiller/gates/gate.txt", gateEvidenceYAML("artifacts/tiller/gates/gate.txt")},
		{"criterion evidence", "artifacts/tiller/evals/c1.txt", `gates:
  evals:
    required: true
    status: pass
    criteria:
      C1: { status: pass, evidence: [{ kind: rubric_output, ref: artifacts/tiller/evals/c1.txt }] }
`},
		{"node evidence", "artifacts/tiller/node.txt", "evidence: [{ kind: test, ref: artifacts/tiller/node.txt }]\n"},
		{"attestation", "artifacts/tiller/attest.txt", `attestations:
  - { id: AT-aaaaaa, target: { kind: gate, id: review }, status: pass, by: jc, evidence_ref: artifacts/tiller/attest.txt }
`},
		{"policy violation", "artifacts/tiller/violation.txt", `policy_violations:
  - { gate: build, status: fail, evidence_ref: artifacts/tiller/violation.txt }
`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, root := evidenceFixture(t)
			writeLedgerChore(t, root, tc.extra)
			writeFixtureFile(t, root, tc.ref, []byte(tc.name+"\n"))

			resp, err := srv.Client().Get(evidenceURL(srv, tc.ref))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.name+"\n", string(body))
		})
	}
}

// TestEvidenceEndpointRefusesOversizedFile pins the read cap: a recorded
// evidence file larger than the served-file limit answers 413 instead of
// being read whole into memory, while a file exactly at the limit serves.
func TestEvidenceEndpointRefusesOversizedFile(t *testing.T) {
	const atLimit = "artifacts/tiller/gates/at-limit.txt"
	const overLimit = "artifacts/tiller/gates/over-limit.txt"
	srv, root := evidenceFixture(t, atLimit, overLimit)
	writeFixtureFile(t, root, atLimit, make([]byte, serve.MaxServedFileBytes))
	writeFixtureFile(t, root, overLimit, make([]byte, serve.MaxServedFileBytes+1))

	resp, err := srv.Client().Get(evidenceURL(srv, overLimit))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	resp, err = srv.Client().Get(evidenceURL(srv, atLimit))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Len(t, body, serve.MaxServedFileBytes)
}

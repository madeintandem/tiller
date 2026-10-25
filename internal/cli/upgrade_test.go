package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func buildUpgradeStubArchive(t *testing.T, body string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "tiller", Mode: 0755, Size: int64(len(body)), Typeflag: tar.TypeReg}))
	_, err := tw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return b.Bytes()
}
func newUpgradeServer(t *testing.T, lookup, tag, body string, bad bool) (*httptest.Server, string) {
	t.Helper()
	asset := selfupdate.AssetName(tag, runtime.GOOS, runtime.GOARCH)
	archive := buildUpgradeStubArchive(t, body)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	if bad {
		digest = strings.Repeat("0", 64)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == lookup:
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[]}`, tag)
		case strings.HasSuffix(r.URL.Path, "/"+asset):
			_, _ = w.Write(archive)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprintf(w, "%s  %s\n", digest, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, asset
}
func setUpgradeSeams(t *testing.T, server *httptest.Server, target string) {
	t.Helper()
	t.Setenv("TILLER_UPGRADE_API_BASE", server.URL)
	t.Setenv("TILLER_UPGRADE_DOWNLOAD_BASE", server.URL)
	t.Setenv("TILLER_UPGRADE_TARGET", target)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
}
func versionScript(v string) string {
	return "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'tiller version " + v + "'; exit 0; fi\nexit 2\n"
}
func upgradeTarget(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "installed tiller")
	require.NoError(t, os.WriteFile(path, []byte(versionScript(version)), 0755))
	return path
}
func maintenanceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("TILLER_CONTRIBUTOR", "")
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "commit.gpgsign", "false")
	out, err, code := execCLI(t, "init", "--name", "Maintenance", "--key", "maintenance", "--kind", "greenfield")
	require.Equal(t, 0, code, out+err)
	_, _, code = execCLI(t, "add", "contributor", "jc", "--name", "JC")
	require.Equal(t, 0, code)
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "fixture")
	return root
}
func readMaintenanceEnvelope(t *testing.T, out string) maintenanceEnvelope {
	t.Helper()
	var e maintenanceEnvelope
	require.NoError(t, json.Unmarshal([]byte(out), &e), out)
	require.NotEqual(t, e.Data == nil, e.Error == nil, "exactly one envelope block")
	return e
}

func TestUpgradeProjectOnlyOfflineUnjoinedAndNoOp(t *testing.T) {
	root := maintenanceFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected", 500) }))
	defer server.Close()
	setUpgradeSeams(t, server, filepath.Join(root, "missing binary"))
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "orientation.md")))
	sub := filepath.Join(root, "directory with spaces")
	require.NoError(t, os.Mkdir(sub, 0755))
	t.Chdir(sub)
	out, _, code := execCLI(t, "upgrade", "--project-only", "--contributor", "not-a-registered-owner", "--json")
	require.Equal(t, 0, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.True(t, e.Data.Complete)
	require.Nil(t, e.Data.Project.Actor)
	require.Equal(t, root, e.Data.Project.Root)
	require.Equal(t, gen.OrientationGeneration(), e.Data.Project.OrientationAfter)
	_, err := os.Stat(filepath.Join(root, ".tiller", "identity.yml"))
	require.True(t, os.IsNotExist(err))
	require.Zero(t, calls.Load())
	file := filepath.Join(root, ".tiller", "orientation.md")
	before, err := os.Stat(file)
	require.NoError(t, err)
	out, _, code = execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	e = readMaintenanceEnvelope(t, out)
	require.Empty(t, e.Data.Project.Artifacts.Files)
	require.Empty(t, e.Data.Project.Migration.Files)
	after, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
	require.Zero(t, calls.Load())
}

func TestUpgradeAlreadyCurrentStillRefreshesAndReportsActualVersion(t *testing.T) {
	root := maintenanceFixture(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".tiller", "orientation.md")))
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/latest") {
			fmt.Fprint(w, `{"tag_name":"v0.1.0","assets":[]}`)
		} else {
			downloads.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setUpgradeSeams(t, server, upgradeTarget(t, t.TempDir()))
	out, _, code := execCLI(t, "upgrade", "--json")
	require.Equal(t, 0, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.Equal(t, "already_current", e.Data.Outcome)
	require.Equal(t, version, e.Data.To)
	require.Equal(t, "v0.1.0", e.Data.Tag)
	require.Equal(t, "ran", e.Data.Refresh)
	require.Zero(t, downloads.Load())
}

func TestUpgradeScopeMisuseAndBrokenMarkerRefuseBeforeNetwork(t *testing.T) {
	t.Chdir(t.TempDir())
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	setUpgradeSeams(t, server, "unused")
	for _, args := range [][]string{{"--project-only", "--binary-only"}, {"--project-only", "--version", "1.0.0"}, {"--inspect"}, {"--version", "bogus"}} {
		out, _, code := execCLI(t, append([]string{"upgrade", "--json"}, args...)...)
		require.Equal(t, 2, code, out)
	}
	out, _, code := execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 3, code, out)
	require.NoError(t, os.WriteFile(".tiller", []byte("broken marker"), 0644))
	out, _, code = execCLI(t, "upgrade", "--json")
	require.Equal(t, 3, code, out)
	require.Zero(t, hits.Load())
}

func TestUpgradeChecksumAndUnsupportedProtocolPreserveInstallation(t *testing.T) {
	for _, badChecksum := range []bool{true, false} {
		t.Run(fmt.Sprint(badChecksum), func(t *testing.T) {
			maintenanceFixture(t)
			target := upgradeTarget(t, t.TempDir())
			before, err := os.ReadFile(target)
			require.NoError(t, err)
			server, _ := newUpgradeServer(t, "/repos/madeintandem/tiller/releases/latest", "v9.9.9", versionScript("9.9.9"), badChecksum)
			setUpgradeSeams(t, server, target)
			out, _, code := execCLI(t, "upgrade", "--json")
			require.Equal(t, 3, code, out)
			e := readMaintenanceEnvelope(t, out)
			require.False(t, e.OK)
			after, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if badChecksum {
				require.Equal(t, "checksum_mismatch", e.Error.Reason)
			} else {
				require.Equal(t, "maintenance_protocol_invalid", e.Error.Reason)
			}
		})
	}
}

func TestUpgradeBinaryOnlyPinnedDowngradeNeedsNoMaintenanceProtocol(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(".tiller", []byte("excluded broken marker"), 0644))
	target := upgradeTarget(t, t.TempDir())
	server, _ := newUpgradeServer(t, "/repos/madeintandem/tiller/releases/tags/v0.1.0", "v0.1.0", versionScript("0.1.0"), false)
	setUpgradeSeams(t, server, target)
	out, _, code := execCLI(t, "upgrade", "--binary-only", "--version", "0.1.0", "--contributor", "ignored", "--json")
	require.Equal(t, 0, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.Equal(t, "excluded", e.Data.Project.Status)
	require.Equal(t, "0.1.0", e.Data.To)
	require.NotNil(t, e.Data.Retained)
	retained, err := os.ReadFile(e.Data.Retained.Path)
	require.NoError(t, err)
	require.Equal(t, versionScript(version), string(retained))
}

func TestMaintenanceDecoderPreservesPartialMigrationAndRejectsLies(t *testing.T) {
	selected := selfupdate.ExecutableIdentity{Path: "/selected/tiller", Version: "1.0.0", SHA256: strings.Repeat("a", 64)}
	r := newMaintenanceResult("/project", false)
	r.Executing = &selected
	r.Project.SchemaBefore = 4
	r.Project.SchemaAfter = 5
	r.Project.Migration = maintenancePhase{Status: "migrated", Files: []string{".tiller/tiller.yml", ".tiller/events/event.yml"}}
	r.Project.Artifacts.Status = "failed"
	e := maintenanceEnvelope{Command: "upgrade", Error: &ErrorBody{Code: 3, Reason: "refresh_failed", Message: "disk full", PartialResult: &r}}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	decoded, err := decodeMaintenanceChild(data, 3, selected, "/project", false)
	require.NoError(t, err)
	require.Error(t, decoded.err)
	require.Equal(t, 5, decoded.result.Project.SchemaAfter)
	require.Equal(t, "migrated", decoded.result.Project.Migration.Status)
	_, err = decodeMaintenanceChild(data, 0, selected, "/project", false)
	require.Error(t, err)
	r.Protocol++
	data, err = json.Marshal(e)
	require.NoError(t, err)
	_, err = decodeMaintenanceChild(data, 3, selected, "/project", false)
	require.Error(t, err)
}

func TestUpgradeRealTargetMigratesBeforeRefreshing(t *testing.T) {
	// A real compiled candidate, not a shim echoing expected fields, proves that
	// the selected process performs schema migration and generated refresh.
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repo := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	binary := filepath.Join(t.TempDir(), "tiller")
	build := exec.Command("go", "build", "-o", binary, "./cmd/tiller")
	build.Dir = repo
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	body, err := os.ReadFile(binary)
	require.NoError(t, err)
	root := maintenanceFixture(t)
	path := filepath.Join(root, ".tiller", "tiller.yml")
	manifest, err := os.ReadFile(path)
	require.NoError(t, err)
	old, err := store.RenderSchemaVersion([]byte(strings.Replace(string(manifest), "contributors:", "owners:", 1)), 4)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, old, 0644))
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "legacy schema")
	require.NoError(t, os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("human edit"), 0644))
	target := upgradeTarget(t, t.TempDir())
	server, _ := newUpgradeServer(t, "/repos/madeintandem/tiller/releases/tags/v"+version, "v"+version, string(body), false)
	setUpgradeSeams(t, server, target)
	out, _, code := execCLI(t, "upgrade", "--version", version, "--contributor", "jc", "--json")
	require.Equal(t, 0, code, out)
	e := readMaintenanceEnvelope(t, out)
	require.True(t, e.Data.Complete)
	require.Equal(t, "migrated", e.Data.Project.Migration.Status)
	require.Equal(t, 4, e.Data.Project.SchemaBefore)
	require.Equal(t, store.CurrentSchemaVersion, e.Data.Project.SchemaAfter)
	require.NotNil(t, e.Data.Retained)
	identity, err := selfupdate.ReadExecutable(context.Background(), target)
	require.NoError(t, err)
	require.Equal(t, identity.SHA256, e.Data.Executing.SHA256)
	_, err = os.Stat(filepath.Join(root, ".tiller", "identity.yml"))
	require.True(t, os.IsNotExist(err))
	tree, err := store.Load(root)
	require.NoError(t, err)
	require.Equal(t, store.CurrentSchemaVersion, tree.Manifest.SchemaVersion)
	out, _, code = execCLI(t, "upgrade", "--project-only", "--json")
	require.Equal(t, 0, code, out)
	repeat := readMaintenanceEnvelope(t, out)
	require.Empty(t, repeat.Data.Project.Migration.Files)
	require.Empty(t, repeat.Data.Project.Artifacts.Files)
	if os.Geteuid() != 0 {
		root = legacyMaintenanceFixture(t, 4)
		directory := filepath.Join(root, ".claude", "skills")
		require.NoError(t, os.MkdirAll(directory, 0o755))
		require.NoError(t, os.Chmod(directory, 0o555))
		t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
		out, _, code = execCLI(t, "upgrade", "--version", version, "--contributor", "jc", "--json")
		require.Equal(t, 3, code, out)
		failure := readMaintenanceEnvelope(t, out).Error
		require.Equal(t, "maintenance_failed", failure.Reason)
		require.Equal(t, "upgraded", failure.PartialResult.Binary.Status)
		require.Equal(t, "migrated", failure.PartialResult.Project.Migration.Status)
		require.Equal(t, store.CurrentSchemaVersion, failure.PartialResult.Project.SchemaAfter)
		require.Equal(t, "failed", failure.PartialResult.Project.Artifacts.Status)
		require.Empty(t, failure.PartialResult.Project.ShimsAdded)
	}
}

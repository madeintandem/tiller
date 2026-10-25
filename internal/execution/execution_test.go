package execution_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const firstID = "T-aaaaaa"
const secondID = "T-bbbbbb"
const firstKey = "pay/checkout/alfa"
const secondKey = "pay/checkout/bravo"
const evidencePath = "artifacts/tiller/gates/build.txt"

// evidenceLimit is the byte cap these tests pass to ReadFile; every fixture
// file is far smaller.
const evidenceLimit = 1 << 20

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Execution Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Execution Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

func put(t *testing.T, root, path, data string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(data), 0o644))
}

func taskPath(key string) string {
	parts := strings.Split(key, "/")
	return filepath.Join(store.TaskRelDir(parts[0], parts[1], parts[2]), store.TaskStateFile)
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, ".tiller/tiller.yml", fmt.Sprintf("schema_version: %d\nschema_epoch: initial\nproject: {name: Execution, key: execution, kind: greenfield}\ncontributors:\n  - {id: sam, name: Sam}\n  - {id: pat, name: Pat}\ngit: {integration_branch: main}\n", store.CurrentSchemaVersion))
	put(t, root, ".tiller/epics/pay/epic.yml", "id: E-aaaaaa\nkey: pay\nstatus: active\n")
	put(t, root, ".tiller/epics/pay/features/checkout/feature.yml", "id: F-aaaaaa\nkey: pay/checkout\nstatus: ready\n")
	put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\ndepends_on: ["+secondID+"]\n")
	put(t, root, taskPath(secondKey), "id: "+secondID+"\nkey: "+secondKey+"\nstatus: ready\n")
	git(t, root, "init", "-b", "main")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "approved integration structure")
	return root
}

func base(t *testing.T, root string) *store.Tree {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

func worktree(t *testing.T, root, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "work")
	git(t, root, "worktree", "add", "-b", "tiller/"+key, path, "main")
	return path
}

func executing(t *testing.T, root, id, key, owner, status string) {
	t.Helper()
	put(t, root, taskPath(key), "id: "+id+"\nkey: "+key+"\nstatus: "+status+"\nowner: "+owner+"\nstarted: true\nbranch: tiller/"+key+"\ngates:\n  build: {status: pass, required: true, evidence: [{kind: command_output, ref: "+evidencePath+"}]}\n")
}

func load(t *testing.T, tree *store.Tree) *execution.Snapshot {
	t.Helper()
	snapshot, err := execution.Load(tree)
	require.NoError(t, err)
	return snapshot
}

// requireSettledAway closes a snapshot whose source moved: only that task's
// observation is dropped, its integration value is displayed again, and its
// source reads are refused from then on.
func requireSettledAway(t *testing.T, s *execution.Snapshot, id, revision, reason string) {
	t.Helper()
	dropped, err := s.Settle()
	require.NoError(t, err)
	require.True(t, dropped)
	require.NotContains(t, s.Observations, id)
	require.Same(t, s.Base.Tasks[id], s.Tree.Tasks[id])
	found := false
	for _, diagnostic := range s.Diagnostics {
		if diagnostic.ID == id && diagnostic.Code == "source_unstable" && strings.Contains(diagnostic.Message, reason) {
			found = true
		}
	}
	require.True(t, found, "source_unstable diagnostic naming %q: %+v", reason, s.Diagnostics)
	_, err = s.ReadFile(id, revision, evidencePath, evidenceLimit)
	require.Error(t, err)
	_, err = s.Events(id)
	require.Error(t, err)
	dropped, err = s.Settle()
	require.NoError(t, err)
	require.False(t, dropped)
	require.NoError(t, s.CheckStable())
}

func hasDiagnostic(s *execution.Snapshot, code string) bool {
	for _, diagnostic := range s.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestTwoUncommittedOwnersWithoutIntegrationBranches(t *testing.T) {
	root := fixture(t)
	first := worktree(t, root, firstKey)
	second := worktree(t, root, secondKey)
	executing(t, first, firstID, firstKey, "sam", "check")
	executing(t, second, secondID, secondKey, "pat", "in_review")
	integration := base(t, root)
	before := git(t, root, "status", "--porcelain")
	refsBefore := git(t, root, "for-each-ref", "--format=%(refname) %(objectname)")
	indexBefore := map[string][]byte{}
	for _, checkout := range []string{root, first, second} {
		index := git(t, checkout, "rev-parse", "--path-format=absolute", "--git-path", "index")
		data, err := os.ReadFile(index)
		require.NoError(t, err)
		indexBefore[index] = data
	}
	s := load(t, integration)
	require.Len(t, s.Observations, 2)
	require.Empty(t, s.Diagnostics)
	for id, owner := range map[string]string{firstID: "sam", secondID: "pat"} {
		require.Equal(t, owner, s.Tree.Tasks[id].Owner)
		require.Equal(t, "working_tree", s.Observations[id].Source)
		require.True(t, s.Observations[id].Uncommitted)
		require.Equal(t, "ready", s.Observations[id].IntegrationStatus)
		require.Equal(t, "ready", integration.Tasks[id].Status)
		require.Empty(t, integration.Tasks[id].Branch)
	}
	require.Same(t, s.Tree.Tasks[firstID], s.Tree.FeatureTasks["F-aaaaaa"][0])
	require.Equal(t, []string{secondID}, s.Tree.Tasks[firstID].DependsOn)
	require.Equal(t, []store.Contributor{{ID: "pat", Name: "Pat"}, {ID: "sam", Name: "Sam"}}, s.Tree.Contributors["F-aaaaaa"])
	require.Empty(t, integration.Contributors["F-aaaaaa"])
	require.NoError(t, s.CheckStable())
	for index, before := range indexBefore {
		data, err := os.ReadFile(index)
		require.NoError(t, err)
		require.Equal(t, before, data, "execution reads do not refresh or write the index")
	}
	require.Equal(t, refsBefore, git(t, root, "for-each-ref", "--format=%(refname) %(objectname)"))
	require.Equal(t, before, git(t, root, "status", "--porcelain"))
	report := s.Report()
	require.Equal(t, firstID, report.Observations[0].ID)
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"diagnostics":[]`)
}

func TestCommittedLocalAndConfiguredRemoteTracking(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "remote"}[remote], func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "in_review")
			put(t, wt, evidencePath, "selected source evidence\n")
			git(t, wt, "add", ".")
			git(t, wt, "commit", "-m", "submitted execution")
			oid := git(t, wt, "rev-parse", "HEAD")
			git(t, root, "worktree", "remove", wt)
			if remote {
				git(t, root, "config", "remote.team.url", filepath.Join(t.TempDir(), "unreachable"))
				git(t, root, "config", "remote.team.fetch", "+refs/heads/*:refs/remotes/team/*")
				git(t, root, "config", "remote.pushDefault", "team")
				git(t, root, "update-ref", "refs/remotes/team/tiller/"+firstKey, oid)
				git(t, root, "branch", "-D", "tiller/"+firstKey)
			}
			put(t, root, evidencePath, "wrong integration evidence\n")
			s := load(t, base(t, root))
			r := s.Observations[firstID]
			require.Equal(t, map[bool]string{false: "local_branch", true: "remote_tracking"}[remote], r.Source)
			require.False(t, r.Uncommitted)
			data, err := s.ReadFile(firstID, r.Revision, evidencePath, evidenceLimit)
			require.NoError(t, err)
			require.Equal(t, "selected source evidence\n", string(data))
			_, err = s.ReadFile(firstID, "wrong-source-token", evidencePath, evidenceLimit)
			require.Error(t, err)
			require.NoError(t, s.CheckStable())
		})
	}
}

func TestPrimaryExecutionWorktree(t *testing.T) {
	root := fixture(t)
	git(t, root, "checkout", "-b", "tiller/"+firstKey)
	executing(t, root, firstID, firstKey, "sam", "in_progress")
	s := load(t, base(t, root))
	require.Equal(t, "working_tree", s.Observations[firstID].Source)
	require.True(t, s.Observations[firstID].Uncommitted)
}

func TestRejectInvalidCandidateWithoutLosingOtherObservations(t *testing.T) {
	for _, delta := range []string{"id: T-cccccc", "key: pay/checkout/impostor", "branch: unrelated", "status: unknown", "status: done", "status: cancelled", "started: false", "owner: ''", "owner: ' sam '"} {
		t.Run(delta, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "in_progress")
			data, err := os.ReadFile(filepath.Join(wt, taskPath(firstKey)))
			require.NoError(t, err)
			var record map[string]any
			require.NoError(t, yaml.Unmarshal(data, &record))
			var change map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(delta), &change))
			for key, value := range change {
				record[key] = value
			}
			raw, err := yaml.Marshal(record)
			require.NoError(t, err)
			put(t, wt, taskPath(firstKey), string(raw))
			second := worktree(t, root, secondKey)
			executing(t, second, secondID, secondKey, "pat", "check")
			s := load(t, base(t, root))
			require.NotContains(t, s.Observations, firstID)
			require.Contains(t, s.Observations, secondID)
			require.True(t, hasDiagnostic(s, "invalid_candidate"))
			require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
		})
	}
}

func TestMissingRecordedBranchButUntouchedReadyTaskIsQuiet(t *testing.T) {
	root := fixture(t)
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.Empty(t, s.Diagnostics)
	executing(t, root, firstID, firstKey, "sam", "in_progress")
	s = load(t, base(t, root))
	require.True(t, hasDiagnostic(s, "branch_missing"))
	require.Empty(t, s.Observations)
}

func TestDivergentRefsAreNotChosenEvenWithWorktree(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "local execution")
	git(t, root, "config", "remote.team.url", "/not-contacted")
	git(t, root, "config", "remote.team.fetch", "+refs/heads/*:refs/remotes/team/*")
	git(t, root, "config", "branch.tiller/"+firstKey+".remote", "team")
	git(t, root, "config", "branch.tiller/"+firstKey+".merge", "refs/heads/tiller/"+firstKey)
	put(t, root, "remote-only.txt", "diverging commit\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "other history")
	git(t, root, "update-ref", "refs/remotes/team/tiller/"+firstKey, "HEAD")
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.True(t, hasDiagnostic(s, "divergent_refs"))
}

func TestIntegrationTerminalReplacementAndParentAuthority(t *testing.T) {
	for _, state := range []string{"done", "cancelled", "replaced", "parent_cancelled", "parent_replaced", "epic_archived"} {
		t.Run(state, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "in_review")
			integration := base(t, root)
			switch state {
			case "replaced":
				integration.Tasks[firstID].SupersededBy = new(secondID)
			case "parent_cancelled":
				integration.Features["F-aaaaaa"].Status = "cancelled"
			case "parent_replaced":
				integration.Features["F-aaaaaa"].SupersededBy = new("F-bbbbbb")
			case "epic_archived":
				integration.Epics["E-aaaaaa"].Status = "archived"
			default:
				integration.Tasks[firstID].Status = state
			}
			s := load(t, integration)
			require.Empty(t, s.Observations)
			require.Equal(t, integration.Tasks[firstID].Status, s.Tree.Tasks[firstID].Status)
		})
	}
}

func TestReviewGrainsOnlyOverlayTaskExecution(t *testing.T) {
	for _, grain := range []string{"task", "feature", "epic"} {
		t.Run(grain, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "in_review")
			raw, err := os.ReadFile(filepath.Join(wt, taskPath(firstKey)))
			require.NoError(t, err)
			var task store.Task
			require.NoError(t, yaml.Unmarshal(raw, &task))
			task.Title = "unapproved branch title"
			task.DependsOn = nil
			task.Supersedes = []string{"T-cccccc"}
			task.PolicyViolations = []store.PolicyViolation{{Gate: "branch-only", Status: "fail"}}
			task.Attestations = []store.Attestation{{ID: "AT-aaaaaa", By: "sam", Status: "pass"}}
			task.Evidence = []store.Evidence{{Kind: "command_output", Ref: evidencePath}}
			if grain == "task" {
				task.PR = new(42)
			}
			task.ReviewRef = &store.ReviewRef{Branch: "tiller/" + firstKey, Head: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:" + git(t, wt, "rev-parse", "HEAD")}}
			raw, err = yaml.Marshal(task)
			require.NoError(t, err)
			put(t, wt, taskPath(firstKey), string(raw))
			executing(t, wt, secondID, secondKey, "sam", "in_progress")
			put(t, wt, ".tiller/epics/pay/epic.yml", "id: E-aaaaaa\nkey: pay\nstatus: in_review\npr: 98\nbranch: reviews/epic\n")
			put(t, wt, ".tiller/epics/pay/features/checkout/feature.yml", "id: F-aaaaaa\nkey: pay/checkout\nstatus: done\npr: 99\nbranch: reviews/feature\n")
			integration := base(t, root)
			integration.Manifest.Execution.ReviewGrain = grain
			integration.Tasks[firstID].PolicyViolations = []store.PolicyViolation{{Gate: "integration-history", Status: "fail"}}
			s := load(t, integration)
			require.Equal(t, "in_review", s.Tree.Tasks[firstID].Status)
			require.Equal(t, "ready", s.Tree.Features["F-aaaaaa"].Status)
			require.Nil(t, s.Tree.Features["F-aaaaaa"].PR)
			require.Nil(t, s.Tree.Epics["E-aaaaaa"].PR)
			require.Empty(t, s.Tree.Features["F-aaaaaa"].Branch)
			require.Equal(t, []string{secondID}, s.Tree.Tasks[firstID].DependsOn)
			require.Equal(t, "ready", s.Tree.Tasks[secondID].Status)
			require.Empty(t, s.Tree.Tasks[secondID].Owner)
			require.Empty(t, s.Tree.Tasks[firstID].Title)
			require.Empty(t, s.Tree.Tasks[firstID].Supersedes)
			require.Nil(t, s.Tree.Tasks[firstID].AcceptedAgainst)
			require.Equal(t, integration.Tasks[firstID].PolicyViolations, s.Tree.Tasks[firstID].PolicyViolations)
			require.Equal(t, task.Attestations, s.Tree.Tasks[firstID].Attestations)
			require.Equal(t, task.Evidence, s.Tree.Tasks[firstID].Evidence)
			if grain == "task" {
				require.Equal(t, 42, *s.Tree.Tasks[firstID].PR)
			} else {
				require.Nil(t, s.Tree.Tasks[firstID].PR)
			}
			require.Equal(t, "tiller/"+firstKey, s.Tree.Tasks[firstID].ReviewRef.Branch)
			require.Equal(t, store.GateStatusPass, s.Tree.Tasks[firstID].Gates["build"].Status)
		})
	}
}

func TestWorktreeSnapshotChangesInvalidateReads(t *testing.T) {
	for _, change := range []string{"task", "generation", "branch", "ref", "lock", "journal"} {
		t.Run(change, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			put(t, wt, evidencePath, "evidence\n")
			s := load(t, base(t, root))
			r := s.Observations[firstID]
			switch change {
			case "task":
				executing(t, wt, firstID, firstKey, "sam", "in_review")
			case "generation":
				put(t, wt, ".tiller/.cache/generation", "new generation")
			case "branch":
				git(t, wt, "symbolic-ref", "HEAD", "refs/heads/elsewhere")
			case "ref":
				git(t, wt, "commit", "--allow-empty", "-m", "changed head")
			case "lock":
				put(t, wt, ".tiller/.cache/lock", "locked")
			case "journal":
				put(t, wt, ".tiller/.cache/journal.yml", "journal")
			}
			require.Error(t, s.CheckStable())
			requireSettledAway(t, s, firstID, r.Revision, "")
			if change == "task" {
				newSnapshot := load(t, base(t, root))
				require.NotEqual(t, r.Revision, newSnapshot.Observations[firstID].Revision)
			}
		})
	}
}

func TestBaseGenerationIncludedInStableBoundary(t *testing.T) {
	root := fixture(t)
	s := load(t, base(t, root))
	put(t, root, ".tiller/.cache/generation", "changed integration")
	require.Error(t, s.CheckStable())
}

func TestSourceFilesAndEventsAreConfinedAndTaskScoped(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			put(t, wt, evidencePath, "task evidence\n")
			put(t, wt, ".tiller/events/2026/10/06/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: 2026-10-06T12:00:00Z\ncontributor: sam\ntargets: ["+firstID+"]\n")
			put(t, wt, ".tiller/events/2026/10/06/EV-bbbbbb.yml", "id: EV-bbbbbb\ntype: start.task\nat: 2026-10-06T12:00:01Z\ncontributor: pat\ntargets: ["+secondID+"]\n")
			put(t, wt, "secret.txt", "secret")
			require.NoError(t, os.Symlink("secret.txt", filepath.Join(wt, "alias.txt")))
			require.NoError(t, os.Symlink("artifacts/tiller", filepath.Join(wt, "alias-dir")))
			if committed {
				git(t, wt, "add", ".")
				git(t, wt, "commit", "-m", "execution with support")
				git(t, root, "worktree", "remove", wt)
			}
			s := load(t, base(t, root))
			record := s.Observations[firstID]
			data, err := s.ReadFile(firstID, record.Revision, evidencePath, evidenceLimit)
			require.NoError(t, err)
			require.Equal(t, "task evidence\n", string(data))
			for _, path := range []string{"../secret.txt", ".git/config", "alias.txt", "alias-dir/gates/build.txt", "missing.txt"} {
				_, err := s.ReadFile(firstID, record.Revision, path, evidenceLimit)
				require.Error(t, err, path)
			}
			evs, err := s.Events(firstID)
			require.NoError(t, err)
			require.Len(t, evs, 1)
			require.Equal(t, "EV-aaaaaa", evs[0].ID)
		})
	}
}

func TestUnsafeCandidateAndSupportAreUnavailableWithoutBlocking(t *testing.T) {
	for _, scenario := range []string{"task_symlink", "task_parent_symlink", "task_fifo", "evidence_fifo", "events_symlink", "events_malformed"} {
		t.Run(scenario, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			switch scenario {
			case "task_symlink":
				require.NoError(t, os.Remove(filepath.Join(wt, taskPath(firstKey))))
				require.NoError(t, os.Symlink(filepath.Join(root, taskPath(firstKey)), filepath.Join(wt, taskPath(firstKey))))
			case "task_parent_symlink":
				dir := filepath.Dir(filepath.Join(wt, taskPath(firstKey)))
				require.NoError(t, os.Rename(dir, dir+"-original"))
				require.NoError(t, os.Symlink(dir+"-original", dir))
			case "task_fifo":
				require.NoError(t, os.Remove(filepath.Join(wt, taskPath(firstKey))))
				require.NoError(t, syscall.Mkfifo(filepath.Join(wt, taskPath(firstKey)), 0o600))
			case "evidence_fifo":
				require.NoError(t, os.MkdirAll(filepath.Join(wt, filepath.Dir(evidencePath)), 0o755))
				require.NoError(t, syscall.Mkfifo(filepath.Join(wt, evidencePath), 0o600))
			case "events_symlink":
				require.NoError(t, os.Symlink(root, filepath.Join(wt, ".tiller/events")))
			case "events_malformed":
				put(t, wt, ".tiller/events/2026/10/06/EV-aaaaaa.yml", "not: [valid")
			}
			s := load(t, base(t, root))
			if strings.HasPrefix(scenario, "task_") {
				require.Empty(t, s.Observations)
				require.True(t, hasDiagnostic(s, "source_unavailable"))
			} else if scenario == "evidence_fifo" {
				_, err := s.ReadFile(firstID, s.Observations[firstID].Revision, evidencePath, evidenceLimit)
				require.Error(t, err)
			} else {
				_, err := s.Events(firstID)
				require.Error(t, err)
			}
		})
	}
}

func TestNonGitFixtureHasEmptyArrayReport(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".tiller/tiller.yml", fmt.Sprintf("schema_version: %d\nschema_epoch: initial\nproject: {name: Fixture, key: fixture, kind: greenfield}\n", store.CurrentSchemaVersion))
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.Empty(t, s.Diagnostics)
	raw, err := json.Marshal(s.Report())
	require.NoError(t, err)
	require.JSONEq(t, `{"observations":[],"diagnostics":[]}`, string(raw))
}

func TestConfiguredPatternAndClaimOnlyDiscovery(t *testing.T) {
	root := fixture(t)
	manifest, err := os.ReadFile(filepath.Join(root, ".tiller/tiller.yml"))
	require.NoError(t, err)
	var m store.Manifest
	require.NoError(t, yaml.Unmarshal(manifest, &m))
	m.Git.BranchPattern = "work/${node.key}"
	raw, err := yaml.Marshal(m)
	require.NoError(t, err)
	put(t, root, ".tiller/tiller.yml", string(raw))
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "configured branch naming")
	wt := filepath.Join(t.TempDir(), "claim")
	git(t, root, "worktree", "add", "-b", "work/"+firstKey, wt)
	put(t, wt, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\nstarted: false\n")
	s := load(t, base(t, root))
	require.Equal(t, "sam", s.Tree.Tasks[firstID].Owner)
	require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
	require.Equal(t, "work/"+firstKey, s.Observations[firstID].Branch)
	require.False(t, s.Tree.Tasks[firstID].Started)
	require.Empty(t, s.Tree.Contributors["F-aaaaaa"])
}

func TestForeignWorktreeGitIdentityIsRejected(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	foreign := fixture(t)
	git(t, foreign, "checkout", "-b", "tiller/"+firstKey)
	put(t, wt, ".git", "gitdir: "+filepath.Join(foreign, ".git")+"\n")
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.True(t, hasDiagnostic(s, "source_unavailable"))
	require.Contains(t, s.Diagnostics[0].Message, "common directory")
}

func TestRefChangesInvalidatePinnedSourceAndTaskTokensAreDistinct(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	put(t, wt, evidencePath, "committed evidence")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "execution")
	git(t, root, "worktree", "remove", wt)
	s := load(t, base(t, root))
	r := s.Observations[firstID]
	_, err := s.ReadFile(secondID, r.Revision, evidencePath, evidenceLimit)
	require.Error(t, err)
	git(t, root, "update-ref", "refs/heads/tiller/"+firstKey, "main")
	require.Error(t, s.CheckStable())
	requireSettledAway(t, s, firstID, r.Revision, "ref refs/heads/tiller/"+firstKey+" changed")
}

func TestRefCandidateSymlinkAndMalformedHistoryAreRejected(t *testing.T) {
	for _, scenario := range []string{"task_symlink", "task_parent_symlink", "events_symlink", "events_malformed"} {
		t.Run(scenario, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			switch scenario {
			case "task_symlink":
				require.NoError(t, os.Remove(filepath.Join(wt, taskPath(firstKey))))
				require.NoError(t, os.Symlink("task.md", filepath.Join(wt, taskPath(firstKey))))
			case "task_parent_symlink":
				dir := filepath.Dir(filepath.Join(wt, taskPath(firstKey)))
				require.NoError(t, os.Rename(dir, dir+"-original"))
				require.NoError(t, os.Symlink(filepath.Base(dir)+"-original", dir))
			case "events_symlink":
				require.NoError(t, os.Symlink("epics", filepath.Join(wt, ".tiller/events")))
			case "events_malformed":
				put(t, wt, ".tiller/events/2026/10/06/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: invalid\n")
			}
			git(t, wt, "add", ".")
			git(t, wt, "commit", "-m", "unsafe or malformed branch support")
			git(t, root, "worktree", "remove", wt)
			s := load(t, base(t, root))
			if strings.HasPrefix(scenario, "task_") {
				require.Empty(t, s.Observations)
				require.True(t, hasDiagnostic(s, "source_unavailable"))
			} else {
				_, err := s.Events(firstID)
				require.Error(t, err)
			}
		})
	}
}

func TestCustomTrackingRefspecAndPushDestination(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "in_review")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "published execution")
	oid := git(t, wt, "rev-parse", "HEAD")
	git(t, root, "worktree", "remove", wt)
	git(t, root, "branch", "-D", "tiller/"+firstKey)
	git(t, root, "config", "remote.team.url", "/unreachable/no-network")
	git(t, root, "config", "remote.team.fetch", "+refs/heads/incoming/*:refs/remotes/team/review/*")
	git(t, root, "config", "remote.team.push", "refs/heads/tiller/*:refs/heads/incoming/*")
	git(t, root, "config", "remote.pushDefault", "team")
	git(t, root, "update-ref", "refs/remotes/team/review/"+firstKey, oid)
	s := load(t, base(t, root))
	require.Equal(t, "remote_tracking", s.Observations[firstID].Source)
	require.Equal(t, "in_review", s.Tree.Tasks[firstID].Status)
	// The record's source already names the tracking ref; it is not a diagnostic.
	require.Empty(t, s.Diagnostics)
}

func TestUntouchedTasksWithMultipleRemotesRemainQuiet(t *testing.T) {
	root := fixture(t)
	for _, remote := range []string{"team", "elsewhere"} {
		git(t, root, "config", "remote."+remote+".url", "/not-contacted")
		git(t, root, "config", "remote."+remote+".fetch", "+refs/heads/*:refs/remotes/"+remote+"/*")
	}
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.Empty(t, s.Diagnostics)
}

func TestCommittedEvidenceGitlinkIsUnavailable(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	git(t, wt, "add", ".")
	oid := git(t, wt, "rev-parse", "HEAD")
	git(t, wt, "update-index", "--add", "--cacheinfo", "160000,"+oid+",linked-evidence")
	git(t, wt, "commit", "-m", "execution with unsupported evidence")
	git(t, root, "worktree", "remove", "--force", wt)
	s := load(t, base(t, root))
	_, err := s.ReadFile(firstID, s.Observations[firstID].Revision, "linked-evidence", evidenceLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "regular Git blob")
}

func TestSourceTokensDoNotCrossWorktreeAndCommittedObservations(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "sam", "check")
	put(t, wt, evidencePath, "same committed bytes")
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-m", "execution")
	working := load(t, base(t, root))
	workingRevision := working.Observations[firstID].Revision
	git(t, root, "worktree", "remove", wt)
	committed := load(t, base(t, root))
	committedRevision := committed.Observations[firstID].Revision
	require.NotEqual(t, workingRevision, committedRevision)
	_, err := committed.ReadFile(firstID, workingRevision, evidencePath, evidenceLimit)
	require.Error(t, err)
	nextWorktree := filepath.Join(t.TempDir(), "work")
	git(t, root, "worktree", "add", nextWorktree, "tiller/"+firstKey)
	next := load(t, base(t, root))
	require.NotEqual(t, committedRevision, next.Observations[firstID].Revision)
	require.NotEqual(t, workingRevision, next.Observations[firstID].Revision)
	_, err = next.ReadFile(firstID, committedRevision, evidencePath, evidenceLimit)
	require.Error(t, err)
}

func TestIntegrationStartedOwnerCannotBeReassignedByObservation(t *testing.T) {
	root := fixture(t)
	wt := worktree(t, root, firstKey)
	executing(t, wt, firstID, firstKey, "pat", "check")
	executing(t, root, firstID, firstKey, "sam", "in_progress")
	s := load(t, base(t, root))
	require.Empty(t, s.Observations)
	require.True(t, hasDiagnostic(s, "invalid_candidate"))
	require.Equal(t, "sam", s.Tree.Tasks[firstID].Owner)
	require.Equal(t, "in_progress", s.Tree.Tasks[firstID].Status)
}

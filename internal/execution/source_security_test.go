package execution_test

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPopulatedWorktreeGitlinksAreUnavailable(t *testing.T) {
	for _, boundary := range []string{"staged", "HEAD"} {
		for _, support := range []string{"task", "evidence", "events", "event_child"} {
			t.Run(boundary+"/"+support, func(t *testing.T) {
				root := fixture(t)
				wt := worktree(t, root, firstKey)
				executing(t, wt, firstID, firstKey, "sam", "check")
				sub := t.TempDir()
				var path string
				switch support {
				case "task":
					path = filepath.Dir(taskPath(firstKey))
					data, err := os.ReadFile(filepath.Join(wt, taskPath(firstKey)))
					require.NoError(t, err)
					put(t, sub, "task.yml", string(data))
					git(t, wt, "rm", "-r", "-f", "--", path)
				case "evidence":
					path = filepath.Dir(evidencePath)
					put(t, sub, "build.txt", "foreign repository evidence\n")
				case "events", "event_child":
					path = ".tiller/events"
					if support == "event_child" {
						path += "/2026/10/06"
					}
					put(t, sub, "EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: 2026-10-06T12:00:00Z\ncontributor: sam\ntargets: ["+firstID+"]\n")
				}
				git(t, sub, "init", "-b", "main")
				git(t, sub, "add", ".")
				git(t, sub, "commit", "-m", "independent repository support")
				git(t, wt, "-c", "protocol.file.allow=always", "submodule", "add", sub, path)
				if boundary == "HEAD" {
					git(t, wt, "add", ".")
					git(t, wt, "commit", "-m", "committed gitlink")
					// Keep the populated checkout but remove the index boundary:
					// HEAD alone must still prohibit these reads.
					git(t, wt, "update-index", "--force-remove", "--", path)
				}
				index := git(t, wt, "rev-parse", "--path-format=absolute", "--git-path", "index")
				before, err := os.ReadFile(index)
				require.NoError(t, err)
				s := load(t, base(t, root))
				if support == "task" {
					require.NotContains(t, s.Observations, firstID)
					require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
					require.True(t, hasDiagnostic(s, "source_unavailable"))
				} else {
					require.Contains(t, s.Observations, firstID)
					if support == "evidence" {
						_, err = s.ReadFile(firstID, s.Observations[firstID].Revision, evidencePath, evidenceLimit)
					} else {
						_, err = s.Events(firstID)
					}
					require.ErrorContains(t, err, "gitlink")
				}
				after, err := os.ReadFile(index)
				require.NoError(t, err)
				require.Equal(t, before, after, "gitlink inspection must not refresh the index")
			})
		}
	}
}

func TestStagingGitlinkInvalidatesExistingSnapshot(t *testing.T) {
	for _, path := range []string{filepath.Dir(taskPath(firstKey)), filepath.Dir(evidencePath), ".tiller/events"} {
		t.Run(path, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			put(t, wt, evidencePath, "unchanged evidence\n")
			put(t, wt, ".tiller/events/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: 2026-10-06T12:00:00Z\ncontributor: sam\ntargets: ["+firstID+"]\n")
			s := load(t, base(t, root))
			r := s.Observations[firstID]
			// Only the index changes; the already-observed filesystem bytes,
			// branch tip, and Tiller generation stay unchanged.
			if path == filepath.Dir(taskPath(firstKey)) {
				git(t, wt, "rm", "--cached", "-r", "--", path)
			}
			oid := git(t, wt, "rev-parse", "HEAD")
			git(t, wt, "update-index", "--add", "--cacheinfo", "160000,"+oid+","+path)
			require.ErrorContains(t, s.CheckStable(), "index changed")
			requireSettledAway(t, s, firstID, r.Revision, "index changed")
		})
	}
}

func TestNestedProjectSourcesNeverUseCopiedOuterPlan(t *testing.T) {
	for _, mode := range []string{"worktree", "committed", "primary", "missing_nested", "nested_symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := fixture(t)
			prefix := filepath.Join("apps", "product")
			project := filepath.Join(root, prefix)
			require.NoError(t, os.CopyFS(filepath.Join(project, ".tiller"), os.DirFS(filepath.Join(root, ".tiller"))))
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "nested project with copied outer identities")
			var wt string
			if mode == "primary" {
				git(t, root, "checkout", "-b", "tiller/"+firstKey)
				wt = root
			} else {
				wt = worktree(t, root, firstKey)
			}
			nested := filepath.Join(wt, prefix)
			executing(t, wt, firstID, firstKey, "pat", "in_review")
			put(t, wt, evidencePath, "wrong outer evidence\n")
			put(t, wt, ".tiller/events/EV-bbbbbb.yml", "id: EV-bbbbbb\ntype: start.task\nat: 2026-10-06T12:00:00Z\ncontributor: pat\ntargets: ["+firstID+"]\n")
			executing(t, nested, firstID, firstKey, "sam", "check")
			put(t, nested, evidencePath, "selected nested evidence\n")
			put(t, nested, ".tiller/events/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: check.task\nat: 2026-10-06T12:00:00Z\ncontributor: sam\ntargets: ["+firstID+"]\n")
			if mode == "missing_nested" {
				require.NoError(t, os.Remove(filepath.Join(nested, taskPath(firstKey))))
			}
			if mode == "nested_symlink" {
				foreign := filepath.Join(t.TempDir(), "foreign-project")
				require.NoError(t, os.Rename(nested, foreign))
				require.NoError(t, os.Symlink(foreign, nested))
			}
			if mode == "committed" {
				git(t, wt, "add", ".")
				git(t, wt, "commit", "-m", "distinct outer and nested execution")
				git(t, root, "worktree", "remove", wt)
			}
			s := load(t, base(t, project))
			if mode == "missing_nested" || mode == "nested_symlink" {
				require.NotContains(t, s.Observations, firstID)
				require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
				require.True(t, hasDiagnostic(s, "source_unavailable"))
				return
			}
			r := s.Observations[firstID]
			require.Equal(t, "sam", s.Tree.Tasks[firstID].Owner)
			require.Equal(t, "check", s.Tree.Tasks[firstID].Status)
			if mode == "committed" {
				require.Equal(t, "local_branch", r.Source)
			} else {
				require.Equal(t, "working_tree", r.Source)
			}
			data, err := s.ReadFile(firstID, r.Revision, evidencePath, evidenceLimit)
			require.NoError(t, err)
			require.Equal(t, "selected nested evidence\n", string(data))
			events, err := s.Events(firstID)
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "EV-aaaaaa", events[0].ID)
			require.NoError(t, s.CheckStable())
			if mode != "committed" {
				put(t, wt, ".tiller/.cache/generation", "outer generation changes independently")
				require.NoError(t, s.CheckStable())
				put(t, nested, ".tiller/.cache/generation", "nested generation changed")
				require.Error(t, s.CheckStable())
			}
		})
	}
}

func objectInventory(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()
	objects := filepath.Join(root, ".git", "objects")
	inventory := map[string][sha256.Size]byte{}
	require.NoError(t, filepath.WalkDir(objects, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			inventory[strings.TrimPrefix(path, objects)] = sha256.Sum256(data)
		}
		return nil
	}))
	return inventory
}

func TestMissingPromisorObjectsNeverFetchOrWriteObjects(t *testing.T) {
	for _, missing := range []string{"task", "evidence", "tree"} {
		t.Run(missing, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			put(t, wt, evidencePath, "locally unavailable promised evidence\n")
			git(t, wt, "add", ".")
			git(t, wt, "commit", "-m", "promised execution")
			head := git(t, wt, "rev-parse", "HEAD")
			path := taskPath(firstKey)
			if missing == "evidence" {
				path = evidencePath
			} else if missing == "tree" {
				path = filepath.Dir(taskPath(firstKey))
			}
			oid := git(t, wt, "rev-parse", "HEAD:"+filepath.ToSlash(path))
			git(t, root, "worktree", "remove", wt)
			remote := filepath.Join(t.TempDir(), "promisor.git")
			git(t, root, "clone", "--bare", "--no-hardlinks", root, remote)
			git(t, remote, "config", "uploadpack.allowFilter", "true")
			git(t, remote, "config", "uploadpack.allowAnySHA1InWant", "true")
			// Mark a local filtered pack as promisor. The full local remote has
			// every omitted object, so an accidental fetch would succeed.
			packPrefix := filepath.Join(root, ".git", "objects", "pack", "promised")
			args := []string{"pack-objects"}
			if missing != "tree" {
				args = append(args, "--revs", "--filter=blob:none")
			}
			args = append(args, packPrefix)
			cmd := exec.Command("git", args...)
			cmd.Dir = root
			cmd.Stdin = strings.NewReader(head + "\n")
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			packName := packPrefix + "-" + strings.TrimSpace(string(out))
			require.NoError(t, os.WriteFile(packName+".promisor", nil, 0o644))
			require.NoError(t, os.Remove(filepath.Join(root, ".git", "objects", oid[:2], oid[2:])))
			marker := filepath.Join(t.TempDir(), "transport-contacted")
			wrapper := filepath.Join(t.TempDir(), "upload-pack")
			uploadPack := filepath.Join(git(t, root, "--exec-path"), "git-upload-pack")
			quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
			require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf contacted >> "+quote(marker)+"\nexec "+quote(uploadPack)+" \"$@\"\n"), 0o755))
			git(t, root, "config", "remote.origin.url", remote)
			git(t, root, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
			git(t, root, "config", "remote.origin.promisor", "true")
			git(t, root, "config", "remote.origin.partialclonefilter", "blob:none")
			git(t, root, "config", "remote.origin.uploadpack", wrapper)
			before := objectInventory(t, root)
			s := load(t, base(t, root))
			if missing == "evidence" {
				require.Contains(t, s.Observations, firstID)
				_, err = s.ReadFile(firstID, s.Observations[firstID].Revision, evidencePath, evidenceLimit)
				require.Error(t, err, "promised evidence must stay unavailable")
			} else {
				require.NotContains(t, s.Observations, firstID)
				require.True(t, hasDiagnostic(s, "source_unavailable"))
			}
			_, err = os.Stat(marker)
			require.ErrorIs(t, err, fs.ErrNotExist, "execution observation must not contact the promisor transport")
			require.Equal(t, before, objectInventory(t, root), "execution observation must not write fetched objects")
			// A control read without the inspection environment must reach
			// this local promisor and retrieve the absent object. This proves
			// the fixture exposes lazy fetching, rather than an inert remote.
			control := exec.Command("git", "cat-file", "-p", oid)
			control.Dir = root
			control.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=0")
			controlOut, err := control.CombinedOutput()
			require.NoError(t, err, "%s", controlOut)
			_, err = os.Stat(marker)
			require.NoError(t, err, "control read must contact the local promisor")
			require.NotEqual(t, before, objectInventory(t, root), "control read must fetch the missing object")
		})
	}
}

func TestRefreshedAcceptedTaskBasisRejectsStaleExecutionGates(t *testing.T) {
	for _, change := range []string{"spec", "eval", "commit", "missing_source", "missing_integration", "matching"} {
		t.Run(change, func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			executing(t, wt, firstID, firstKey, "sam", "check")
			integration := base(t, root)
			approved := &store.BasisRefs{
				Spec:   store.RevisionRef{Kind: store.KindGitBlob, Value: "git:original-spec"},
				Eval:   store.RevisionRef{Kind: store.KindGitBlob, Value: "git:original-eval"},
				Commit: store.RevisionRef{Kind: store.KindGitCommit, Value: "git:original-code"},
			}
			candidate := base(t, wt).Tasks[firstID]
			candidate.AcceptedAgainst = approved
			if change == "missing_source" {
				candidate.AcceptedAgainst = nil
			}
			raw, err := yaml.Marshal(candidate)
			require.NoError(t, err)
			put(t, wt, taskPath(firstKey), string(raw))
			refreshed := *approved
			switch change {
			case "spec":
				refreshed.Spec.Value = "git:newly-approved-spec"
			case "eval":
				refreshed.Eval.Value = "git:newly-approved-eval"
			case "commit":
				refreshed.Commit.Value = "git:newly-approved-code"
			}
			integration.Tasks[firstID].AcceptedAgainst = &refreshed
			if change == "missing_integration" {
				integration.Tasks[firstID].AcceptedAgainst = nil
			}
			raw, err = yaml.Marshal(integration.Tasks[firstID])
			require.NoError(t, err)
			put(t, root, taskPath(firstKey), string(raw))
			s := load(t, base(t, root))
			if change == "matching" {
				require.Contains(t, s.Observations, firstID)
				require.Equal(t, store.GateStatusPass, s.Tree.Tasks[firstID].Gates["build"].Status)
				return
			}
			require.NotContains(t, s.Observations, firstID)
			require.True(t, hasDiagnostic(s, "invalid_candidate"))
			require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
			require.Empty(t, s.Tree.Tasks[firstID].Gates)
			_, err = s.ReadFile(firstID, "", evidencePath, evidenceLimit)
			require.Error(t, err, "rejected stale gates must not retain an evidence source")
		})
	}
}

func TestSelectedReadyReleaseClearsIntegrationReservation(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "worktree", true: "committed"}[committed], func(t *testing.T) {
			root := fixture(t)
			wt := worktree(t, root, firstKey)
			put(t, root, taskPath(firstKey), "id: "+firstID+"\nkey: "+firstKey+"\nstatus: ready\nowner: sam\nstarted: false\n")
			if committed {
				git(t, wt, "commit", "--allow-empty", "-m", "released ready reservation")
				git(t, root, "worktree", "remove", wt)
			}
			integration := base(t, root)
			s := load(t, integration)
			require.Contains(t, s.Observations, firstID)
			require.Equal(t, "ready", s.Tree.Tasks[firstID].Status)
			require.Empty(t, s.Tree.Tasks[firstID].Owner)
			require.False(t, s.Tree.Tasks[firstID].Started)
			require.Equal(t, "sam", integration.Tasks[firstID].Owner)
			require.Empty(t, s.Observations[firstID].Owner)
			require.NoError(t, s.CheckStable())
		})
	}
}

package vcs

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestHistoricalResolveCommit(t *testing.T) {
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	for _, input := range []string{head, head[:8], strings.ToUpper(head)} {
		got, err := g.ResolveCommit(input)
		require.NoError(t, err)
		require.Equal(t, head, got)
	}
	run(t, dir, "tag", "-a", "review", "-m", "review")
	for _, input := range []string{"HEAD", "main", head + "^", head + "^{commit}", " " + head, head[:3], strings.Repeat("0", 40), run(t, dir, "rev-parse", "HEAD:README.md"), run(t, dir, "rev-parse", "HEAD^{tree}"), run(t, dir, "rev-parse", "review")} {
		_, err := g.ResolveCommit(input)
		var nv *NotVersionedError
		require.ErrorAs(t, err, &nv, "input %q must not resolve as a commit", input)
	}
}

func TestHistoricalResolveCommitRejectsAmbiguousObjects(t *testing.T) {
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	prefix := head[:4]
	for i := range 2000000 {
		content := fmt.Sprintf("historical ambiguity %d\n", i)
		oid := fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content))))
		if oid[:4] != prefix {
			continue
		}
		cmd := exec.Command("git", "hash-object", "-w", "--stdin")
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(content)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		_, err = g.ResolveCommit(prefix)
		var nv *NotVersionedError
		require.ErrorAs(t, err, &nv)
		full, err := g.ResolveCommit(head)
		require.NoError(t, err)
		require.Equal(t, head, full)
		return
	}
	t.Fatal("could not create ambiguous four-digit commit prefix")
}

func TestHistoricalCommitParentsAreExactAndOrdered(t *testing.T) {
	dir, g := newRepo(t)
	root := run(t, dir, "rev-parse", "HEAD")
	parents, err := g.CommitParents(root)
	require.NoError(t, err)
	require.Empty(t, parents)
	run(t, dir, "checkout", "-b", "review")
	require.NoError(t, writeRepoFile(dir, "review.yml", "submitted\n"))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "review")
	side := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "checkout", "main")
	run(t, dir, "merge", "--no-ff", "review", "-m", "integration")
	merge := run(t, dir, "rev-parse", "HEAD")
	parents, err = g.CommitParents(merge[:8])
	require.NoError(t, err)
	require.Equal(t, []string{root, side}, parents)
	// Replacement refs must not rewrite immutable historical topology.
	run(t, dir, "replace", merge, side)
	parents, err = g.CommitParents(merge)
	require.NoError(t, err)
	require.Equal(t, []string{root, side}, parents)
}

func TestHistoricalFilesAreProjectRootedAndPinned(t *testing.T) {
	dir, _ := newRepo(t)
	project := "projects/nested"
	contents := "id: T-old\nstatus: in_review\n\x00tail \n"
	for rel, value := range map[string]string{
		project + "/.tiller/work/feature/tasks/a/task.yml": contents,
		project + "/.tiller/work/feature/feature.yml":      "id: F-old\n",
		project + "/.tiller/workish/task.yml":              "outside selected directory\n",
		project + "/:literal[1].yml":                       "literal\n",
		".tiller/work/other/task.yml":                      "outside project\n",
	} {
		require.NoError(t, writeRepoFile(dir, rel, value))
	}
	require.NoError(t, os.Chmod(filepath.Join(dir, project, ".tiller/work/feature/tasks/a/task.yml"), 0o755))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "submitted state")
	head := run(t, dir, "rev-parse", "HEAD")
	g := New(filepath.Join(dir, project))
	g.Clock = fixedClock{t: clockAt}
	taskPath := ".tiller/work/feature/tasks/a/task.yml"
	blob := run(t, dir, "rev-parse", head+":"+project+"/"+taskPath)
	require.NoError(t, writeRepoFile(dir, project+"/"+taskPath, "changed today\n"))
	beforeRefs := run(t, dir, "show-ref")
	beforeIndex := run(t, dir, "ls-files", "--stage")
	data, ref, err := g.ReadFileAt(head[:8], taskPath)
	require.NoError(t, err)
	require.Equal(t, []byte(contents), data)
	require.Equal(t, store.KindGitBlob, ref.Kind)
	require.Equal(t, "git:"+blob, ref.Value)
	require.Equal(t, taskPath, ref.Path)
	require.Equal(t, clockAt, ref.ObservedAt)
	files, err := g.FilesAt(head, ".tiller/work")
	require.NoError(t, err)
	require.Equal(t, []string{".tiller/work/feature/feature.yml", taskPath}, files)
	for _, root := range []string{"", "."} {
		files, err := g.FilesAt(head, root)
		require.NoError(t, err)
		require.Equal(t, []string{".tiller/work/feature/feature.yml", taskPath, ".tiller/workish/task.yml", ":literal[1].yml"}, files)
	}
	data, _, err = g.ReadFileAt(head, ":literal[1].yml")
	require.NoError(t, err)
	require.Equal(t, []byte("literal\n"), data)
	require.Equal(t, beforeRefs, run(t, dir, "show-ref"))
	require.Equal(t, beforeIndex, run(t, dir, "ls-files", "--stage"))
	require.Equal(t, "changed today\n", string(mustReadHistoricalFixture(t, filepath.Join(dir, project, taskPath))))
}

func mustReadHistoricalFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestHistoricalFilesRefuseUnsafeEntriesAndPaths(t *testing.T) {
	for _, mode := range []string{"symlink", "gitlink"} {
		t.Run(mode, func(t *testing.T) {
			dir, g := newRepo(t)
			const entry = ".tiller/work/task.yml"
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".tiller/work"), 0o755))
			if mode == "symlink" {
				require.NoError(t, os.Symlink("../../README.md", filepath.Join(dir, entry)))
				run(t, dir, "add", entry)
			} else {
				head := run(t, dir, "rev-parse", "HEAD")
				run(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+head+","+entry)
			}
			run(t, dir, "commit", "-m", mode)
			head := run(t, dir, "rev-parse", "HEAD")
			_, _, err := g.ReadFileAt(head, entry)
			var nv *NotVersionedError
			require.ErrorAs(t, err, &nv)
			_, err = g.FilesAt(head, ".tiller/work")
			require.ErrorAs(t, err, &nv)
		})
	}
	dir, g := newRepo(t)
	head := run(t, dir, "rev-parse", "HEAD")
	for _, bad := range []string{"../README.md", "/README.md", "app/../README.md", "./README.md", "app//main.go", "README.md\x00", "README.md\n", "app\\main.go", "missing", "README.md/child"} {
		_, _, err := g.ReadFileAt(head, bad)
		var nv *NotVersionedError
		require.ErrorAs(t, err, &nv, "path %q", bad)
		_, err = g.FilesAt(head, bad)
		require.ErrorAs(t, err, &nv, "directory %q", bad)
	}
	_, _, err := g.ReadFileAt(head, "app")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	_, err = g.FilesAt(head, "README.md")
	require.ErrorAs(t, err, &nv)
	files, err := g.FilesAt(head, ".")
	require.NoError(t, err)
	require.Equal(t, []string{"README.md", "app/main.go"}, files)
}

func TestHistoricalReadBlobByObjectID(t *testing.T) {
	dir, g := newRepo(t)
	contents := "---\nfeature: F-old\n---\nverified contract\n\x00tail \n"
	require.NoError(t, writeRepoFile(dir, "feature.eval.md", contents))
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-m", "verified contract")
	blob := run(t, dir, "hash-object", "feature.eval.md")
	require.NoError(t, writeRepoFile(dir, "feature.eval.md", "changed today\n"))

	for _, input := range []string{blob, "git:" + blob, blob[:8], strings.ToUpper(blob)} {
		data, err := g.ReadBlob(input)
		require.NoError(t, err, input)
		require.Equal(t, []byte(contents), data, input)
	}

	var nv *NotVersionedError
	_, err := g.ReadBlob(strings.Repeat("0", 40))
	require.ErrorAs(t, err, &nv, "a well-formed absent id is not versioned")

	for _, bad := range []string{"", "git:", "HEAD", "HEAD:feature.eval.md", "main", blob + "^", "xyz1", blob[:3]} {
		_, err := g.ReadBlob(bad)
		require.ErrorAs(t, err, &nv, "input %q must not read as a blob", bad)
	}

	tree := run(t, dir, "rev-parse", "HEAD^{tree}")
	commit := run(t, dir, "rev-parse", "HEAD")
	for _, oid := range []string{tree, commit} {
		data, err := g.ReadBlob(oid)
		require.ErrorAs(t, err, &nv, "non-blob object %s must refuse", oid)
		require.Nil(t, data)
	}

	require.NoError(t, writeRepoFile(dir, "large.md", strings.Repeat("x", 4*1024*1024+1)))
	data, err := g.ReadBlob(run(t, dir, "hash-object", "-w", "large.md"))
	require.ErrorAs(t, err, &nv)
	require.Nil(t, data)
}

func TestHistoricalReadIsBounded(t *testing.T) {
	dir, g := newRepo(t)
	require.NoError(t, writeRepoFile(dir, "large.yml", strings.Repeat("x", 4*1024*1024+1)))
	run(t, dir, "add", "large.yml")
	run(t, dir, "commit", "-m", "large state")
	head := run(t, dir, "rev-parse", "HEAD")
	data, ref, err := g.ReadFileAt(head, "large.yml")
	var nv *NotVersionedError
	require.ErrorAs(t, err, &nv)
	require.Nil(t, data)
	require.Empty(t, ref.Value)
}

func TestHistoricalGitUnavailableRetainsEnvironmentClass(t *testing.T) {
	g := New(t.TempDir())
	_, err := g.ResolveCommit(strings.Repeat("a", 40))
	var env *store.EnvError
	require.ErrorAs(t, err, &env)
	require.Equal(t, ReasonGitUnavailable, env.Reason)
}

func TestIsHexObjectID(t *testing.T) {
	for _, valid := range []string{"abcd", "ABCDEF12", strings.Repeat("a", 40), strings.Repeat("f", 64)} {
		require.True(t, IsHexObjectID(valid), valid)
	}
	for _, invalid := range []string{"", "abc", strings.Repeat("a", 65), "main", "HEAD^", "abcd~1", " abcd", "g123"} {
		require.False(t, IsHexObjectID(invalid), invalid)
	}
}

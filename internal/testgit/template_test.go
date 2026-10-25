package testgit_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

// recorder stands in for a test's t. It hands everything else to the real t
// but keeps the failures to itself, and a fatal one ends only the goroutine it
// was called on, so a test can watch a Copy fail without failing.
type recorder struct {
	testing.TB
	mu       sync.Mutex
	failures []string
}

func (r *recorder) record(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = append(r.failures, msg)
}

func (r *recorder) Fail()                             { r.record("Fail") }
func (r *recorder) FailNow()                          { r.record("FailNow"); runtime.Goexit() }
func (r *recorder) Error(args ...any)                 { r.record(fmt.Sprint(args...)) }
func (r *recorder) Errorf(format string, args ...any) { r.record(fmt.Sprintf(format, args...)) }
func (r *recorder) Fatal(args ...any)                 { r.Error(args...); runtime.Goexit() }
func (r *recorder) Fatalf(format string, args ...any) { r.Errorf(format, args...); runtime.Goexit() }
func (r *recorder) SkipNow()                          { r.record("SkipNow"); runtime.Goexit() }
func (r *recorder) Skip(args ...any)                  { r.record(fmt.Sprint(args...)); runtime.Goexit() }
func (r *recorder) Skipf(format string, args ...any) {
	r.record(fmt.Sprintf(format, args...))
	runtime.Goexit()
}

// copyOnOwnGoroutine runs copy for r on a goroutine of its own and returns the
// directory it produced, or "" when it stopped on a fatal failure.
func copyOnOwnGoroutine(copy func(testing.TB) string, r *recorder) string {
	var dir string
	done := make(chan struct{})
	go func() {
		defer close(done)
		dir = copy(r)
	}()
	<-done
	return dir
}

// layCommittedFile lays a one-commit repo holding a.txt, counting its lays.
func layCommittedFile(lays *atomic.Int32) func(testing.TB, string) {
	return func(t testing.TB, dir string) {
		lays.Add(1)
		testgit.Init(t, dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
		testgit.Run(t, dir, "add", "-A")
		testgit.Run(t, dir, "commit", "-q", "-m", "lay")
	}
}

func TestTemplateCopiesAreIndependent(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "independent"}
	first := tpl.Copy(t, layCommittedFile(&lays))
	second := tpl.Copy(t, layCommittedFile(&lays))
	require.NotEqual(t, first, second)
	head := testgit.Run(t, second, "rev-parse", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(first, "a.txt"), []byte("changed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(first, "b.txt"), []byte("b\n"), 0o644))
	testgit.Run(t, first, "add", "-A")
	testgit.Run(t, first, "commit", "-q", "-m", "write in the first copy")

	data, err := os.ReadFile(filepath.Join(second, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "a\n", string(data))
	require.NoFileExists(t, filepath.Join(second, "b.txt"))
	require.Equal(t, head, testgit.Run(t, second, "rev-parse", "HEAD"))
	require.Empty(t, testgit.Run(t, second, "status", "--porcelain"))

	third := tpl.Copy(t, layCommittedFile(&lays))
	require.Equal(t, head, testgit.Run(t, third, "rev-parse", "HEAD"), "a later copy comes from the snapshot, not from a copy a test wrote in")
	require.Equal(t, int32(1), lays.Load())
}

func TestTemplateFailureIsReportedToEveryCaller(t *testing.T) {
	t.Parallel()
	const reported = "template broken failed to lay; see the first failure in this package"
	failingLays := map[string]func(testing.TB, string){
		"fatal": func(t testing.TB, _ string) { t.Fatal("the lay broke") },
		"error": func(t testing.TB, _ string) { t.Error("the lay broke") },
		"skip":  func(t testing.TB, _ string) { t.Skip("the lay skipped") },
		"embedded path": func(t testing.TB, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "root"), []byte(dir), 0o644))
		},
		"symlinked file": func(t testing.TB, dir string) {
			require.NoError(t, os.Symlink("elsewhere", filepath.Join(dir, "link")))
		},
	}
	for name, failing := range failingLays {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var lays atomic.Int32
			laying, release := make(chan struct{}), make(chan struct{})
			lay := func(lt testing.TB, dir string) {
				lays.Add(1)
				close(laying)
				<-release
				failing(lt, dir)
			}
			tpl := &testgit.Template{Name: "broken"}
			copy := func(r testing.TB) string { return tpl.Copy(r, lay) }

			// The first caller lays; the others arrive while it is laying and
			// must neither hang nor be handed an empty repo.
			first := &recorder{TB: t}
			firstDir := make(chan string)
			go func() { firstDir <- copyOnOwnGoroutine(copy, first) }()
			<-laying
			waiters := make([]*recorder, 6)
			waiterDirs := make([]string, len(waiters))
			var wg sync.WaitGroup
			for i := range waiters {
				waiters[i] = &recorder{TB: t}
				wg.Add(1)
				go func() {
					defer wg.Done()
					waiterDirs[i] = copyOnOwnGoroutine(copy, waiters[i])
				}()
			}
			close(release)

			require.Empty(t, <-firstDir, "the laying caller gets no repo")
			require.NotEmpty(t, first.failures, "the laying caller reports the lay's own failure")
			wg.Wait()
			late := &recorder{TB: t}
			require.Empty(t, copyOnOwnGoroutine(copy, late))
			for _, r := range append(waiters, late) {
				require.Equal(t, []string{reported}, r.failures)
			}
			for _, dir := range waiterDirs {
				require.Empty(t, dir)
			}
			require.Equal(t, int32(1), lays.Load(), "a failed template is never laid again")
		})
	}
}

func TestTemplateCopyHasCleanStatus(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "clean"}
	dir := tpl.Copy(t, layCommittedFile(&lays))
	require.Empty(t, testgit.Run(t, dir, "status", "--porcelain"))
}

// TestTemplateRefusesTheResolvedLayPath covers a temp directory behind a
// symlink, such as macOS's /var: git reports a repo by its resolved path, so a
// lay can embed that form, and a copy would point back at a directory that no
// longer exists. The alias here shares no suffix with its target, so only a
// check of the resolved path catches it.
//
// Serial: t.Setenv points TMPDIR at the symlink.
func TestTemplateRefusesTheResolvedLayPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.Mkdir(target, 0o755))
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(target, alias))
	t.Setenv("TMPDIR", alias)

	tpl := &testgit.Template{Name: "resolved"}
	r := &recorder{TB: t}
	dir := copyOnOwnGoroutine(func(r testing.TB) string {
		return tpl.Copy(r, func(lt testing.TB, dir string) {
			resolved, err := filepath.EvalSymlinks(dir)
			require.NoError(lt, err)
			require.NotEqual(lt, dir, resolved)
			require.NoError(lt, os.WriteFile(filepath.Join(dir, "root"), []byte(resolved), 0o644))
		})
	}, r)
	require.Empty(t, dir)
	require.Len(t, r.failures, 1)
	require.Contains(t, r.failures[0], "names the directory the template was laid in")
}

// TestTemplateCopyKeepsTheLaidStatus covers a lay that leaves uncommitted
// work: copying drops the stat data in .git/index, and git must still see the
// same modified and untracked files, no more and no fewer.
func TestTemplateCopyKeepsTheLaidStatus(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "dirty"}
	var laidStatus string
	dir := tpl.Copy(t, func(lt testing.TB, dir string) {
		layCommittedFile(&lays)(lt, dir)
		require.NoError(lt, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited\n"), 0o644))
		require.NoError(lt, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644))
		laidStatus = testgit.Run(lt, dir, "status", "--porcelain")
	})
	require.Equal(t, "M a.txt\n?? new.txt", laidStatus)
	require.Equal(t, laidStatus, testgit.Run(t, dir, "status", "--porcelain"))
}

func TestTemplatesKeyedSeparately(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	layKey := func(key string) func(testing.TB, string) {
		return func(lt testing.TB, dir string) {
			lays.Add(1)
			testgit.Init(lt, dir)
			require.NoError(lt, os.WriteFile(filepath.Join(dir, "key.txt"), []byte(key), 0o644))
		}
	}
	var ts testgit.Templates
	ts.Name = "keyed"
	alpha := ts.Copy(t, "alpha", layKey("alpha"))
	beta := ts.Copy(t, "beta", layKey("beta"))
	alphaAgain := ts.Copy(t, "alpha", layKey("alpha"))

	for dir, want := range map[string]string{alpha: "alpha", beta: "beta", alphaAgain: "alpha"} {
		data, err := os.ReadFile(filepath.Join(dir, "key.txt"))
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}
	require.Equal(t, int32(2), lays.Load(), "two keys lay twice; the same key lays once")
}

func TestTemplateLaysOnceForConcurrentCallers(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "concurrent"}
	copy := func(r testing.TB) string { return tpl.Copy(r, layCommittedFile(&lays)) }
	callers := make([]*recorder, 8)
	dirs := make([]string, len(callers))
	var wg sync.WaitGroup
	for i := range callers {
		callers[i] = &recorder{TB: t}
		wg.Add(1)
		go func() {
			defer wg.Done()
			dirs[i] = copyOnOwnGoroutine(copy, callers[i])
		}()
	}
	wg.Wait()

	seen := map[string]bool{}
	for i, dir := range dirs {
		require.Empty(t, callers[i].failures)
		require.FileExists(t, filepath.Join(dir, "a.txt"))
		require.False(t, seen[dir], "two callers were handed the same copy")
		seen[dir] = true
	}
	require.Equal(t, int32(1), lays.Load())
}

// TestTemplateLaysOncePerGitEnvironment covers a test that changes git's
// environment, such as GIT_DEFAULT_HASH, before asking for a fixture: it must
// get a repo laid under that environment, and must not hand its repo to
// callers that did not change it.
//
// Serial: t.Setenv changes the process environment.
func TestTemplateLaysOncePerGitEnvironment(t *testing.T) {
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "environment"}
	sha1 := tpl.Copy(t, layCommittedFile(&lays))
	require.Equal(t, "sha1", testgit.Run(t, sha1, "rev-parse", "--show-object-format"))

	t.Setenv("GIT_DEFAULT_HASH", "sha256")
	sha256 := tpl.Copy(t, layCommittedFile(&lays))
	require.Equal(t, "sha256", testgit.Run(t, sha256, "rev-parse", "--show-object-format"))
	tpl.Copy(t, layCommittedFile(&lays))
	require.Equal(t, int32(2), lays.Load(), "one lay per git environment")
}

// TestTemplateResultIsKeptPerGitEnvironment covers a lay that returns values
// alongside its repo, such as the ids it minted: every caller gets the value
// its own copy was laid with, so a caller under one git environment never
// reads the value of a lay under another.
//
// Serial: t.Setenv changes the process environment.
func TestTemplateResultIsKeptPerGitEnvironment(t *testing.T) {
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "result"}
	layFormat := func(lt testing.TB, dir string) string {
		layCommittedFile(&lays)(lt, dir)
		return testgit.Run(lt, dir, "rev-parse", "--show-object-format")
	}
	_, format := testgit.CopyWithResult(t, tpl, layFormat)
	require.Equal(t, "sha1", format)

	t.Run("sha256", func(t *testing.T) {
		t.Setenv("GIT_DEFAULT_HASH", "sha256")
		_, format := testgit.CopyWithResult(t, tpl, layFormat)
		require.Equal(t, "sha256", format)
	})

	dir, format := testgit.CopyWithResult(t, tpl, layFormat)
	require.Equal(t, "sha1", format, "the sha256 lay's result never reaches a sha1 caller")
	require.Equal(t, "sha1", testgit.Run(t, dir, "rev-parse", "--show-object-format"))
	require.Equal(t, int32(2), lays.Load())
}

// TestCopyWithResultFailsOnAResultTypeMismatch covers a template laid with one
// result type and read with another: the caller fails loudly instead of
// getting a zero value that looks like a real result.
func TestCopyWithResultFailsOnAResultTypeMismatch(t *testing.T) {
	t.Parallel()
	var lays atomic.Int32
	tpl := &testgit.Template{Name: "mismatch"}
	_, name := testgit.CopyWithResult(t, tpl, func(lt testing.TB, dir string) string {
		layCommittedFile(&lays)(lt, dir)
		return "lantern"
	})
	require.Equal(t, "lantern", name)

	r := &recorder{TB: t}
	copyOnOwnGoroutine(func(rt testing.TB) string {
		dir, _ := testgit.CopyWithResult(rt, tpl, func(lt testing.TB, dir string) int {
			layCommittedFile(&lays)(lt, dir)
			return 7
		})
		return dir
	}, r)
	require.Len(t, r.failures, 1)
	require.Contains(t, r.failures[0], "template mismatch")
	require.Contains(t, r.failures[0], "string")
	require.Contains(t, r.failures[0], "int")
}

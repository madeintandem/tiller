package testgit

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

// Template lays a fixture repo once and hands every caller its own copy. The
// first Copy runs lay into a temporary directory, snapshots the result in
// memory (working tree and .git), and removes the directory; every Copy then
// writes the snapshot into a fresh t.TempDir. Copy is safe for concurrent
// callers: the others wait while the first one lays.
//
// A copy's files carry new modification times, so the stat data in its
// .git/index is stale and git re-hashes the files on its first look. Status
// and content are unchanged, and the first git status may rewrite the index,
// as it also does on a fresh lay within git's one-second racy window.
//
// A lay that fails, whether it stops the first caller or only reports an
// error, marks the template failed, and every later Copy fails rather than
// hand out an empty or half-laid repo. A failed template is never laid again.
//
// A template is laid once per git environment (the GIT_* variables), so a
// test that sets GIT_DEFAULT_HASH gets a repo laid under it and never hands
// that repo to callers that did not set it.
//
// The lay must depend only on its inputs, never on the calling test: its
// output is every caller's fixture. It runs on the first caller's t, so it
// must not call t.Setenv or t.Chdir. A lay that calls Skip marks the template
// failed like any other lay that stops early, so every later caller fails.
type Template struct {
	// Name labels the template in the failure every later caller reports.
	Name string

	mu   sync.Mutex
	laid map[string]*laidSnapshot // keyed by git environment
}

// laidSnapshot is one lay of a template; files stays nil unless it succeeded.
// result is what the lay returned, kept with the files it describes.
type laidSnapshot struct {
	once   sync.Once
	files  fstest.MapFS
	result any
}

// Copy returns a fresh t.TempDir holding the template's snapshot, laying it
// with lay on the first call.
func (tpl *Template) Copy(t testing.TB, lay func(t testing.TB, dir string)) string {
	t.Helper()
	dir, _ := tpl.copy(t, func(t testing.TB, dir string) any {
		lay(t, dir)
		return nil
	})
	return dir
}

// CopyWithResult is Template.Copy for a lay that also returns a value, such as
// the ids it minted. The value is kept with the snapshot it was laid with, so
// every caller gets the value that describes its own copy, even when another
// git environment laid the template differently. A template is used with one
// lay, so one result type.
func CopyWithResult[R any](t testing.TB, tpl *Template, lay func(t testing.TB, dir string) R) (string, R) {
	t.Helper()
	dir, result := tpl.copy(t, func(t testing.TB, dir string) any { return lay(t, dir) })
	typed, ok := result.(R)
	if !ok && result != nil {
		var want R
		t.Fatalf("template %s: lay result is %T, not %T", tpl.Name, result, want)
	}
	return dir, typed
}

func (tpl *Template) copy(t testing.TB, lay func(t testing.TB, dir string) any) (string, any) {
	t.Helper()
	snap := tpl.snapshotFor(gitEnvironment())
	snap.once.Do(func() { snap.files, snap.result = layAndSnapshot(t, lay) })
	if snap.files == nil {
		t.Fatalf("template %s failed to lay; see the first failure in this package", tpl.Name)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, snap.files); err != nil {
		t.Fatalf("template %s: copy: %v", tpl.Name, err)
	}
	return dir, snap.result
}

func (tpl *Template) snapshotFor(env string) *laidSnapshot {
	tpl.mu.Lock()
	defer tpl.mu.Unlock()
	if tpl.laid == nil {
		tpl.laid = map[string]*laidSnapshot{}
	}
	snap, ok := tpl.laid[env]
	if !ok {
		snap = &laidSnapshot{}
		tpl.laid[env] = snap
	}
	return snap
}

// Templates is a set of templates keyed by the input their lay depends on,
// for a fixture helper that takes an argument, such as a manifest's text.
type Templates struct {
	// Name labels the set; each template is named after it and its key.
	Name string

	mu    sync.Mutex
	byKey map[string]*Template
}

// Copy is Template.Copy for the template under key.
func (ts *Templates) Copy(t testing.TB, key string, lay func(t testing.TB, dir string)) string {
	t.Helper()
	return ts.template(key).Copy(t, lay)
}

func (ts *Templates) template(key string) *Template {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.byKey == nil {
		ts.byKey = map[string]*Template{}
	}
	tpl, ok := ts.byKey[key]
	if !ok {
		tpl = &Template{Name: fmt.Sprintf("%s %q", ts.Name, key)}
		ts.byKey[key] = tpl
	}
	return tpl
}

// gitEnvironment is the caller's GIT_* variables, sorted, as one string.
func gitEnvironment() string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	sort.Strings(env)
	return strings.Join(env, "\n")
}

// layAndSnapshot runs lay into a directory of its own and returns the
// snapshot and the lay's result, or nil files when the lay reported a
// failure. A lay that stops the caller (Fatal, FailNow, Skip) never returns
// here at all.
func layAndSnapshot(t testing.TB, lay func(testing.TB, string) any) (fstest.MapFS, any) {
	t.Helper()
	dir, err := os.MkdirTemp("", "testgit-template-")
	if err != nil {
		t.Fatalf("testgit: template directory: %v", err)
	}
	defer os.RemoveAll(dir)
	watch := &failureWatch{TB: t}
	result := lay(watch, dir)
	if watch.failed.Load() {
		return nil, nil
	}
	files, err := snapshot(dir)
	if err != nil {
		t.Fatalf("testgit: snapshot the laid template: %v", err)
	}
	return files, result
}

// failureWatch passes every call through to the first caller's t and notes
// whether the lay reported a failure, so a lay that calls Error and returns
// still marks the template failed.
type failureWatch struct {
	testing.TB
	failed atomic.Bool
}

func (w *failureWatch) Fail()    { w.TB.Helper(); w.failed.Store(true); w.TB.Fail() }
func (w *failureWatch) FailNow() { w.TB.Helper(); w.failed.Store(true); w.TB.FailNow() }
func (w *failureWatch) Error(args ...any) {
	w.TB.Helper()
	w.failed.Store(true)
	w.TB.Error(args...)
}
func (w *failureWatch) Errorf(format string, args ...any) {
	w.TB.Helper()
	w.failed.Store(true)
	w.TB.Errorf(format, args...)
}
func (w *failureWatch) Fatal(args ...any) {
	w.TB.Helper()
	w.failed.Store(true)
	w.TB.Fatal(args...)
}
func (w *failureWatch) Fatalf(format string, args ...any) {
	w.TB.Helper()
	w.failed.Store(true)
	w.TB.Fatalf(format, args...)
}

// snapshot reads every directory and regular file under root into memory. A
// lock file under .git is another process's transient state, never part of
// the fixture, so it is left out. A file that names root is refused: a copy
// would point back at a directory that no longer exists. git reports a
// directory by its resolved path, so root's resolved form is refused too.
func snapshot(root string) (fstest.MapFS, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	files := fstest.MapFS{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			files[rel] = &fstest.MapFile{Mode: fs.ModeDir | info.Mode().Perm()}
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", rel)
		case strings.HasPrefix(rel, ".git/") && strings.HasSuffix(rel, ".lock"):
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(root)) || bytes.Contains(data, []byte(resolved)) {
			return fmt.Errorf("%s names the directory the template was laid in", rel)
		}
		files[rel] = &fstest.MapFile{Data: data, Mode: info.Mode().Perm()}
		return nil
	})
	return files, err
}

// Package testgit is test-support only: it imports testing and must never be
// imported by non-test code. It holds the git setup the repo-making fixtures
// share, so each test repo costs one git spawn instead of six.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// config is the identity and maintenance settings every fixture repo carries.
// Commit and merge start detached background maintenance, which can repack
// mid-test and change the object counts a no-write assertion compares, and
// whose transient .git/objects/maintenance.lock races the tests that snapshot
// a tree to prove a refusal wrote nothing.
const config = `[user]
	email = test@example.com
	name = Tiller Test
[commit]
	gpgsign = false
[gc]
	auto = 0
[maintenance]
	auto = false
`

// Init runs `git init -q -b main` in dir, then appends the identity and
// maintenance settings to dir/.git/config directly rather than spawning git
// once per key. The branch is always main, never the machine's default.
func Init(t testing.TB, dir string) {
	t.Helper()
	Run(t, dir, "init", "-q", "-b", "main")
	f, err := os.OpenFile(filepath.Join(dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("testgit: open .git/config: %v", err)
	}
	if _, err := f.WriteString(config); err != nil {
		f.Close()
		t.Fatalf("testgit: write .git/config: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("testgit: close .git/config: %v", err)
	}
}

// Run runs one git command in dir and fails the test with its combined output
// on error. It returns the output with surrounding whitespace trimmed.
func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

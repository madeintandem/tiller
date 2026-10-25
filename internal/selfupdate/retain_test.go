package selfupdate_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/stretchr/testify/require"
)

func writeVersionExecutable(t *testing.T, path, version, extra string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	body := "#!/bin/sh\n" + extra + "\nif [ \"$1\" = '--child-version' ]; then exec tiller --version; fi\nprintf '%s\\n' 'tiller version " + version + "'\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0755))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func TestRetainExecutableKeepsDistinctBuildsAndSelectsChildPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "installation with spaces", "tiller")
	writeVersionExecutable(t, path, "0.12.1", "# original build")
	original, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	first, err := selfupdate.RetainExecutable(context.Background(), original)
	require.NoError(t, err)
	reused, err := selfupdate.RetainExecutable(context.Background(), original)
	require.NoError(t, err)
	require.Equal(t, first, reused)
	require.Equal(t, first.Version+"-"+first.SHA256, filepath.Base(first.Directory))
	info, err := os.Stat(first.Path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	for directory := first.Directory; filepath.Base(directory) != "share"; directory = filepath.Dir(directory) {
		info, err := os.Stat(directory)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	}

	// Two builds of the same stamped version must not rotate away the first.
	next := path + ".next"
	writeVersionExecutable(t, next, "0.12.1", "# different build")
	require.NoError(t, os.Rename(next, path))
	secondIdentity, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	second, err := selfupdate.RetainExecutable(context.Background(), secondIdentity)
	require.NoError(t, err)
	require.NotEqual(t, first.Path, second.Path)
	firstIdentity, err := selfupdate.ReadExecutable(context.Background(), first.Path)
	require.NoError(t, err)
	require.Equal(t, original.SHA256, firstIdentity.SHA256)

	writeVersionExecutable(t, next, "0.13.0", "# newer installation")
	require.NoError(t, os.Rename(next, path))
	child := exec.Command(first.Path, "--child-version")
	child.Env = append(os.Environ(), "PATH="+first.Directory+string(os.PathListSeparator)+filepath.Dir(path))
	out, err := child.Output()
	require.NoError(t, err)
	require.Equal(t, "tiller version 0.12.1\n", string(out))
}

func TestRetainExecutableRefusesConflictingFileWithoutExecutingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "tiller")
	writeVersionExecutable(t, path, "0.12.1", "")
	identity, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	dir := filepath.Join(home, ".local", "share", "tiller", "previous", identity.Version+"-"+identity.SHA256)
	require.NoError(t, os.MkdirAll(dir, 0700))
	marker := filepath.Join(home, "conflict-executed")
	conflict := []byte("#!/bin/sh\ntouch " + shellQuote(marker) + "\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tiller"), conflict, 0700))
	retained, err := selfupdate.RetainExecutable(context.Background(), identity)
	require.ErrorContains(t, err, "conflicting")
	require.Empty(t, retained.Path)
	actual, err := os.ReadFile(filepath.Join(dir, "tiller"))
	require.NoError(t, err)
	require.Equal(t, conflict, actual)
	require.NoFileExists(t, marker)
	require.NoError(t, selfupdate.CheckExecutable(identity))
}

func TestRetainExecutableRejectsSymlinksAtEveryStorageBoundary(t *testing.T) {
	for index := range 6 {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(t.TempDir(), "tiller")
			writeVersionExecutable(t, path, "0.12.1", "")
			identity, err := selfupdate.ReadExecutable(context.Background(), path)
			require.NoError(t, err)
			parts := []string{".local", "share", "tiller", "previous", identity.Version + "-" + identity.SHA256, "tiller"}
			parent := filepath.Join(append([]string{home}, parts[:index]...)...)
			require.NoError(t, os.MkdirAll(parent, 0700))
			outside := t.TempDir()
			target := outside
			if index == 5 {
				target = filepath.Join(outside, "tiller")
				writeVersionExecutable(t, target, "0.12.1", "")
			}
			require.NoError(t, os.Symlink(target, filepath.Join(parent, parts[index])))
			_, err = selfupdate.RetainExecutable(context.Background(), identity)
			require.Error(t, err)
			entries, err := os.ReadDir(outside)
			require.NoError(t, err)
			if index == 5 {
				require.Len(t, entries, 1)
			} else {
				require.Empty(t, entries)
			}
			require.NoError(t, selfupdate.CheckExecutable(identity))
		})
	}
}

func TestExecutableIdentityDetectsSameBytesReplacementAndAliasChanges(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "tiller")
	writeVersionExecutable(t, path, "0.12.1", "")
	link := filepath.Join(directory, "selected")
	require.NoError(t, os.Symlink(path, link))
	identity, err := selfupdate.ReadExecutable(context.Background(), link)
	require.NoError(t, err)
	next := filepath.Join(directory, "next")
	writeVersionExecutable(t, next, "0.12.1", "")
	require.NoError(t, os.Rename(next, path))
	require.ErrorContains(t, selfupdate.CheckExecutable(identity), "changed")
	identity, err = selfupdate.ReadExecutable(context.Background(), link)
	require.NoError(t, err)
	writeVersionExecutable(t, next, "0.12.2", "")
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(next, link))
	require.ErrorContains(t, selfupdate.CheckExecutable(identity), "changed")
}

func TestExecutableVersionCannotChangeSourceWhileBeingVerified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiller")
	writeVersionExecutable(t, path, "0.12.1", `printf '# changed\n' >> "$0"`)
	_, err := selfupdate.ReadExecutable(context.Background(), path)
	require.ErrorContains(t, err, "changed")
}

func TestRetainExecutableReportsVerifiedCopyWhenSourceChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "tiller")
	// Resolve the source so the script's exact-path comparison also works on
	// macOS, where the temporary directory can have a /var alias.
	directory, err := filepath.EvalSymlinks(filepath.Dir(path))
	require.NoError(t, err)
	path = filepath.Join(directory, "tiller")
	next := path + ".next"
	writeVersionExecutable(t, next, "0.12.2", "")
	extra := "if [ \"$0\" != " + shellQuote(path) + " ] && [ -f " + shellQuote(next) + " ]; then /bin/mv " + shellQuote(next) + " " + shellQuote(path) + "; fi"
	writeVersionExecutable(t, path, "0.12.1", extra)
	identity, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	retained, err := selfupdate.RetainExecutable(context.Background(), identity)
	require.ErrorContains(t, err, "changed")
	require.NotEmpty(t, retained.Path)
	verified, err := selfupdate.ReadExecutable(context.Background(), retained.Path)
	require.NoError(t, err)
	require.Equal(t, identity.SHA256, verified.SHA256)
	require.Equal(t, "0.12.1", verified.Version)
	current, err := selfupdate.ReadExecutable(context.Background(), path)
	require.NoError(t, err)
	require.Equal(t, "0.12.2", current.Version)
}

func TestExecutableVersionCancellationIsBoundedWithInheritedPipes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiller")
	writeVersionExecutable(t, path, "0.12.1", "sleep 60 &\nwait")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := selfupdate.ReadExecutable(ctx, path)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 3*time.Second)
}

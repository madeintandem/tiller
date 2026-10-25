package selfupdate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/selfupdate"
	"github.com/stretchr/testify/require"
)

type homebrewFixture struct {
	prefix string
	path   string
	next   string
	kind   string
}

func makeHomebrewFixture(t *testing.T, kind string) homebrewFixture {
	t.Helper()
	prefix := filepath.Join(t.TempDir(), "owning brew with spaces")
	require.NoError(t, os.MkdirAll(filepath.Join(prefix, "bin"), 0700))
	prefix, err := filepath.EvalSymlinks(prefix)
	require.NoError(t, err)
	room, source := "Caskroom", "tiller"
	if kind == "formula" {
		room, source = "Cellar", "bin/tiller"
	}
	fixture := homebrewFixture{
		prefix: prefix,
		path:   filepath.Join(prefix, room, "tiller", "0.12.1", source),
		next:   filepath.Join(prefix, room, "tiller", "0.12.2", source),
		kind:   kind,
	}
	writeVersionExecutable(t, fixture.path, "0.12.1", "")
	writeVersionExecutable(t, fixture.next, "0.12.2", "")
	require.NoError(t, os.Symlink(fixture.path, filepath.Join(prefix, "bin", "tiller")))
	require.NoError(t, os.WriteFile(filepath.Join(prefix, "version"), []byte("0.12.1\n"), 0600))
	for _, version := range []string{"0.12.1", "0.12.2"} {
		var metadata any
		if kind == "cask" {
			metadata = map[string]any{"casks": []any{map[string]any{
				"token": "tiller", "full_token": "madeintandem/tap/tiller", "tap": "madeintandem/tap", "installed": version,
				"artifacts": []any{map[string]any{"binary": []any{"tiller"}, "target": filepath.Join(prefix, "bin", "tiller")}},
			}}, "formulae": []any{}}
		} else {
			metadata = map[string]any{"formulae": []any{map[string]any{
				"name": "tiller", "full_name": "madeintandem/tap/tiller", "tap": "madeintandem/tap",
				"installed": []any{map[string]any{"version": version}},
			}}, "casks": []any{}}
		}
		data, err := json.Marshal(metadata)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(prefix, version+".json"), data, 0600))
	}
	// Only this controlled executable is ever invoked. In particular, cask
	// list deliberately emits real human-shaped output rather than a fake
	// file-per-line API: ownership must come from JSON artifact metadata.
	script := "#!/bin/sh\nset -eu\nprefix=" + shellQuote(prefix) + "\nroom=" + shellQuote(room) + "\nsource=" + shellQuote(source) + "\nkind=" + shellQuote(kind) + `
printf '%s|deps=%s|cleanup=%s\n' "$*" "${HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK-}" "${HOMEBREW_NO_INSTALL_CLEANUP-}" >> "$prefix/calls"
version=$(cat "$prefix/version")
case "$*" in
  --prefix)
    if [ -f "$prefix/wrong-prefix" ]; then cat "$prefix/wrong-prefix"; else printf '%s\n' "$prefix"; fi ;;
  --caskroom|--cellar) printf '%s/%s\n' "$prefix" "$room" ;;
  "info --json=v2 --$kind madeintandem/tap/tiller")
    if [ -f "$prefix/override.json" ]; then cat "$prefix/override.json"; else cat "$prefix/$version.json"; fi ;;
  "list --cask madeintandem/tap/tiller")
    printf '==> Binary\n%s/bin/tiller -> %s/Caskroom/tiller/%s/tiller (12MB)\n' "$prefix" "$prefix" "$version" ;;
  "list --formula madeintandem/tap/tiller")
    if [ -f "$prefix/wrong-list" ]; then cat "$prefix/wrong-list"; else printf '%s/%s/tiller/%s/%s\n' "$prefix" "$room" "$version" "$source"; fi ;;
  "upgrade --$kind madeintandem/tap/tiller")
    [ "$HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK" = 1 ] && [ "$HOMEBREW_NO_INSTALL_CLEANUP" = 1 ] || exit 92
    if [ -f "$prefix/fail" ]; then printf 'private asset authentication failed\n' >&2; exit 17; fi
    if [ -f "$prefix/wait" ]; then sleep 60 & wait; fi
    if [ -f "$prefix/current" ]; then exit 0; fi
    ln -s "$prefix/$room/tiller/0.12.2/$source" "$prefix/bin/tiller.next"
    mv "$prefix/bin/tiller.next" "$prefix/bin/tiller"
    printf '0.12.2\n' > "$prefix/version"
    rm "$prefix/$room/tiller/0.12.1/$source" ;;
  *) printf 'unexpected brew command: %s\n' "$*" >&2; exit 91 ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(prefix, "bin", "brew"), []byte(script), 0755))
	return fixture
}

func TestHomebrewDelegatesToOwningPrefixAndResolvesMovedExecutable(t *testing.T) {
	for _, kind := range []string{"cask", "formula"} {
		t.Run(kind, func(t *testing.T) {
			fixture := makeHomebrewFixture(t, kind)
			other := t.TempDir()
			marker := filepath.Join(other, "unrelated-brew-used")
			require.NoError(t, os.WriteFile(filepath.Join(other, "brew"), []byte("#!/bin/sh\ntouch "+shellQuote(marker)+"\nexit 99\n"), 0755))
			t.Setenv("PATH", other+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK", "0")
			t.Setenv("HOMEBREW_NO_INSTALL_CLEANUP", "0")
			install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
			require.NoError(t, err)
			require.Equal(t, filepath.Join(fixture.prefix, "bin", "brew"), install.Brew)
			require.Equal(t, kind, install.Kind)
			result, err := selfupdate.UpgradeHomebrew(context.Background(), install)
			require.NoError(t, err)
			require.Equal(t, fixture.next, result)
			require.NoFileExists(t, fixture.path)
			identity, err := selfupdate.ReadExecutable(context.Background(), result)
			require.NoError(t, err)
			require.Equal(t, "0.12.2", identity.Version)
			require.NoFileExists(t, marker)
			calls, err := os.ReadFile(filepath.Join(fixture.prefix, "calls"))
			require.NoError(t, err)
			require.Contains(t, string(calls), "upgrade --"+kind+" madeintandem/tap/tiller|deps=1|cleanup=1\n")
			require.Equal(t, "0", os.Getenv("HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK"))
			require.Equal(t, "0", os.Getenv("HOMEBREW_NO_INSTALL_CLEANUP"))
		})
	}
}

func TestHomebrewAlreadyCurrentReturnsVerifiedInstalledExecutable(t *testing.T) {
	fixture := makeHomebrewFixture(t, "cask")
	require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "current"), nil, 0600))
	install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
	require.NoError(t, err)
	result, err := selfupdate.UpgradeHomebrew(context.Background(), install)
	require.NoError(t, err)
	require.Equal(t, fixture.path, result)
}

func TestHomebrewRejectsUnverifiedPackageOwnership(t *testing.T) {
	for _, scenario := range []string{"uninstalled", "foreign-tap", "wrong-prefix", "unlisted-file", "wrong-cask-artifact", "missing-owner", "outside-owner"} {
		t.Run(scenario, func(t *testing.T) {
			kind := "cask"
			if scenario == "unlisted-file" {
				kind = "formula"
			}
			fixture := makeHomebrewFixture(t, kind)
			switch scenario {
			case "uninstalled":
				require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "override.json"), []byte(`{"casks":[{"token":"tiller","full_token":"madeintandem/tap/tiller","tap":"madeintandem/tap","installed":null}]}`), 0600))
			case "foreign-tap":
				require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "override.json"), []byte(`{"casks":[{"token":"tiller","full_token":"someone/else/tiller","tap":"someone/else","installed":"0.12.1"}]}`), 0600))
			case "wrong-prefix":
				require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "wrong-prefix"), []byte(t.TempDir()), 0600))
			case "unlisted-file":
				require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "wrong-list"), []byte(fixture.path+".other\n"), 0600))
			case "wrong-cask-artifact":
				require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "override.json"), []byte(`{"casks":[{"token":"tiller","full_token":"madeintandem/tap/tiller","tap":"madeintandem/tap","installed":"0.12.1","artifacts":[{"binary":["not-tiller"]}]}]}`), 0600))
			case "missing-owner":
				require.NoError(t, os.Remove(filepath.Join(fixture.prefix, "bin", "brew")))
			case "outside-owner":
				outside := filepath.Join(t.TempDir(), "brew")
				require.NoError(t, os.Rename(filepath.Join(fixture.prefix, "bin", "brew"), outside))
				require.NoError(t, os.Symlink(outside, filepath.Join(fixture.prefix, "bin", "brew")))
			}
			install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
			require.Error(t, err)
			require.Nil(t, install)
			calls, err := os.ReadFile(filepath.Join(fixture.prefix, "calls"))
			if err == nil {
				require.NotContains(t, string(calls), "upgrade ")
			}
			identity, err := selfupdate.ReadExecutable(context.Background(), fixture.path)
			require.NoError(t, err)
			require.Equal(t, "0.12.1", identity.Version)
		})
	}
}

func TestHomebrewFailureNeverFallsBackToDirectReplacement(t *testing.T) {
	fixture := makeHomebrewFixture(t, "cask")
	require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "fail"), nil, 0600))
	before, err := selfupdate.ReadExecutable(context.Background(), fixture.path)
	require.NoError(t, err)
	install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
	require.NoError(t, err)
	path, err := selfupdate.UpgradeHomebrew(context.Background(), install)
	require.ErrorContains(t, err, "private asset authentication failed")
	require.ErrorContains(t, err, "state may have changed")
	require.Empty(t, path)
	require.NoError(t, selfupdate.CheckExecutable(before))
	link, err := filepath.EvalSymlinks(filepath.Join(fixture.prefix, "bin", "tiller"))
	require.NoError(t, err)
	require.Equal(t, fixture.path, link)
}

func TestHomebrewRejectsChangedOwnerBeforeDelegation(t *testing.T) {
	fixture := makeHomebrewFixture(t, "formula")
	install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
	require.NoError(t, err)
	brew := filepath.Join(fixture.prefix, "bin", "brew")
	data, err := os.ReadFile(brew)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(brew+".next", data, 0755))
	require.NoError(t, os.Rename(brew+".next", brew))
	_, err = selfupdate.UpgradeHomebrew(context.Background(), install)
	require.ErrorContains(t, err, "changed")
	calls, err := os.ReadFile(filepath.Join(fixture.prefix, "calls"))
	require.NoError(t, err)
	require.NotContains(t, string(calls), "upgrade ")
}

func TestHomebrewCancellationBoundsDelegatedSubprocesses(t *testing.T) {
	fixture := makeHomebrewFixture(t, "cask")
	require.NoError(t, os.WriteFile(filepath.Join(fixture.prefix, "wait"), nil, 0600))
	install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = selfupdate.UpgradeHomebrew(ctx, install)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 3*time.Second)
}

func TestHomebrewLookingPathNeverBecomesSelfManaged(t *testing.T) {
	for _, room := range []string{"Cellar", "Caskroom"} {
		t.Run(room, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), room, "tiller", "0.12.1", "tiller")
			writeVersionExecutable(t, path, "0.12.1", "")
			owner, err := selfupdate.DetectHomebrew(context.Background(), path)
			require.Error(t, err)
			require.Nil(t, owner)
		})
	}
	path := filepath.Join(t.TempDir(), "tiller")
	writeVersionExecutable(t, path, "0.12.1", "")
	owner, err := selfupdate.DetectHomebrew(context.Background(), path)
	require.NoError(t, err)
	require.Nil(t, owner)
}

func TestHomebrewRejectsUnusableResultAfterSuccessfulDelegation(t *testing.T) {
	fixture := makeHomebrewFixture(t, "cask")
	require.NoError(t, os.WriteFile(fixture.next, []byte("#!/bin/sh\nprintf 'not tiller\\n'\n"), 0755))
	install, err := selfupdate.DetectHomebrew(context.Background(), fixture.path)
	require.NoError(t, err)
	path, err := selfupdate.UpgradeHomebrew(context.Background(), install)
	require.ErrorContains(t, err, "verifying executable after Homebrew upgrade")
	require.Empty(t, path)
	calls, err := os.ReadFile(filepath.Join(fixture.prefix, "calls"))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(calls), fmt.Sprintf("upgrade --%s madeintandem/tap/tiller|", fixture.kind)))
	require.NoFileExists(t, fixture.path, "Homebrew's completed mutation must not be rolled back")
}

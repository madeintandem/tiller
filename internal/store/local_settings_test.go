package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeLocalSettings(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "local.yml"), []byte(body), 0o644))
}

func TestReadLocalSettingsAbsentFileMeansDefaults(t *testing.T) {
	root := t.TempDir()
	settings, err := ReadLocalSettings(root)
	require.NoError(t, err, "a checkout without local.yml is the common case, not an error")
	require.Nil(t, settings.Console.Open, "nothing is set")
}

func TestReadLocalSettingsReadsConsoleOpen(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       bool
	}{
		{"false", "console:\n  open: false\n", false},
		{"true", "console:\n  open: true\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeLocalSettings(t, root, tt.body)
			settings, err := ReadLocalSettings(root)
			require.NoError(t, err)
			require.NotNil(t, settings.Console.Open)
			require.Equal(t, tt.want, *settings.Console.Open)
		})
	}
}

// Keys this binary does not know are a newer binary's or a human's note, not a
// reason to refuse the whole file; an empty console block keeps the defaults.
func TestReadLocalSettingsToleratesUnknownAndEmptyKeys(t *testing.T) {
	root := t.TempDir()
	writeLocalSettings(t, root, "# my preferences\nconsole:\n  theme: dark\neditor: vim\n")
	settings, err := ReadLocalSettings(root)
	require.NoError(t, err)
	require.Nil(t, settings.Console.Open)
}

// A present but unparseable file is a real error the caller surfaces, the same
// posture ReadLocalIdentity takes: silently reading defaults would hide the typo
// that made the preference stop applying.
func TestReadLocalSettingsMalformedFileIsAnError(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"not yaml", "console: [\n"},
		{"wrong shape", "console: yes\n"},
		{"open not a bool", "console:\n  open: sometimes\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeLocalSettings(t, root, tt.body)
			_, err := ReadLocalSettings(root)
			require.Error(t, err)
			require.Contains(t, err.Error(), "local.yml")
		})
	}
}

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const validBaseline = `docs:
  - path: modules/api.md
    describes: [app]
    derived_from:
      - kind: git_blob
        value: git:1234567
        path: docs/old.md
        observed_at: 2026-08-28T12:00:00Z
    confirmed:
      by: retired-owner
      commit:
        kind: git_commit
        value: git:abcdef0
        observed_at: 2026-08-28T12:00:00Z
      surfaces:
        - path: app
          kind: git_tree
          value: git:2345678
          inventory: [app/main.go]
          observed_at: 2026-08-28T12:00:00Z
`

func TestBaselineRejectsMalformedConfirmationAndProvenance(t *testing.T) {
	for name, input := range map[string]string{
		"empty file":               "",
		"missing docs":             "confirmed: {}",
		"empty confirmation":       "docs: [{path: api.md, describes: [app], confirmed: {}}]",
		"duplicate doc":            "docs: [{path: api.md, describes: [app]}, {path: api.md, describes: [lib]}]",
		"escaping doc":             strings.Replace(validBaseline, "modules/api.md", "../api.md", 1),
		"noncanonical doc":         strings.Replace(validBaseline, "modules/api.md", "modules/../api.md", 1),
		"reserved ledger":          strings.Replace(validBaseline, "modules/api.md", "baseline.yml", 1),
		"empty describes":          strings.Replace(validBaseline, "describes: [app]", "describes: []", 1),
		"missing owner":            strings.Replace(validBaseline, "by: retired-owner", "by: ''", 1),
		"unsupported source":       strings.Replace(validBaseline, "kind: git_blob", "kind: sha256", 1),
		"missing source timestamp": strings.Replace(validBaseline, "        observed_at: 2026-08-28T12:00:00Z\n", "", 1),
		"bad commit":               strings.Replace(validBaseline, "git:abcdef0", "sha256:abcdef0", 1),
		"bad hash":                 strings.Replace(validBaseline, "git:abcdef0", "git:not-a-hash", 1),
		"uncovered surface":        strings.Replace(validBaseline, "- path: app\n", "- path: other\n", 1),
		"escaping inventory":       strings.Replace(validBaseline, "[app/main.go]", "[../main.go]", 1),
		"duplicate describes":      strings.Replace(validBaseline, "describes: [app]", "describes: [app, app]", 1),
		"empty inventory path":     strings.Replace(validBaseline, "[app/main.go]", "['']", 1),
		"nul inventory path":       strings.Replace(validBaseline, "[app/main.go]", `["app/\0.go"]`, 1),
		"noncanonical inventory":   strings.Replace(validBaseline, "[app/main.go]", "[app/../main.go]", 1),
		"duplicate inventory":      strings.Replace(validBaseline, "[app/main.go]", "[app/main.go, app/main.go]", 1),
		"tree inventory root":      strings.Replace(validBaseline, "[app/main.go]", "[app]", 1),
		"sibling inventory":        strings.Replace(validBaseline, "[app/main.go]", "[application/main.go]", 1),
		"absolute inventory":       strings.Replace(validBaseline, "[app/main.go]", "[/app/main.go]", 1),
		"backslash coordinate":     strings.Replace(validBaseline, "describes: [app]", `describes: ["app\\main.go"]`, 1),
		"newline coordinate":       strings.Replace(validBaseline, "describes: [app]", `describes: ["app/main.go\n"]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBaseline([]byte(input))
			require.ErrorContains(t, err, BaselineRelPath)
		})
	}
}

func TestBaselineLoadsHistoricalOwnerAndMissingProse(t *testing.T) {
	root := t.TempDir()
	empty, err := LoadBaseline(root)
	require.NoError(t, err)
	require.False(t, empty.HasConfirmedDocs())
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "design"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, BaselineRelPath), []byte(validBaseline), 0o644))
	ledger, err := LoadBaseline(root)
	require.NoError(t, err)
	require.True(t, ledger.HasConfirmedDocs())
	require.Equal(t, "retired-owner", ledger.Docs[0].Confirmed.By)
	require.NoFileExists(t, filepath.Join(root, ".tiller", "design", "modules", "api.md"))
}

func TestBaselineAcceptsEmptyTreeAndExternalCoverage(t *testing.T) {
	input := strings.ReplaceAll(validBaseline, "[app]", "[../legacy]")
	input = strings.ReplaceAll(input, "path: app\n", "path: ../legacy\n")
	input = strings.ReplaceAll(input, "inventory: [app/main.go]", "inventory: []")
	ledger, err := ParseBaseline([]byte(input))
	require.NoError(t, err)
	require.True(t, ledger.HasConfirmedDocs())
	empty, err := ParseBaseline([]byte("docs: []\n"))
	require.NoError(t, err)
	require.False(t, empty.HasConfirmedDocs())
}

func TestBaselineRefusesSymlinkedLedger(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "baseline.yml")
	require.NoError(t, os.WriteFile(external, []byte(validBaseline), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller", "design"), 0o755))
	require.NoError(t, os.Symlink(external, filepath.Join(root, BaselineRelPath)))
	_, err := LoadBaseline(root)
	require.Error(t, err)
}

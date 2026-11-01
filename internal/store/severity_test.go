package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidSeverity(t *testing.T) {
	for _, s := range []string{"blocker", "important", "medium", "minor"} {
		require.True(t, ValidSeverity(s), s)
	}
	for _, s := range []string{"", "High", "critical"} {
		require.False(t, ValidSeverity(s), s)
	}
}

func TestSeverityRank(t *testing.T) {
	require.Equal(t, 0, SeverityRank(SeverityBlocker))
	require.Equal(t, 1, SeverityRank(SeverityImportant))
	require.Equal(t, 2, SeverityRank(SeverityMedium))
	require.Equal(t, 3, SeverityRank(SeverityMinor))
	require.Equal(t, 4, SeverityRank(""))
	require.Equal(t, 4, SeverityRank("bogus"))
}

func TestLoadRefusesSeverityOnChore(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	p := filepath.Join(root, ".tiller", "chores", "bump-rails", "chore.yml")
	writeFile(t, p, "id: CH-4m2x9q\nkey: chores/bump-rails\nstatus: ready\nblocked: false\nseverity: minor\n")
	_, err := Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "severity applies to bugs")
}

func TestLoadRefusesUnknownSeverity(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	p := filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml")
	writeFile(t, p, "id: B-7k2p9x\nkey: bugs/blank-on-reload\nstatus: ready\nblocked: false\nseverity: High\n")
	_, err := Load(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocker, important, medium, or minor")
}

func TestLoadReadsBugSeverity(t *testing.T) {
	root := copyTree(t, fixtureTreeRoot)
	p := filepath.Join(root, ".tiller", "bugs", "blank-on-reload", "bug.yml")
	writeFile(t, p, "id: B-7k2p9x\nkey: bugs/blank-on-reload\nstatus: ready\nblocked: false\nseverity: important\n")
	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, "important", tree.Bugs["B-7k2p9x"].Severity)
}

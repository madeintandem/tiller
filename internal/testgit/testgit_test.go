package testgit_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/testgit"
	"github.com/stretchr/testify/require"
)

func TestInitWritesTheSameConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testgit.Init(t, dir)

	want := map[string]string{
		"user.email":       "test@example.com",
		"user.name":        "Tiller Test",
		"commit.gpgsign":   "false",
		"gc.auto":          "0",
		"maintenance.auto": "false",
	}
	for key, value := range want {
		require.Equal(t, value, testgit.Run(t, dir, "config", "--get", key), key)
	}
	require.Equal(t, "refs/heads/main", testgit.Run(t, dir, "symbolic-ref", "HEAD"))
}

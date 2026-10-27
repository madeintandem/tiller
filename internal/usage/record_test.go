package usage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/usage"
	"github.com/stretchr/testify/require"
)

func sampleFile(session string) usage.File {
	return usage.File{
		Harness:    "claude-code",
		Session:    session,
		Reader:     usage.ReaderVersion,
		CapturedAt: time.Date(2026, 10, 26, 19, 2, 11, 0, time.UTC),
		FirstCall:  time.Date(2026, 10, 20, 14, 3, 55, 0, time.UTC),
		LastCall:   time.Date(2026, 10, 20, 18, 41, 2, 0, time.UTC),
		Rows: []usage.Row{
			{Hour: time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC), Node: "T-d8icjq", Stage: usage.Implementation, Model: "claude-opus-5-5", Calls: 41,
				Tokens: usage.Tokens{Input: 1203, CacheWrite: 290114, CacheRead: 3920551, Output: 18220, Reasoning: 4410}},
			{Hour: time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC), Stage: usage.Other, Model: "claude-opus-5-5", Calls: 2,
				Tokens: usage.Tokens{Input: 5, CacheWrite: 6, CacheRead: 7, Output: 8}},
		},
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	f := sampleFile("abc")
	data, err := usage.Encode(f)
	require.NoError(t, err)
	text := string(data)
	require.Equal(t, 1, strings.Count(text, "node:"))
	require.Equal(t, 1, strings.Count(text, "reasoning:"))
	got, err := usage.Decode(data)
	require.NoError(t, err)
	require.Equal(t, f, got)
	require.Equal(t, 43, got.Calls())
}

func TestEncodeSortsRows(t *testing.T) {
	f := sampleFile("abc")
	f.Rows[0], f.Rows[1] = f.Rows[1], f.Rows[0]
	data, err := usage.Encode(f)
	require.NoError(t, err)
	got, err := usage.Decode(data)
	require.NoError(t, err)
	require.Equal(t, "T-d8icjq", got.Rows[0].Node)
}

func TestTotalExcludesReasoning(t *testing.T) {
	require.Equal(t, int64(10), usage.Tokens{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4, Reasoning: 100}.Total())
	var sum usage.Tokens
	sum.Add(usage.Tokens{Input: 1, Reasoning: 2})
	sum.Add(usage.Tokens{Input: 1, Reasoning: 2})
	require.Equal(t, usage.Tokens{Input: 2, Reasoning: 4}, sum)
}

func TestPathShape(t *testing.T) {
	require.Equal(t, ".tiller/usage/claude-code/abc.yml", usage.Path("claude-code", "abc"))
}

func TestLoadFilesMissingDirIsEmpty(t *testing.T) {
	files, err := usage.LoadFiles(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestLoadFilesReadsEveryHarness(t *testing.T) {
	root := t.TempDir()
	for _, c := range [][2]string{{"claude-code", "a"}, {"claude-code", "b"}, {"pi", "c"}} {
		f := sampleFile(c[1])
		f.Harness = c[0]
		data, err := usage.Encode(f)
		require.NoError(t, err)
		p := filepath.Join(root, usage.Path(c[0], c[1]))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(t, os.WriteFile(p, data, 0644))
	}
	files, err := usage.LoadFiles(root)
	require.NoError(t, err)
	require.Len(t, files, 3)
}

func TestStagesDisplayOrderAndLabels(t *testing.T) {
	keys := []string{"spec_writing", "planning", "implementation", "checks", "acceptance_evals", "code_review", "publishing", "closeout", "other"}
	labels := []string{"Spec writing", "Planning", "Implementation", "Checks", "Acceptance evals", "Code review", "Publishing", "Closeout", "Other"}
	require.Len(t, usage.Stages, len(keys))
	for i, s := range usage.Stages {
		require.Equal(t, keys[i], string(s))
		require.Equal(t, labels[i], s.Label())
	}
}

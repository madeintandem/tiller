package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The full mapping form, per artifact-schema.md "Revisions".
func TestRevisionRef_FullMapping(t *testing.T) {
	src := `
kind: git_blob
value: "git:31fd9c2"
path: epics/form-engine/features/certification-form-loading/feature.md
observed_at: "2026-06-23T00:00:00Z"
`
	var ref RevisionRef
	require.NoError(t, yaml.Unmarshal([]byte(src), &ref))

	assert.Equal(t, KindGitBlob, ref.Kind)
	assert.Equal(t, "git:31fd9c2", ref.Value)
	assert.Equal(t, "epics/form-engine/features/certification-form-loading/feature.md", ref.Path)
	want, _ := time.Parse(time.RFC3339, "2026-06-23T00:00:00Z")
	assert.True(t, ref.ObservedAt.Equal(want), "observed_at parses")
}

// The compact value-only "git:<sha>" shorthand. artifact-schema.md: a bare
// value-only ref with no path is a commit.
func TestRevisionRef_CompactGit(t *testing.T) {
	var ref RevisionRef
	require.NoError(t, yaml.Unmarshal([]byte(`"git:31fd9c2"`), &ref))

	assert.Equal(t, KindGitCommit, ref.Kind)
	assert.Equal(t, "git:31fd9c2", ref.Value)
	assert.Empty(t, ref.Path)
}

// The compact "sha256:<hex>" shorthand resolves to the sha256 kind.
func TestRevisionRef_CompactSHA256(t *testing.T) {
	var ref RevisionRef
	require.NoError(t, yaml.Unmarshal([]byte(`"sha256:deadbeef"`), &ref))

	assert.Equal(t, KindSHA256, ref.Kind)
	assert.Equal(t, "sha256:deadbeef", ref.Value)
}

// Both forms appear in real node files: an inline mapping (gate `against`)
// and a compact scalar. They must coexist in one doc.
func TestRevisionRef_MixedFormsInStruct(t *testing.T) {
	type holder struct {
		Against   RevisionRef `yaml:"against"`
		StartedAt RevisionRef `yaml:"started_at"`
	}
	src := `
against: { kind: git_commit, value: "git:abc1234" }
started_at: git:31fd9c2
`
	var h holder
	require.NoError(t, yaml.Unmarshal([]byte(src), &h))

	assert.Equal(t, KindGitCommit, h.Against.Kind)
	assert.Equal(t, "git:abc1234", h.Against.Value)
	assert.Equal(t, KindGitCommit, h.StartedAt.Kind)
	assert.Equal(t, "git:31fd9c2", h.StartedAt.Value)
}

// A null value field leaves the zero ref rather than erroring; feature.yml
// carries `verified_against.spec: null` before completion.
func TestRevisionRef_Null(t *testing.T) {
	type holder struct {
		Spec RevisionRef `yaml:"spec"`
	}
	var h holder
	require.NoError(t, yaml.Unmarshal([]byte(`spec: null`), &h))
	assert.Equal(t, RevisionRef{}, h.Spec)
}

// A malformed compact ref (no recognized prefix) is an error.
func TestRevisionRef_BadCompact(t *testing.T) {
	var ref RevisionRef
	err := yaml.Unmarshal([]byte(`"abc1234"`), &ref)
	assert.Error(t, err)
}

// An unknown kind in the full mapping is rejected.
func TestRevisionRef_BadKind(t *testing.T) {
	var ref RevisionRef
	err := yaml.Unmarshal([]byte(`{ kind: git_branch, value: "git:abc1234" }`), &ref)
	assert.Error(t, err)
}

// Marshal then unmarshal preserves every field.
func TestRevisionRef_RoundTrip(t *testing.T) {
	observed, _ := time.Parse(time.RFC3339, "2026-06-23T00:00:00Z")
	orig := RevisionRef{
		Kind:       KindGitTree,
		Value:      "git:3f6a9d2",
		Path:       "app/models/calc",
		ObservedAt: observed,
	}
	data, err := yaml.Marshal(orig)
	require.NoError(t, err)

	var back RevisionRef
	require.NoError(t, yaml.Unmarshal(data, &back))
	assert.Equal(t, orig.Kind, back.Kind)
	assert.Equal(t, orig.Value, back.Value)
	assert.Equal(t, orig.Path, back.Path)
	assert.True(t, orig.ObservedAt.Equal(back.ObservedAt))
}

func TestValidKind(t *testing.T) {
	for _, k := range []string{KindGitBlob, KindGitCommit, KindGitTree, KindSHA256} {
		assert.True(t, ValidKind(k), k)
	}
	assert.False(t, ValidKind("git_branch"))
	assert.False(t, ValidKind(""))
}

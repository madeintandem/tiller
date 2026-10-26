package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestVerifiedAgainstAlwaysSerializesSurfaces pins the schema requirement that a
// materialized verified_against records surfaces as a list, the empty list `[]`
// for a create feature that touched no surface (artifact-schema.md, verified_against).
// A node that never reached done has a zero verified_against and omits the block
// entirely, so a draft feature does not grow a noise `surfaces: []`.
func TestVerifiedAgainstAlwaysSerializesSurfaces(t *testing.T) {
	// A done create feature: spec and eval set, no surfaces. surfaces must serialize
	// as the empty list, not be omitted.
	done := Feature{
		ID:     "F-aaaaaa",
		Status: "done",
		VerifiedAgainst: VerifiedAgainst{
			Spec:     RevisionRef{Kind: KindGitBlob, Value: "git:abc"},
			Eval:     RevisionRef{Kind: KindGitBlob, Value: "git:def"},
			Surfaces: []SurfaceFingerprint{},
		},
	}
	out, err := yaml.Marshal(done)
	require.NoError(t, err)
	require.Contains(t, string(out), "surfaces: []",
		"a done create feature records surfaces as the empty list, never omitted")

	// A draft feature never reached done: the zero verified_against is omitted whole,
	// so it does not grow a stray surfaces key.
	draft := Feature{ID: "F-bbbbbb", Status: "draft"}
	out2, err := yaml.Marshal(draft)
	require.NoError(t, err)
	require.NotContains(t, string(out2), "verified_against",
		"a never-done feature omits the verified_against block entirely")
}

// TestSourceRoundTripsOnEpicAndFeature pins Source as a field the store structs
// themselves model, not a concern the mutation layer bolts on with a private
// wrapper doc. A hand-authored source: value in epic.yml/feature.yml must
// unmarshal onto the struct and marshal back out as the same literal value, so
// store.Load (which unmarshals the bare struct) never silently drops it
// (artifact-schema.md, Epics and Features).
func TestSourceRoundTripsOnEpicAndFeature(t *testing.T) {
	epicYAML := `id: E-aaaaaa
key: form-engine
title: Form Engine
status: draft
owner: jc
source: design/04-phase-1-mvp-build.md
`
	var epic Epic
	require.NoError(t, yaml.Unmarshal([]byte(epicYAML), &epic))
	require.Equal(t, "design/04-phase-1-mvp-build.md", epic.Source)

	out, err := yaml.Marshal(&epic)
	require.NoError(t, err)
	require.Contains(t, string(out), "source: design/04-phase-1-mvp-build.md",
		"a hand-authored source value round-trips literally, never rewritten")

	featureYAML := `id: F-aaaaaa
key: form-engine/certification-form-loading
title: Certification form loading
status: draft
owner: jc
change_type: create
source: design/04-phase-1-mvp-build.md
`
	var feat Feature
	require.NoError(t, yaml.Unmarshal([]byte(featureYAML), &feat))
	require.Equal(t, "design/04-phase-1-mvp-build.md", feat.Source)

	fout, err := yaml.Marshal(&feat)
	require.NoError(t, err)
	require.Contains(t, string(fout), "source: design/04-phase-1-mvp-build.md",
		"a hand-authored source value round-trips literally, never rewritten")
}

// TestSupersedesRoundTripsOnTask pins Supersedes as a field Task carries the
// same way Feature already does. A cancel-with-superseded-by amendment
// operation writes supersedes onto the replacement task the same way a
// supersede operation writes it onto a replacement feature (artifact-schema.md,
// Features; History is append-only), so the field has to survive a load intact.
func TestSupersedesRoundTripsOnTask(t *testing.T) {
	taskYAML := `id: T-aaaaaa
key: form-engine/certification-form-loading/schema-transcription
title: Transcribe the schema
status: proposed
owner: jc
supersedes: [T-bbbbbb]
`
	var task Task
	require.NoError(t, yaml.Unmarshal([]byte(taskYAML), &task))
	assert.Equal(t, []string{"T-bbbbbb"}, task.Supersedes)

	out, err := yaml.Marshal(&task)
	require.NoError(t, err)
	var roundTripped Task
	require.NoError(t, yaml.Unmarshal(out, &roundTripped))
	assert.Equal(t, task.Supersedes, roundTripped.Supersedes)
}

// TestSupersedesAbsentOnTaskLoadsAsNil pins the tolerant read: most tasks are
// never superseded, and a task.yml written before this field existed, or one
// that simply has nothing to record, must load with a nil slice rather than
// failing to parse.
func TestSupersedesAbsentOnTaskLoadsAsNil(t *testing.T) {
	var task Task
	require.NoError(t, yaml.Unmarshal([]byte("id: T-aaaaaa\nstatus: proposed\n"), &task))
	assert.Nil(t, task.Supersedes)
}

// TestShelfNounsIsBugThenChoreAndFresh pins the helper every shelf-noun
// walk uses instead of its own literal (issue #96): bug then chore, the
// order the literals used, and a fresh slice per call so no caller can
// mutate a shared one.
func TestShelfNounsIsBugThenChoreAndFresh(t *testing.T) {
	first := ShelfNouns()
	require.Equal(t, []string{NounBug, NounChore, NounTweak}, first)
	first[0] = "mutated"
	require.Equal(t, []string{NounBug, NounChore, NounTweak}, ShelfNouns(), "a caller's write never reaches the next call")
}

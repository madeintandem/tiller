package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// A GatesBlock carrying a needs_attestation criterion and an AT- attestation
// round-trips through YAML (artifact-schema.md, The stored gate block).
func TestGatesBlock_RoundTrip(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2026-06-23T00:00:00Z")

	orig := GatesBlock{
		Gates: map[string]Gate{
			"evals": {
				Required: true,
				Kind:     GateKindCommand,
				Status:   GateStatusPending,
				RunBy:    "sam",
				RunAt:    ts,
				Against:  RevisionRef{Kind: KindGitCommit, Value: "git:abc1234"},
				Criteria: map[string]Criterion{
					"C3": {
						Status:   CriterionStatusNeedsAttestation,
						Evidence: []Evidence{{Kind: "rubric_output", Ref: "artifacts/tiller/evals/F-79dh3q-C3.json"}},
					},
				},
			},
		},
		Attestations: []Attestation{
			{
				ID:          "AT-4m9x2q",
				Target:      AttestationTarget{Kind: "criterion", ID: "C3"},
				Status:      "pass",
				By:          "jc",
				At:          ts,
				EvidenceRef: "artifacts/tiller/evals/F-79dh3q-C3.json",
			},
		},
	}

	data, err := yaml.Marshal(orig)
	require.NoError(t, err)

	var back GatesBlock
	require.NoError(t, yaml.Unmarshal(data, &back))

	c3 := back.Gates["evals"].Criteria["C3"]
	assert.Equal(t, CriterionStatusNeedsAttestation, c3.Status)
	require.Len(t, c3.Evidence, 1)
	assert.Equal(t, "rubric_output", c3.Evidence[0].Kind)

	require.Len(t, back.Attestations, 1)
	at := back.Attestations[0]
	assert.Equal(t, "AT-4m9x2q", at.ID)
	assert.Equal(t, "criterion", at.Target.Kind)
	assert.Equal(t, "C3", at.Target.ID)
	assert.Equal(t, "pass", at.Status)
	assert.True(t, at.At.Equal(ts))

	assert.True(t, back.Gates["evals"].Against.Kind == KindGitCommit)
	assert.True(t, back.Gates["evals"].RunAt.Equal(ts))
}

// A GatesBlock embedded inline on a Feature surfaces gates/attestations/
// policy_violations as top-level siblings, not nested under a gates key.
func TestGatesBlock_InlineOnFeature(t *testing.T) {
	key := "form-engine/x"
	f := Feature{
		ID:  "F-test01",
		Key: &key,
		GatesBlock: GatesBlock{
			Gates: map[string]Gate{
				"build": {Required: true, Kind: GateKindCommand, Status: GateStatusPass},
			},
		},
	}
	data, err := yaml.Marshal(f)
	require.NoError(t, err)

	// gates is a top-level key, and there is no double-nested gates.gates.
	assert.Contains(t, string(data), "\ngates:\n")
	assert.NotContains(t, string(data), "gatesblock")

	var back Feature
	require.NoError(t, yaml.Unmarshal(data, &back))
	assert.Equal(t, GateStatusPass, back.Gates["build"].Status)
}

// The code identity, config hash, and reuse provenance fields round-trip
// byte-stable on a gate and on a criterion, and a record that carries none of
// them marshals without the keys, so a pre-reuse ledger is unchanged on disk.
func TestGatesBlock_RoundTripReuseFields(t *testing.T) {
	t.Parallel()
	ts, _ := time.Parse(time.RFC3339, "2026-10-21T00:00:00Z")

	orig := GatesBlock{
		Gates: map[string]Gate{
			"tests": {
				Required:   true,
				Kind:       GateKindCommand,
				Status:     GateStatusPass,
				RunBy:      "sam",
				RunAt:      ts,
				Against:    RevisionRef{Kind: KindGitCommit, Value: "git:abc1234"},
				Code:       "sha256:aaaa",
				Config:     "sha256:bbbb",
				ReusedAt:   ts.Add(time.Hour),
				ReusedFrom: "T-aaaaaa",
			},
			"evals": {
				Required: true,
				Kind:     GateKindCommand,
				Status:   GateStatusPass,
				Criteria: map[string]Criterion{
					"C1": {
						Status:   CriterionStatusPass,
						Code:     "sha256:cccc",
						Config:   "sha256:dddd",
						ReusedAt: ts.Add(2 * time.Hour),
					},
				},
			},
			"build": {Required: true, Kind: GateKindCommand, Status: GateStatusPass},
		},
	}

	first, err := yaml.Marshal(orig)
	require.NoError(t, err)

	var back GatesBlock
	require.NoError(t, yaml.Unmarshal(first, &back))
	assert.Equal(t, "sha256:aaaa", back.Gates["tests"].Code)
	assert.Equal(t, "sha256:bbbb", back.Gates["tests"].Config)
	assert.True(t, ts.Add(time.Hour).Equal(back.Gates["tests"].ReusedAt))
	assert.Equal(t, "T-aaaaaa", back.Gates["tests"].ReusedFrom)
	c := back.Gates["evals"].Criteria["C1"]
	assert.Equal(t, "sha256:cccc", c.Code)
	assert.Equal(t, "sha256:dddd", c.Config)
	assert.True(t, ts.Add(2*time.Hour).Equal(c.ReusedAt))

	second, err := yaml.Marshal(back)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "marshal, unmarshal, marshal must be byte-stable")

	bare, err := yaml.Marshal(Gate{Required: true, Kind: GateKindCommand, Status: GateStatusPass})
	require.NoError(t, err)
	for _, key := range []string{"code", "config", "reused_at", "reused_from"} {
		assert.NotContains(t, string(bare), key+":")
	}
	bareCrit, err := yaml.Marshal(Criterion{Status: CriterionStatusPass})
	require.NoError(t, err)
	for _, key := range []string{"code", "config", "reused_at"} {
		assert.NotContains(t, string(bareCrit), key+":")
	}
}

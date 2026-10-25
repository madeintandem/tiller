package mutate

import (
	"testing"

	"github.com/madeintandem/tiller/internal/core/gates"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// requiredGateConfigs is unexported, so this pin lives in the mutate package's
// own test binary rather than gate_scoping_test.go (package mutate_test).

// A manifest's `reuse: false` opt-out reaches the runnable config; a gate
// that says nothing keeps reuse on.
func TestRequiredGateConfigsCarryReuse(t *testing.T) {
	t.Parallel()
	off := false
	m := &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
		"build": {Kind: store.GateKindCommand, Run: "go build", On: store.StringList{"task"}},
		"tests": {Kind: store.GateKindCommand, Run: "go test", On: store.StringList{"task"}, Reuse: &off},
	}}}
	byName := map[string]gates.GateConfig{}
	for _, c := range requiredGateConfigs(m, store.NounTask) {
		byName[c.Name] = c
	}
	require.Len(t, byName, 2)
	require.True(t, byName["build"].Reuse)
	require.False(t, byName["tests"].Reuse)
}

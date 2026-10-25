package gates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The config hash is a pure function of the gate kind and its command, and
// the separator keeps a shifted boundary from colliding.
func TestConfigHash(t *testing.T) {
	t.Parallel()
	a := ConfigHash("command", "go test ./...")
	assert.Equal(t, a, ConfigHash("command", "go test ./..."))
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, a)
	assert.NotEqual(t, a, ConfigHash("command", "go test ./cmd/..."))
	assert.NotEqual(t, a, ConfigHash("test", "go test ./..."))
	assert.NotEqual(t, ConfigHash("command", "a b"), ConfigHash("command a", "b"))
}

package gates

import (
	"crypto/sha256"
	"encoding/hex"
)

// ConfigHash is the config half of a gate-reuse key: the hash of what would run,
// the gate kind and its interpolated command. Two records may stand in for each
// other only when this matches, so editing the manifest's command retires every
// stored pass for it without any explicit invalidation (design
// 2026-10-21-gate-reuse). The NUL separator keeps a shifted boundary between
// kind and run from colliding.
func ConfigHash(kind, run string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + run))
	return "sha256:" + hex.EncodeToString(sum[:])
}

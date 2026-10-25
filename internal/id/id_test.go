package id

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validPrefixes is the canonical set from docs/spec/artifact-schema.md (IDs)
// plus the EV (event) and AT (attestation) classes the CLI mints internally.
var allPrefixes = []string{"E", "F", "T", "Q", "A", "ADR", "EV", "AT", "B", "CH", "TW"}

// TestMintShape asserts every minted id matches <prefix>-[a-z0-9]{6} exactly.
func TestMintShape(t *testing.T) {
	for _, p := range allPrefixes {
		re := regexp.MustCompile("^" + regexp.QuoteMeta(p) + `-[a-z0-9]{6}$`)
		got := Mint(p)
		assert.Regexp(t, re, got, "Mint(%q) = %q", p, got)
	}
}

// TestMintUnique mints 10k tokens of one prefix and asserts they are
// collision-resistant. The token space is 36^6 (~2.18e9), so 10k draws have a
// ~2.3% birthday-collision chance: asserting EXACTLY zero collisions would make
// this test flaky (it would spuriously fail ~1 run in 43). The real property to
// pin is that the minter draws high-entropy tokens, not a counter, so we allow a
// tiny tolerance well above the expected count (~0.023) but far below what a
// broken (low-entropy or counter-like) minter would produce. A tolerance of 3
// gives a false-failure probability on the order of 1e-6 while still catching a
// minter that collides systematically.
func TestMintUnique(t *testing.T) {
	const n = 10000
	const tolerance = 3
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		seen[Mint("F")] = struct{}{}
	}
	collisions := n - len(seen)
	require.LessOrEqualf(t, collisions, tolerance,
		"minter produced %d collisions over %d draws (tolerance %d); expected ~0 for a high-entropy minter",
		collisions, n, tolerance)
}

// TestValidAcceptsMinted asserts Valid accepts ids from Mint for every prefix.
func TestValidAcceptsMinted(t *testing.T) {
	for _, p := range allPrefixes {
		got := Mint(p)
		assert.True(t, Valid(got), "Valid(%q) should be true", got)
	}
}

// TestValidAcceptsKnownLiterals pins the spec's worked examples and the
// overlapping-prefix edge cases (A vs ADR, E vs EV, A vs AT).
func TestValidAcceptsKnownLiterals(t *testing.T) {
	good := []string{
		"E-j8m4q2",
		"F-79dh3q",
		"T-k3p9w6",
		"Q-h2n7m5",
		"A-d6w3f9",
		"ADR-v7b2d5",
		"EV-abc123",
		"AT-0z9y8x",
		"F-000000", // an all-digit token is a legitimate base36 token
	}
	for _, id := range good {
		assert.True(t, Valid(id), "Valid(%q) should be true", id)
	}
}

// TestValidRejectsUppercaseTokens asserts the token half must be lowercase.
func TestValidRejectsUppercaseTokens(t *testing.T) {
	bad := []string{
		"F-XYZ",    // uppercase, and too short
		"F-ABCDEF", // uppercase, right length
		"F-AbC123", // mixed case
		"T-K3P9W6", // uppercase variant of a real token
	}
	for _, id := range bad {
		assert.False(t, Valid(id), "Valid(%q) should be false", id)
	}
}

// TestValidRejectsCounters asserts sequential counter ids are not valid ids.
func TestValidRejectsCounters(t *testing.T) {
	bad := []string{
		"F-1",
		"E-42",
		"T-7",
		"Q-001",
		"Q-12345",   // 5 chars, too short
		"A-1234567", // 7 chars, too long
	}
	for _, id := range bad {
		assert.False(t, Valid(id), "Valid(%q) should be false", id)
	}
}

// TestValidRejectsMalformed covers structural failures.
func TestValidRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"abc123",    // no prefix
		"F79dh3q",   // missing dash
		"F-79dh3",   // 5 chars
		"F-79dh3q1", // 7 chars
		"X-79dh3q",  // unknown prefix
		"FF-79dh3q", // not a known prefix
		"f-79dh3q",  // lowercase prefix
		"F-79dh3q ", // trailing space
		" F-79dh3q", // leading space
		"F-79_h3q",  // illegal token char
		"F--9dh3q",  // illegal token char
		"ADR-79dh3", // ADR with 5-char token
	}
	for _, id := range bad {
		assert.False(t, Valid(id), "Valid(%q) should be false", id)
	}
}

// TestPrefixFor maps each addressable noun to its fixed prefix.
func TestPrefixFor(t *testing.T) {
	cases := map[string]string{
		"epic":        "E",
		"feature":     "F",
		"task":        "T",
		"question":    "Q",
		"amendment":   "A",
		"adr":         "ADR",
		"event":       "EV",
		"attestation": "AT",
	}
	for noun, want := range cases {
		got, err := PrefixFor(noun)
		require.NoError(t, err, "PrefixFor(%q)", noun)
		assert.Equal(t, want, got, "PrefixFor(%q)", noun)
	}
}

// TestPrefixForUnknown errors on a noun with no id class.
func TestPrefixForUnknown(t *testing.T) {
	_, err := PrefixFor("widget")
	assert.Error(t, err)
}

// TestPrefixForRoundTrip asserts every noun's prefix mints a valid id, so
// Mint and Valid never drift from the noun table.
func TestPrefixForRoundTrip(t *testing.T) {
	for noun := range nounPrefix {
		p, err := PrefixFor(noun)
		require.NoError(t, err)
		assert.True(t, Valid(Mint(p)), "noun %s", noun)
	}
}

// TestMintPanicsOnInvalidPrefix asserts an unknown prefix is a programming
// error, surfaced loudly rather than minting a malformed id.
func TestMintPanicsOnInvalidPrefix(t *testing.T) {
	assert.Panics(t, func() { Mint("X") })
	assert.Panics(t, func() { Mint("f") })
	assert.Panics(t, func() { Mint("") })
}

func TestWorkItemPrefixes(t *testing.T) {
	for noun, want := range map[string]string{"bug": "B", "chore": "CH"} {
		got, err := PrefixFor(noun)
		if err != nil {
			t.Fatalf("PrefixFor(%q): %v", noun, err)
		}
		if got != want {
			t.Fatalf("PrefixFor(%q) = %q, want %q", noun, got, want)
		}
		id := Mint(want)
		if !Valid(id) {
			t.Fatalf("Mint(%q) = %q is not Valid", want, id)
		}
	}
	if Valid("C-abcdef") {
		t.Fatal("C- is not a prefix; chores are CH-")
	}
}

func TestTweakIdentifierIsDistinctFromChore(t *testing.T) {
	prefix, err := PrefixFor("tweak")
	require.NoError(t, err)
	require.Equal(t, "TW", prefix)
	require.True(t, Valid(Mint(prefix)))
	require.True(t, Valid("TW-aaaaaa"))
	require.False(t, Valid("TW-AAAAAA"))
}

// Package id mints and validates Tiller node identifiers.
//
// A Tiller id is a type-prefixed, collision-resistant token of the shape
// <prefix>-<token>, where token is exactly six characters of lowercase base36
// ([a-z0-9]{6}) drawn from cryptographic randomness. The token carries no
// owner, branch, date, order, or path, so two agents on two branches can mint
// ids without sharing a counter and a rename never changes an id. See
// docs/spec/artifact-schema.md, "IDs".
package id

import (
	"crypto/rand"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

const (
	// tokenLen is the fixed length of the random token half of an id.
	tokenLen = 6
	// alphabet is lowercase base36, indexed by a value in [0,36).
	alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

// nounPrefix maps each id-bearing noun to its fixed id prefix: the
// addressable node classes from docs/spec/cli-surface.md, the event and
// attestation classes the CLI mints internally, and the work shelves. It is
// the one list of prefixes; Mint and Valid derive theirs from it.
var nounPrefix = map[string]string{
	"epic":        "E",
	"feature":     "F",
	"task":        "T",
	"question":    "Q",
	"amendment":   "A",
	"adr":         "ADR",
	"event":       "EV",
	"attestation": "AT",
	"bug":         "B",
	"chore":       "CH",
	"tweak":       "TW",
}

// idRe matches a well-formed id: a known prefix, a hyphen, then exactly six
// lowercase base36 characters. Anchored, so it rejects any surrounding text.
var idRe = regexp.MustCompile(`^(` + strings.Join(slices.Sorted(maps.Keys(prefixes)), "|") + `)-[a-z0-9]{6}$`)

// prefixes is the set of known id prefixes.
var prefixes = knownPrefixes()

func knownPrefixes() map[string]bool {
	set := make(map[string]bool, len(nounPrefix))
	for _, p := range nounPrefix {
		set[p] = true
	}
	return set
}

// Mint returns a new id of the form <prefix>-<token>. The prefix must be one
// of the known classes in nounPrefix; an unknown prefix
// is a programming error and panics, because the signature has no error to
// return and a malformed id must never reach the store. Use PrefixFor to
// derive a prefix from a noun.
func Mint(prefix string) string {
	if !prefixes[prefix] {
		panic(fmt.Sprintf("id: invalid prefix %q", prefix))
	}
	return prefix + "-" + token()
}

// Valid reports whether s is a well-formed id: a known prefix and a six-char
// lowercase base36 token. It rejects uppercase tokens, counters, unknown
// prefixes, and any wrong-length token.
func Valid(s string) bool {
	return idRe.MatchString(s)
}

// PrefixFor returns the id prefix for an addressable noun, or an error if the
// noun has no id class.
func PrefixFor(noun string) (string, error) {
	p, ok := nounPrefix[noun]
	if !ok {
		return "", fmt.Errorf("id: no id prefix for noun %q", noun)
	}
	return p, nil
}

// token returns a uniformly random six-character lowercase base36 string. It
// draws from crypto/rand and uses rejection sampling to avoid the modulo bias
// that would otherwise favor the first four symbols of the alphabet.
func token() string {
	// 252 is the largest multiple of 36 that fits in a byte, so any byte in
	// [0,252) maps to a base36 symbol with uniform probability; bytes in
	// [252,256) are rejected and resampled.
	const cutoff = 252

	out := make([]byte, tokenLen)
	filled := 0
	buf := make([]byte, tokenLen)
	for filled < tokenLen {
		if _, err := rand.Read(buf); err != nil {
			panic(fmt.Sprintf("id: crypto/rand failed: %v", err))
		}
		for _, b := range buf {
			if int(b) >= cutoff {
				continue
			}
			out[filled] = alphabet[int(b)%36]
			filled++
			if filled == tokenLen {
				break
			}
		}
	}
	return string(out)
}

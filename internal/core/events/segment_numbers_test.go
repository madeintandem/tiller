package events

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin the numeric half of the lossless-compaction guarantee: a
// whole number keeps every digit in both encodings, or the strict decode
// refuses the event so compaction leaves its file loose.

// wideNumberHead opens a loose event; a test appends the before and after maps.
const wideNumberHead = "id: EV-wwwwww\ntype: add.task\nat: 2026-10-08T04:43:03Z\ncontributor: lantern\ncommand: add task\n"

// throughSegment encodes one event as a segment and decodes it back.
func throughSegment(t *testing.T, e Event) (Event, string) {
	t.Helper()
	data, err := EncodeSegment([]Event{e})
	require.NoError(t, err)
	back, err := DecodeSegment("2026/10/08-x.jsonl", data)
	require.NoError(t, err)
	require.Len(t, back, 1)
	return back[0], string(data)
}

// TestAWholeNumberAboveInt64KeepsEveryDigitInBothEncodings covers whole numbers
// on both sides of the int64 limit and up to the uint64 limit, nested in a map
// and a list beside ordinary fractions. Each is read exactly from the loose
// file, written exactly to the segment, and read back exactly, so the
// round-trip proof compares the real values and not two copies of one rounded
// float.
func TestAWholeNumberAboveInt64KeepsEveryDigitInBothEncodings(t *testing.T) {
	loose, err := DecodeLoose("x.yml", []byte(wideNumberHead+
		"before:\n    seen: [18446744073709551614, 3]\n"+
		"after:\n    max_int: 9223372036854775807\n    over_int: 9223372036854775808\n    max_uint: 18446744073709551615\n"+
		"    ratio: 0.25\n    nested:\n        list: [9223372036854775809, 1.5]\n"))
	require.NoError(t, err)

	back, data := throughSegment(t, loose)

	for name, e := range map[string]Event{"loose": loose, "segment": back} {
		require.Equal(t, math.MaxInt64, e.After["max_int"], name)
		require.Equal(t, uint64(math.MaxInt64)+1, e.After["over_int"], name)
		require.Equal(t, uint64(math.MaxUint64), e.After["max_uint"], name)
		require.Equal(t, 0.25, e.After["ratio"], name)
		require.Equal(t, []any{uint64(math.MaxInt64) + 2, 1.5}, e.After["nested"].(map[string]any)["list"], name)
		require.Equal(t, []any{uint64(math.MaxUint64) - 1, 3}, e.Before["seen"], name)
	}
	require.True(t, Equal(loose, back))
	require.Contains(t, data, `"max_uint":18446744073709551615`)
	require.Contains(t, data, `"list":[9223372036854775809,1.5]`)
	require.Contains(t, data, `"seen":[18446744073709551614,3]`)
}

// TestTwoDifferentWideNumbersAreDifferentEvents pins conflict detection for
// numbers a float would round together: two records of one id that differ only
// in such a number differ, loose against loose, segment against segment, and
// one against the other.
func TestTwoDifferentWideNumbersAreDifferentEvents(t *testing.T) {
	one, err := DecodeLoose("x.yml", []byte(wideNumberHead+"after:\n    n: 18446744073709551615\n"))
	require.NoError(t, err)
	other, err := DecodeLoose("x.yml", []byte(wideNumberHead+"after:\n    n: 18446744073709551614\n"))
	require.NoError(t, err)
	oneCompacted, _ := throughSegment(t, one)
	otherCompacted, _ := throughSegment(t, other)

	require.False(t, Equal(one, other), "loose against loose")
	require.False(t, Equal(oneCompacted, otherCompacted), "segment against segment")
	require.False(t, Equal(one, otherCompacted), "loose against segment")
	require.True(t, Equal(one, oneCompacted))
	require.Error(t, Conflict(Located{Event: oneCompacted, Path: "a.jsonl"}, Located{Event: other, Path: "b.yml"}))
}

// TestAWholeFloatReadsAsTheSameNumberInEitherEncoding extends the 2 and 2.0
// rule past int64: a float spelling of a whole number in the uint64 range is
// that number, so it equals the integer spelling and survives a segment. Past
// uint64 a float stays a float, and still survives.
func TestAWholeFloatReadsAsTheSameNumberInEitherEncoding(t *testing.T) {
	asFloat, err := DecodeLoose("x.yml", []byte(wideNumberHead+"after:\n    n: 9.223372036854775808e+18\n"))
	require.NoError(t, err)
	asInt, err := DecodeLoose("x.yml", []byte(wideNumberHead+"after:\n    n: 9223372036854775808\n"))
	require.NoError(t, err)
	back, _ := throughSegment(t, asFloat)

	require.True(t, Equal(asFloat, asInt))
	require.True(t, Equal(asFloat, back))
	require.Equal(t, uint64(1)<<63, back.After["n"])

	for _, text := range []string{"1e+300", "-1e+19", "1.8446744073709551616e+19"} {
		wide, err := DecodeLoose("x.yml", []byte(wideNumberHead+"after:\n    n: "+text+"\n"))
		require.NoError(t, err)
		back, _ := throughSegment(t, wide)
		require.IsType(t, float64(0), back.After["n"], text)
		require.True(t, Equal(wide, back), text)
	}
}

// TestDecodeLooseStrictRefusesAWholeNumberTooWideFor64Bits pins what the
// strict decode does with a number nothing here can hold exactly. YAML reads a
// whole number outside the 64-bit range as a rounded float, so both sides of a
// round trip would agree on the rounded value; the strict decode refuses the
// event instead, and a plain read still takes it. Fractions, and whole numbers
// 64 bits can hold, are not refused.
func TestDecodeLooseStrictRefusesAWholeNumberTooWideFor64Bits(t *testing.T) {
	for name, body := range map[string]string{
		"just past uint64":   "after:\n    n: 18446744073709551617\n",
		"just below int64":   "after:\n    n: -9223372036854775809\n",
		"nested in a list":   "before:\n    m:\n        list: [1, 99999999999999999999999]\n",
		"with underscores":   "after:\n    n: 18_446_744_073_709_551_617\n",
		"through an alias":   "before:\n    n: &wide 18446744073709551617\nafter:\n    n: *wide\n",
		"under an alias map": "before: &m\n    n: 18446744073709551617\nafter: *m\n",
	} {
		_, err := DecodeLooseStrict("x.yml", []byte(wideNumberHead+body))
		require.ErrorContains(t, err, "x.yml", name)
		require.ErrorContains(t, err, "64 bits", name)

		_, err = DecodeLoose("x.yml", []byte(wideNumberHead+body))
		require.NoError(t, err, "%s: a plain read still takes the event", name)
	}
	for name, body := range map[string]string{
		"uint64 limit":   "after:\n    n: 18446744073709551615\n",
		"int64 floor":    "after:\n    n: -9223372036854775808\n",
		"a fraction":     "after:\n    n: 1.5\n",
		"an exponent":    "after:\n    n: 1e+300\n",
		"digits as text": "after:\n    n: \"18446744073709551617\"\n",
	} {
		_, err := DecodeLooseStrict("x.yml", []byte(wideNumberHead+body))
		require.NoError(t, err, name)
	}

	// An anchor that contains itself is refused by the decode, before the walk
	// that follows aliases could loop on it.
	_, err := DecodeLooseStrict("x.yml", []byte(wideNumberHead+"after:\n    loop: &a [*a]\n"))
	require.ErrorContains(t, err, "contains itself")
}

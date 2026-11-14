package mutate_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/stretchr/testify/require"
)

// These tests pin the lossless guarantee for numbers at the compaction
// surface: a loose file is deleted only when the segment holds every digit of
// every whole number in it, and otherwise the file stays exactly as it was.

// looseEventWithAfter writes a loose event whose after map carries the given
// extra YAML lines (indented four spaces), and returns the file's path.
func looseEventWithAfter(t *testing.T, root, id, when, lines string) string {
	t.Helper()
	path := looseEvent(t, root, id, "add.task", when)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	rewritten := strings.Replace(string(raw), "    attempt: 2\n", "    attempt: 2\n"+lines, 1)
	require.NotEqual(t, string(raw), rewritten)
	require.NoError(t, os.WriteFile(path, []byte(rewritten), 0o644))
	return path
}

// TestCompactCarriesAWholeNumberAboveInt64WithEveryDigit covers whole numbers
// around the int64 and uint64 limits, nested in a map and a list beside an
// ordinary fraction. The event compacts, and the segment holds each number
// exactly, so nothing was proved against a rounded copy.
func TestCompactCarriesAWholeNumberAboveInt64WithEveryDigit(t *testing.T) {
	t.Parallel()
	root := writeOwnedRepo(t)
	loose := looseEventWithAfter(t, root, "EV-aaaaaa", "2026-11-03T10:00:00Z",
		"    max_int: 9223372036854775807\n"+
			"    over_int: 9223372036854775808\n"+
			"    max_uint: 18446744073709551615\n"+
			"    ratio: 0.25\n"+
			"    nested:\n        list: [18446744073709551614, 1.5]\n")
	before := listEvents(t, root)

	res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

	require.NoError(t, err)
	require.Zero(t, res.Skipped)
	require.Equal(t, []string{"2026-11-03"}, compactedDays(res))
	require.NoFileExists(t, loose)
	segment := string(readFile(t, root, res.Days[0].Segment))
	require.Contains(t, segment, `"max_int":9223372036854775807`)
	require.Contains(t, segment, `"over_int":9223372036854775808`)
	require.Contains(t, segment, `"max_uint":18446744073709551615`)
	require.Contains(t, segment, `"list":[18446744073709551614,1.5]`)
	require.Contains(t, segment, `"ratio":0.25`)

	after := listEvents(t, root)
	requireSameEvents(t, before, after)
	require.Equal(t, uint64(math.MaxUint64), after[0].After["max_uint"])
	require.Equal(t, []any{uint64(math.MaxUint64) - 1, 1.5}, after[0].After["nested"].(map[string]any)["list"])
}

// TestCompactLeavesAWholeNumberTooWideFor64BitsLoose covers the numbers
// nothing can carry exactly: a whole number past either 64-bit limit reads as
// a rounded float, so the event stays loose, byte for byte, is counted as
// skipped, and the rest of its day still compacts.
func TestCompactLeavesAWholeNumberTooWideFor64BitsLoose(t *testing.T) {
	t.Parallel()
	for name, line := range map[string]string{
		"just past uint64": "    wide: 18446744073709551617\n",
		"just below int64": "    wide: -9223372036854775809\n",
		"nested in a list": "    nested:\n        list: [1, 99999999999999999999999]\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeOwnedRepo(t)
			looseEvent(t, root, "EV-aaaaaa", "add.task", "2026-11-03T10:00:00Z")
			wide := looseEventWithAfter(t, root, "EV-bbbbbb", "2026-11-03T11:00:00Z", line)
			exact, err := os.ReadFile(wide)
			require.NoError(t, err)
			before := listEvents(t, root)

			res, err := mutate.Compact(root, mutate.CompactOptions{Now: at(t, "2026-11-11T12:00:00Z")})

			require.NoError(t, err)
			require.Equal(t, 1, res.Skipped)
			require.Len(t, res.Days, 1)
			require.Equal(t, 1, res.Days[0].Events)
			require.Equal(t, []string{"EV-aaaaaa"}, segmentIDs(t, root, res.Days[0].Segment))
			kept, err := os.ReadFile(wide)
			require.NoError(t, err, "the loose file is still there")
			require.Equal(t, string(exact), string(kept))
			require.NoFileExists(t, filepath.Join(root, ".tiller/events/2026/11/03/EV-aaaaaa.yml"))
			requireSameEvents(t, before, listEvents(t, root))
		})
	}
}

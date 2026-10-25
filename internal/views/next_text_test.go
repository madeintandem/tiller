package views_test

import (
	"testing"

	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
)

// Sentence capitalizes a plain leading word but never a leading node key,
// and ends with exactly one period, unless it already ends a question or an
// exclamation.
func TestSentenceLeavesALeadingKeyAlone(t *testing.T) {
	for in, want := range map[string]string{
		"no work is planned yet":            "No work is planned yet.",
		"in review, waiting on your merge.": "In review, waiting on your merge.",
		"checkout/alpha/t1 (T-x) is ready":  "checkout/alpha/t1 (T-x) is ready.",
		"rides-alone review":                "rides-alone review.",
		"2 more remain..":                   "2 more remain.",
		"holds it; Which schema?":           "Holds it; Which schema?",
		"really!":                           "Really!",
		"   ":                               "",
	} {
		require.Equal(t, want, views.Sentence(in), "input %q", in)
	}
}

// Two predicates answer "has this prose been written yet?", and these tests
// pin the difference as well as the agreement. IsProseStub is the lifecycle
// guards' rule, the sentinel still being in the file. IsProseUnwritten is the
// console's, strictly narrower, because the page draws none of what it calls a
// stub. Both are pinned to the text `tiller add` actually generates by feeding
// renderEpicMD/renderFeatureMD/renderTaskMD's own output back in, rather than
// retyping the sentinel and testing the retyped copy.

package mutate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/madeintandem/tiller/internal/store"
)

func TestIsProseStubRecognizesWhatAddGenerates(t *testing.T) {
	t.Parallel()
	assert.True(t, IsProseStub(store.NounFeature, renderFeatureMD("Selection Policy and Counters")))
	assert.True(t, IsProseStub(store.NounEpic, renderEpicMD("Audit Engine")))
	assert.True(t, IsProseStub(store.NounTask, renderTaskMD("Wire the counter reset")))
}

func TestIsProseStubOnWrittenProse(t *testing.T) {
	t.Parallel()
	written := "# Audit Engine\n\n## Intent\n\nRun QA continuously from phase submission.\n"
	assert.False(t, IsProseStub(store.NounEpic, written))
	assert.False(t, IsProseStub(store.NounFeature, written))
}

func TestIsProseStubOnBlankProse(t *testing.T) {
	t.Parallel()
	// Emptied is no more written than generated, the same rule mdIsStub has
	// always applied to a file it could read.
	assert.True(t, IsProseStub(store.NounFeature, ""))
	assert.True(t, IsProseStub(store.NounEpic, "   \n\n\t"))
	assert.True(t, IsProseStub(store.NounTask, ""))
}

func TestIsProseStubOnANounWithNoGeneratedStub(t *testing.T) {
	t.Parallel()
	// A question is a ledger entry with no prose file at all, so there is no
	// sentinel to find and nothing to claim about it.
	assert.False(t, IsProseStub(store.NounQuestion, ""))
	assert.False(t, IsProseStub(store.NounQuestion, renderFeatureMD("x")))
}

func TestIsProseStubIsNounScoped(t *testing.T) {
	t.Parallel()
	// An epic.md carrying the feature sentinel is not an epic stub: the markers
	// are separate strings and the noun picks which one applies.
	assert.False(t, IsProseStub(store.NounEpic, "# X\n\n"+featureStubMarker+" and more.\n"))
	assert.False(t, IsProseStub(store.NounFeature, "# X\n\n"+epicStubMarker+" and more.\n"))
	assert.False(t, IsProseStub(store.NounTask, "# X\n\n"+featureStubMarker+" and more.\n"))
}

// TestIsProseUnwrittenAgreesOnWhatAddGenerates: on the two files that matter
// most, a fresh stub and a written spec, the strict predicate answers exactly
// what the guards' predicate answers.
func TestIsProseUnwrittenAgreesOnWhatAddGenerates(t *testing.T) {
	t.Parallel()
	assert.True(t, IsProseUnwritten(store.NounFeature, renderFeatureMD("Selection Policy and Counters")))
	assert.True(t, IsProseUnwritten(store.NounEpic, renderEpicMD("Audit Engine")))
	assert.True(t, IsProseUnwritten(store.NounTask, renderTaskMD("Wire the counter reset")))
	assert.True(t, IsProseUnwritten(store.NounFeature, ""))
	assert.True(t, IsProseUnwritten(store.NounEpic, "   \n\n\t"))

	written := "# Audit Engine\n\n## Intent\n\nRun QA continuously from phase submission.\n"
	assert.False(t, IsProseUnwritten(store.NounEpic, written))
	assert.False(t, IsProseUnwritten(store.NounQuestion, ""))
}

// TestIsProseUnwrittenOnASpecWrittenIntoTheStub is the regression: filling in
// the generated headings and leaving the intro paragraph is the ordinary way
// this file gets written, and it made the console draw "No spec written yet"
// over 972 bytes of real spec with no way to reach them. The guards keep
// refusing on the leftover sentinel, which is a different and correct answer.
func TestIsProseUnwrittenOnASpecWrittenIntoTheStub(t *testing.T) {
	t.Parallel()
	spec := renderFeatureMD("Multi-Program Tenancy") +
		"\nEvery row carries a program_id. We deliberately do NOT use\n" +
		"schema-per-tenant: one schema, one scoped repository.\n"

	assert.False(t, IsProseUnwritten(store.NounFeature, spec), "written prose is not a stub")
	assert.True(t, IsProseStub(store.NounFeature, spec), "the gate still sees its sentinel")
}

// TestIsProseUnwrittenIgnoresScaffolding pins what does NOT count as writing:
// the title, the generated paragraph, and bare headings, including a heading
// the writer added and has not filled in yet.
func TestIsProseUnwrittenIgnoresScaffolding(t *testing.T) {
	t.Parallel()
	assert.True(t, IsProseUnwritten(store.NounFeature,
		renderFeatureMD("Selection Policy")+"\n## Open questions\n\n### Later\n"))
	// One line under one heading is writing, however short.
	assert.False(t, IsProseUnwritten(store.NounFeature,
		renderFeatureMD("Selection Policy")+"\n## Open questions\n\nWhich counter resets nightly?\n"))
}

// TestIsProseUnwrittenIsNounScoped keeps the strict predicate on the same
// noun-scoped footing: a noun that generates no stub has nothing to detect.
func TestIsProseUnwrittenIsNounScoped(t *testing.T) {
	t.Parallel()
	assert.False(t, IsProseUnwritten(store.NounEpic, renderFeatureMD("x")))
	assert.False(t, IsProseUnwritten(store.NounTask, renderFeatureMD("x")))
	assert.False(t, IsProseUnwritten(store.NounQuestion, ""))
}

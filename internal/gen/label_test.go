package gen

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// labelTree builds a small in-memory tree with one node of every noun, so the
// reference helper can be exercised across the whole convention table.
func labelTree() *store.Tree {
	epicKey := "form-engine"
	featKey := "form-engine/certification-form-loading"
	taskKey := "form-engine/certification-form-loading/schema-transcription"
	amendKey := "iphone-only"
	return &store.Tree{
		Epics: map[string]*store.Epic{
			"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &featKey, Status: "active", EpicKey: "form-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-aaaaaa": {ID: "T-aaaaaa", Key: &taskKey, Status: "in_progress", FeatureID: "F-aaaaaa"},
			"T-nokey":  {ID: "T-nokey", Status: "proposed", FeatureID: "F-aaaaaa"},
		},
		Questions: map[string]*store.Question{
			"Q-aaaaaa": {ID: "Q-aaaaaa", Statement: "Does audit count projects or phases?", Status: "open"},
		},
		Amendments: map[string]*store.Amendment{
			"A-aaaaaa": {ID: "A-aaaaaa", Key: &amendKey, Status: store.AmendmentStatusProposed},
		},
	}
}

// TestLabelIsTheKeyPath pins the key-path half of the reference convention: a
// keyed node's label is its own canonical key, never an ancestor's, and a node
// with no key has no label at all.
func TestLabelIsTheKeyPath(t *testing.T) {
	tree := labelTree()

	require.Equal(t, "form-engine", Label(tree, tree.Epics["E-aaaaaa"]))
	require.Equal(t, "form-engine/certification-form-loading", Label(tree, tree.Features["F-aaaaaa"]))
	require.Equal(t, "form-engine/certification-form-loading/schema-transcription", Label(tree, tree.Tasks["T-aaaaaa"]))
	require.Equal(t, "iphone-only", Label(tree, tree.Amendments["A-aaaaaa"]))

	// No key: a question (never keyed) and a task whose task.yml carries none
	// both label as empty, leaving Ref to fall back to the bare id rather than
	// borrowing a path that resolves to the parent feature.
	require.Equal(t, "", Label(tree, tree.Questions["Q-aaaaaa"]))
	require.Equal(t, "", Label(tree, tree.Tasks["T-nokey"]))
}

// TestRefIsKeyPathWithID pins the full human reference: "<key-path> (<id>)" for
// anything with a path, the bare id otherwise. The degenerate "T-x (T-x)" the
// hand-rolled fallbacks used to produce is impossible by construction.
func TestRefIsKeyPathWithID(t *testing.T) {
	tree := labelTree()

	require.Equal(t, "form-engine (E-aaaaaa)", Ref(tree, tree.Epics["E-aaaaaa"]))
	require.Equal(t, "form-engine/certification-form-loading (F-aaaaaa)", Ref(tree, tree.Features["F-aaaaaa"]))
	require.Equal(t, "form-engine/certification-form-loading/schema-transcription (T-aaaaaa)", Ref(tree, tree.Tasks["T-aaaaaa"]))
	require.Equal(t, "iphone-only (A-aaaaaa)", Ref(tree, tree.Amendments["A-aaaaaa"]))

	// Bare id, never "id (id)".
	require.Equal(t, "Q-aaaaaa", Ref(tree, tree.Questions["Q-aaaaaa"]))
	require.Equal(t, "T-nokey", Ref(tree, tree.Tasks["T-nokey"]))
}

// TestRefHandlesUnkeyedFolderNodes covers the fallback arm for a folder-backed
// node whose key is missing (nil or empty): the reference degrades to the bare
// id rather than an empty label.
func TestRefHandlesUnkeyedFolderNodes(t *testing.T) {
	tree := labelTree()
	empty := ""
	tree.Epics["E-nokey"] = &store.Epic{ID: "E-nokey", Status: "draft"}
	tree.Epics["E-empty"] = &store.Epic{ID: "E-empty", Key: &empty, Status: "draft"}

	require.Equal(t, "E-nokey", Ref(tree, tree.Epics["E-nokey"]))
	require.Equal(t, "E-empty", Ref(tree, tree.Epics["E-empty"]))
}

// TestRefByIDResolvesAcrossTheTree pins the by-id variant the render sites use
// when they hold only an id (sync crossings, depends_on lists): every noun's map
// is consulted, and an id the tree does not know comes back unchanged.
func TestRefByIDResolvesAcrossTheTree(t *testing.T) {
	tree := labelTree()

	require.Equal(t, "form-engine (E-aaaaaa)", RefByID(tree, "E-aaaaaa"))
	require.Equal(t, "form-engine/certification-form-loading (F-aaaaaa)", RefByID(tree, "F-aaaaaa"))
	require.Equal(t, "form-engine/certification-form-loading/schema-transcription (T-aaaaaa)", RefByID(tree, "T-aaaaaa"))
	require.Equal(t, "Q-aaaaaa", RefByID(tree, "Q-aaaaaa"))
	require.Equal(t, "iphone-only (A-aaaaaa)", RefByID(tree, "A-aaaaaa"),
		"an amendment resolves the same way every other noun's map does")
	require.Equal(t, "F-gone", RefByID(tree, "F-gone"))
}

// TestRefParts pins the formatting primitive result-struct renderers use when
// they already carry both halves: key path plus id, or the bare id when the
// path is empty.
func TestRefParts(t *testing.T) {
	require.Equal(t, "form-engine/certification-form-loading (F-aaaaaa)", RefParts("form-engine/certification-form-loading", "F-aaaaaa"))
	require.Equal(t, "T-aaaaaa", RefParts("", "T-aaaaaa"))
}

func TestQuestionTitleUsesExplicitTitleWithoutMutation(t *testing.T) {
	q := &store.Question{
		ID:        "Q-aaaaaa",
		Title:     "Canonical schema",
		Statement: "Which schema version is canonical?",
	}
	before := *q

	require.Equal(t, "Canonical schema", QuestionTitle(q))
	require.Equal(t, before, *q)
}

func TestQuestionTitleCollapsesLegacyWhitespaceWithoutMutation(t *testing.T) {
	q := &store.Question{
		ID:        "Q-legacy",
		Statement: "  Which\u2003schema\nversion\tis canonical?  ",
	}
	before := *q

	require.Equal(t, "Which schema version is canonical?", QuestionTitle(q))
	require.Equal(t, before, *q)
}

func TestQuestionTitleTruncatesLegacyTextToEightyUnicodeCodePoints(t *testing.T) {
	q := &store.Question{
		ID:        "Q-legacy",
		Statement: strings.Repeat("界", 81),
	}
	before := *q

	got := QuestionTitle(q)
	require.Equal(t, strings.Repeat("界", 79)+"…", got)
	require.Equal(t, 80, utf8.RuneCountInString(got))
	require.Equal(t, before, *q)
}

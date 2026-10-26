package resolve

import (
	"errors"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyPtr returns a pointer to s, the shape store uses for a node's optional key.
func keyPtr(s string) *string { return &s }

// fixtureTree builds an in-memory tree mirroring the canonical example shapes:
// one keyed epic, one keyed feature whose canonical key carries the epic
// segment, one keyed task whose canonical key carries both, and the one ledger
// item, a question, which carries an id but no key (artifact-schema.md,
// References and IDs).
func fixtureTree() *store.Tree {
	return &store.Tree{
		Root: "/does/not/matter",
		Epics: map[string]*store.Epic{
			"E-j8m4q2": {ID: "E-j8m4q2", Key: keyPtr("form-engine")},
		},
		Features: map[string]*store.Feature{
			"F-79dh3q": {ID: "F-79dh3q", Key: keyPtr("form-engine/certification-form-loading"), EpicKey: "form-engine"},
		},
		Tasks: map[string]*store.Task{
			"T-k3p9w6": {
				ID:        "T-k3p9w6",
				Key:       keyPtr("form-engine/certification-form-loading/schema-transcription"),
				FeatureID: "F-79dh3q",
			},
		},
		Questions: map[string]*store.Question{
			"Q-h2n7m5": {ID: "Q-h2n7m5"},
		},
		Amendments: map[string]*store.Amendment{
			"A-d6w3f9": {ID: "A-d6w3f9", Key: keyPtr("iphone-only")},
		},
	}
}

// Id lookup resolves any addressable node, ledger items included
// (cli-surface.md, "Id lookup resolves any addressable node").
func TestResolve_IdLookupResolvesAnyNode(t *testing.T) {
	r := NewResolver(fixtureTree())

	cases := []struct {
		noun string
		id   string
	}{
		{store.NounEpic, "E-j8m4q2"},
		{store.NounFeature, "F-79dh3q"},
		{store.NounTask, "T-k3p9w6"},
		{store.NounQuestion, "Q-h2n7m5"},
		{store.NounAmendment, "A-d6w3f9"},
	}
	for _, c := range cases {
		node, err := r.Resolve(Ref{Noun: c.noun, Raw: c.id})
		require.NoError(t, err, "id %s should resolve", c.id)
		assert.Equal(t, c.id, node.NodeID())
		assert.Equal(t, c.noun, node.Noun())
	}
}

// An id lookup with an empty noun skips the checksum and still resolves the
// node, because the id alone is canonical (cli-surface.md, address resolution).
func TestResolve_IdLookupNounAgnostic(t *testing.T) {
	r := NewResolver(fixtureTree())
	node, err := r.Resolve(Ref{Raw: "T-k3p9w6"})
	require.NoError(t, err)
	assert.Equal(t, "T-k3p9w6", node.NodeID())
}

// Key lookup resolves a feature by its canonical <epic-key>/<feature-key>
// (cli-surface.md, "A feature's canonical key includes the epic segment").
func TestResolve_FeatureByCanonicalKey(t *testing.T) {
	r := NewResolver(fixtureTree())
	node, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "form-engine/certification-form-loading"})
	require.NoError(t, err)
	assert.Equal(t, "F-79dh3q", node.NodeID())
}

// The local feature segment alone is not the canonical key, so it does not
// resolve. This locks the epic-segment requirement.
func TestResolve_FeatureLocalSegmentDoesNotResolve(t *testing.T) {
	r := NewResolver(fixtureTree())
	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "certification-form-loading"})
	var nf *ErrNotFound
	require.ErrorAs(t, err, &nf)
}

// Key lookup also resolves the other keyed noun, epic.
func TestResolve_EpicByKey(t *testing.T) {
	r := NewResolver(fixtureTree())

	epic, err := r.Resolve(Ref{Noun: store.NounEpic, Raw: "form-engine"})
	require.NoError(t, err)
	assert.Equal(t, "E-j8m4q2", epic.NodeID())
}

// Key lookup also resolves an amendment, a keyed but not folder-backed node
// (cli-surface.md, "the keyed nouns [...] amendment, adr").
func TestResolve_AmendmentByKey(t *testing.T) {
	r := NewResolver(fixtureTree())

	amendment, err := r.Resolve(Ref{Noun: store.NounAmendment, Raw: "iphone-only"})
	require.NoError(t, err)
	assert.Equal(t, "A-d6w3f9", amendment.NodeID())
	assert.Equal(t, store.NounAmendment, amendment.Noun())
}

// Two amendments sharing a key with neither of them open is an ambiguity the
// caller must see, the same bad-merge ambiguity two features sharing a key is.
// The fixture's own amendment carries no status, which is the has-no-open-holder
// tier, so this is the case the open-proposal preference deliberately does not
// rescue.
func TestResolve_AmbiguousAmendmentKey(t *testing.T) {
	tree := fixtureTree()
	tree.Amendments["A-aaaaaa"] = &store.Amendment{ID: "A-aaaaaa", Key: keyPtr("iphone-only")}
	r := NewResolver(tree)

	_, err := r.Resolve(Ref{Noun: store.NounAmendment, Raw: "iphone-only"})
	var ak *ErrAmbiguousKey
	require.ErrorAs(t, err, &ak)
	require.Len(t, ak.Candidates, 2)
	assert.ElementsMatch(t, []string{"A-d6w3f9", "A-aaaaaa"},
		[]string{ak.Candidates[0].NodeID(), ak.Candidates[1].NodeID()})
}

// An amendment key can be carried by more than one amendment legitimately, and
// that is what separates this noun from every other keyed one: mutate refuses a
// duplicate key only among PROPOSED amendments, so withdrawing one and opening
// another under the same key is a supported move. Status-blind resolution turned
// that move into a permanently unaddressable key, including in the readout's own
// copyable `tiller approve amendment <key>` row. The key resolves to the open
// proposal instead.
func TestResolve_AmendmentKeyPrefersTheOpenProposal(t *testing.T) {
	tree := fixtureTree()
	tree.Amendments["A-d6w3f9"].Status = store.AmendmentStatusWithdrawn
	tree.Amendments["A-bbbbbb"] = &store.Amendment{
		ID: "A-bbbbbb", Key: keyPtr("iphone-only"), Status: store.AmendmentStatusProposed,
	}
	r := NewResolver(tree)

	node, err := r.Resolve(Ref{Noun: store.NounAmendment, Raw: "iphone-only"})
	require.NoError(t, err, "the reused key addresses the amendment still open")
	assert.Equal(t, "A-bbbbbb", node.NodeID())

	// The retired holder keeps its own address, which is how a human reads the
	// proposal that was dropped.
	node, err = r.Resolve(Ref{Noun: store.NounAmendment, Raw: "A-d6w3f9"})
	require.NoError(t, err)
	assert.Equal(t, "A-d6w3f9", node.NodeID())
}

// With no open holder the terminal tier answers, so a key a single approved or
// withdrawn amendment carries still resolves. Without this the preference would
// have made every retired amendment key-unaddressable, trading one dead end for
// another.
func TestResolve_AmendmentKeyResolvesASoleRetiredHolder(t *testing.T) {
	tree := fixtureTree()
	tree.Amendments["A-d6w3f9"].Status = store.AmendmentStatusApproved
	r := NewResolver(tree)

	node, err := r.Resolve(Ref{Noun: store.NounAmendment, Raw: "iphone-only"})
	require.NoError(t, err)
	assert.Equal(t, "A-d6w3f9", node.NodeID())
}

// Two OPEN proposals under one key stay ambiguous, and the candidate list
// narrows to that tier. mutate refuses the second open one, so reaching this
// state takes a bad merge or a hand edit; when it happens, the two ids a caller
// has to choose between are the open ones, and naming a withdrawn holder
// alongside them would point at an amendment nothing can be staged onto.
func TestResolve_AmendmentKeyAmbiguousAmongOpenProposals(t *testing.T) {
	tree := fixtureTree()
	tree.Amendments["A-d6w3f9"].Status = store.AmendmentStatusWithdrawn
	for _, id := range []string{"A-bbbbbb", "A-cccccc"} {
		tree.Amendments[id] = &store.Amendment{
			ID: id, Key: keyPtr("iphone-only"), Status: store.AmendmentStatusProposed,
		}
	}
	r := NewResolver(tree)

	_, err := r.Resolve(Ref{Noun: store.NounAmendment, Raw: "iphone-only"})
	var ak *ErrAmbiguousKey
	require.ErrorAs(t, err, &ak)
	require.Len(t, ak.Candidates, 2, "the withdrawn holder is not one of the choices")
	assert.ElementsMatch(t, []string{"A-bbbbbb", "A-cccccc"},
		[]string{ak.Candidates[0].NodeID(), ak.Candidates[1].NodeID()})
}

// The preference is confined to the amendment arm. Two features sharing a key is
// a corrupt tree with no legitimate reading, so nothing about their statuses may
// pick a winner: that ambiguity stays an ambiguity.
func TestResolve_FeatureKeyAmbiguityIsNotStatusResolved(t *testing.T) {
	tree := fixtureTree()
	tree.Features["F-aaaaaa"] = &store.Feature{
		ID: "F-aaaaaa", Key: keyPtr("form-engine/certification-form-loading"), EpicKey: "form-engine", Status: "draft",
	}
	tree.Features["F-79dh3q"].Status = "cancelled"
	r := NewResolver(tree)

	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "form-engine/certification-form-loading"})
	var ak *ErrAmbiguousKey
	require.ErrorAs(t, err, &ak)
	require.Len(t, ak.Candidates, 2)
}

// The question ledger item is id-only, so a key lookup returns ErrNotFound
// (cli-surface.md, "The one ledger item, question, has no key").
func TestResolve_LedgerKeyLookupNotFound(t *testing.T) {
	r := NewResolver(fixtureTree())

	_, err := r.Resolve(Ref{Noun: store.NounQuestion, Raw: "some-question-key"})
	var nf *ErrNotFound
	require.ErrorAs(t, err, &nf, "a question key lookup is ErrNotFound")
}

// A task key that names no task is ErrNotFound, and so is the local task
// segment alone: a task's canonical key carries both ancestor segments, the same
// rule the feature one level up follows.
func TestResolve_TaskLocalSegmentDoesNotResolve(t *testing.T) {
	r := NewResolver(fixtureTree())

	_, err := r.Resolve(Ref{Noun: store.NounTask, Raw: "schema-transcription"})
	var nf *ErrNotFound
	require.ErrorAs(t, err, &nf)

	_, err = r.Resolve(Ref{Noun: store.NounTask, Raw: "certification-form-loading/schema-transcription"})
	require.ErrorAs(t, err, &nf)
}

// A task key that resolves under the wrong noun is the same checksum failure a
// mismatched feature key is.
func TestResolve_TaskKeyUnderAnotherNounIsNotFound(t *testing.T) {
	r := NewResolver(fixtureTree())
	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "form-engine/certification-form-loading/schema-transcription"})
	var nf *ErrNotFound
	require.ErrorAs(t, err, &nf)
}

// Two tasks sharing a canonical key is the same bad-merge ambiguity two features
// sharing one is, and it carries the same candidate list.
func TestResolve_AmbiguousTaskKey(t *testing.T) {
	tree := fixtureTree()
	tree.Tasks["T-aaaaaa"] = &store.Task{
		ID:        "T-aaaaaa",
		Key:       keyPtr("form-engine/certification-form-loading/schema-transcription"),
		FeatureID: "F-79dh3q",
	}
	r := NewResolver(tree)

	_, err := r.Resolve(Ref{Noun: store.NounTask, Raw: "form-engine/certification-form-loading/schema-transcription"})
	var ak *ErrAmbiguousKey
	require.ErrorAs(t, err, &ak)
	require.Len(t, ak.Candidates, 2)
	assert.ElementsMatch(t, []string{"T-k3p9w6", "T-aaaaaa"},
		[]string{ak.Candidates[0].NodeID(), ak.Candidates[1].NodeID()})
	assert.Equal(t, store.NounTask, ak.Noun)
}

// Key lookup resolves a task by its canonical three-segment key
// (cli-surface.md, "A task's adds its own: <epic-key>/<feature-key>/<task-key>").
func TestResolveAnswersTheThreeSegmentTaskKey(t *testing.T) {
	r := NewResolver(fixtureTree())
	node, err := r.Resolve(Ref{Noun: store.NounTask, Raw: "form-engine/certification-form-loading/schema-transcription"})
	require.NoError(t, err)
	assert.Equal(t, "T-k3p9w6", node.NodeID())
	assert.Equal(t, store.NounTask, node.Noun())
}

// A noun that mismatches the resolved node is the same checksum failure as a
// mismatched id prefix (cli-surface.md, "A noun that mismatches ... exits 2").
func TestResolve_NounMismatch(t *testing.T) {
	r := NewResolver(fixtureTree())
	_, err := r.Resolve(Ref{Noun: store.NounEpic, Raw: "F-79dh3q"})
	var nm *ErrNounMismatch
	require.ErrorAs(t, err, &nm)
	assert.Equal(t, store.NounEpic, nm.Requested)
	assert.Equal(t, store.NounFeature, nm.Actual)
	assert.Equal(t, "F-79dh3q", nm.ID)
}

// An ambiguous key lookup carries the candidate matches so a caller can
// disambiguate by id (cli-surface.md, "An ambiguous key lookup exits 2 with the
// candidate matches in the JSON error"). Two features sharing a canonical key
// is the bad-merge case the IDs section names.
func TestResolve_AmbiguousKey(t *testing.T) {
	tree := fixtureTree()
	tree.Features["F-aaaaaa"] = &store.Feature{
		ID:      "F-aaaaaa",
		Key:     keyPtr("form-engine/certification-form-loading"),
		EpicKey: "form-engine",
	}
	r := NewResolver(tree)

	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "form-engine/certification-form-loading"})
	var ak *ErrAmbiguousKey
	require.ErrorAs(t, err, &ak)
	require.Len(t, ak.Candidates, 2, "ambiguity carries every candidate")

	ids := []string{ak.Candidates[0].NodeID(), ak.Candidates[1].NodeID()}
	assert.ElementsMatch(t, []string{"F-79dh3q", "F-aaaaaa"}, ids)
	assert.Equal(t, "form-engine/certification-form-loading", ak.Key)
	assert.Equal(t, store.NounFeature, ak.Noun)
}

// The candidate list is gathered by ranging a map and reaches the human as the
// id list in the error message, so it is sorted: the same ambiguity reports the
// same bytes on every run.
func TestResolve_AmbiguousCandidatesAreOrderedByID(t *testing.T) {
	tree := fixtureTree()
	for _, id := range []string{"F-zzzzzz", "F-aaaaaa", "F-mmmmmm"} {
		tree.Features[id] = &store.Feature{ID: id, Key: keyPtr("form-engine/certification-form-loading"), EpicKey: "form-engine"}
	}
	r := NewResolver(tree)

	var first string
	for i := 0; i < 20; i++ {
		_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "form-engine/certification-form-loading"})
		var ak *ErrAmbiguousKey
		require.ErrorAs(t, err, &ak)
		ids := make([]string, len(ak.Candidates))
		for j, c := range ak.Candidates {
			ids[j] = c.NodeID()
		}
		assert.Equal(t, []string{"F-79dh3q", "F-aaaaaa", "F-mmmmmm", "F-zzzzzz"}, ids)
		if i == 0 {
			first = err.Error()
			continue
		}
		require.Equal(t, first, err.Error(), "the message is the same bytes every run")
	}
}

// An id-shaped address that matches nothing is ErrNotFound, not a panic.
func TestResolve_UnknownIdNotFound(t *testing.T) {
	r := NewResolver(fixtureTree())
	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "F-zzzzzz"})
	var nf *ErrNotFound
	require.ErrorAs(t, err, &nf)
}

// The typed errors are distinct: an ErrNotFound is not an ErrNounMismatch.
func TestResolve_ErrorsAreDistinct(t *testing.T) {
	r := NewResolver(fixtureTree())
	_, err := r.Resolve(Ref{Noun: store.NounFeature, Raw: "F-zzzzzz"})

	var nm *ErrNounMismatch
	assert.False(t, errors.As(err, &nm))
	var ak *ErrAmbiguousKey
	assert.False(t, errors.As(err, &ak))
}

// The resolver answers a shelf node by id, by its canonical key (bugs/<k>),
// and by the bare local key alone, since the noun already names the shelf
// (docs/plans/2026-08-28-bugs-and-chores-design.md, Verbs).
func TestResolveShelfNounsByIDCanonicalAndLocalKey(t *testing.T) {
	key := "bugs/blank-on-reload"
	tree := &store.Tree{
		Epics: map[string]*store.Epic{}, Features: map[string]*store.Feature{}, Tasks: map[string]*store.Task{},
		Questions: map[string]*store.Question{}, Amendments: map[string]*store.Amendment{},
		Bugs:   map[string]*store.Bug{"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &key}}},
		Chores: map[string]*store.Chore{},
	}
	r := NewResolver(tree)
	for _, raw := range []string{"B-aaaaaa", "bugs/blank-on-reload", "blank-on-reload"} {
		n, err := r.Resolve(Ref{Noun: store.NounBug, Raw: raw})
		require.NoError(t, err, raw)
		require.Equal(t, "B-aaaaaa", n.NodeID())
	}
	_, err := r.Resolve(Ref{Noun: store.NounChore, Raw: "B-aaaaaa"})
	var mismatch *ErrNounMismatch
	require.ErrorAs(t, err, &mismatch)
	_, err = r.Resolve(Ref{Noun: store.NounTask, Raw: "blank-on-reload"})
	var notFound *ErrNotFound
	require.ErrorAs(t, err, &notFound)
}

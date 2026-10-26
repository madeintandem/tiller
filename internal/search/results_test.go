package search_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madeintandem/tiller/internal/search"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

// run searches with no kind filter and the default limit, the shape most
// tests want.
func run(t *testing.T, root string, tree *store.Tree, query string) search.Result {
	t.Helper()
	res, err := search.Search(root, tree, query, search.Options{})
	require.NoError(t, err)
	return res
}

// nodeLabels names each node result by id and how it matched, the two facts
// the ranking tests pin.
func nodeLabels(res search.Result) []string {
	out := []string{}
	for _, n := range res.Nodes {
		out = append(out, n.ID+" "+n.Match)
	}
	return out
}

// TestAnExactIDLeadsWithNoProseNeeded is the defect the redesign answers:
// pasting a node's id used to return nothing at all, because ids appear in no
// title and rarely in prose. An id, its bare suffix, and its full reference
// all find the node, in any case, as an id match carrying no text hits.
func TestAnExactIDLeadsWithNoProseNeeded(t *testing.T) {
	root, tree := searchShelfFixture(t)
	for _, q := range []string{"B-aaaaaa", "b-AAAAAA", "bugs/blank-on-reload (B-aaaaaa)"} {
		res := run(t, root, tree, q)
		require.Equal(t, []string{"B-aaaaaa id"}, nodeLabels(res), "query %q", q)
		require.Equal(t, search.NodeResult{
			Noun: "bug", ID: "B-aaaaaa", Key: "bugs/blank-on-reload", Title: "Blank on reload",
			Status: "proposed", Ref: "bugs/blank-on-reload (B-aaaaaa)", Match: "id", Count: 0,
			Hits: []search.Hit{},
		}, res.Nodes[0])
	}
	require.Equal(t, []string{"TW-cccccc id"}, nodeLabels(run(t, root, tree, "cccccc")),
		"the id's suffix alone is still that id")
}

// TestAKeyMatchesExactlyOrAsAFragment pins key lookup: the full key or its
// local segment is an exact key match and leads, while a key that merely
// contains the query is a weaker key match behind it.
func TestAKeyMatchesExactlyOrAsAFragment(t *testing.T) {
	root, tree := searchShelfFixture(t)
	require.Equal(t, []string{"B-aaaaaa key"}, nodeLabels(run(t, root, tree, "bugs/blank-on-reload")))
	require.Equal(t, []string{"B-aaaaaa key"}, nodeLabels(run(t, root, tree, "blank-on-reload")))

	res := run(t, root, tree, "certification-form-loading")
	require.Equal(t, []string{"F-aaaaaa key", "T-aaaaaa key_fragment"}, nodeLabels(res)[:2],
		"the feature's own key outranks the task key that contains it")
}

// TestTitleMatchesOutrankKeyFragmentsAndText pins the tiers below exact
// lookup: a title that starts with the query, then a title that contains it,
// then a key fragment, then text alone. A document whose path names the query
// is grouped apart from the nodes.
func TestTitleMatchesOutrankKeyFragmentsAndText(t *testing.T) {
	root, tree := fixture(t)
	res := run(t, root, tree, "form")
	require.Equal(t, []string{"E-aaaaaa title", "F-aaaaaa title", "T-aaaaaa key_fragment"}, nodeLabels(res))
	require.Len(t, res.Documents, 1)
	require.Equal(t, ".tiller/design/01-form-shapes.md", res.Documents[0].Path)
}

// TestTextOnlyNodesRankByHowOftenTheyMatch: among nodes that matched only in
// their prose, the one that says the term more often leads.
func TestTextOnlyNodesRankByHowOftenTheyMatch(t *testing.T) {
	root, tree := fixture(t)
	writeLines(t, filepath.Join(root, ".tiller", "epics", "form-engine", "notes.md"), "ocelot %d", 1)
	writeLines(t, filepath.Join(root, ".tiller", "epics", "form-engine", "features", "certification-form-loading", "notes.md"), "ocelot %d", 4)
	res := run(t, root, tree, "ocelot")
	require.Equal(t, []string{"F-aaaaaa text", "E-aaaaaa text"}, nodeLabels(res))
	require.Equal(t, 4, res.Nodes[0].Count)
}

// TestANodeIsOneResultWithCappedHits: every match inside one node folds into
// that node's single result. Count is the whole tally and Hits the first few
// lines, so one chatty node cannot fill the page.
func TestANodeIsOneResultWithCappedHits(t *testing.T) {
	root, tree := fixture(t)
	writeLines(t, filepath.Join(root, ".tiller", "epics", "form-engine", "notes.md"), "marmoset %d", 5)
	res := run(t, root, tree, "marmoset")
	require.Equal(t, 1, res.Count)
	require.Len(t, res.Nodes, 1)
	require.Equal(t, 5, res.Nodes[0].Count)
	require.Len(t, res.Nodes[0].Hits, search.MaxNodeHits)
	for i, h := range res.Nodes[0].Hits {
		require.Equal(t, ".tiller/epics/form-engine/notes.md", h.Path)
		require.Equal(t, i+1, h.Line)
	}
	require.Empty(t, res.Documents)
}

// TestQuestionFieldsAreHitsOnTheQuestion: the title is the result's own
// label, so it ranks the question without repeating as a hit, while a
// matching statement and recorded answer each appear as a field hit on the
// ledger, statement first.
func TestQuestionFieldsAreHitsOnTheQuestion(t *testing.T) {
	root, tree := fixture(t)
	answer := "The 2025 revision, per the county memo."
	for _, qn := range tree.Questions {
		qn.Answer = &answer
		qn.Status = "answered"
	}
	res := run(t, root, tree, "revision")
	require.Equal(t, []string{"Q-aaaaaa title"}, nodeLabels(res))
	q := res.Nodes[0]
	require.Equal(t, "Canonical certification revision", q.Title)
	require.Equal(t, 2, q.Count, "the title ranks the question; it is not one of its hits")
	require.Equal(t, []search.Hit{
		{Path: ".tiller/decisions/open-questions.yml", Snippet: "Which certification revision is canonical?", Field: "statement"},
		{Path: ".tiller/decisions/open-questions.yml", Snippet: "The 2025 revision, per the county memo.", Field: "answer"},
	}, q.Hits)

	res = run(t, root, tree, "county memo")
	require.Equal(t, []string{"Q-aaaaaa text"}, nodeLabels(res))
}

// TestDocumentsGroupByFile: prose nobody owns groups one result per file. A
// file whose path names the query leads, then files by how often they match.
func TestDocumentsGroupByFile(t *testing.T) {
	root, tree := fixture(t)
	writeFlood(t, root, "02-alpha.md", "zebra %d", 4)
	writeFlood(t, root, "03-beta.md", "zebra %d", 1)
	writeFlood(t, root, "04-zebra.md", "zebra %d", 1)
	res := run(t, root, tree, "zebra")
	require.Empty(t, res.Nodes)
	var paths []string
	for _, d := range res.Documents {
		paths = append(paths, d.Path)
	}
	require.Equal(t, []string{
		".tiller/design/04-zebra.md", ".tiller/design/02-alpha.md", ".tiller/design/03-beta.md",
	}, paths)
	require.Equal(t, 4, res.Documents[1].Count)
	require.Len(t, res.Documents[1].Hits, search.MaxNodeHits)
	require.Equal(t, 3, res.Count)
}

// TestKindsCountEveryResultAndTheFilterNarrows: kinds always tallies the
// whole answer, so the console's chips stay honest while a kind filter
// narrows the page (and Count) to one kind.
func TestKindsCountEveryResultAndTheFilterNarrows(t *testing.T) {
	root, tree := fixture(t)
	all := []search.KindCount{{Kind: "epic", Count: 1}, {Kind: "feature", Count: 1}, {Kind: "task", Count: 1}, {Kind: "document", Count: 1}}

	res := run(t, root, tree, "form")
	require.Equal(t, all, res.Kinds)
	require.Equal(t, 4, res.Count)

	res, err := search.Search(root, tree, "form", search.Options{Kind: "task"})
	require.NoError(t, err)
	require.Equal(t, all, res.Kinds)
	require.Equal(t, "task", res.Kind)
	require.Equal(t, []string{"T-aaaaaa key_fragment"}, nodeLabels(res))
	require.Empty(t, res.Documents)
	require.Equal(t, 1, res.Count)

	res, err = search.Search(root, tree, "form", search.Options{Kind: "document"})
	require.NoError(t, err)
	require.Empty(t, res.Nodes)
	require.Len(t, res.Documents, 1)

	_, err = search.Search(root, tree, "form", search.Options{Kind: "widget"})
	var unknown *search.UnknownKindError
	require.ErrorAs(t, err, &unknown)
	require.Equal(t, 2, unknown.ExitCode())
	require.Equal(t, "unknown_kind", unknown.Reason())
}

// TestLimitBoundsResultsAndCountStaysTrue: the limit caps results, shared
// between nodes and documents so neither crowds the other off the page, and
// Count still states the whole answer. Zero or anything above MaxResults
// means MaxResults.
func TestLimitBoundsResultsAndCountStaysTrue(t *testing.T) {
	root, tree := fixture(t)
	res, err := search.Search(root, tree, "form", search.Options{Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []string{"E-aaaaaa title"}, nodeLabels(res))
	require.Len(t, res.Documents, 1)
	require.Equal(t, 4, res.Count)
	require.True(t, res.Truncated)

	for i := 0; i < search.MaxResults+5; i++ {
		writeFlood(t, root, fmt.Sprintf("flood-%03d.md", i), "gnu %d", 1)
	}
	res, err = search.Search(root, tree, "gnu", search.Options{Limit: 10_000})
	require.NoError(t, err)
	require.Len(t, res.Documents, search.MaxResults)
	require.Equal(t, search.MaxResults+5, res.Count)
	require.True(t, res.Truncated)
}

// TestAmendmentProseBelongsToItsAmendment: an amendment's flat
// <A-id>-<key>.md file is its prose, so a match there lands on the
// amendment, and its title is searchable like any node's.
func TestAmendmentProseBelongsToItsAmendment(t *testing.T) {
	root, tree := fixture(t)
	key := "reshape-intake"
	if tree.Amendments == nil {
		tree.Amendments = map[string]*store.Amendment{}
	}
	tree.Amendments["A-aaaaaa"] = &store.Amendment{ID: "A-aaaaaa", Key: &key, Title: "Reshape intake", Status: "proposed"}
	dir := filepath.Join(root, ".tiller", "amendments")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	writeLines(t, filepath.Join(dir, "A-aaaaaa-reshape-intake.md"), "capybara %d", 1)

	res := run(t, root, tree, "capybara")
	require.Equal(t, []string{"A-aaaaaa text"}, nodeLabels(res))
	require.Equal(t, "amendment", res.Nodes[0].Noun)
	require.Equal(t, ".tiller/amendments/A-aaaaaa-reshape-intake.md", res.Nodes[0].Hits[0].Path)

	require.Equal(t, []string{"A-aaaaaa title"}, nodeLabels(run(t, root, tree, "reshape intake")))
}

func TestShelfTitlesAndProseLandOnTheShelfNode(t *testing.T) {
	root, tree := searchShelfFixture(t)
	for _, tc := range []struct {
		query, id, match string
		hits             []search.Hit
	}{
		{"blank on reload", "B-aaaaaa", "title", []search.Hit{}},
		{"reload after saving", "B-aaaaaa", "text", []search.Hit{{Path: ".tiller/bugs/blank-on-reload/bug.md", Line: 3, Snippet: "The page must reload after saving."}}},
		{"bump go", "CH-bbbbbb", "title", []search.Hit{}},
		{"toolchain", "CH-bbbbbb", "text", []search.Hit{{Path: ".tiller/chores/bump-go/chore.md", Line: 3, Snippet: "Refresh the Go toolchain."}}},
		{"polish button", "TW-cccccc", "title", []search.Hit{}},
		{"button spacing", "TW-cccccc", "text", []search.Hit{{Path: ".tiller/tweaks/polish-button/tweak.md", Line: 3, Snippet: "Adjust the button spacing."}}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			res := run(t, root, tree, tc.query)
			require.Equal(t, []string{tc.id + " " + tc.match}, nodeLabels(res))
			require.Equal(t, tc.hits, res.Nodes[0].Hits)
		})
	}

	loose := run(t, root, tree, "uploaded pdf")
	require.Empty(t, loose.Nodes)
	require.Len(t, loose.Documents, 1)
	require.Equal(t, []search.Hit{{Path: ".tiller/design/01-form-shapes.md", Line: 3,
		Snippet: "The certification form loads from uploaded PDF manifests."}}, loose.Documents[0].Hits)
}

// TestResultJSONShape pins the payload bytes `tiller search --json` and GET
// /api/search share.
func TestResultJSONShape(t *testing.T) {
	root, tree := fixture(t)
	got, err := json.MarshalIndent(run(t, root, tree, "schema registry"), "", "  ")
	require.NoError(t, err)
	want := `{
  "query": "schema registry",
  "count": 1,
  "truncated": false,
  "kinds": [
    {
      "kind": "task",
      "count": 1
    }
  ],
  "nodes": [
    {
      "noun": "task",
      "id": "T-aaaaaa",
      "key": "form-engine/certification-form-loading/wire-the-schema-registry-loader",
      "title": "Wire the schema registry loader",
      "status": "STATUS",
      "ref": "form-engine/certification-form-loading/wire-the-schema-registry-loader (T-aaaaaa)",
      "match": "title",
      "count": 1,
      "hits": [
        {
          "path": ".tiller/epics/form-engine/features/certification-form-loading/tasks/wire-the-schema-registry-loader/task.md",
          "line": 1,
          "snippet": "# Wire the schema registry loader"
        }
      ]
    }
  ],
  "documents": []
}`
	want = strings.Replace(want, "STATUS", tree.Tasks["T-aaaaaa"].Status, 1)
	require.Equal(t, want, string(got))
}

func TestOrderIsDeterministic(t *testing.T) {
	root, tree := fixture(t)
	require.Equal(t, run(t, root, tree, "form"), run(t, root, tree, "form"))
}

// TestSnippetCentersOnTheMatch: a snippet is the trimmed matching line,
// capped in length, and a match deep in a long line is windowed into view
// rather than cut off.
func TestSnippetCentersOnTheMatch(t *testing.T) {
	root, tree := fixture(t)
	early := "   needle-long " + strings.Repeat("x", 300) + "   "
	late := strings.Repeat("x", 300) + " needle-deep " + strings.Repeat("y", 100)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "design", "03-long.md"),
		[]byte(early+"\n"+late+"\n"), 0o644))

	s := run(t, root, tree, "needle-long").Documents[0].Hits[0].Snippet
	require.True(t, strings.HasPrefix(s, "needle-long "), "leading whitespace is trimmed")
	require.LessOrEqual(t, len([]rune(s)), 200)

	s = run(t, root, tree, "needle-deep").Documents[0].Hits[0].Snippet
	require.Contains(t, s, "needle-deep")
	require.True(t, strings.HasPrefix(s, "…"))
	require.LessOrEqual(t, len([]rune(s)), 201)
}

// TestNoMatchesIsAnEmptyAnswer: empty arrays, not nulls, so a caller parses
// one shape.
func TestNoMatchesIsAnEmptyAnswer(t *testing.T) {
	root, tree := fixture(t)
	got, err := json.Marshal(run(t, root, tree, "zz-nothing-here"))
	require.NoError(t, err)
	require.JSONEq(t, `{"query":"zz-nothing-here","count":0,"truncated":false,"kinds":[],"nodes":[],"documents":[]}`, string(got))
}

// TestAttributeFallsOutwardFromAnUnplaceableTaskFolder: a task folder the
// tree cannot place still sits inside a feature it can, and the enclosing
// feature is a better answer than nobody's.
func TestAttributeFallsOutwardFromAnUnplaceableTaskFolder(t *testing.T) {
	root, tree := fixture(t)
	orphan := filepath.Join(root, ".tiller", "epics", "form-engine", "features",
		"certification-form-loading", store.TaskDirName, "no-such-task")
	require.NoError(t, os.MkdirAll(orphan, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(orphan, "notes.md"),
		[]byte("# Notes\n\nA distinctive marmoset phrase.\n"), 0o644))

	res := run(t, root, tree, "marmoset")
	require.Equal(t, []string{"F-aaaaaa text"}, nodeLabels(res), "it falls outward to the enclosing feature")
	require.Empty(t, res.Documents)
}

// writeFlood plants a design doc of `lines` matching lines.
func writeFlood(t *testing.T, root, name, format string, lines int) {
	t.Helper()
	writeLines(t, filepath.Join(root, ".tiller", "design", name), format, lines)
}

func writeLines(t *testing.T, file, format string, lines int) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, format+"\n", i)
	}
	require.NoError(t, os.WriteFile(file, []byte(b.String()), 0o644))
}

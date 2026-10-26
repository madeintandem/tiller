package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin `tiller search <query>` (cli-surface.md, "Inspecting and
// maintaining"): the human result blocks with refs per the reference
// convention, lookup by id, the --kind filter, the exact --json payload, the
// empty answer, the misuse refusals, and the environment refusal for a corpus
// the walk cannot read. The fixture reuses the read-tree shape plus one
// design doc with known content.

// searchIDRe normalizes every noun's minted id (the shared idTokenRe covers
// only E/F/T), so search goldens stay byte-stable across random mints.
var searchIDRe = regexp.MustCompile(`([EFTBQ])-[a-z0-9]{6}`)

func normalizeSearchIDs(s string) string {
	return searchIDRe.ReplaceAllString(s, "$1-xxxxxx")
}

// laySearchTree lays the read tree (one epic, one feature) plus a design doc
// whose content only these tests know, and chdirs into the repo.
func laySearchTree(t *testing.T) {
	t.Helper()
	layReadTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	designDir := filepath.Join(cwd, ".tiller", "design")
	require.NoError(t, os.MkdirAll(designDir, 0o755))
	doc := "# Upload notes\n\nThe uploader parses certification PDFs.\n"
	require.NoError(t, os.WriteFile(filepath.Join(designDir, "01-notes.md"), []byte(doc), 0o644))
}

// TestSearchHumanDocumentResult pins the block for prose no node owns: the
// file and its match count, then each kept line as path:line and snippet.
func TestSearchHumanDocumentResult(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "uploader parses")
	require.Equal(t, 0, code)
	require.Equal(t, ".tiller/design/01-notes.md  [document · 1 match]\n"+
		"    .tiller/design/01-notes.md:3  The uploader parses certification PDFs.\n", out)
}

// TestSearchHumanNodeResult pins a node's block: its "<key-path> (<id>)"
// reference, title, and a bracket naming kind, status, and how it matched,
// then the node's matching lines. The title is the head, so a title match is
// not repeated as a line.
func TestSearchHumanNodeResult(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "Certification form loading")
	require.Equal(t, 0, code)
	require.Equal(t,
		"form-engine/certification-form-loading (F-xxxxxx)  Certification form loading  [feature · draft · title match]\n"+
			"    .tiller/epics/form-engine/features/certification-form-loading/feature.md:1  # Certification form loading\n",
		normalizeSearchIDs(out))
}

// TestSearchHumanSaysHowManyMoreLines pins the tail of a node's block: past
// the first three matching lines, one line says how many more it holds.
func TestSearchHumanSaysHowManyMoreLines(t *testing.T) {
	laySearchTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	notes := "ocelot 1\nocelot 2\nocelot 3\nocelot 4\nocelot 5\n"
	require.NoError(t, os.WriteFile(filepath.Join(cwd, ".tiller", "epics", "form-engine", "notes.md"), []byte(notes), 0o644))

	out, _, code := execCLI(t, "search", "ocelot")
	require.Equal(t, 0, code)
	require.True(t, strings.HasSuffix(out, ".tiller/epics/form-engine/notes.md:3  ocelot 3\n    (+2 more)\n"), out)
}

// TestSearchByIDNamesTheNode is the lookup a reader expects from pasting an
// id: the node itself, first, as an id match.
func TestSearchByIDNamesTheNode(t *testing.T) {
	laySearchTree(t)
	out, _, code := execCLI(t, "search", "certification-form-loading", "--json")
	require.Equal(t, 0, code)
	var found struct {
		Data struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &found))
	require.NotEmpty(t, found.Data.Nodes)
	id := found.Data.Nodes[0].ID

	out, _, code = execCLI(t, "search", id)
	require.Equal(t, 0, code)
	require.True(t, strings.HasPrefix(normalizeSearchIDs(out),
		"form-engine/certification-form-loading (F-xxxxxx)  Certification form loading  [feature · draft · id match]"), out)
}

// TestSearchHumanQuestionResult pins the question grammar at the terminal:
// the stable Q-id and short title lead, and a matching statement prints as a
// named field rather than a file line.
func TestSearchHumanQuestionResult(t *testing.T) {
	laySearchTree(t)
	_, _, code := execCLI(t, "add", "question",
		"Should audit exports retain rejected rows for seven years?",
		"--title", "Audit export retention", "--contributor", "jc")
	require.Equal(t, 0, code)

	out, _, code := execCLI(t, "search", "rejected rows")
	require.Equal(t, 0, code)
	require.Equal(t,
		"Q-xxxxxx  Audit export retention  [question · open · text match]\n"+
			"    statement  Should audit exports retain rejected rows for seven years?\n",
		normalizeSearchIDs(out))
}

// TestSearchJSONGolden pins the exact --json envelope: the grouped search
// payload rides in Data.
func TestSearchJSONGolden(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "Certification form loading", "--json")
	require.Equal(t, 0, code)
	want := `{
  "ok": true,
  "command": "search",
  "data": {
    "query": "Certification form loading",
    "count": 1,
    "truncated": false,
    "kinds": [
      {
        "kind": "feature",
        "count": 1
      }
    ],
    "nodes": [
      {
        "noun": "feature",
        "id": "F-xxxxxx",
        "key": "form-engine/certification-form-loading",
        "title": "Certification form loading",
        "status": "draft",
        "ref": "form-engine/certification-form-loading (F-xxxxxx)",
        "match": "title",
        "count": 1,
        "hits": [
          {
            "path": ".tiller/epics/form-engine/features/certification-form-loading/feature.md",
            "line": 1,
            "snippet": "# Certification form loading"
          }
        ]
      }
    ],
    "documents": []
  }
}
`
	require.Equal(t, want, normalizeSearchIDs(out))
}

// TestSearchEmptyResultIsAnAnswer pins the empty answer: exit 0 with the
// query named in human output, and count 0 with empty arrays in JSON.
func TestSearchEmptyResultIsAnAnswer(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "zz-nothing-here")
	require.Equal(t, 0, code)
	require.Equal(t, "No matches for \"zz-nothing-here\".\n", out)

	out, _, code = execCLI(t, "search", "zz-nothing-here", "--json")
	require.Equal(t, 0, code)
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Count     int               `json:"count"`
			Nodes     []json.RawMessage `json:"nodes"`
			Documents []json.RawMessage `json:"documents"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, 0, env.Data.Count)
	require.NotNil(t, env.Data.Nodes, "nodes is [] rather than null")
	require.NotNil(t, env.Data.Documents, "documents is [] rather than null")
}

// TestSearchCappedPageSaysSo pins the cap's visibility to a human: a capped
// page ends by naming both numbers, what it printed and what it found.
func TestSearchCappedPageSaysSo(t *testing.T) {
	laySearchTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for i := 1; i <= 205; i++ {
		name := filepath.Join(cwd, ".tiller", "design", fmt.Sprintf("cap-%03d.md", i))
		require.NoError(t, os.WriteFile(name, []byte("cap-note\n"), 0o644))
	}

	out, _, code := execCLI(t, "search", "cap-note")
	require.Equal(t, 0, code)
	require.True(t, strings.HasSuffix(out,
		"(showing 200 of 205 results; narrow the query or pass --kind to see the rest)\n"), out)
}

// TestSearchKindKeepsOneKind pins --kind: it narrows the results, and a kind
// search does not know is misuse.
func TestSearchKindKeepsOneKind(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "certification", "--kind", "document")
	require.Equal(t, 0, code)
	require.True(t, strings.HasPrefix(out, ".tiller/design/01-notes.md  [document"), out)
	require.NotContains(t, out, "F-")

	out, _, code = execCLI(t, "search", "certification", "--kind", "widget", "--json")
	require.Equal(t, 2, code)
	require.Contains(t, out, `"reason": "unknown_kind"`)
}

// TestSearchUnreadableCorpusIsExitThree pins the failure class end to end: a
// dangling .md symlink under .tiller/ (the portable stand-in for any I/O
// failure mid-walk) aborts the search rather than answering a silent partial
// page, and the refusal is exit 3 environment, not exit 2 misuse. The query
// was fine; the repo is not.
func TestSearchUnreadableCorpusIsExitThree(t *testing.T) {
	laySearchTree(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Symlink(
		filepath.Join(cwd, "no-such-target.md"),
		filepath.Join(cwd, ".tiller", "design", "02-dangling.md"),
	))

	out, _, code := execCLI(t, "search", "uploader", "--json")
	require.Equal(t, 3, code)
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code   int    `json:"code"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.False(t, env.OK)
	require.Equal(t, 3, env.Error.Code)
	require.Equal(t, "corpus_unreadable", env.Error.Reason)
}

// TestSearchEmptyQueryIsMisuse pins the typed refusal: a whitespace query is
// exit 2 with the stable empty_query reason in the error envelope.
func TestSearchEmptyQueryIsMisuse(t *testing.T) {
	laySearchTree(t)

	out, _, code := execCLI(t, "search", "   ", "--json")
	require.Equal(t, 2, code)
	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Code   int    `json:"code"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.False(t, env.OK)
	require.Equal(t, 2, env.Error.Code)
	require.Equal(t, "empty_query", env.Error.Reason)
}

// TestSearchArgCountIsMisuse pins the grammar: search takes exactly one
// query argument.
func TestSearchArgCountIsMisuse(t *testing.T) {
	laySearchTree(t)

	_, _, code := execCLI(t, "search")
	require.Equal(t, 2, code)

	_, _, code = execCLI(t, "search", "one", "two")
	require.Equal(t, 2, code)
}

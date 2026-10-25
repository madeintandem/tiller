package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// rosterManifest is a commented manifest with an empty roster, the shape `tiller
// init` now writes. The comments around the owners block prove the renderer is
// surgical.
const rosterManifest = `# Tiller project manifest.

schema_version: 12
schema_epoch: initial

project:
  name: Test Project
  key: test
  kind: greenfield

# The humans who hold work. Added by ` + "`tiller join`" + `.
contributors: []

execution:
  review_grain: task
`

// writeManifestFixture lays rosterManifest (or a given body) under a temp repo
// root and returns the root.
func writeManifestFixture(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, writeFileAtomic(filepath.Join(root, ".tiller", "tiller.yml"), []byte(body)))
	return root
}

// TestAddOwnerUpserts confirms AddOwner appends an absent id, leaves a present id
// untouched when the name matches, and updates only the display name when a new
// non-empty name differs.
func TestAddOwnerUpserts(t *testing.T) {
	m := &Manifest{}

	require.True(t, AddContributor(m, Contributor{ID: "jc", Name: "JC Grubbs"}), "first add changes the roster")
	require.Len(t, m.Contributors, 1)
	require.Equal(t, Contributor{ID: "jc", Name: "JC Grubbs"}, m.Contributors[0])

	require.False(t, AddContributor(m, Contributor{ID: "jc", Name: "JC Grubbs"}), "re-adding the same owner is a no-op")
	require.Len(t, m.Contributors, 1)

	require.True(t, AddContributor(m, Contributor{ID: "bob", Name: "Bob"}), "a second owner is added without dropping the first")
	require.Len(t, m.Contributors, 2)
	require.Equal(t, "jc", m.Contributors[0].ID)
	require.Equal(t, "bob", m.Contributors[1].ID)

	require.True(t, AddContributor(m, Contributor{ID: "jc", Name: "JC G."}), "a differing non-empty name updates the display name")
	require.Equal(t, "JC G.", m.Contributors[0].Name)

	require.False(t, AddContributor(m, Contributor{ID: "jc", Name: ""}), "an empty name never overwrites an existing one")
	require.Equal(t, "JC G.", m.Contributors[0].Name)
}

func TestRenderOwnersPreservesComments(t *testing.T) {
	root := writeManifestFixture(t, rosterManifest)

	data, err := RenderContributors([]byte(rosterManifest), []Contributor{{ID: "jc", Name: "JC Grubbs"}})
	require.NoError(t, err)
	require.NoError(t, writeFileAtomic(filepath.Join(root, ".tiller", "tiller.yml"), data))
	text := string(data)

	require.Contains(t, text, "# The humans who hold work.", "the owners comment is preserved")
	require.Contains(t, text, "# Tiller project manifest.", "the header comment is preserved")
	require.Contains(t, text, "review_grain: task", "blocks after owners are preserved")
	require.NotContains(t, text, "contributors: []", "the empty roster line is replaced")
	require.Contains(t, text, "  - { id: \"jc\", name: \"JC Grubbs\" }", "the owner is rendered in the block")

	m, err := LoadManifest(root)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 1)
	require.Equal(t, "jc", m.Contributors[0].ID)
	require.Equal(t, "JC Grubbs", m.Contributors[0].Name)

	again, err := RenderContributors(data, []Contributor{{ID: "jc", Name: "JC Grubbs"}})
	require.NoError(t, err)
	require.Equal(t, text, string(again), "re-rendering the same roster is byte-stable")
}

func TestRenderOwnersAppendsSecond(t *testing.T) {
	root := writeManifestFixture(t, rosterManifest)

	data, err := RenderContributors([]byte(rosterManifest), []Contributor{{ID: "jc", Name: "JC Grubbs"}, {ID: "bob", Name: "Bob"}})
	require.NoError(t, err)
	require.NoError(t, writeFileAtomic(filepath.Join(root, ".tiller", "tiller.yml"), data))

	m, err := LoadManifest(root)
	require.NoError(t, err)
	require.Len(t, m.Contributors, 2)
	require.Equal(t, "jc", m.Contributors[0].ID)
	require.Equal(t, "bob", m.Contributors[1].ID)
	require.True(t, strings.Contains(string(data), "review_grain: task"), "trailing blocks survive a multi-owner write")
}

// TestLocalIdentityRoundTrip confirms WriteLocalIdentity records the acting contributor
// id and ReadLocalIdentity reads it back, while a checkout with no identity file
// reads the empty string with no error.
func TestLocalIdentityRoundTrip(t *testing.T) {
	root := t.TempDir()

	id, err := ReadLocalIdentity(root)
	require.NoError(t, err, "an absent identity file is not an error")
	require.Equal(t, "", id, "an un-joined checkout has no identity")

	require.NoError(t, WriteLocalIdentity(root, "jc"))

	got, err := ReadLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "jc", got)

	// The file lives at the known Tiller-local path.
	require.FileExists(t, filepath.Join(root, ".tiller", "identity.yml"))

	// A re-write of the same id is byte-identical.
	before, err := os.ReadFile(filepath.Join(root, ".tiller", "identity.yml"))
	require.NoError(t, err)
	require.NoError(t, WriteLocalIdentity(root, "jc"))
	after, err := os.ReadFile(filepath.Join(root, ".tiller", "identity.yml"))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "the identity file is byte-stable across writes")
}

// contributorLayouts are valid manifests whose contributors value takes a shape
// the old indentation-only scan mishandled. Each trails a comment-led key so the
// rewrite must stop at the structure, not at the next unindented line.
var contributorLayouts = map[string]string{
	"indentless sequence": "schema_version: 12\ncontributors:\n- { id: a, name: A }\n- { id: b, name: B }\n# review dial\nexecution:\n  review_grain: task\n",
	"blank separated":     "schema_version: 12\ncontributors:\n  - { id: a, name: A }\n\n  - { id: b, name: B }\n\n# review dial\nexecution:\n  review_grain: task\n",
	"comment separated":   "schema_version: 12\ncontributors:\n  - { id: a, name: A }\n# who b is\n  - { id: b, name: B }\n# review dial\nexecution:\n  review_grain: task\n",
	"flow sequence":       "schema_version: 12\ncontributors: [{ id: a, name: A }, { id: b, name: B }]\n# review dial\nexecution:\n  review_grain: task\n",
	"multiline flow":      "schema_version: 12\ncontributors: [\n{ id: a, name: A },\n{ id: b, name: B }]\n# review dial\nexecution:\n  review_grain: task\n",
	"empty roster":        "schema_version: 12\ncontributors: []\n# review dial\nexecution:\n  review_grain: task\n",
	"last key":            "schema_version: 12\nexecution:\n  review_grain: task\ncontributors:\n- { id: a, name: A }\n- { id: b, name: B }\n",
}

func TestRenderContributorsHandlesLayouts(t *testing.T) {
	rosters := map[string][]Contributor{
		"add":    {{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}},
		"remove": {{ID: "a", Name: "A"}},
		"empty":  nil,
	}
	for layout, body := range contributorLayouts {
		for change, roster := range rosters {
			t.Run(layout+"/"+change, func(t *testing.T) {
				out, err := RenderContributors([]byte(body), roster)
				require.NoError(t, err)

				var got struct {
					Contributors []Contributor `yaml:"contributors"`
					Execution    struct {
						ReviewGrain string `yaml:"review_grain"`
					} `yaml:"execution"`
				}
				require.NoError(t, yaml.Unmarshal(out, &got), "rendered manifest must parse:\n%s", out)
				require.Equal(t, len(roster), len(got.Contributors), "no entry survives twice:\n%s", out)
				for i := range roster {
					require.Equal(t, roster[i], got.Contributors[i])
				}
				require.Equal(t, "task", got.Execution.ReviewGrain)
				if strings.Contains(body, "# review dial") {
					require.Contains(t, string(out), "# review dial\nexecution:", "the unrelated comment stays attached to its key")
				}
			})
		}
	}
}

// TestVerifyRenderedContributorsRefusesUnsafeOutput pins the guard that keeps a bad
// rewrite from being committed: output that changes another value, carries the
// wrong roster, or does not parse must each be refused.
func TestVerifyRenderedContributorsRefusesUnsafeOutput(t *testing.T) {
	original := []byte("schema_version: 12\ncontributors: []\nexecution:\n  review_grain: task\n")
	owners := []Contributor{{ID: "a", Name: "A"}}
	good := []byte("schema_version: 12\ncontributors:\n  - { id: \"a\", name: \"A\" }\nexecution:\n  review_grain: task\n")
	require.NoError(t, verifyRenderedContributors(original, good, owners))

	for name, rendered := range map[string]string{
		"other value changed": "schema_version: 12\ncontributors:\n  - { id: \"a\", name: \"A\" }\nexecution:\n  review_grain: epic\n",
		"other key dropped":   "schema_version: 12\ncontributors:\n  - { id: \"a\", name: \"A\" }\n",
		"roster mismatch":     "schema_version: 12\ncontributors:\n  - { id: \"b\", name: \"B\" }\nexecution:\n  review_grain: task\n",
		"roster too long":     "schema_version: 12\ncontributors:\n  - { id: \"a\", name: \"A\" }\n  - { id: \"b\", name: \"B\" }\nexecution:\n  review_grain: task\n",
		"unparsable":          "schema_version: 12\ncontributors: [\nexecution:\n",
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, verifyRenderedContributors(original, []byte(rendered), owners))
		})
	}
}

// TestRenderContributorsReplacesBlockScalarTail covers a roster whose last entry
// ends in a block scalar line that looks like a comment. It is roster content, so
// the rewrite must replace it rather than leave a stray line behind.
func TestRenderContributorsReplacesBlockScalarTail(t *testing.T) {
	body := "schema_version: 12\ncontributors:\n  - id: a\n    name: |\n      A\n      # not a comment\nexecution:\n  review_grain: task\n"
	out, err := RenderContributors([]byte(body), []Contributor{{ID: "a", Name: "A"}})
	require.NoError(t, err)
	require.NotContains(t, string(out), "not a comment")
	require.Contains(t, string(out), "review_grain: task")
}

// TestRenderContributorsRefusesDanglingAlias is a reachable refusal: another key
// aliases an anchor defined on the roster, so replacing the roster would leave an
// alias with no anchor. The rewrite must error rather than return that manifest.
func TestRenderContributorsRefusesDanglingAlias(t *testing.T) {
	body := "schema_version: 12\ncontributors: &roster\n  - { id: a, name: A }\nshared: *roster\n"
	out, err := RenderContributors([]byte(body), []Contributor{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}})
	require.Error(t, err)
	require.Nil(t, out)
}

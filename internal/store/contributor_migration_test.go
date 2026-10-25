package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCurrentManifestRejectsLegacyRosterRatherThanDroppingIt(t *testing.T) {
	for _, roster := range []string{"owners: [{id: sam, name: Sam}]\n", "owners: []\ncontributors: [{id: sam, name: Sam}]\n"} {
		data := []byte("schema_version: 12\n" + roster)
		for _, parse := range []func(string, []byte) (*Manifest, error){ParseManifest, ParseManifestUnchecked} {
			_, err := parse("/repo", data)
			var refusal *EnvError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, "obsolete_contributor_identity", refusal.Reason)
		}
	}
	manifest, err := ParseManifest("/repo", []byte("schema_version: 12\ncontributors: [{id: sam, name: Sam}]\n"))
	require.NoError(t, err)
	require.Equal(t, []Contributor{{ID: "sam", Name: "Sam"}}, manifest.Contributors)
	written, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	var fields map[string]yaml.Node
	require.NoError(t, yaml.Unmarshal(written, &fields))
	require.Contains(t, fields, "contributors")
	require.NotContains(t, fields, "owners")
}

func TestLegacyIdentityOnlyResolvesDuringOldSchemaMigration(t *testing.T) {
	root := writeManifestFixture(t, "schema_version: 8\nowners: [{id: sam, name: Sam}]\n")
	path := filepath.Join(root, ".tiller/identity.yml")
	legacy := []byte("# local person\nowner: sam\n")
	require.NoError(t, os.WriteFile(path, legacy, 0o644))
	_, err := ReadLocalIdentity(root)
	require.Error(t, err, "ordinary identity resolution never treats owner as an alias")
	id, err := ReadLocalIdentityForMigration(root)
	require.NoError(t, err)
	require.Equal(t, "sam", id)
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, legacy, unchanged, "migration inspection does not select or rewrite identity")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller/tiller.yml"), []byte("schema_version: 12\ncontributors: [{id: sam, name: Sam}]\n"), 0o644))
	_, err = ReadLocalIdentityForMigration(root)
	require.Error(t, err, "current-schema maintenance cannot retain the legacy alias")
}

func TestContributorMigrationPreservesFormattingAndUnknownContent(t *testing.T) {
	for _, key := range []string{"owners", "'owners'", "\"owners\""} {
		t.Run(key, func(t *testing.T) {
			data := "# manifest\r\nschema_version: 8 # team note\r\nschema_epoch: original\r\n" + key + ": &team [{id: sam, name: 'Sam'}] # roster note\r\ncustom: {owners: keep, roster: *team}\r\n"
			rendered, err := RenderContributorMigration([]byte(data))
			require.NoError(t, err)
			expected := strings.Replace(strings.Replace(data, "schema_version: 8", "schema_version: 9", 1), "\n"+key+":", "\n"+strings.Replace(key, "owners", "contributors", 1)+":", 1)
			require.Equal(t, expected, string(rendered))
			manifest, err := ParseManifestUnchecked("/repo", rendered)
			require.NoError(t, err)
			require.Equal(t, "sam", manifest.Contributors[0].ID)
		})
	}
	_, err := RenderContributorMigration([]byte("schema_version: 8\nowners: [{id: sam}]\ncontributors: [{id: jc}]\n"))
	require.Error(t, err, "two different roster vocabularies cannot be reconciled by guessing")
}

func TestMigratedQuotedRosterCanBeUpdated(t *testing.T) {
	for _, key := range []string{"'owners'", "\"owners\""} {
		converted, err := RenderContributorMigration([]byte("schema_version: 8\n" + key + ": [{id: sam, name: Sam}]\ncustom: keep\n"))
		require.NoError(t, err)
		roster := []Contributor{{ID: "sam", Name: "Sam"}, {ID: "jc", Name: "JC"}}
		updated, err := RenderContributors(converted, roster)
		require.NoError(t, err)
		manifest, err := ParseManifestUnchecked("/repo", updated)
		require.NoError(t, err)
		require.Equal(t, roster, manifest.Contributors)
		require.Contains(t, string(updated), "custom: keep\n")
	}
}

func TestLocalIdentityWritesContributorAndRejectsMixedVocabulary(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteLocalIdentity(root, "sam"))
	path := filepath.Join(root, ".tiller/identity.yml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fields map[string]yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &fields))
	require.Contains(t, fields, "contributor")
	require.NotContains(t, fields, "owner")
	require.NoError(t, os.WriteFile(path, []byte("owner: sam\ncontributor: jc\n"), 0o644))
	_, err = ReadLocalIdentity(root)
	require.Error(t, err)
}

func TestLegacyLocalIdentityRenamesOnlyTheOwnerKey(t *testing.T) {
	root := writeManifestFixture(t, "schema_version: 12\ncontributors: [{id: sam, name: Sam}]\n")
	path := filepath.Join(root, ".tiller/identity.yml")

	id, renamed, err := LegacyLocalIdentity(root)
	require.NoError(t, err)
	require.Empty(t, id)
	require.Nil(t, renamed, "an absent identity file needs no rename")

	require.NoError(t, os.WriteFile(path, []byte("contributor: sam\n"), 0o644))
	_, renamed, err = LegacyLocalIdentity(root)
	require.NoError(t, err)
	require.Nil(t, renamed, "a current identity file needs no rename")

	legacy := []byte("# local person\nowner: \"sam\" # me\n")
	require.NoError(t, os.WriteFile(path, legacy, 0o644))
	id, renamed, err = LegacyLocalIdentity(root)
	require.NoError(t, err)
	require.Equal(t, "sam", id)
	require.Equal(t, "# local person\ncontributor: \"sam\" # me\n", string(renamed))
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, legacy, unchanged, "planning a rename never writes")

	require.NoError(t, os.WriteFile(path, []byte("owner: sam\ncontributor: sam\n"), 0o644))
	_, _, err = LegacyLocalIdentity(root)
	require.ErrorContains(t, err, "both owner and contributor")
}

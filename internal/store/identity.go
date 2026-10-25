package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file holds the identity half of the store: the committed contributor roster in
// the manifest (who can hold work) and the gitignored per-checkout identity file
// (who I am here). Tiller is self-contained in the repo: there is no global, user,
// or machine-level config. The roster is shared and committed; the identity file
// is local and never committed; nothing identity-related lives outside the repo.

// identityFile is the repo-local "who I am on this checkout" file. It is
// gitignored (declared by `tiller init`) and written by `tiller join`.
const identityFile = "identity.yml"

// localIdentity is the on-disk shape of .tiller/identity.yml.
type localIdentity struct {
	Contributor string `yaml:"contributor"`
}

// AddContributor upserts o into the manifest roster in memory: it appends o when its id
// is absent, or updates the display name when o.Name is non-empty and differs
// from the recorded one. It reports whether the roster changed, so the caller can
// skip a no-op write and keep the manifest byte-stable. It never removes or
// reorders contributors, so a teammate joining later adds themselves without disturbing
// the first joiner.
func AddContributor(m *Manifest, o Contributor) bool {
	for i := range m.Contributors {
		if m.Contributors[i].ID != o.ID {
			continue
		}
		if o.Name != "" && o.Name != m.Contributors[i].Name {
			m.Contributors[i].Name = o.Name
			return true
		}
		return false
	}
	m.Contributors = append(m.Contributors, o)
	return true
}

// RemoveContributor removes o's row from the manifest roster in memory by id,
// reporting whether the roster changed: false when no contributor with that id
// was found. It never reorders the remaining contributors.
func RemoveContributor(m *Manifest, id string) bool {
	for i := range m.Contributors {
		if m.Contributors[i].ID != id {
			continue
		}
		m.Contributors = append(m.Contributors[:i], m.Contributors[i+1:]...)
		return true
	}
	return false
}

// RenderContributors rewrites only the `contributors:` entry of manifestData, leaving
// every other line, comment, and formatting byte intact. The entry's extent comes
// from the YAML structure: it runs from the key's line to the line before the next
// top-level key, minus any blank or column-0 comment lines directly above that key (they
// belong to it). That covers indentless and indented block sequences, blank or
// comment lines between entries, and flow sequences, single or multi-line. The
// result is parsed again and must carry exactly the requested roster and every other
// top-level value unchanged, or no bytes are returned.
func RenderContributors(manifestData []byte, owners []Contributor) ([]byte, error) {
	lines := strings.Split(string(manifestData), "\n")
	var doc yaml.Node
	if err := yaml.Unmarshal(manifestData, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style == yaml.FlowStyle {
		return nil, fmt.Errorf("manifest needs a block mapping to update contributors")
	}
	keys := doc.Content[0].Content
	start, end := -1, len(lines)
	for i := 0; i < len(keys); i += 2 {
		key := keys[i]
		if start != -1 {
			end = key.Line - 1
			break
		}
		if key.Value != "contributors" {
			continue
		}
		if strings.TrimSpace(lines[key.Line-1][:yamlColumnOffset(lines[key.Line-1], key.Column)]) != "" {
			return nil, fmt.Errorf("contributors key must be on its own line to update the roster")
		}
		start = key.Line - 1
	}
	if start == -1 {
		return nil, fmt.Errorf("manifest has no contributors block to update")
	}
	// Blank and comment lines just above the next key (or the end of the file)
	// are not part of the roster.
	for end-1 > start {
		// Only column-0 comments can head the next top-level key; an indented
		// line that looks like a comment may be roster content (a block scalar).
		if strings.TrimSpace(lines[end-1]) != "" && !strings.HasPrefix(lines[end-1], "#") {
			break
		}
		end--
	}

	block := strings.Split(renderContributorsBlock(owners), "\n")
	next := append([]string{}, lines[:start]...)
	next = append(next, block...)
	next = append(next, lines[end:]...)
	rendered := []byte(strings.Join(next, "\n"))
	if err := verifyRenderedContributors(manifestData, rendered, owners); err != nil {
		return nil, err
	}
	return rendered, nil
}

// verifyRenderedContributors proves a contributors rewrite is safe to commit: the
// rendered manifest parses, its roster is exactly owners, and every other top-level
// value equals the original's.
func verifyRenderedContributors(original, rendered []byte, owners []Contributor) error {
	var after map[string]any
	if err := yaml.Unmarshal(rendered, &after); err != nil {
		return fmt.Errorf("rendered manifest does not parse after updating contributors: %w", err)
	}
	var roster struct {
		Contributors []Contributor `yaml:"contributors"`
	}
	if err := yaml.Unmarshal(rendered, &roster); err != nil {
		return fmt.Errorf("rendered manifest has an unreadable contributors roster: %w", err)
	}
	if len(roster.Contributors) != len(owners) {
		return fmt.Errorf("rendered manifest holds %d contributors, want %d; refusing to write it", len(roster.Contributors), len(owners))
	}
	for i := range owners {
		if roster.Contributors[i] != owners[i] {
			return fmt.Errorf("rendered manifest contributor %d does not match the requested roster; refusing to write it", i+1)
		}
	}
	var before map[string]any
	if err := yaml.Unmarshal(original, &before); err != nil {
		return err
	}
	delete(before, "contributors")
	delete(after, "contributors")
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("rendering contributors changed other manifest content; refusing to write it")
	}
	return nil
}

// renderContributorsBlock renders the contributors block exactly as `tiller init` and
// `tiller join` agree to write it: an empty roster as the flow form `contributors: []`,
// and a populated roster as a block sequence of quoted flow maps. Quoting the id
// and name as JSON scalars (valid YAML) keeps an arbitrary display name safe.
func renderContributorsBlock(owners []Contributor) string {
	if len(owners) == 0 {
		return "contributors: []"
	}
	var b strings.Builder
	b.WriteString("contributors:")
	for _, o := range owners {
		fmt.Fprintf(&b, "\n  - { id: %s, name: %s }", quoteScalar(o.ID), quoteScalar(o.Name))
	}
	return b.String()
}

// quoteScalar renders s as a double-quoted scalar valid in both JSON and YAML.
func quoteScalar(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// WriteLocalIdentity writes .tiller/identity.yml under root, recording the acting
// contributor id for this checkout. The file is gitignored and per checkout: a fresh
// clone or Git worktree has none, so every checkout runs its own `tiller join`. The write is
// atomic and byte-stable for a given id.
func WriteLocalIdentity(root, id string) error {
	body := "# Tiller local identity: who you are on this checkout.\n" +
		"# Git-ignored, per checkout, never committed. Written by `tiller join --as <id>`.\n" +
		fmt.Sprintf("contributor: %s\n", quoteScalar(id))
	return WriteRepositoryFileAtomic(root, filepath.Join(".tiller", identityFile), []byte(body))
}

// ReadLocalIdentity reads the acting contributor id from .tiller/identity.yml under
// root, returning the empty string when the file is absent (an un-joined
// checkout). A malformed file is a real error the caller surfaces.
func ReadLocalIdentity(root string) (string, error) {
	path := filepath.Join(root, ".tiller", identityFile)
	data, err := readStateFile(root, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if err := rejectLegacyContributorKey(path, data, "owner"); err != nil {
		return "", err
	}
	var li localIdentity
	if err := yaml.Unmarshal(data, &li); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return strings.TrimSpace(li.Contributor), nil
}

// ReadLocalIdentityForMigration allows legacy local identity only for a supported
// old-schema checkout. Ordinary commands must use ReadLocalIdentity.
func ReadLocalIdentityForMigration(root string) (string, error) {
	manifestPath := filepath.Join(root, ".tiller", "tiller.yml")
	manifestData, err := readStateFile(root, manifestPath)
	if err != nil {
		return "", err
	}
	version, refusal := SchemaVersionOf(manifestPath, manifestData)
	if refusal != nil {
		return "", refusal
	}
	if version < MinMigratableSchemaVersion || version >= CurrentSchemaVersion {
		return ReadLocalIdentity(root)
	}
	id, renamed, err := LegacyLocalIdentity(root)
	if err != nil || renamed != nil {
		return id, err
	}
	return ReadLocalIdentity(root)
}

// LegacyLocalIdentity reads a checkout identity file that still spells the id
// with the pre-schema-9 owner key. identity.yml is gitignored, so the checkout
// that migrates cannot rename it for teammates; their refresh renames it here.
// renamed is the file with only that key renamed, keeping comments and
// formatting. It is nil when the file holds no legacy key it can read: absent,
// already current, unsafe to read, or malformed. ReadLocalIdentity owns the
// refusal of an unreadable file, so this never adds a second one. It never
// writes, and refuses only a legacy key it cannot rename, such as a file
// holding both keys.
func LegacyLocalIdentity(root string) (id string, renamed []byte, err error) {
	path := filepath.Join(root, ".tiller", identityFile)
	data, err := readStateFile(root, path)
	if err != nil {
		return "", nil, nil
	}
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return "", nil, nil
	}
	if _, legacy := fields["owner"]; !legacy {
		return "", nil, nil
	}
	renamed, err = RenderLocalIdentityMigration(data)
	if err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var li localIdentity
	if err := yaml.Unmarshal(renamed, &li); err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return strings.TrimSpace(li.Contributor), renamed, nil
}

package store

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func rejectLegacyContributorKey(path string, data []byte, key string) error {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if _, exists := fields[key]; exists {
		remedy := "migrate the original old-schema snapshot with `tiller upgrade --project-only`; do not just change its schema stamp"
		if key == "owner" {
			remedy = "run `tiller upgrade --project-only` (or a bare `tiller join`) to rename the key in place"
		}
		return &EnvError{Code: 3, Reason: ReasonObsoleteIdentity, Msg: fmt.Sprintf("%s contains legacy %s; schema 9 uses contributor identity and contributors roster; %s", path, key, remedy)}
	}
	return nil
}

// RenderContributorMigration renames only the roster key and schema stamp.
// Roster values, comments, formatting, unknown fields, and epoch stay intact.
func RenderContributorMigration(data []byte) ([]byte, error) {
	renamed, err := renameMigrationKey(data, "owners", "contributors")
	if err != nil {
		return nil, err
	}
	return RenderSchemaVersion(renamed, 9)
}

// RenderLocalIdentityMigration renames a checkout's legacy owner key, during
// migration or a pulled checkout's refresh (LegacyLocalIdentity). Ordinary
// identity reads deliberately reject owner rather than alias it.
func RenderLocalIdentityMigration(data []byte) ([]byte, error) {
	renamed, err := renameMigrationKey(data, "owner", "contributor")
	if err != nil {
		return nil, err
	}
	var identity localIdentity
	if err := yaml.Unmarshal(renamed, &identity); err != nil {
		return nil, err
	}
	return renamed, nil
}

// renameMigrationKey uses YAML's key location rather than reserializing the
// document, which would discard human formatting and unknown manifest fields.
func renameMigrationKey(data []byte, old, next string) ([]byte, error) {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if _, exists := fields[next]; exists {
		if _, legacy := fields[old]; legacy {
			return nil, fmt.Errorf("cannot migrate identity containing both %s and %s", old, next)
		}
		return data, nil
	}
	if _, exists := fields[old]; !exists {
		return data, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	mapping := doc.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Value != old {
			continue
		}
		lines := strings.SplitAfter(string(data), "\n")
		line := lines[key.Line-1]
		at := yamlColumnOffset(line, key.Column)
		for _, spelling := range []string{old, "'" + old + "'", "\"" + old + "\""} {
			if !strings.HasPrefix(line[at:], spelling) {
				continue
			}
			replacement := next
			if spelling[0] == '\'' || spelling[0] == '"' {
				replacement = spelling[:1] + next + spelling[:1]
			}
			lines[key.Line-1] = line[:at] + replacement + line[at+len(spelling):]
			return []byte(strings.Join(lines, "")), nil
		}
		return nil, fmt.Errorf("cannot safely rename %s key at line %d; use a plain or quoted key before migration", old, key.Line)
	}
	return nil, fmt.Errorf("cannot locate %s key for migration", old)
}

func yamlColumnOffset(line string, column int) int {
	for at := range line {
		column--
		if column == 0 {
			return at
		}
	}
	return len(line)
}

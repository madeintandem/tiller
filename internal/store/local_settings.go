package store

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// This file holds the per-checkout settings file, .tiller/local.yml: a
// developer's own preferences for this checkout. It is the third home beside
// the committed manifest (the team's shared configuration) and the gitignored
// identity file (who you are here), and like identity it never leaves the repo:
// Tiller has no global, user, or machine-level config. Unlike identity, Tiller
// never writes it. A human edits it by hand, absence means every default, and
// `init` and `join` keep it git-ignored.

// localSettingsFile is the repo-local, hand-edited preferences file.
const localSettingsFile = "local.yml"

// LocalSettings is the on-disk shape of .tiller/local.yml. Every field is
// optional; a nil or absent value means the default. Keys this binary does not
// know are tolerated so an older binary never refuses a newer file.
type LocalSettings struct {
	Console ConsoleSettings `yaml:"console"`
}

// ConsoleSettings holds the preferences `tiller serve` reads.
type ConsoleSettings struct {
	// Open says whether serve opens the console in a browser. nil is unset,
	// which means yes.
	Open *bool `yaml:"open"`
}

// ReadLocalSettings reads .tiller/local.yml under root. An absent file is the
// common case and yields the defaults; a present but malformed file is a real
// error the caller surfaces, so a typo never silently reverts a preference.
func ReadLocalSettings(root string) (LocalSettings, error) {
	var settings LocalSettings
	path := filepath.Join(root, ".tiller", localSettingsFile)
	data, err := readStateFile(root, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return settings, nil
		}
		return settings, err
	}
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return LocalSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

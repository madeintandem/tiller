package store

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/madeintandem/tiller/internal/id"
)

// Shelf describes one work shelf. Dir is the folder under .tiller/, which is
// also the shelf's plural, its canonical-key prefix, and a reserved epic key.
// IDPrefix is the id class the id package mints for Noun, without the hyphen.
type Shelf struct {
	Noun     string
	Dir      string
	IDPrefix string
}

// shelves is the one place a shelf kind is declared. Every per-kind fact
// elsewhere derives from this table.
var shelves = []Shelf{
	newShelf(NounBug, "bugs"),
	newShelf(NounChore, "chores"),
	newShelf(NounTweak, "tweaks"),
}

// newShelf takes the id prefix from the id package, which owns every noun's
// id class, so the two can never disagree.
func newShelf(noun, dir string) Shelf {
	prefix, err := id.PrefixFor(noun)
	if err != nil {
		panic(fmt.Sprintf("store: shelf %q has no id prefix: %v", noun, err))
	}
	return Shelf{Noun: noun, Dir: dir, IDPrefix: prefix}
}

// Shelves returns every work shelf in stable order (bug, chore, tweak) as a
// fresh slice.
func Shelves() []Shelf {
	return slices.Clone(shelves)
}

// ShelfNouns returns every work shelf's noun in stable order as a fresh slice.
func ShelfNouns() []string {
	nouns := make([]string, len(shelves))
	for i, s := range shelves {
		nouns[i] = s.Noun
	}
	return nouns
}

// ShelfFor returns the shelf a noun lives on.
func ShelfFor(noun string) (Shelf, bool) {
	for _, s := range shelves {
		if s.Noun == noun {
			return s, true
		}
	}
	return Shelf{}, false
}

// mustShelf is ShelfFor for callers whose noun is already validated: an
// unknown noun is a programming error, and it panics rather than quietly
// reading or writing some other shelf.
func mustShelf(noun string) Shelf {
	s, ok := ShelfFor(noun)
	if !ok {
		panic(notAShelf(noun))
	}
	return s
}

// notAShelf is the panic message for a noun no shelf accessor knows.
func notAShelf(noun string) string {
	return fmt.Sprintf("store: %q is not a shelf noun", noun)
}

// ShelfOfRef classifies a raw reference by its form: an id with a shelf's
// prefix (B-xxxxxx) or a canonical key under a shelf's folder (bugs/<key>).
// It does not check that the node exists.
func ShelfOfRef(raw string) (Shelf, bool) {
	for _, s := range shelves {
		if strings.HasPrefix(raw, s.IDPrefix+"-") || strings.HasPrefix(raw, s.Dir+"/") {
			return s, true
		}
	}
	return Shelf{}, false
}

// IsWorkNoun reports whether noun lives on a work shelf.
func IsWorkNoun(noun string) bool {
	_, ok := ShelfFor(noun)
	return ok
}

// IsReservedEpicKey reports whether key is a shelf folder name, which an epic
// may not take: an epic key is a single segment and its folder sits beside
// the shelves.
func IsReservedEpicKey(key string) bool {
	for _, s := range shelves {
		if s.Dir == key {
			return true
		}
	}
	return false
}

// ShelfDir is the folder under .tiller/ a shelf noun lives in.
func ShelfDir(noun string) string { return mustShelf(noun).Dir }

// ShelfProseFile and ShelfStateFile name the two files in a shelf folder.
func ShelfProseFile(noun string) string { return mustShelf(noun).Noun + ".md" }
func ShelfStateFile(noun string) string { return mustShelf(noun).Noun + ".yml" }

// ShelfRelDir is the repo-relative folder of one shelf node, by LOCAL key.
func ShelfRelDir(noun, localKey string) string {
	return filepath.Join(".tiller", ShelfDir(noun), localKey)
}

// ShelfCanonicalKey prepends the shelf, the same way a feature's canonical
// key prepends its epic (artifact-schema.md, Keys and paths).
func ShelfCanonicalKey(noun, localKey string) string {
	return ShelfDir(noun) + "/" + localKey
}

// ShelfOrderRelPath is the shelf's soft order file.
func ShelfOrderRelPath(noun string) string {
	return filepath.Join(".tiller", ShelfDir(noun), "order.yml")
}

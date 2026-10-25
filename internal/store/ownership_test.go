package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func ownershipTreeFixture(t *testing.T) string {
	t.Helper()
	root := writeManifestFixture(t, "schema_version: 12\ncontributors: [{id: ada, name: Ada}, {id: bob, name: Bob}]\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/work/epic.yml"), "id: E-aaaaaa\nkey: work\nstatus: ready\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/work/features/build/feature.yml"), "id: F-aaaaaa\nkey: work/build\nstatus: active\n")
	return root
}

func TestLoadContributorsCountOnlyStartedTreeAssignments(t *testing.T) {
	root := ownershipTreeFixture(t)
	for _, row := range []struct {
		key, id, status, owner string
		started                bool
	}{
		{"active", "T-aaaaaa", "in_progress", "bob", true},
		{"done", "T-bbbbbb", "done", "ada", true},
		{"again", "T-cccccc", "cancelled", "bob", true},
		{"reserved", "T-dddddd", "ready", "not-contributor", false},
		{"cancelled-before-start", "T-eeeeee", "cancelled", "not-contributor", false},
		{"historical", "T-ffffff", "cancelled", "former", true},
		{"missing-owner", "T-gggggg", "done", "", true},
	} {
		writeFile(t, filepath.Join(root, ".tiller/epics/work/features/build/tasks", row.key, "task.yml"), fmt.Sprintf("id: %s\nkey: work/build/%s\nstatus: %s\nowner: %s\nstarted: %t\n", row.id, row.key, row.status, row.owner, row.started))
	}
	writeFile(t, filepath.Join(root, ".tiller/bugs/attached/bug.yml"), "id: B-aaaaaa\nkey: bugs/attached\nstatus: in_progress\nstarted: true\nowner: shelf-only\nattached_to: F-aaaaaa\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/work/features/cancelled/feature.yml"), "id: F-bbbbbb\nkey: work/cancelled\nstatus: cancelled\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/work/features/cancelled/tasks/past/task.yml"), "id: T-hhhhhh\nkey: work/cancelled/past\nstatus: cancelled\nstarted: true\nowner: old\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/empty/epic.yml"), "id: E-bbbbbb\nkey: empty\nstatus: ready\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/empty/features/empty/feature.yml"), "id: F-cccccc\nkey: empty/empty\nstatus: ready\n")
	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, []Contributor{{ID: "ada", Name: "Ada"}, {ID: "bob", Name: "Bob"}, {ID: "former", Name: "former"}}, tree.Contributors["F-aaaaaa"])
	require.Equal(t, []Contributor{{ID: "ada", Name: "Ada"}, {ID: "bob", Name: "Bob"}, {ID: "former", Name: "former"}, {ID: "old", Name: "old"}}, tree.Contributors["E-aaaaaa"])
	require.Equal(t, []Contributor{}, tree.Contributors["E-bbbbbb"])
	require.Equal(t, []Contributor{}, tree.Contributors["F-cccccc"])
}

func TestLoadRefusesErasedExecutionBoundary(t *testing.T) {
	for _, noun := range []string{NounTask, NounBug, NounChore, NounTweak} {
		for _, suffix := range []string{"status: in_progress\n", "status: check\nstarted: false\n", "status: in_review\n", "status: cancelled\nbranch: retained/branch\n", "status: done\n"} {
			t.Run(noun+"/"+suffix, func(t *testing.T) {
				root := ownershipTreeFixture(t)
				path, key, id := ".tiller/epics/work/features/build/tasks/unit/task.yml", "work/build/unit", "T-aaaaaa"
				if noun != NounTask {
					path, key, id = ".tiller/"+ShelfDir(noun)+"/unit/"+noun+".yml", ShelfDir(noun)+"/unit", "B-aaaaaa"
					if noun == NounChore {
						id = "CH-aaaaaa"
					}
					if noun == NounTweak {
						id = "TW-aaaaaa"
					}
				}
				writeFile(t, filepath.Join(root, path), fmt.Sprintf("id: %s\nkey: %s\nowner: ada\n%s", id, key, suffix))
				_, err := Load(root)
				require.ErrorContains(t, err, "started: true")
			})
		}
	}
}

func TestLoadPreservesCarriedCompletionWithoutIndependentStart(t *testing.T) {
	root := ownershipTreeFixture(t)
	writeFile(t, filepath.Join(root, ".tiller/bugs/carried/bug.yml"), "id: B-aaaaaa\nkey: bugs/carried\nstatus: done\ndone_basis: carried\nresolved_by: F-aaaaaa\n")
	tree, err := Load(root)
	require.NoError(t, err)
	require.False(t, tree.Bugs["B-aaaaaa"].Started)
	require.Equal(t, []Contributor{}, tree.Contributors["F-aaaaaa"])
}

func TestLoadRefusesRetiredAncestorOwnershipAndClaiming(t *testing.T) {
	for _, field := range []string{"epic", "feature", "claiming"} {
		t.Run(field, func(t *testing.T) {
			root := ownershipTreeFixture(t)
			switch field {
			case "epic":
				writeFile(t, filepath.Join(root, ".tiller/epics/work/epic.yml"), "id: E-aaaaaa\nkey: work\nowner: ada\n")
			case "feature":
				writeFile(t, filepath.Join(root, ".tiller/epics/work/features/build/feature.yml"), "id: F-aaaaaa\nkey: work/build\nowner: ada\n")
			case "claiming":
				writeFile(t, filepath.Join(root, ".tiller/tiller.yml"), "schema_version: 12\nclaiming: {scope: feature}\n")
			}
			_, err := Load(root)
			var refused *EnvError
			require.ErrorAs(t, err, &refused)
			require.Equal(t, "obsolete_ownership", refused.Reason)
		})
	}
}

func TestLoadValidatesTransferredShelfLineage(t *testing.T) {
	for _, kind := range []string{"valid", "missing reciprocal", "cross noun", "cycle", "duplicate predecessor", "not cancelled", "not started", "duplicate resolution"} {
		t.Run(kind, func(t *testing.T) {
			root := ownershipTreeFixture(t)
			original := "id: B-aaaaaa\nkey: bugs/old\nstatus: cancelled\nstarted: true\nowner: ada\nresolution: transferred\nsuperseded_by: B-bbbbbb\n"
			replacement := "id: B-bbbbbb\nkey: bugs/new\nstatus: ready\nowner: bob\nsupersedes: [B-aaaaaa]\n"
			if kind == "missing reciprocal" {
				replacement = "id: B-bbbbbb\nkey: bugs/new\nstatus: ready\nowner: bob\n"
			}
			if kind == "cross noun" {
				original = "id: B-aaaaaa\nkey: bugs/old\nstatus: cancelled\nstarted: true\nowner: ada\nresolution: transferred\nsuperseded_by: CH-aaaaaa\n"
			}
			if kind == "cycle" {
				original += "supersedes: [B-bbbbbb]\n"
				replacement += "superseded_by: B-aaaaaa\n"
			}
			if kind == "duplicate predecessor" {
				replacement = "id: B-bbbbbb\nkey: bugs/new\nstatus: ready\nowner: bob\nsupersedes: [B-aaaaaa, B-aaaaaa]\n"
			}
			if kind == "not cancelled" {
				original = "id: B-aaaaaa\nkey: bugs/old\nstatus: in_progress\nstarted: true\nowner: ada\nresolution: transferred\nsuperseded_by: B-bbbbbb\n"
			}
			if kind == "not started" {
				original = "id: B-aaaaaa\nkey: bugs/old\nstatus: cancelled\nowner: ada\nresolution: transferred\nsuperseded_by: B-bbbbbb\n"
			}
			if kind == "duplicate resolution" {
				original += "duplicate_of: B-bbbbbb\n"
			}
			writeFile(t, filepath.Join(root, ".tiller/bugs/old/bug.yml"), original)
			writeFile(t, filepath.Join(root, ".tiller/bugs/new/bug.yml"), replacement)
			writeFile(t, filepath.Join(root, ".tiller/chores/new/chore.yml"), "id: CH-aaaaaa\nkey: chores/new\nstatus: ready\nsupersedes: [B-aaaaaa]\n")
			if kind != "cross noun" {
				writeFile(t, filepath.Join(root, ".tiller/chores/new/chore.yml"), "id: CH-aaaaaa\nkey: chores/new\nstatus: ready\n")
			}
			tree, err := Load(root)
			if kind == "valid" {
				require.NoError(t, err)
				require.Equal(t, "B-bbbbbb", *tree.Bugs["B-aaaaaa"].SupersededBy)
				require.False(t, tree.Bugs["B-bbbbbb"].Started)
			} else {
				require.Error(t, err)
			}
		})
	}
}

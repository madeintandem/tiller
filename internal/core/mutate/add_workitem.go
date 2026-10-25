package mutate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// WorkItemInput is what `tiller add bug|chore` records: the shelf noun,
// the LOCAL key, a title, and optionally where it was found, a source
// citation, and the bugs or chores it follows.
type WorkItemInput struct {
	Noun       string
	Key        string
	Title      string
	AttachedTo string
	Source     string
	DependsOn  []string
}

// AddWorkItem creates a proposed bug or chore on its shelf: the contract
// stub and the state file, and one add.<noun> event. Re-adding an existing key is the
// idempotent no-op every other add keeps.
func AddWorkItem(tree *store.Tree, deps Deps, in WorkItemInput) (Result, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return Result{}, err
	}
	plan, err := planAddWorkItem(tree, deps, owner, in)
	if err != nil {
		return Result{}, err
	}
	if !plan.result.Created {
		return plan.result, nil
	}
	if err := commitMutation(tree, "add "+in.Noun, plan.writes(), []events.Event{plan.event}); err != nil {
		return Result{}, err
	}
	return plan.result, nil
}

func planAddWorkItem(tree *store.Tree, deps Deps, owner store.Contributor, in WorkItemInput) (plannedAdd, error) {
	if !store.IsWorkNoun(in.Noun) {
		return plannedAdd{}, notShelfNoun(in.Noun)
	}
	local := strings.TrimSpace(in.Key)
	if strings.Contains(local, "/") || !renameKeySegmentRE.MatchString(local) {
		return plannedAdd{}, &usageError{reason: "invalid_key", msg: fmt.Sprintf("a %s key is one local key segment (%s), not %q; the shelf prefix is implied", in.Noun, renameKeySegmentRE.String(), in.Key)}
	}
	if strings.TrimSpace(in.Title) == "" {
		return plannedAdd{}, &usageError{reason: "invalid_title", msg: "add needs a non-blank --title"}
	}
	attached, err := resolveAttachTarget(tree, in.AttachedTo)
	if err != nil {
		return plannedAdd{}, err
	}
	dependsOn, err := resolveDependsOn(tree, in.DependsOn, "", dependsScope{Shelf: true})
	if err != nil {
		return plannedAdd{}, err
	}

	canonical := store.ShelfCanonicalKey(in.Noun, local)
	relDir := store.ShelfRelDir(in.Noun, local)
	relMd := filepath.Join(relDir, store.ShelfProseFile(in.Noun))
	relYml := filepath.Join(relDir, store.ShelfStateFile(in.Noun))
	mdPath := filepath.Join(tree.Root, relMd)
	ymlPath := filepath.Join(tree.Root, relYml)

	if fileExists(tree.Root, ymlPath) {
		doc, err := store.NewWorkNode(in.Noun)
		if err != nil {
			return plannedAdd{}, err
		}
		if err := readYAML(tree.Root, relYml, doc); err != nil {
			return plannedAdd{}, err
		}
		b := doc.Body()
		if err := refuseUnrecordedOnReAdd(tree, in.Noun, canonical, b.DependsOn, dependsOn); err != nil {
			return plannedAdd{}, err
		}
		return plannedAdd{result: Result{
			Noun: in.Noun, ID: b.ID, Key: deref(b.Key), Title: b.Title, Status: b.Status,
			AttachedTo: b.AttachedTo, Source: b.Source, DependsOn: b.DependsOn, Path: relDir, Created: false,
		}}, nil
	}

	prefix, err := id.PrefixFor(in.Noun)
	if err != nil {
		return plannedAdd{}, err
	}
	nid, err := deps.mintIn(tree, prefix)
	if err != nil {
		return plannedAdd{}, err
	}
	doc, err := store.NewWorkNode(in.Noun)
	if err != nil {
		return plannedAdd{}, err
	}
	*doc.Body() = store.WorkItem{
		ID: nid, Key: &canonical, Title: in.Title, Status: statusProposed,
		ProposedBy: owner.ID, AttachedTo: attached, Source: strings.TrimSpace(in.Source), DependsOn: dependsOn,
	}
	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return plannedAdd{}, err
	}

	ev := events.NewEvent(deps.clock(), workItemEventType("add", in.Noun), owner.ID, "add "+in.Noun+" "+local, nid)
	ev.ID = deps.mintRaw("EV")
	after := map[string]any{"status": statusProposed, "key": canonical}
	if attached != "" {
		after["attached_to"] = attached
	}
	ev.After = after
	ev.Files = []events.FileChange{{Path: relMd}, {Path: relYml}}

	prose := renderWorkItemMD(in.Noun, heading(in.Title, local))
	return plannedAdd{
		result: Result{
			Noun: in.Noun, ID: nid, Key: canonical, Title: in.Title, Status: statusProposed,
			AttachedTo: attached, Source: strings.TrimSpace(in.Source), DependsOn: dependsOn, Path: relDir, Created: true,
		},
		prose:  store.PlanWrite(mdPath, store.AbsentSnapshot(), []byte(prose)),
		state:  store.PlanWrite(ymlPath, store.AbsentSnapshot(), ydata),
		event:  ev,
		doc:    doc,
		ymlRel: relYml,
	}, nil
}

// renderWorkItemMD is a new shelf node's contract stub. The noun is
// validated before any write is planned, so an unknown one is a programming
// error and panics rather than writing a bug's stub.
func renderWorkItemMD(noun, title string) string {
	switch noun {
	case store.NounBug:
		return renderBugMD(title)
	case store.NounChore:
		return renderChoreMD(title)
	case store.NounTweak:
		return renderTweakMD(title)
	default:
		panic(fmt.Sprintf("mutate: no prose stub for %q", noun))
	}
}

// renderBugMD is the contract stub triage reads: reproduction, expected,
// actual, fixed-when (docs/plans/2026-08-28-bugs-and-chores-design.md,
// Two nouns, one body).
func renderBugMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Proposed bug created by `tiller add bug`. Write the contract here before\n"+
		"a human triages it: what happens, what should happen, and what \"fixed\" means.\n\n"+
		"## Reproduction\n\n## Expected\n\n## Actual\n\n## Fixed when\n", title)
}

// renderChoreMD is the chore's contract stub: what, why, done-when.
func renderChoreMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Proposed chore created by `tiller add chore`. Write the contract here before\n"+
		"a human triages it: what is being done, why, and what \"done\" means.\n\n"+
		"## What\n\n## Why\n\n## Done when\n", title)
}

func renderTweakMD(title string) string {
	return fmt.Sprintf("# %s\n\n"+
		"Proposed tweak created by `tiller add tweak`. Write the contract here before\n"+
		"a human triages it: what is being adjusted, why, and what \"done\" means.\n\n"+
		"## What\n\n## Why\n\n## Done when\n", title)
}

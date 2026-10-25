package views

// The correction block is the lighter readout a correction or other small
// action inside a gate renders (issue #94): `tiller readout <seam> --since
// <sha>`. A seam exit keeps the full block (readout.go); a rename, a remove,
// an added feature, or a link applied while the developer refines a draft
// used to re-emit that same full block, byte-identical but for the commit
// sha, so the developer read twenty-five lines to learn one feature was
// added. This block says only what changed since the commit the last block
// named, and keeps the two lines a reader still needs at the gate: the
// standing shelf queue line and the ranked Next list, produced by the same
// code the full block uses so the two can never disagree about what comes
// next.
//
// Changed is derived from the event stream, never from the agent's account:
// the CLI hands in the events git names as added since the sha, and each one
// renders as one line here, verb first, the node named by key path with its
// id. That keeps the readout's reason for existing intact for a correction:
// one format whoever is driving, copied rather than composed.

import (
	"fmt"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/workflow"
)

// Correction is the correction block before rendering. Changed is one line
// per event in stream order, already worded; the CLI renders the bullets.
// Queue and Next are the full block's own (ReadyShelfQueueLine, NextOf).
type Correction struct {
	Moment   Moment
	Headline string
	// Artifact is the commit the corrected tree sits at, under the identical
	// rule the full block applies (artifactOf): named only when the tree is
	// clean, empty otherwise.
	Artifact string
	// Since is the commit the previous block reported, as the caller gave it.
	Since    string
	Changed  []string
	Queue    string
	Next     Next
	Baseline *BaselineReadout
}

// CorrectionInput carries the facts CorrectionOfInput cannot derive from the
// tree alone, because views performs no I/O: the git facts the full block
// also reads, the acting contributor Next is ranked for, and the events git
// named as added since the sha, parsed by the caller. Recent and Now are the
// reader's recent events and the clock reading Next ranks them against,
// distinct from Events, which only feeds Changed.
type CorrectionInput struct {
	Commit           string
	Dirty            bool
	Contributor      string
	Since            string
	Events           []events.Event
	DesignDocPresent map[string]bool
	Workflow         workflow.Input
	Recent           []events.Event
	Now              time.Time
}

// CorrectionOfInput derives the correction block for a moment.
func CorrectionOfInput(t *store.Tree, m Moment, in CorrectionInput) Correction {
	return Correction{
		Moment:   m,
		Headline: correctionHeadlineFor(m),
		Artifact: artifactOf(in.Commit, in.Dirty),
		Since:    in.Since,
		Changed:  changedLines(t, in.Events),
		Queue:    ReadyShelfQueueLine(t),
		Next:     NextOf(t, NextInput{Reader: in.Contributor, Workflow: in.Workflow, Recent: in.Recent, Now: in.Now}),
		Baseline: baselineReadoutFor(t, m, in.DesignDocPresent, false),
	}
}

// correctionHeadlineFor is headlineFor's sibling for the correction block:
// the same subject each seam's full headline names, corrected rather than
// laid down.
func correctionHeadlineFor(m Moment) string {
	switch m {
	case MomentOrient:
		return "Project corrected"
	case MomentDeriveBaseline:
		return "Design baseline corrected"
	case MomentDecompose:
		return "Decomposition corrected"
	case MomentSpecify:
		return "Specs corrected"
	case MomentExecute:
		return "Work corrected"
	case MomentAmend:
		return "Amendments corrected"
	default:
		return string(m) + " corrected"
	}
}

// changedLines renders one line per event in the stream's own order
// (timestamp, then id), whatever order the caller handed them in. It always
// returns a non-nil slice: an empty Changed is a real answer ("nothing
// recorded"), which the renderer states in its own words.
func changedLines(t *store.Tree, evs []events.Event) []string {
	sorted := append([]events.Event(nil), evs...)
	events.Sort(sorted)
	lines := make([]string, 0, len(sorted))
	for _, ev := range sorted {
		lines = append(lines, changedLine(t, ev))
	}
	return lines
}

// changedLine words one event. The kinds a correction loop actually produces
// (add, add question, rename, remove, link, unlink, order) each get their own
// sentence; anything else renders as its type and targets, so no recorded
// event is ever silently dropped from the block.
func changedLine(t *store.Tree, ev events.Event) string {
	target := ""
	if len(ev.Targets) > 0 {
		target = ev.Targets[0]
	}
	switch {
	case ev.Type == "link.design", ev.Type == "unlink.design", ev.Type == "approve.design":
		return designChangedLine(ev, target)
	case ev.Type == "add.question":
		return addedQuestionLine(t, target, ev)
	case strings.HasPrefix(ev.Type, "add."):
		return "added " + refOrRecorded(t, target, mapString(ev.After, "key"))
	case strings.HasPrefix(ev.Type, "rename.title."):
		return fmt.Sprintf("renamed %s title from %q to %q",
			gen.RefByID(t, target), mapString(ev.Before, "title"), mapString(ev.After, "title"))
	case strings.HasPrefix(ev.Type, "rename.key."):
		return fmt.Sprintf("renamed %s key from %s to %s",
			gen.RefByID(t, target), mapString(ev.Before, "key"), mapString(ev.After, "key"))
	case strings.HasPrefix(ev.Type, "remove."):
		if line, ok := removedLine(ev); ok {
			return line
		}
	case strings.HasPrefix(ev.Type, "unlink.surface."):
		// Surface paths, not dependency ids: checked ahead of the unlink. edge
		// arm, which reads the same "removed" key.
		if removed, ok := mapStrings(ev.After, "removed"); ok {
			return fmt.Sprintf("removed surface %s from %s", strings.Join(removed, ", "), gen.RefByID(t, target))
		}
	case strings.HasPrefix(ev.Type, "link."):
		if added, ok := mapStrings(ev.After, "added"); ok {
			return edgesLine(t, "linked", target, added)
		}
	case strings.HasPrefix(ev.Type, "unlink."):
		if removed, ok := mapStrings(ev.After, "removed"); ok {
			return edgesLine(t, "unlinked", target, removed)
		}
	case strings.HasPrefix(ev.Type, "order."):
		if line, ok := orderedLine(t, target, ev); ok {
			return line
		}
	}
	return genericLine(t, ev)
}

func designChangedLine(ev events.Event, target string) string {
	if doc := mapString(ev.After, "doc"); doc != "" {
		target = ".tiller/design/" + doc
	}
	if ev.Type == "approve.design" {
		line := "confirmed design " + target
		if commit := mapString(ev.After, "commit"); commit != "" {
			line += " at " + commit
		}
		return line
	}
	verb := "linked design "
	if ev.Type == "unlink.design" {
		verb = "unlinked design "
	}
	line := verb + target
	for _, field := range []struct{ key, label string }{
		{"describes_added", "describes added"},
		{"describes_removed", "describes removed"},
		{"from_added", "source provenance added"},
		{"from_updated", "source provenance updated"},
		{"from_removed", "source provenance removed"},
	} {
		if paths, ok := mapStrings(ev.After, field.key); ok && len(paths) > 0 {
			line += "; " + field.label + ": " + strings.Join(paths, ", ")
		}
	}
	return line
}

// addedQuestionLine names a question the way the CLI's own add-question
// confirmation does, id then quoted short title, and what it blocks. The
// title comes from the tree when the question still exists, else from the
// event's own record.
func addedQuestionLine(t *store.Tree, target string, ev events.Event) string {
	title := mapString(ev.After, "title")
	if q, ok := t.Questions[target]; ok {
		title = gen.QuestionTitle(q)
	}
	line := fmt.Sprintf("added question %s (%q)", target, title)
	if blocks, ok := mapStrings(ev.After, "blocks"); ok && len(blocks) > 0 {
		refs := make([]string, 0, len(blocks))
		for _, id := range blocks {
			refs = append(refs, gen.RefByID(t, id))
		}
		line += "; blocks " + strings.Join(refs, ", ")
	}
	return line
}

// removedLine names the removed node from the event's own record (the tree
// no longer holds it) and counts what fell with it: an epic's removal takes
// its features, and the event lists every one.
func removedLine(ev events.Event) (string, bool) {
	raw, ok := ev.Before["removed"].([]any)
	if !ok || len(raw) == 0 {
		return "", false
	}
	first, ok := raw[0].(map[string]any)
	if !ok {
		return "", false
	}
	id, _ := first["id"].(string)
	key, _ := first["key"].(string)
	if id == "" {
		return "", false
	}
	line := "removed " + gen.RefParts(key, id)
	if beneath := len(raw) - 1; beneath > 0 {
		line += fmt.Sprintf(" and %d %s beneath it", beneath, plural(beneath, "node", "nodes"))
	}
	return line, true
}

// edgesLine summarizes the edges one link or unlink laid or lifted on a
// node: "linked 2 edges (1 cross-epic) on <ref>". The cross-epic clause
// appears only when the tree can resolve both ends of every edge as
// features, the one grain DependencyShape (depcheck.go) counts cross-epic
// edges at, so this count and the full block's "of which N cross-epic" can
// never disagree.
func edgesLine(t *store.Tree, verb, source string, edges []string) string {
	line := fmt.Sprintf("%s %d %s", verb, len(edges), plural(len(edges), "edge", "edges"))
	if crossEpic, ok := crossEpicEdges(t, source, edges); ok {
		line += fmt.Sprintf(" (%d cross-epic)", crossEpic)
	}
	return line + " on " + gen.RefByID(t, source)
}

// crossEpicEdges counts the edges from source to targets whose two ends sit
// in different epics. ok is false when source or any target is not a feature
// the tree holds, since an edge whose end cannot be resolved cannot be
// classified.
func crossEpicEdges(t *store.Tree, source string, targets []string) (int, bool) {
	from, ok := t.Features[source]
	if !ok {
		return 0, false
	}
	crossEpic := 0
	for _, id := range targets {
		to, ok := t.Features[id]
		if !ok {
			return 0, false
		}
		if from.EpicKey != to.EpicKey {
			crossEpic++
		}
	}
	return crossEpic, true
}

// orderedLine counts what one order call arranged: features under their
// epic, tasks under their feature, or one shelf's nodes, which have no
// parent to name.
func orderedLine(t *store.Tree, target string, ev events.Event) (string, bool) {
	order, ok := mapStrings(ev.After, "order")
	if !ok {
		return "", false
	}
	n := len(order)
	switch ev.Type {
	case "order.features":
		return fmt.Sprintf("ordered %d %s under %s", n, plural(n, "feature", "features"), gen.RefByID(t, target)), true
	case "order.tasks":
		return fmt.Sprintf("ordered %d %s under %s", n, plural(n, "task", "tasks"), gen.RefByID(t, target)), true
	}
	for _, shelf := range store.Shelves() {
		if ev.Type == "order."+shelf.Dir {
			return fmt.Sprintf("ordered %d %s", n, plural(n, shelf.Noun, shelf.Dir)), true
		}
	}
	return "", false
}

// genericLine is the fallback for an event kind with no sentence of its own:
// the type as recorded, then its targets resolved to references.
func genericLine(t *store.Tree, ev events.Event) string {
	if len(ev.Targets) == 0 {
		return ev.Type
	}
	refs := make([]string, 0, len(ev.Targets))
	for _, id := range ev.Targets {
		refs = append(refs, gen.RefByID(t, id))
	}
	return ev.Type + ": " + strings.Join(refs, ", ")
}

// refOrRecorded names a node by the tree's own reference when it still
// exists, else by the key the event recorded with the id: an added feature
// removed later in the same correction still reads as a key path, not a
// bare id.
func refOrRecorded(t *store.Tree, id, recordedKey string) string {
	if ref := gen.RefByID(t, id); ref != id {
		return ref
	}
	return gen.RefParts(recordedKey, id)
}

// mapString reads one string field off an event's before or after map, ""
// when absent or not a string.
func mapString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// mapStrings reads one list-of-strings field off an event's before or after
// map. YAML decodes a sequence into []any, so each element is checked; ok
// is false when the field is absent or not a sequence of strings.
func mapStrings(m map[string]any, key string) ([]string, bool) {
	if values, ok := m[key].([]string); ok {
		return values, true
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

package views

// The recap is the console's "what changed while you were away" (ui-surface.md,
// "History"): a mechanically derived digest of the event stream since a
// horizon, grouped by stream and owning epic, with the entries that created an
// ask for the reader flagged. It never narrates: every field is lifted from
// the recorded events and the tree. claim.feature, release.feature, and
// reassign.feature render like any other event (owner-lifecycle design,
// "Events": "recap and node timelines render them like any other event").

import (
	"sort"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

// recapRoutineCap bounds the routine entries one (epic, stream) cell carries.
// Asks are never capped: they are the whole point of the digest, and dropping
// one to make room for a routine event would lose the reader's own ask
// silently.
//
// It was 25, justified by a page the console rendered 27,000 pixels tall. That
// half of the justification is void: the recap view now collapses a run of
// same-type entries into ONE counted row, so a cell's height tracks how many
// KINDS of thing happened in it, not how many events (the live Acme digest,
// 167 events across 11 cells, renders 42 rows). Payload size is the only
// surviving constraint and it is not binding: those same 167 events serialize
// to 96,066 bytes, about 575 per entry, so a single cell sitting at 200 serves
// roughly 115KB, the same order as the whole digest the console already loads
// without complaint.
//
// The low cap was also not free, which is the reason to raise it rather than
// keep it. A capped cell makes every collapsed row beneath it a count of what
// the digest CARRIED rather than of what happened, and the honest repair is to
// stop the cap firing on real trees: the largest cell any real tree has
// produced is 32. Unlike /api/search this endpoint has exactly one consumer,
// the recap view, so there is no CLI twin the cap could regress.
const recapRoutineCap = 200

// Recap is the JSON payload of /api/recap. Since is the horizon rendered as
// RFC3339 in UTC, empty when no horizon exists (a first visit recaps nothing
// rather than everything). Window and Reader echo the caller verbatim: Reader
// is the resolved contributor id the asks were scoped to, empty when the checkout
// has no identity. Total is the true count of digested events BEFORE the
// per-group cap, so a reader is never told the served number is the whole
// number. Groups is always an array, never null.
type Recap struct {
	Since  string       `json:"since"`
	Window string       `json:"window"`
	Reader string       `json:"reader"`
	Total  int          `json:"total"`
	Groups []RecapGroup `json:"groups"`
}

// RecapGroup is one (epic, stream) cell of the digest: what one contributor's lane
// recorded under one owning epic. Stream is the acting contributor id, the same
// per-contributor lane StreamsOf rolls up for the board. Epic is nil for the
// project-level groups, the events whose first target resolves to no owning
// epic (a question, an unknown id), which always sort last. Entries
// is ONE newest-first timeline, asks included in their real place. Omitted is
// how many routine entries the cap dropped from this cell, 0 when nothing was
// dropped: the client states it rather than guessing, because a truncated list
// that poses as complete is worse than a long one.
type RecapGroup struct {
	Epic    *RecapEpic   `json:"epic"`
	Stream  string       `json:"stream"`
	Entries []RecapEntry `json:"entries"`
	Omitted int          `json:"omitted"`
}

// RecapEpic names a group's owning epic. It carries the reference explicitly
// because the recap is a serve-facing shape: the id and key ride structurally
// and Ref is the composed "<key-path> (<id>)" human label. Title rides for the
// same reason it rides on TargetRef: a rendered row leads with the node's
// name and its key follows as muted mono (ui-surface.md's console-scoped
// carve-out), and a group head that carried only the key had no name to lead
// with. Empty when the epic has no title, which is the client's cue to fall
// back to the key rather than render a blank.
type RecapEpic struct {
	Key   string `json:"key"`
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	Ref   string `json:"ref"`
}

// RecapEntry is one recorded event in the digest: when, what, who, the literal
// command, whether it created an ask for the reader, and the nodes it touched.
type RecapEntry struct {
	At          string      `json:"at"`
	Type        string      `json:"type"`
	Contributor string      `json:"contributor"`
	Command     string      `json:"command"`
	Ask         bool        `json:"ask"`
	Targets     []TargetRef `json:"targets"`
}

// TargetRef is one event target resolved against the tree: its noun, its id,
// its canonical key where the noun carries one, its prose handle, its human
// reference, and its recorded owner. An id the tree no longer knows keeps the
// bare id as its reference with no noun and no owner, so a recap never invents
// a node; an unowned node drops the owner the same way. Key and Title ride
// structurally like every other rendered row's payload (a list row, a queue
// entry, a related node), because a console row leads with a name and keeps
// the id off the line (ui-surface.md's console-scoped reference carve-out);
// Ref stays the composed label for callers that want the CLI's full
// convention.
//
// Title is what makes the carve-out reachable for the keyless nouns. An
// add.question is the digest's most common ask and a question has no key, so
// an ask row once had nothing to render but the bare id. The shared short title
// is the handle a reader recognizes, including the bounded legacy fallback
// supplied by handleOf through gen.QuestionTitle.
type TargetRef struct {
	Noun         string              `json:"noun"`
	ID           string              `json:"id"`
	Key          string              `json:"key,omitempty"`
	Title        string              `json:"title,omitempty"`
	Ref          string              `json:"ref"`
	Owner        string              `json:"owner,omitempty"`
	Contributors []store.Contributor `json:"contributors,omitzero"`
}

// RecapOf derives the digest from the event stream: the events after since,
// grouped by owning epic (resolved from each event's first target) and by the
// acting contributor's stream. Groups order by epic key with the project-level
// groups last, then by stream within the same epic.
// Inside a group the entries are ONE newest-first timeline, with the routine
// ones capped at recapRoutineCap (oldest dropped first) and the drop count
// reported as the group's Omitted; an ask is never counted against the cap and
// so can never be dropped. A zero since yields an empty recap, never
// everything. Window and reader are echoed verbatim; reader is the resolved
// owner id, empty when the checkout has no identity.
//
// A group used to emit its asks as a partition ahead of the rest, and that was
// wrong twice over. It never achieved asks-first on screen, because an
// add.question resolves to no owning epic and the epic-less group sorts LAST,
// so on a decomposition-phase tree every ask sat at the bottom of the payload
// anyway. And it reordered a group against itself: a lane whose feature was
// specced at 10:02 and submitted at 14:40 emitted submitted, specced, started,
// which a full-width view printing a timestamp per row reads as plainly wrong,
// and which breaks the adjacency the view's burst collapsing depends on. The
// cap-safety property that motivated the partition is kept without it: the cap
// counts routine entries only. Ask still rides on every entry, so a consumer
// that wants the asks picked out filters rather than relying on position.
func RecapOf(t *store.Tree, evs []events.Event, since time.Time, window, reader string) Recap {
	r := Recap{Since: formatTime(since), Window: window, Reader: reader, Groups: []RecapGroup{}}
	if since.IsZero() {
		return r
	}

	kept := make([]events.Event, 0, len(evs))
	for _, ev := range evs {
		if !ev.At.After(since) {
			continue
		}
		kept = append(kept, ev)
	}
	events.Sort(kept)

	// Walk the canonical order backward so entries land newest first, with
	// the event-id tiebreak the stream order defines. One list per bucket:
	// the capping below reads Ask off the entries rather than needing them
	// pre-partitioned, which is what keeps the emitted order chronological.
	type recapBucket struct {
		epic    *RecapEpic
		stream  string
		entries []RecapEntry
	}
	type recapGroupKey struct{ epic, stream string }
	buckets := map[recapGroupKey]*recapBucket{}
	for i := len(kept) - 1; i >= 0; i-- {
		ev := kept[i]
		epic := recapOwningEpic(t, ev)
		key := recapGroupKey{stream: ev.Contributor}
		if epic != nil {
			key.epic = epic.ID
		}
		b := buckets[key]
		if b == nil {
			b = &recapBucket{stream: ev.Contributor}
			if epic != nil {
				b.epic = &RecapEpic{
					Key: Key(epic), ID: epic.ID, Title: handleOf(epic), Ref: gen.Ref(t, epic),
				}
			}
			buckets[key] = b
		}
		b.entries = append(b.entries, recapEntryOf(t, ev, reader))
	}

	ordered := make([]*recapBucket, 0, len(buckets))
	for _, b := range buckets {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool {
		bi, bj := ordered[i], ordered[j]
		if (bi.epic == nil) != (bj.epic == nil) {
			return bj.epic == nil
		}
		if bi.epic != nil && bi.epic.Key != bj.epic.Key {
			return bi.epic.Key < bj.epic.Key
		}
		return bi.stream < bj.stream
	})
	for _, b := range ordered {
		// Cap in one pass over the timeline, counting only routine entries
		// against the cap. Because the walk is newest first, the entries the
		// cap refuses are exactly the OLDEST routine ones, and the survivors
		// are still in the order they arrived: capping never reorders.
		entries := make([]RecapEntry, 0, len(b.entries))
		routine, omitted := 0, 0
		for _, e := range b.entries {
			if !e.Ask {
				routine++
				if routine > recapRoutineCap {
					omitted++
					continue
				}
			}
			entries = append(entries, e)
		}
		r.Groups = append(r.Groups, RecapGroup{
			Epic: b.epic, Stream: b.stream, Entries: entries, Omitted: omitted,
		})
		// Total counts what the window held, not what the cap served.
		r.Total += len(b.entries)
	}
	return r
}

// recapAsk reports whether an event created an ask for the reader. A new open
// question always does: a question asks the team, whoever reads it. Ancestor
// arrivals at a human gate are shared asks, independent of historical
// contributors. Execution-unit arrivals ask their scalar owner; an empty
// reader keeps every gate arrival flagged.
//
// Arriving is a CROSSING, not a status: the after-status alone is not the
// question, because commitTransition writes the node's status on every event
// it records, including the ones that change nothing about it. mutate's
// saveEvalTarget records `tiller attest` and `tiller eval` with before and
// after both in_review, since attesting a gate settles evidence and leaves
// the node exactly where it was. Testing the after-status alone therefore read
// a human's own attestation as a fresh arrival at the gate they were standing
// at, and on a real tree, four gates attested and one eval run, the page's
// headline said "5 asked you" about five things the human had just done
// themselves. So the before-status has to differ: absent (an add that mints a
// node straight onto a ledger at proposed) or a genuinely different status.
func recapAsk(t *store.Tree, ev events.Event, reader string) bool {
	after, _ := ev.After["status"].(string)
	if ev.Type == "add.question" {
		return after == "" || after == "open"
	}
	switch after {
	case "specced", "proposed", "in_review":
	default:
		return false
	}
	if before, _ := ev.Before["status"].(string); before == after {
		return false
	}
	if !recapProposedTaskIsItsOwnAsk(t, ev, after) {
		return false
	}
	if reader == "" {
		return true
	}
	if len(ev.Targets) == 0 {
		return false
	}
	n := gen.NodeByID(t, ev.Targets[0])
	return n != nil && (n.Noun() == store.NounEpic || n.Noun() == store.NounFeature || Owner(n) == reader)
}

// recapProposedTaskIsItsOwnAsk answers, for an arrival at proposed, whether that
// arrival is a decision of its own. It is for a task added under a feature past
// its plan gate, the mid-flight addition the human approves individually. It is
// not for a task laid down as part of the specify explosion: that plan is
// approved by the feature's own gate, whose specced arrival is already the ask,
// and counting both would report two asks for one decision (cli-surface.md,
// "Specifying a feature"). Every other arrival passes through untouched.
func recapProposedTaskIsItsOwnAsk(t *store.Tree, ev events.Event, after string) bool {
	if after != "proposed" || len(ev.Targets) == 0 {
		return true
	}
	tk, ok := gen.NodeByID(t, ev.Targets[0]).(*store.Task)
	if !ok {
		return true
	}
	f := t.Features[tk.FeatureID]
	return f != nil && approvesTasksIndividually(f)
}

// recapEntryOf projects one event onto the digest entry, resolving each target
// against the tree. Targets is always an array, never null.
func recapEntryOf(t *store.Tree, ev events.Event, reader string) RecapEntry {
	targets := make([]TargetRef, 0, len(ev.Targets))
	for _, id := range ev.Targets {
		targets = append(targets, recapTargetRef(t, id))
	}
	return RecapEntry{
		At:          formatTime(ev.At),
		Type:        ev.Type,
		Contributor: ev.Contributor,
		Command:     ev.Command,
		Ask:         recapAsk(t, ev, reader),
		Targets:     targets,
	}
}

// recapTargetRef resolves one target id across the tree's node maps.
func recapTargetRef(t *store.Tree, id string) TargetRef {
	n := gen.NodeByID(t, id)
	if n == nil {
		return TargetRef{ID: id, Ref: id}
	}
	return TargetRef{
		Noun:         n.Noun(),
		ID:           id,
		Key:          Key(n),
		Title:        handleOf(n),
		Ref:          gen.Ref(t, n),
		Owner:        Owner(n),
		Contributors: Contributors(t, n),
	}
}

// recapOwningEpic resolves the epic an event belongs under from its FIRST
// target: a task climbs to its feature and then the epic, a feature to its
// epic, an epic is its own group, and everything else (a question, a dangling
// id, no targets at all) lands in the project-level nil group.
func recapOwningEpic(t *store.Tree, ev events.Event) *store.Epic {
	if len(ev.Targets) == 0 {
		return nil
	}
	id := ev.Targets[0]
	if e, ok := t.Epics[id]; ok {
		return e
	}
	if f, ok := t.Features[id]; ok {
		return recapEpicByKey(t, f.EpicKey)
	}
	if tk, ok := t.Tasks[id]; ok {
		if f, ok := t.Features[tk.FeatureID]; ok {
			return recapEpicByKey(t, f.EpicKey)
		}
	}
	return nil
}

// recapEpicByKey finds an epic by its canonical key, the link a feature
// carries through the directory layout.
func recapEpicByKey(t *store.Tree, key string) *store.Epic {
	if key == "" {
		return nil
	}
	for _, e := range t.Epics {
		if e.Key != nil && *e.Key == key {
			return e
		}
	}
	return nil
}

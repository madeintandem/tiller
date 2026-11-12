# Event compaction: design

**Status.** Settled with JC on 2026-11-11, seven decisions. Not built. The task breakdown is in [2026-11-11-event-compaction-implementation.md](./2026-11-11-event-compaction-implementation.md).

**Scope.** How the event stream is stored once it grows: what a compacted segment is, when compaction runs, how readers treat the two forms, and how existing projects cross over. The write path does not change. [artifact-schema.md](../spec/artifact-schema.md) owns the on-disk model and takes this design's wording when it is built.

## The problem

Every mutation writes one YAML file under `.tiller/events/YYYY/MM/DD/`. On the Acme platform build that is 7,508 files at roughly the halfway point, which is 60 percent of the 12,435 files the repository tracks. They hold 2.5 MB of content and take 29 MB on disk, and each of the project's 28 worktrees carries its own copy. Nothing bounds the count: a finished project of that size ends near 15,000 event files, and a larger one ends at a multiple of that.

This is not mainly a speed problem. The console caches parsed events, and the read paths were tuned in October. The problem is the shape of the repository: a state folder that grows by a file per command, forever.

## What stays the same

Writes stay one immutable YAML file per event. The reason the schema gives still holds: parallel branches merge their histories by adding files, never by editing a shared log, and the event write joins the journaled transaction of the state change it describes.

Events are still not the source of truth for current state. Sorting is still by timestamp, then event id.

## The design

### Two tiers

Recent events stay loose, exactly as today. Older events are folded into segments, one or more per day:

```
.tiller/events/2026/11/09/EV-abc123.yml        loose, one per mutation
.tiller/events/2026/10/13-3f9a2c41d0aa.jsonl   segment, every event of that day
.tiller/events/2026/10/13-b81e07c2f913.jsonl   a later segment for the same day
```

A segment sits beside its day directory, named by the day and the first twelve hex characters of the SHA-256 of its own bytes. Once a day is compacted its directory is gone and only the segment remains.

### Segments are immutable

Compaction never opens an existing segment for writing and never deletes one. Events that reach the integration branch after their day was compacted go into an additional segment for that day.

This carries the existing rule up one level. Two checkouts that compact the same set of events write the same name and the same bytes, so git sees one change. Two checkouts that compact different sets write different names, so both files survive and nothing conflicts. No `.gitattributes` entry, merge driver, or hook is involved, which keeps managed projects independent of Tiller.

The cost is that a day can hold more than one segment, and that two segments can overlap. Overlap is never cleaned up. Removing a redundant segment from two checkouts at once is a way to lose events, and the overlap costs only a few duplicated lines.

### Readers count each event once

A reader takes the union of loose files and segments and counts an event id once, whichever form or forms hold it. An id present twice with equal content is one event. An id present twice with different content refuses the read and names both paths, as an unparseable event does today.

### Segment format

A segment is JSONL: one event per line, sorted by timestamp then id, each line the canonical JSON encoding of the decoded event with keys in a fixed order.

JSONL was chosen over a YAML multi-document file for read cost at scale. Measured on the Acme history replicated to 300,000 events (about 100 MB), a full read took 8.45 seconds in YAML on one core and 2.39 seconds on fifteen, against 1.42 seconds and 0.34 seconds in JSONL. YAML's parse cost is the one downside that grows with the project. JSONL's downsides are fixed, and the design pays them once:

- **Compaction re-encodes.** It cannot prove a segment by comparing bytes, so it proves it by value: it decodes the new segment and compares every event with the loose event it came from before deleting anything.
- **Legacy records are normalized.** Events written before schema 9 carry `owner:` where current ones carry `contributor:`. Compaction writes `contributor`. This is the one place Tiller rewrites an old event, and nothing is lost but the old key name.
- **One decode boundary normalizes types.** A whole number is an `int`, another number a `float64`, and a timestamp inside `before` or `after` an RFC 3339 string, whether the event was read from YAML or JSON. A consumer never sees a different Go type because an event was compacted.

An event that compaction cannot carry faithfully stays loose: one with a key the event shape does not define, a value JSON cannot hold, or content that does not survive the round trip. Compaction reports how many it left and carries on. Loose files are readable forever, so this is a safe place to stop.

### The delay

A day is compacted once it is a set number of whole UTC days old. The default is 7. A project sets its own in the manifest:

```yaml
events:
  compact_after_days: 14
```

With immutable segments the delay no longer protects correctness. It trades two kinds of clutter. A longer delay leaves more loose files waiting; a shorter one produces more small straggler segments, because more events land after their day was compacted. On Acme 98 percent of events reached the integration branch the day they were written and the slowest took five days, so a delay of 7 would have produced no stragglers. That figure only sizes the default. Nothing depends on it, and a branch that lands three weeks late is handled the same way as one that lands in an hour.

Different delays on different checkouts are harmless. The setting decides which days are eligible, not what goes into a day's segment.

### Where compaction runs

Compaction runs on the integration branch. `tiller compact` is the verb, and `tiller sync` calls it in the normal flow. `tiller compact --all` ignores the delay and compacts every day, today included. `tiller compact --dry-run` reports what would be compacted and writes nothing. Run on a work branch, `tiller compact` refuses and names the integration branch, as `tiller usage` does.

Work branches keep adding loose files and keep merging cleanly. Deleting a loose file on the integration branch cannot conflict with a work branch, because no branch ever modifies an event file.

### Inside sync

Sync compacts as a best-effort step after its own work succeeds, the way it captures agent usage. A compaction problem never fails sync and never changes its exit code; it becomes one warning line. Sync reports compaction only when it compacted something, and says nothing off the integration branch.

Compaction records no event. It changes how history is stored, not what happened in the project, and git already shows who compacted and when. Compaction should be invisible everywhere except in sync's own output: no console page and no CLI read may answer differently because a day was compacted.

### Committing a compaction

Sync still does not commit. A compaction leaves deletions and new segments in the working tree, and whoever commits the sync commits them.

The risk here is a partial commit. An agent that stages the deletions without the new segment produces a commit that has lost a day of events. They stay recoverable from older history, but timelines would silently show a hole. Two things close that gap. Sync's compaction line prints the exact `git add` command, naming each emptied day directory and each new segment. The orientation and the execute skill say to stage those paths as given, as the one exception to the rule against staging a whole directory.

A two-step variant, writing the segment in one run and deleting the loose files only after the segment is seen in committed history, was considered and rejected. It makes the partial commit impossible but turns every day's compaction into two commits, and a process the user must watch across runs is more brittle than one that finishes.

### Existing projects

The schema moves from 13 to 14. Binaries already require an exact schema match, so an older binary refuses a compacted tree instead of misreading it.

The upgrade does the first compaction itself. `tiller upgrade` and `tiller migrate` stamp schema 14 and then compact, in the same command, under the ordinary rules including the delay, on whichever branch runs the upgrade. That is the single exception to integration-only, and immutable segments make it safe: a work branch that runs the migration on its own writes identical segments for the history it shares.

The stamp is one transaction and each day's compaction is its own, so the journal stays small and an interrupted upgrade leaves a valid schema 14 tree. The next sync on the integration branch finishes whatever is left. A first compaction on a large project may take a while; that is accepted.

Migrate's promise changes. It preserved "historical event bytes"; it now preserves event content.

## Decisions settled

- **Integration only.** Compaction runs on the integration branch, through `tiller compact`, which `sync` calls.
- **Immutable, content-named segments.** Late events get another segment for their day. Readers count each event id once. Overlap is never garbage-collected.
- **JSONL, re-encoded.** Loose files stay YAML. Legacy `owner:` records are normalized. One decode boundary normalizes types. Compaction verifies by value before deleting.
- **Schema 14, and the upgrade compacts.** The migration stamps, then compacts under the ordinary rules, on the branch that runs it.
- **A 7-day default, set per project.** Whole UTC days, `events.compact_after_days`, with `compact --all` to ignore it.
- **Quiet inside sync, and no event.** Best effort, one line when it did something, nothing recorded in the stream.
- **One step, with the paths handed over.** Sync prints the command to stage; the skills say to run it.

## Not built here

- **A local index or cache for very large histories.** At 300,000 events a full read still costs 0.3 to 1.4 seconds and holds about 450 MB once parsed. The fix at that size is to stop reading the whole history per command. Content-named immutable segments make a cache keyed by segment name easy to add later.
- **Loose events in JSON.** Switching the write format would give one decoder for everything and would not touch any segment already written.
- **Splitting a very large day.** The execution view reads a committed blob of at most 8 MiB. A single day reaches that at roughly 23,000 events, about thirty times Acme's busiest day.

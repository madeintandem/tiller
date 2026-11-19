# The console does not show shelf batches

## What

Show shelf batches in the console. A batch is the record that reviews several
bugs, chores, or tweaks on one branch and one pull request
(tweaks/several-shelf-items-one-review, TW-taq0ps, which this tweak follows).

## Why

TW-taq0ps delivers batches to the CLI, the queue, the readout, and the
guidance, and leaves the console alone by developer ruling on 2026-11-19, to
keep each review small. Until this tweak lands, the console lists a batched
member with its stored status, usually `ready`, while its fix is in progress
or in review on the batch, and it shows no batch anywhere. A reader of the Fix
page would take batched work for untouched work.

## Done when

- The Fix page lists each live batch as one row with its title, status,
  owner, member count, and highest member severity.
- A batch has its own page showing its status, branch, PR, gates, and every
  member with a link to it.
- A batched member's page names its batch with a link, and its status reads
  `batched` rather than its stored status.
- A batched member is not listed on Fix or Home as work waiting to start.
- Home's needs-you columns treat a batch as one unit of work.
- Search finds a batch by id, key, and title.
- An in-review batch waiting on the human's merge appears on Home, and Home
  never reads "All clear" while one is waiting.
- An owner's in-progress batch appears in Home's In flight.
- Every batch link in Up next, Needs you, Just happened, and History opens the
  batch page.
- Batch events read correctly in History and Just happened: membership edits
  have a past-tense phrase, the plural reads "batches", and a membership edit
  is not ranked as a milestone above the batch being added or claimed.
- `TestVocabularyCoversEveryTransition` covers the batch noun again, with its
  event types added to `ui/src/lib/vocabulary.json`.
- The status glyph for `batched` is distinguishable by shape, not only color.
- `ui/dist` is rebuilt in the same change.

# An On this page link reloads the console in Safari and does not land on its heading

## Reproduction

Reported from Safari on macOS against a console served over the network
(v1.0.3). Not reproduced in headless WebKit, where the same links scroll to
their heading in place, so the cause in Safari is not established.

1. Run `tiller serve` and open Explore in Safari.
2. Open an epic or feature whose spec has at least three section headings, in
   a window wide enough to show the "On this page" list beside the spec.
3. Click any link in that list.

## Expected

The reading pane scrolls so the chosen heading sits just below the pinned
header and tab rail, and nothing is reread. `ui/src/components/ProseOutline.tsx`
describes the list as navigation only, one in-page anchor per section heading,
and `docs/spec/ui-surface.md` says an outline link rereads nothing.

## Actual

The console reloads in full and comes back at the top of the page, not at the
chosen heading. Two things are wrong at once: the link is left to the browser
as a bare `#id` anchor, so whether it stays in the page is the browser's
decision, and the console does nothing with a heading id in the address when
a node page loads, so a reload or a shared link never lands on the heading.

## Fixed when

- In Safari, clicking an "On this page" link scrolls the reading pane to that
  heading, below the pinned header and rail, with no reload and no reread.
- The address carries the heading's id after the click, without adding a
  history entry.
- Opening or reloading a node page whose address names a heading id lands on
  that heading once the spec has rendered.
- Tests cover the click and the landing.

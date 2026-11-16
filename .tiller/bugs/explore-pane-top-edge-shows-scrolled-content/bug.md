# A one-pixel line of scrolled content shows above the pinned node header in Safari

## Reproduction

Observed in Safari on macOS at a 2x display, console v1.0.3, on a project with
long epic and feature specs. Reported from two screenshots; not reproduced in
headless WebKit, which lays the page out identically but does not paint the
line.

1. Run `tiller serve` and open Explore in Safari on the desktop-width split.
2. Open an epic or feature whose Spec tab is longer than the reading pane.
3. Scroll the reading pane until the tab rail pins and the one-line header
   (kind pill, status chip, title) appears above it.
4. Keep scrolling and watch the top edge of the reading pane, just above the
   one-line header.

## Expected

Nothing shows above the pinned one-line header. `ui/src/components/node-pin.css`
states the rule: the pin and the compact header repaint the pane's paper "so
scrolled content never shows through".

## Actual

One device-pixel row of the scrolled spec text shows at the very top edge of
the reading pane, above the header's paper. In the screenshots the row sits
22.5 CSS px above the kind pill, which is the pane's top edge. The pane's top
edge lands on a fractional device pixel there (130.75 CSS px in a WebKit
measurement of the same layout, 261.5 device px at 2x), and the line is that
partial row.

## Fixed when

- In Safari at 2x, scrolling a long epic or feature spec on Explore shows no
  scrolled content above the pinned one-line header, at any scroll position.
- The reading pane's content, the split divider, and the navigator look and
  behave as before at rest and while scrolling.
- A stylesheet test pins the rule that covers the pane's top edge.

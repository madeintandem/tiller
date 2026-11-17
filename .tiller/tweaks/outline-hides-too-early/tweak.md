# On this page outline hides while it still fits beside the spec

## What

Lower the width at which a node page's "On this page" outline hides, so it stays beside the spec until the reading pane is truly too narrow for both. Today `ui/src/styles/prose.css` hides the outline whenever the outline host is under 1100px. The outline takes 220px plus a 40px gap, and the prose column already shrinks (`minmax(0, 72ch)`), so the outline only needs to go when the prose would drop below a comfortable reading width. Keep the outline while the prose column keeps at least 480px (about 54 characters of body text): hide it under a 740px host.

## Why

Measured in WebKit on 2026-11-17 with the default 300px Explore tree, 72ch of 14px Inter is 636px, so the full layout needs 896px of host. At a 1280px window the host is 903px and the outline is hidden although it fits at full measure; at 1440px the host is 1063px and it is still hidden. Widening the tree on a laptop hides it sooner still, well before the spec runs out of room.

## Done when

- At a 1280px window with the default tree, and at 1440px with the tree widened by up to about 300px, the outline shows beside the spec.
- The outline hides, and the spec runs single-column, only when the outline host is under 740px.
- A stylesheet test pins the threshold.
- `ui/dist` is rebuilt in the same change.

# Stage 2, video 2: from idea to approved spec. The baseline is confirmed and
# no work is planned; the developer writes a one-page design, the agent
# decomposes it, the developer approves the shape, the agent specifies the
# feature, and the developer approves the plan.
# Sourced by run.sh after stage 1, in the same checkout.

E=orders F=orders/status-filter T=orders/status-filter/filter-by-status
FDIR=.tiller/epics/orders/features/status-filter

# ---------- no work yet; the developer writes the idea down ----------
run s2-guide-empty you tiller guide
say s2-say-idea you "The warehouse wants orders by status. Writing that down."
edit s2-write-design you .tiller/design/status-filter.md put stage2/status-filter-design.md .tiller/design/status-filter.md
run s2-design-add you git add .tiller/design/status-filter.md
run s2-design-commit you git commit -m "Design: filter orders by status"
serve_step s2-serve you
mark s2-design
say s2-say-ask you "Run tiller guide and follow what it says."
run s2-guide agent tiller guide

# ---------- the agent proposes the shape ----------
say s2-say-decompose agent "Reading the design and the as-is docs, then proposing the work."
run s2-add-epic agent tiller add epic "$E" --title "Order filtering"
run s2-add-feature agent tiller add feature "$F" --title "Filter orders by status" --change-type modify
run s2-surface agent tiller link feature "$F" --surface src/server.js --surface src/orders.js
edit s2-write-epic agent .tiller/epics/orders/epic.md put stage2/epic.md .tiller/epics/orders/epic.md
edit s2-write-feature agent "$FDIR/feature.md" put stage2/feature-draft.md "$FDIR/feature.md"
run s2-decompose-add agent git add -- .tiller/epics .tiller/events
run s2-decompose-commit agent git commit -m "decompose: order filtering"
say s2-say-shape agent "One epic, one feature. It changes existing code, so it names both surfaces."
run s2-readout-decompose agent tiller readout decompose
mark s2-proposed

# ---------- the developer approves the shape ----------
run s2-approve-epic you tiller approve epic "$E"
run s2-approve-epic-add you git add .tiller
run s2-approve-epic-commit you git commit -m "Approve order filtering"
say s2-say-specify you "Specify the status filter."

# ---------- the agent specifies: spec, contract, task ----------
var FEATURE_ID "$(node_id feature "$F")"
say s2-say-spec agent "Writing the spec, its acceptance contract, and the task."
edit s2-write-spec agent "$FDIR/feature.md" put stage2/feature.md "$FDIR/feature.md"
edit s2-write-contract agent "$FDIR/feature.eval.md" put stage2/feature.eval.md "$FDIR/feature.eval.md"
run s2-add-task agent tiller add task "$T" --title "Filter the order list by status"
edit s2-write-task agent "$FDIR/tasks/filter-by-status/task.md" put stage2/task.md "$FDIR/tasks/filter-by-status/task.md"
run s2-spec agent tiller spec feature "$F"
run s2-spec-add agent git add -- .tiller/epics .tiller/events
run s2-spec-commit agent git commit -m "specify: status-filter"
say s2-say-planned agent "Spec, two acceptance criteria, and one task. Ready for your review."
run s2-readout-specify agent tiller readout specify
mark s2-specced

# ---------- the developer approves the plan ----------
say s2-say-read you "Reading the spec, the contract, and the task."
run s2-approve-feature you tiller approve feature "$F"
run s2-approve-feature-add you git add .tiller
run s2-approve-feature-commit you git commit -m "Approve the status filter plan"
mark s2-end

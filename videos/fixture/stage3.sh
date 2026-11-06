# Stage 3, video 3: build it and ship it. The agent builds the task on its
# own branch and submits it; a code review finds the unknown-status bug; the
# developer sends it back to the task branch and the agent fixes it; the
# developer merges; sync, the feature eval on main, and a second sync roll
# the feature up to done.
# Sourced by run.sh after stage 2, in the same checkout.

E=orders F=orders/status-filter T=orders/status-filter/filter-by-status
BRANCH=tiller/$T
REVIEW_DATE=2026-11-06
TASK_ID=$(node_id task "$T")
FEATURE_ID=$(node_id feature "$F")
REVIEW=.tiller/reviews/$REVIEW_DATE-$FEATURE_ID.md

# ---------- the agent builds on the task branch ----------
say s3-say-ask you "Run tiller guide and follow what it says."
run s3-guide agent tiller guide
run s3-claim agent tiller claim task "$T"
run s3-start agent tiller start task "$T"
run s3-branch agent git checkout -b "$BRANCH"
run s3-start-add agent git add .tiller
run s3-start-commit agent git commit -m "$TASK_ID start filter-by-status"

say s3-say-tests agent "Writing the tests first."
put stage3/impl/status-filter.test.js test/status-filter.test.js
run s3-tests-add agent git add test/status-filter.test.js
run s3-tests-commit agent git commit -m "$TASK_ID tests for the status filter"
run s3-check-red-expect-fail agent tiller check task "$T"
say s3-say-red agent "Red, as it should be. Now the implementation."
run s3-red-add agent git add .tiller
run s3-red-commit agent git commit -m "$TASK_ID record the failing check"

put stage3/impl/orders.js src/orders.js
put stage3/impl/server.js src/server.js
run s3-impl-add agent git add src/orders.js src/server.js
run s3-impl-commit agent git commit -m "$TASK_ID filter GET /orders by status"
run s3-check agent tiller check task "$T"
run s3-check-add agent git add .tiller
run s3-check-commit agent git commit -m "$TASK_ID record gate evidence"
run s3-submit agent tiller submit task "$T"
run s3-submit-add agent git add .tiller
run s3-submit-commit agent git commit -m "$TASK_ID record review ref"
run s3-readout-execute agent tiller readout execute
say s3-say-submitted agent "In review on its branch. Over to you."
mark s3-submitted

# ---------- the developer asks for a code review ----------
say s3-say-review you "code review feature $FEATURE_ID"
run s3-review-tree agent env GIT_OPTIONAL_LOCKS=0 tiller tree feature "$F" --status --files
run s3-review-diff agent git --no-optional-locks diff --no-ext-diff --no-textconv --stat main "$BRANCH" -- . ":(exclude).tiller"
var FEATURE_ID "$FEATURE_ID"
var TASK_ID "$TASK_ID"
var REVIEW_DATE "$REVIEW_DATE"
var MAIN_SHA "$(git rev-parse --short main)"
var BRANCH_SHA "$(git rev-parse --short "$BRANCH")"
put stage3/review.md "$REVIEW"
say s3-say-finding agent "Full review: $REVIEW. 1 bug (1 blocker): B1, an unknown status returns every order instead of a 400 (violates C2)."
say s3-say-unmerged agent "Unmerged: \"fix B1 on its branch\" sends it back to $T ($TASK_ID)."
mark s3-review

# ---------- fix B1 on its branch ----------
say s3-say-fix you "fix B1 on its branch"
run s3-to-main agent git checkout main
sed -i.bak "s|^#### B1 .*|& → requested change on $T ($TASK_ID), rechecked at $(git rev-parse --short "$BRANCH")|" "$REVIEW"
rm "$REVIEW.bak"
run s3-review-add agent git add -- "$REVIEW"
run s3-review-commit agent git commit -m "code-review: $FEATURE_ID fix B1 on $T"
run s3-to-branch agent git checkout "$BRANCH"
run s3-resume agent tiller resume task "$T" --reason "address review B1: answer 400 for an unknown status"
put stage3/fix/orders.js src/orders.js
put stage3/fix/server.js src/server.js
put stage3/fix/status-filter.test.js test/status-filter.test.js
run s3-fix-add agent git add src/orders.js src/server.js test/status-filter.test.js .tiller
run s3-fix-commit agent git commit -m "$TASK_ID answer 400 for an unknown status (review B1)"
run s3-recheck agent tiller check task "$T"
run s3-recheck-add agent git add .tiller
run s3-recheck-commit agent git commit -m "$TASK_ID record gate evidence"
run s3-resubmit agent tiller submit task "$T"
run s3-resubmit-add agent git add .tiller
run s3-resubmit-commit agent git commit -m "$TASK_ID record review ref"
say s3-say-fixed agent "B1 fixed on the same branch, with tests for lost, Shipped, and empty. Back in review."
run s3-readout-resubmit agent tiller readout execute
mark s3-fixed

# ---------- the developer merges; the agent closes out on main ----------
say s3-say-merge you "Reads right. Merging."
run s3-checkout-main you git checkout main
run s3-merge you git merge --no-ff "$BRANCH" -m "Merge filter orders by status"
say s3-say-merged you "Merged."
run s3-sync agent tiller sync
run s3-sync-add agent git add .tiller
run s3-sync-commit agent git commit -m "Record the task merge"
run s3-check-feature agent tiller check feature "$F"
run s3-check-feature-add agent git add .tiller
run s3-check-feature-commit agent git commit -m "Record feature acceptance"
run s3-sync-rollup agent tiller sync
run s3-rollup-add agent git add .tiller
run s3-rollup-commit agent git commit -m "Record the rollup"
run s3-status agent tiller status feature "$F"
run s3-tree agent tiller tree --status
say s3-say-done agent "Filter orders by status is done on main."
mark s3-end

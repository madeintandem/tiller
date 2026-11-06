# Stage 1, video 1: set Tiller up on a repo you already have. The Acme Orders
# app already exists with its own history; the developer adds Tiller, the
# agent derives the as-is baseline, and the developer confirms it.
# Sourced by run.sh inside the empty fixture checkout.

# ---------- the existing app: two commits of history, no .tiller ----------
git init -q -b main
git config user.name "JC Grubbs"
git config user.email "jc@example.com"
git config commit.gpgsign false
cp -R "$APP/." .
git add package.json src test bin
GIT_AUTHOR_DATE="2026-10-12T15:04:00Z" GIT_COMMITTER_DATE="2026-10-12T15:04:00Z" \
  git commit -qm "Order book and the read-only orders API"
git add README.md docs
GIT_AUTHOR_DATE="2026-10-14T10:31:00Z" GIT_COMMITTER_DATE="2026-10-14T10:31:00Z" \
  git commit -qm "Document the orders API"

# ---------- add Tiller ----------
run s1-version you tiller --version
run s1-init you tiller init --name "Acme Orders" --key acme --kind brownfield
run s1-add-contributor you tiller add contributor jc --name "JC Grubbs"
run s1-join you tiller join --as jc

# The tests gate is the human's to wire, from the stack the repo already has.
say s1-say-gate you "Point the tests gate at bin/test."
sed -i.bak 's|^    # tests: { kind: command, run: "<test command>",  on: \[feature, task\] }|    tests: { kind: command, run: "bin/test", on: [feature, task] }|' .tiller/tiller.yml
rm .tiller/tiller.yml.bak
grep -q '^    tests: { kind: command, run: "bin/test"' .tiller/tiller.yml

run s1-git-add you git add -A
run s1-commit you git commit -m "Add Tiller"
mark s1-start

# ---------- the agent derives the baseline ----------
say s1-say-ask you "Run tiller guide and follow what it says."
run s1-guide agent tiller guide
say s1-say-read agent "Reading the README, docs/orders.md, and the code under src/."
run s1-readiness-expect-fail agent tiller check readiness
say s1-say-choice agent "docs/orders.md covers the API and the order shape. Move it into Tiller, or keep it and point to it?"
say s1-say-move you "Move it."

put stage1/orders-api.md .tiller/design/as-is/orders-api.md
put stage1/order-store.md .tiller/design/as-is/order-store.md
run s1-link-api agent tiller link design as-is/orders-api.md --describes src/server.js --from docs/orders.md
run s1-link-store agent tiller link design as-is/order-store.md --describes src/orders.js --from docs/orders.md
run s1-retire agent git rm -q docs/orders.md
put stage1/README.md README.md
run s1-baseline-add agent git add -- README.md .tiller/design .tiller/events
run s1-baseline-commit agent git commit -m "derive-baseline: as-is orders API and order store"
say s1-say-notes agent "Split docs/orders.md into two module docs, moved the source, repointed the README. No disagreements with the code."
run s1-readout agent tiller readout derive-baseline
mark s1-readout

# ---------- the developer confirms it ----------
say s1-say-review you "Both docs match the code."
run s1-approve-design you tiller approve design as-is/orders-api.md as-is/order-store.md
run s1-confirm-add you git add .tiller
run s1-confirm-commit you git commit -m "Confirm the as-is baseline"
run s1-status you tiller status
mark s1-end

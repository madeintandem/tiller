# Shared helpers for the screenshot fixtures (acme.sh, homepage.sh). Both run
# inside the fixture's checkout with TILLER naming the binary.

commit() { git add -A && git commit -qm "$1"; }

# A feature's specify-stage output: the specification, the acceptance
# contract, and one task.md per task already added under it.
spec_feature() { # <epic/feature> "<Title>" "<summary sentence>"
  local key=$1 title=$2 summary=$3
  local dir=".tiller/epics/${key%%/*}/features/${key##*/}"
  local id
  id=$($TILLER show feature "$key" --json | grep -om1 'F-[a-z0-9]\{6\}')
  cat > "$dir/feature.md" <<EOF
# $title

## Summary

$summary

## Specification

The feature lands behind the existing application shell and reuses the
project's own conventions for models, routes, and views. Every user-facing
state is reachable from the primary navigation, and every write is covered
by a request spec.
EOF
  cat > "$dir/feature.eval.md" <<EOF
---
node: $id
criteria:
  - id: C1
    statement: The feature's primary route renders and its request specs pass
    polarity: introduce
    binding: { type: command, run: "true" }
---

# Acceptance

The acceptance contract binds to the project's test command; the criterion
above is satisfied when the route renders and its specs are green.
EOF
  for task in "$dir"/tasks/*/; do
    [ -d "$task" ] || continue
    local name
    name=$(basename "$task")
    # A task.md already written past the add-task stub keeps its own words.
    grep -q 'Proposed task created by' "$task/task.md" || continue
    cat > "$task/task.md" <<EOF
# ${name//-/ }

## Approach

Implement the slice named in the title, prove it with a request spec, and
leave the build green.
EOF
  done
  $TILLER spec feature "$key" --contributor robin >/dev/null
}

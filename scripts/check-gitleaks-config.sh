#!/usr/bin/env bash
# Prove the repository's fixture allowlist ignores node-key slug paths without
# hiding credential-shaped values from gitleaks' default rules.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
TMP=$(mktemp -d "${TMPDIR:-/tmp}/tiller-gitleaks.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

if [[ $# -gt 0 ]]; then
  GITLEAKS=("$1")
else
  GOBIN="$TMP/bin" go install github.com/zricethezav/gitleaks/v8@v8.30.1 >/dev/null
  GITLEAKS=("$TMP/bin/gitleaks")
fi

mkdir -p "$TMP/safe" "$TMP/identifier-secret" "$TMP/value-secret" "$TMP/unrelated-secret"
cat >"$TMP/safe/node-keys.txt" <<'EOF'
key: "pay/refunds/second"
Key: 'pay/refunds/second'
key := "pay/refunds/second"
task.Key == "pay/refunds/second"
"key": "pay/refunds/second"
"--key", "pay/refunds/second"
EOF
GENERIC_SECRET=$(printf '%s%s' \
  'm8Q2vK7xR4nP9cT6' \
  'wY3hF5jL1sD0bG4z')
printf 'apiKey: "%s"\n' "$GENERIC_SECRET" \
  >"$TMP/identifier-secret/credential.txt"
printf 'key: "%s"\n' "$GENERIC_SECRET" \
  >"$TMP/value-secret/credential.txt"
GITHUB_PAT=$(printf 'ghp_%s%s' \
  'A1b2C3d4E5f6G7h8' \
  'I9j0K1l2M3n4O5p6Q7r8')
printf 'key: "form-engine/loading"; github_token: "%s"\n' "$GITHUB_PAT" \
  >"$TMP/unrelated-secret/credential.txt"

run_scan() {
  "${GITLEAKS[@]}" dir --no-banner --redact --config "$ROOT/.gitleaks.toml" --exit-code 42 "$1" >/dev/null 2>&1
}

cat >"$TMP/default.toml" <<'EOF'
[extend]
useDefault = true
EOF
set +e
"${GITLEAKS[@]}" dir --no-banner --redact --config "$TMP/default.toml" \
  --report-format json --report-path "$TMP/default-report.json" \
  --exit-code 42 "$TMP/safe" >/dev/null 2>&1
status=$?
set -e
if [[ $status -ne 42 ]]; then
  echo "safe fixture no longer exercises a default gitleaks rule (exit $status)" >&2
  exit 1
fi
default_report=$(<"$TMP/default-report.json")
if [[ "$default_report" != *'"RuleID": "generic-api-key"'* ]]; then
  echo "safe fixture did not exercise the generic-api-key rule" >&2
  exit 1
fi

if ! run_scan "$TMP/safe"; then
  echo "gitleaks fixture allowlist rejected a lowercase node-key slug" >&2
  exit 1
fi

for fixture in "$TMP/identifier-secret" "$TMP/value-secret" "$TMP/unrelated-secret"; do
  set +e
  run_scan "$fixture"
  status=$?
  set -e
  if [[ $status -ne 42 ]]; then
    echo "gitleaks fixture allowlist hid a credential in $(basename "$fixture") (exit $status)" >&2
    exit 1
  fi
done

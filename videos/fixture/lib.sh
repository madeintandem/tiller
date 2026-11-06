# Helpers the stage scripts share. They run inside the fixture checkout with
# TILLER naming the binary, SNAP the stage's JSONL, CONTENT the authored-files
# directory, and MARKS the directory marks are written to.

# quote_args <word...>: the command line as a person would type it.
quote_args() {
  local out="" w
  for w in "$@"; do
    if [[ "$w" =~ ^[A-Za-z0-9_./:=@%+,-]+$ ]]; then
      out+=" $w"
    else
      out+=" \"${w//\"/\\\"}\""
    fi
  done
  printf '%s' "${out# }"
}

# run <step-id> <actor> <command...>: run the command, record it and its
# combined output, and fail the stage on a non-zero exit unless the step ID
# ends in -expect-fail (which must then fail). A `tiller` command word, also
# after an `env VAR=value` prefix, runs $TILLER and is recorded as `tiller`.
run() {
  local id=$1 actor=$2
  shift 2
  local shown
  shown=$(quote_args "$@")
  local -a argv=("$@")
  local i=0
  [[ "${argv[0]}" == env ]] && i=1
  while [[ $i -lt ${#argv[@]} && "${argv[$i]}" == *=* ]]; do i=$((i + 1)); done
  [[ "${argv[$i]}" == tiller ]] && argv[$i]=$TILLER
  local out code=0
  out=$("${argv[@]}" 2>&1) || code=$?
  jq -nc --arg id "$id" --arg actor "$actor" --arg cmd "$shown" \
    --arg out "$out" --argjson exit "$code" \
    '{id: $id, actor: $actor, kind: "cmd", cmd: $cmd,
      out: (if $out == "" then [] else ($out | split("\n")) end), exit: $exit}' >>"$SNAP"
  if [[ "$id" == *-expect-fail ]]; then
    if [[ $code -eq 0 ]]; then
      echo "step $id was expected to fail but exited 0" >&2
      return 1
    fi
  elif [[ $code -ne 0 ]]; then
    printf 'step %s failed (exit %s): %s\n%s\n' "$id" "$code" "$shown" "$out" >&2
    return "$code"
  fi
}

# say <step-id> <actor> <text>: a short in-terminal note from the agent or you.
say() {
  jq -nc --arg id "$1" --arg actor "$2" --arg text "$3" \
    '{id: $id, actor: $actor, kind: "say", text: $text}' >>"$SNAP"
}

# put <content-path> <repo-path>: the agent or the human writes an authored
# file. {{NAME}} placeholders are filled from FIXTURE_VARS entries (NAME=value).
put() {
  local src="$CONTENT/$1" dest=$2
  [[ -f "$src" ]] || { echo "no authored file $src" >&2; return 1; }
  mkdir -p "$(dirname "$dest")"
  local text pair
  text=$(<"$src")
  for pair in ${FIXTURE_VARS[@]+"${FIXTURE_VARS[@]}"}; do
    text=${text//"{{${pair%%=*}}}"/${pair#*=}}
  done
  printf '%s\n' "$text" >"$dest"
  if grep -q '{{[A-Z_]*}}' "$dest"; then
    echo "unfilled placeholder in $dest" >&2
    return 1
  fi
}

# var <NAME> <value>: make a value available to put's placeholders.
var() { FIXTURE_VARS+=("$1=$2"); }

# node_id <noun> <ref>: a node's ID, read the way the agent would.
node_id() {
  "$TILLER" show "$1" "$2" --json | jq -r .data.id
}

# mark <name>: save the whole checkout, .git included, as a named state.
mark() {
  mkdir -p "$MARKS"
  COPYFILE_DISABLE=1 tar -czf "$MARKS/$1.tar.gz" -C "$PWD" .
}

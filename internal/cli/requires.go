package cli

// The requires_tiller advisory (distribution.md, "The optional
// project.requires_tiller field"). project.requires_tiller records the minimum
// binary a team expects. Every command probes it before running and prints
// one stderr line naming `tiller upgrade` when this binary is below it. It is
// advisory only: never a refusal, never an exit-code change, never on stdout,
// so a --json envelope stays parseable. The wall stays the schema stamp; this
// is the nudge the spec promised and the binary never delivered.

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
)

// requiresTillerPattern is the floor grammar: an optional ">=", an optional
// "v", then two or three dot-separated numbers. ">= 1.0", "1.0", "v0.11.2",
// and ">=0.10.3" all parse; anything else is named as unparseable rather than
// silently ignored, since a floor nobody can read is a floor nobody honors.
var requiresTillerPattern = regexp.MustCompile(`^(?:>=\s*)?v?(\d+)\.(\d+)(?:\.(\d+))?$`)

// parseRequiresTiller parses a floor into three comparable ints, a missing
// patch reading as 0.
func parseRequiresTiller(floor string) ([3]int, bool) {
	m := requiresTillerPattern.FindStringSubmatch(strings.TrimSpace(floor))
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i, s := range m[1:] {
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// requiresTillerAdvisory is the one-line warning for a binary at `running`
// (the bare stamped version, "0.10.3") against a project whose recorded floor
// is `floor`, or "" when there is nothing to say: no floor, or a binary at or
// above it. A floor that does not parse gets its own line so the typo is
// visible instead of silently disabling the check.
func requiresTillerAdvisory(running, floor string) string {
	if floor == "" {
		return ""
	}
	want, ok := parseRequiresTiller(floor)
	if !ok {
		return fmt.Sprintf("tiller: warning: project.requires_tiller %q is not a version floor (want \">= X.Y[.Z]\"); the advisory version check is skipped", floor)
	}
	have, err := parseUpgradeVersion(running)
	if err != nil {
		return ""
	}
	if compareUpgradeVersions(have, want) >= 0 {
		return ""
	}
	return fmt.Sprintf("tiller: warning: this binary is v%s, below the project's requires_tiller floor (%s); run tiller upgrade to update the binary and this checkout", running, floor)
}

// warnRequiresTiller is the root command's PersistentPreRun: it probes the
// nearest manifest and writes the advisory, if any, to w. It never fails the
// command and never touches stdout.
func warnRequiresTiller(w io.Writer) {
	root, err := repoRoot()
	if err != nil {
		return
	}
	if line := requiresTillerAdvisory(version, store.RequiresTillerOf(root)); line != "" {
		fmt.Fprintln(w, line)
	}
}

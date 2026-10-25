// Package skills embeds the neutral Tiller skill set: the reasoning procedures
// an AI coding agent runs against the Tiller structure, paired with the exact
// CLI commands or read-only export each one terminates in (skill-set.md). The files are the
// canonical, harness-agnostic artifact; a harness adapter or `tiller guide`
// surfaces them without any of them naming a harness-specific tool.
//
// This build ships eleven skills: orient, derive-baseline, decompose, specify,
// execute, capture, mend-drift, amend, intake, design-brief, and code-review.
// derive-baseline establishes the current-state design layer before
// decomposition on existing-code projects.
// capture records a complete shelf proposal and stops before triage.
// design-brief exports a derived UI/UX handoff without changing project state.
// code-review reviews one node's net change read-only into .tiller/reviews/ and
// logs only the findings the developer names, through capture and intake shapes.
//
// mend-drift still ends its cascade arm by surfacing the cluster to the
// developer as one finding rather than naming amend, and that is now a pacing
// decision rather than a gap: a cascade is a plan reshape, amend's trigger is
// the developer's judgment that a reshape is due, and the crossing between
// seams is the developer's move everywhere else in this set too. Nothing is
// lost, because orient's dispatch names the amend seam from the same drift
// run.
//
// There is no break-down skill: specify writes the specification, contract,
// and tasks together, reviewed at one gate.
package skills

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed orient/SKILL.md derive-baseline/SKILL.md decompose/SKILL.md specify/SKILL.md execute/SKILL.md capture/SKILL.md mend-drift/SKILL.md amend/SKILL.md intake/SKILL.md design-brief/SKILL.md code-review/SKILL.md
var files embed.FS

// order is the canonical lifecycle order: the dispatcher, the existing-code
// baseline, the delivery seams, capture, drift and plan-change seams, then the
// explicitly requested export and review.
var order = []string{"orient", "derive-baseline", "decompose", "specify", "execute", "capture", "mend-drift", "amend", "intake", "design-brief", "code-review"}

// Names returns the embedded skill names in canonical lifecycle order. The slice
// is a copy, so a caller cannot mutate the registry.
func Names() []string {
	out := make([]string, len(order))
	copy(out, order)
	return out
}

// Has reports whether name is an embedded skill.
func Has(name string) bool {
	for _, n := range order {
		if n == name {
			return true
		}
	}
	return false
}

// Lookup returns the full SKILL.md text for name. The boolean is false for an
// unknown skill, so a caller maps it to a misuse error rather than panicking.
func Lookup(name string) (string, bool) {
	if !Has(name) {
		return "", false
	}
	data, err := files.ReadFile(name + "/SKILL.md")
	if err != nil {
		// An embedded file in the order list that cannot be read is a build-time
		// inconsistency, not a runtime condition, so surface it loudly.
		panic(fmt.Sprintf("skills: embedded skill %q is missing: %v", name, err))
	}
	return string(data), true
}

// Description returns the trigger description from a skill's YAML frontmatter: the
// "Use when ..." line a harness reads to decide when to auto-activate the skill.
// A harness adapter (and `tiller join`) carries this onto the installed shim so
// the shim triggers the same way the embedded skill would. The boolean is false
// for an unknown skill.
func Description(name string) (string, bool) {
	text, ok := Lookup(name)
	if !ok {
		return "", false
	}
	return frontmatterValue(text, "description"), true
}

// frontmatterValue reads a single-line scalar value out of a SKILL.md YAML
// frontmatter block (the region delimited by the leading and next "---"). It
// returns the empty string when the key is absent. The skill descriptions are all
// single-line plain scalars, so a line-oriented read is enough and avoids pulling
// in a YAML dependency for one field.
func frontmatterValue(text, key string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			break
		}
		if rest, found := strings.CutPrefix(ln, key+":"); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

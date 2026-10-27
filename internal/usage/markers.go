package usage

import (
	"regexp"
	"strings"
)

// MarkerKind says how a marker moves the replay state.
type MarkerKind int

const (
	// MarkVerb is a mutating tiller verb: it sets stage and node.
	MarkVerb MarkerKind = iota
	// MarkSkill is the loading of a Tiller skill: it sets the stage, and the
	// node only when one is named.
	MarkSkill
	// MarkRead is a read-only command: it names a node only if none is set.
	MarkRead
)

// Marker is one signal, found in a tool call, that a session moved to
// another work item or stage. Refs holds every candidate id or key token, in
// order: each non-flag argument after the verb and an optional noun word.
// Flag values land here too, since the reader does not know which flags take
// one; Credit picks the candidate that names a node. For MarkRead, Stage is "".
type Marker struct {
	Kind  MarkerKind
	Stage Stage
	Refs  []string
	Verb  string
}

var verbStages = map[string]Stage{
	"start": Implementation, "claim": Implementation, "resume": Implementation, "transfer": Implementation,
	"check": Checks,
	"eval":  AcceptanceEvals, "attest": AcceptanceEvals,
	"submit": Publishing,
	"spec":   SpecWriting, "return": SpecWriting,
	"add": Planning, "link": Planning, "order": Planning, "amend": Planning, "intake": Planning,
	"withdraw": Planning, "unlink": Planning, "rename": Planning, "remove": Planning,
	"configure": Planning, "resolve": Planning,
	"approve": Other, "answer": Other,
	"sync": Closeout, "archive": Closeout, "cancel": Closeout,
}

var readVerbs = map[string]bool{
	"show": true, "status": true, "readout": true, "next": true, "queue": true, "ls": true,
	"tree": true, "search": true, "query": true, "drift": true, "guide": true,
}

var skillStages = map[string]Stage{
	"execute":         Implementation,
	"specify":         SpecWriting,
	"code-review":     CodeReview,
	"capture":         Planning,
	"amend":           Planning,
	"intake":          Planning,
	"decompose":       Planning,
	"orient":          Other,
	"mend-drift":      Other,
	"derive-baseline": Other,
}

var nouns = map[string]bool{
	"task": true, "feature": true, "bug": true, "chore": true, "tweak": true, "epic": true,
}

// tillerCmd finds `tiller [root flags] <verb> <rest of the simple command>`.
// The word must start the string or follow whitespace or one of ;&|(`'" so
// that .tiller/ and ./cmd/tiller never match. Root flags (only the boolean
// --json today) may sit between tiller and the verb.
var tillerCmd = regexp.MustCompile("(?:^|[\\s;&|(`'\"])tiller(?:[ \\t]+-[^\\s;&|()]*)*[ \\t]+([^\\s;&|()\\-][^\\s;&|()]*)([^;&|()\\n]*)")

// unescape turns the escapes a command picks up when it sits inside a JSON
// or JS string back into the characters they stand for, so an escaped
// newline separates two commands and an escaped quote ends a token.
var unescape = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\'`, `'`)

// ShellMarkers returns the markers in a shell command, in order.
func ShellMarkers(command string) []Marker {
	var out []Marker
	for _, m := range tillerCmd.FindAllStringSubmatch(unescape.Replace(command), -1) {
		verb := cleanToken(m[1])
		args := strings.Fields(m[2])
		if verb == "guide" {
			if skill, ok := firstArg(args); ok {
				if sm, ok := SkillMarker(skill, ""); ok {
					sm.Verb = "guide"
					out = append(out, sm)
					continue
				}
			}
			out = append(out, Marker{Kind: MarkRead, Verb: verb})
			continue
		}
		if stage, ok := verbStages[verb]; ok {
			out = append(out, Marker{Kind: MarkVerb, Stage: stage, Refs: refsOf(args), Verb: verb})
		} else if readVerbs[verb] {
			out = append(out, Marker{Kind: MarkRead, Refs: refsOf(args), Verb: verb})
		}
	}
	return out
}

// SkillMarker reports the marker for loading a skill. The name may carry a
// plugin namespace and a tiller- prefix; args is the skill's argument text.
func SkillMarker(skill, args string) (Marker, bool) {
	name := skill
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimPrefix(name, "tiller-")
	stage, ok := skillStages[name]
	if !ok {
		return Marker{}, false
	}
	return Marker{Kind: MarkSkill, Stage: stage, Refs: refsOf(strings.Fields(args)), Verb: name}, true
}

// cleanToken cuts a token at its first backslash and trims it at both ends
// to the characters an id, key or verb uses, which drops the quotes,
// brackets and commas a command picks up inside JS or JSON text.
func cleanToken(tok string) string {
	if i := strings.IndexByte(tok, '\\'); i >= 0 {
		tok = tok[:i]
	}
	return strings.TrimFunc(tok, func(r rune) bool { return !isRefRune(r) })
}

func isRefRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		r == '/' || r == '_' || r == '.' || r == '-'
}

// arguments is the cleaned non-flag tokens of args, in order.
func arguments(args []string) []string {
	var out []string
	for _, a := range args {
		a = cleanToken(a)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// firstArg is the first token that is not a flag.
func firstArg(args []string) (string, bool) {
	rest := arguments(args)
	if len(rest) == 0 {
		return "", false
	}
	return rest[0], true
}

// refsOf is the candidate refs: every non-flag argument, skipping a leading
// noun word. It is nil when there are none.
func refsOf(args []string) []string {
	rest := arguments(args)
	if len(rest) > 0 && nouns[rest[0]] {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return nil
	}
	return rest
}

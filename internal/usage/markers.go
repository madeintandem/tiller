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
// another work item or stage. Ref is the raw id or key token ("" when none).
// For MarkRead, Stage is "".
type Marker struct {
	Kind  MarkerKind
	Stage Stage
	Ref   string
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

// tillerCmd finds `tiller <verb> <rest of the simple command>`. The word must
// start the string or follow whitespace or one of ;&|(`'" so that .tiller/
// and ./cmd/tiller never match.
var tillerCmd = regexp.MustCompile("(?:^|[\\s;&|(`'\"])tiller[ \\t]+([^\\s;&|()]+)([^;&|()\\n]*)")

const quoteChars = "\"'`"

// ShellMarkers returns the markers in a shell command, in order.
func ShellMarkers(command string) []Marker {
	var out []Marker
	for _, m := range tillerCmd.FindAllStringSubmatch(command, -1) {
		verb := strings.Trim(m[1], quoteChars)
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
			out = append(out, Marker{Kind: MarkVerb, Stage: stage, Ref: refOf(args), Verb: verb})
		} else if readVerbs[verb] {
			out = append(out, Marker{Kind: MarkRead, Ref: refOf(args), Verb: verb})
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
	return Marker{Kind: MarkSkill, Stage: stage, Ref: refOf(strings.Fields(args)), Verb: name}, true
}

// firstArg is the first token that is not a flag.
func firstArg(args []string) (string, bool) {
	for _, a := range args {
		a = strings.Trim(a, quoteChars)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a, true
	}
	return "", false
}

// refOf takes the first non-flag argument, skipping a leading noun word.
func refOf(args []string) string {
	var rest []string
	for _, a := range args {
		a = strings.Trim(a, quoteChars)
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) > 0 && nouns[rest[0]] {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return ""
	}
	return rest[0]
}

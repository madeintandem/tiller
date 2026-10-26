package views

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CaughtUpText is what every surface says when no move is legal (ranked-next
// design).
const CaughtUpText = "Caught up: nothing is legal to do right now."

// NextLine is one move as every text surface prints it: the actor, the
// action and its node, why, then how to do it. A move on a branch other than
// this checkout continues there; otherwise a command names the move, and an
// agent move without one names the skill to load.
func NextLine(c NextCandidate) string {
	var b strings.Builder
	if c.Actor == actorHuman {
		b.WriteString("You: ")
	} else {
		b.WriteString("Agent: ")
	}
	b.WriteString(c.Action)
	if c.Node != nil && c.Node.Ref != "" {
		b.WriteString(" " + c.Node.Ref)
	}
	b.WriteString(".")
	if reason := Sentence(c.Reason); reason != "" {
		b.WriteString(" " + reason)
	}
	switch {
	case c.Command != "":
		b.WriteString(" `" + c.Command + "`")
	case c.Branch != "":
		b.WriteString(" Continue it on " + c.Branch + ".")
	case c.Skill != "":
		b.WriteString(" `tiller guide " + c.Skill + "`")
	}
	return b.String()
}

// Sentence ends text with one period, unless it already ends on "?" or "!",
// and capitalizes a leading plain word. A reason may open with a node key
// ("checkout/alpha/t1 (T-x) is ready"), which is case-sensitive and is never
// altered: only a first token holding no '/', '-', '(' or digit is
// capitalized.
func Sentence(text string) string {
	text = strings.TrimRight(strings.TrimSpace(text), ".")
	if text == "" {
		return ""
	}
	// A reason ending on a question's title already ends its sentence.
	end := "."
	if strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") {
		end = ""
	}
	token, _, _ := strings.Cut(text, " ")
	if strings.ContainsFunc(token, func(r rune) bool { return r == '/' || r == '-' || r == '(' || unicode.IsDigit(r) }) {
		return text + end
	}
	first, size := utf8.DecodeRuneInString(text)
	return string(unicode.ToUpper(first)) + text[size:] + end
}

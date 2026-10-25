package cli

// The TTY rendering layer. The CLI's human output speaks one markdown-lite
// grammar everywhere: bold markers, inline code, and the occasional command
// fence. A human at a terminal should never see the raw markers, so when stdout
// is a TTY the markers RENDER: bold via ANSI, fences as indented command lines,
// inline backticks stripped. Piped output is left byte-identical to the source,
// which is what keeps agents, tests, and the
// json-equals-human contract untouched; the whole piped test suite is the
// regression net for that.
//
// Color follows the console's own rule: one loud ink, magenta, only on the
// exact Needs attention heading. Everything else gets bold at most. NO_COLOR
// (https://no-color.org) suppresses the escape codes while keeping the rendered
// structure, because the markers are noise in any terminal, colored or not.

import (
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// ttyDetect reports whether stdout is a terminal. A var so tests can force
// either mode; the suite itself runs piped.
var ttyDetect = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func colorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiLoud    = "\x1b[35;1m" // the chart's magenta, bold: the one loud ink
	fenceIndent = "  "
)

// The singular Needs attention heading is the only loud text. Exact matching
// keeps count bullets and near-match labels in the ordinary terminal ink.
const loudHeader = "Needs attention"

func styleBold(s string) string {
	if !colorEnabled() {
		return s
	}
	return ansiBold + s + ansiReset
}

func styleLoud(s string) string {
	if !colorEnabled() {
		return s
	}
	return ansiLoud + s + ansiReset
}

var boldSpan = regexp.MustCompile(`\*\*(.+?)\*\*`)
var codeSpan = regexp.MustCompile("`([^`\n]+)`")

// renderHumanTTY renders the markdown-lite wire format for a terminal reader,
// and is the identity function everywhere else. Fence markers drop and their
// content indents verbatim (it is command text, kept byte-exact for copying);
// bold markers become bold; the one needs-you label becomes the loud ink;
// inline backticks outside fences are stripped.
func renderHumanTTY(s string) string {
	if !ttyDetect() {
		return s
	}
	var out []string
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, fenceIndent+line)
			continue
		}
		line = codeSpan.ReplaceAllString(line, "$1")
		line = boldSpan.ReplaceAllStringFunc(line, func(m string) string {
			inner := boldSpan.FindStringSubmatch(m)[1]
			if inner == loudHeader {
				return styleLoud(inner)
			}
			return styleBold(inner)
		})
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

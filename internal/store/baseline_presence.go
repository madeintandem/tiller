package store

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/rootpath"
)

// BaselineDocPresent reports whether a registered design path can be opened as
// a regular file beneath the project root without traversing symlinks. It reads
// no prose bytes; missing, unsafe, and unreadable paths are all absent.
func BaselineDocPresent(root, doc string) bool {
	file, ok := openBaselineDoc(root, doc)
	if !ok {
		return false
	}
	defer file.Close()
	return true
}

// baselineTitleScanBytes bounds how far BaselineDocTitle reads looking for a
// heading, so a huge document costs no more than its opening.
const baselineTitleScanBytes = 64 << 10

// BaselineDocTitle is a registered design document's own title: the text of
// its first level-one ATX heading ("# Routing design"), read through the same
// symlink-refusing open BaselineDocPresent uses. It answers "" when the file
// cannot be opened safely or carries no such heading near its top, and a
// caller then names the document by its path alone. Front matter (a leading
// "---" block) and fenced code blocks (``` or ~~~) are skipped, so a shell or
// YAML comment is never taken for a heading.
func BaselineDocTitle(root, doc string) string {
	file, ok := openBaselineDoc(root, doc)
	if !ok {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, baselineTitleScanBytes))
	fence := ""
	frontMatter := false
	for first := true; scanner.Scan(); first = false {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case first && trimmed == "---":
			frontMatter = true
		case frontMatter:
			if trimmed == "---" || trimmed == "..." {
				frontMatter = false
			}
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
		default:
			if title, found := atxH1(line); found {
				return title
			}
		}
	}
	return ""
}

// atxH1 reads line as a CommonMark level-one ATX heading: up to three spaces
// of indent (four is a code block), "#" then a space, a tab, or the end of the
// line ("#5" is not a heading, "##" is level two), and an optional closing run
// of #s that follows a space or tab stripped from the text.
func atxH1(line string) (string, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return "", false
	}
	rest, ok := strings.CutPrefix(line[indent:], "#")
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if closed := strings.TrimRight(rest, "#"); closed == "" {
		rest = ""
	} else if closed != rest && (closed[len(closed)-1] == ' ' || closed[len(closed)-1] == '\t') {
		rest = strings.TrimSpace(closed)
	}
	return rest, true
}

func openBaselineDoc(root, doc string) (*os.File, bool) {
	if ValidateDesignPath(doc) != nil {
		return nil, false
	}
	anchored, err := os.OpenRoot(root)
	if err != nil {
		return nil, false
	}
	defer anchored.Close()
	file, err := (rootpath.Walker{}).Open(anchored, filepath.Join(".tiller", "design", filepath.FromSlash(doc)), false)
	if err != nil {
		return nil, false
	}
	return file, true
}

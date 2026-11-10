package workflow

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/madeintandem/tiller/internal/store"
)

// The closeout reason reads as plain sentences a reviewer would say. The CLI
// readout shares the string, so both surfaces change together. Advice.Outstanding
// keeps the raw tokens for JSON consumers; only this prose is humanized.

// humanizeToken turns an enum value (a status, a work kind) into words.
// Identifiers a reader looks up (criterion ids, gate keys, node ids) are names
// and are never passed through it.
func humanizeToken(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

// joinList joins clauses with "and", using a serial comma from three up.
func joinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

func workLabel(w Work) string {
	label := w.ID
	if w.NodeID != "" {
		label = w.NodeID + " " + label
	}
	return label
}

// itemsClause names outstanding items: those a human owes first, then those an
// agent owes. Within each, items that failed lead in their own clause and say
// they need the work again; pending or unrecorded items keep the plain wording.
// Agreement follows the count of items, not the look of a key.
func itemsClause(items []Work) string {
	var humanFailed, humanOpen, agentFailed, agentOpen []string
	for _, w := range items {
		didFail := w.Status == store.CriterionStatusFail
		switch {
		case w.Actor == "human" && didFail:
			humanFailed = append(humanFailed, workLabel(w))
		case w.Actor == "human":
			humanOpen = append(humanOpen, workLabel(w))
		case didFail:
			agentFailed = append(agentFailed, workLabel(w)+" failed and needs the agent's "+humanizeToken(w.Kind)+" again")
		default:
			agentOpen = append(agentOpen, workLabel(w)+" needs the agent's "+humanizeToken(w.Kind))
		}
	}
	var clauses []string
	if len(humanFailed) > 0 {
		verb := "needs"
		if len(humanFailed) > 1 {
			verb = "need"
		}
		clauses = append(clauses, joinList(humanFailed)+" failed and "+verb+" your attestation again")
	}
	if len(humanOpen) > 0 {
		verb := "needs"
		if len(humanOpen) > 1 {
			verb = "need"
		}
		clauses = append(clauses, joinList(humanOpen)+" "+verb+" your attestation")
	}
	if len(agentFailed) > 0 {
		clauses = append(clauses, joinList(agentFailed))
	}
	if len(agentOpen) > 0 {
		clauses = append(clauses, joinList(agentOpen))
	}
	if len(clauses) > 2 {
		return strings.Join(clauses[:len(clauses)-1], ", ") + ", and " + clauses[len(clauses)-1]
	}
	return strings.Join(clauses, ", and ")
}

// openAcceptanceSentence names what keeps acceptance open.
func openAcceptanceSentence(items []Work) string {
	return "Acceptance is still open: " + itemsClause(items) + "."
}

// policyCountSentence reports recorded policy violations or warnings; noun is
// "violation" or "warning" and kind qualifies them. Zero yields no sentence.
func policyCountSentence(noun, kind string, n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return fmt.Sprintf("1 %s %s remains on record.", kind, noun)
	}
	return fmt.Sprintf("%d %s %ss remain on record.", n, kind, noun)
}

// addSentence appends a sentence to a reason, closing the prior one first.
func addSentence(reason, sentence string) string {
	if sentence == "" {
		return reason
	}
	if reason != "" && !strings.HasSuffix(reason, ".") {
		reason += "."
	}
	if reason == "" {
		return sentence
	}
	return reason + " " + sentence
}

// sentence closes a fragment into one capitalized sentence ending in a period.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	s = string(runes)
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "!") && !strings.HasSuffix(s, "?") {
		s += "."
	}
	return s
}

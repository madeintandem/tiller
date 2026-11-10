package workflow

import (
	"fmt"
	"strings"
)

// The closeout reason reads as plain sentences a reviewer would say. The CLI
// readout shares the string, so both surfaces change together. Advice.Outstanding
// keeps the raw tokens for JSON consumers; only this prose is humanized.

// humanizeToken turns an identifier-shaped token into words.
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
	label := humanizeToken(w.ID)
	if w.NodeID != "" {
		label = w.NodeID + " " + label
	}
	return label
}

// openAcceptanceSentence names what keeps acceptance open: items a human owes
// first, then items an agent owes.
func openAcceptanceSentence(items []Work) string {
	var human, agent []string
	for _, w := range items {
		if w.Actor == "human" {
			human = append(human, workLabel(w))
		} else {
			agent = append(agent, workLabel(w)+" needs the agent's "+humanizeToken(w.Kind))
		}
	}
	var clauses []string
	if len(human) > 0 {
		verb := "needs"
		if len(human) > 1 {
			verb = "need"
		}
		clauses = append(clauses, joinList(human)+" "+verb+" your attestation")
	}
	if len(agent) > 0 {
		clauses = append(clauses, joinList(agent))
	}
	return "Acceptance is still open: " + strings.Join(clauses, ", and ") + "."
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

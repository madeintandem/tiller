package views

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
)

// TestVocabularyCoversEveryStatus pins the console's vocabulary.json against
// the stored status enums, so a status added in Go cannot reach the screen
// without the UI's label list learning it.
func TestVocabularyCoversEveryStatus(t *testing.T) {
	raw, err := os.ReadFile("../../ui/src/lib/vocabulary.json")
	if err != nil {
		t.Fatalf("read vocabulary.json: %v", err)
	}
	var vocab struct {
		Statuses []string `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &vocab); err != nil {
		t.Fatalf("parse vocabulary.json: %v", err)
	}
	have := map[string]bool{}
	for _, s := range vocab.Statuses {
		have[s] = true
	}

	var want []string
	for _, statuses := range statusOrder {
		want = append(want, statuses...)
	}
	want = append(want,
		store.GateStatusPass, store.GateStatusFail, store.GateStatusPending,
		store.GateStatusSkipped, store.GateStatusStale,
		store.CriterionStatusPass, store.CriterionStatusFail, store.CriterionStatusPending,
		store.CriterionStatusNeedsAttestation, store.CriterionStatusStale,
		"open", "answered", "cancelled",
	)
	for _, s := range want {
		if !have[s] {
			t.Errorf("vocabulary.json statuses is missing %q", s)
		}
	}
}

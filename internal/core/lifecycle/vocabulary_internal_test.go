package lifecycle

import (
	"encoding/json"
	"os"
	"testing"
)

// TestVocabularyCoversEveryTransition pins the console's vocabulary.json
// event_types against the lifecycle table, so a transition added in Go cannot
// mint an event type the console has no phrase for.
func TestVocabularyCoversEveryTransition(t *testing.T) {
	raw, err := os.ReadFile("../../../ui/src/lib/vocabulary.json")
	if err != nil {
		t.Fatalf("read vocabulary.json: %v", err)
	}
	var vocab struct {
		EventTypes []string `json:"event_types"`
	}
	if err := json.Unmarshal(raw, &vocab); err != nil {
		t.Fatalf("parse vocabulary.json: %v", err)
	}
	have := map[string]bool{}
	for _, typ := range vocab.EventTypes {
		have[typ] = true
	}
	for noun, rows := range transitions {
		for _, row := range rows {
			if typ := row.Verb + "." + noun; !have[typ] {
				t.Errorf("vocabulary.json event_types is missing %q", typ)
			}
		}
	}
}

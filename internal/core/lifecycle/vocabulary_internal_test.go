package lifecycle

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/madeintandem/tiller/internal/store"
)

// TestVocabularyCoversEveryTransition pins the console's vocabulary.json
// event_types against the lifecycle table, so a transition added in Go cannot
// mint an event type the console has no phrase for.
//
// The batch noun is exempt for now: the console's batch surface, phrases
// included, is its own tweak (tweaks/console-shows-shelf-batches), and that
// change removes this exemption when it adds the batch event types.
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
		if noun == store.NounBatch {
			continue
		}
		for _, row := range rows {
			if typ := row.Verb + "." + noun; !have[typ] {
				t.Errorf("vocabulary.json event_types is missing %q", typ)
			}
		}
	}
}

package drift

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type driftVocabulary struct {
	Markers      []string `json:"drift_markers"`
	ExtraMarkers []string `json:"drift_extra_markers"`
	Reasons      []string `json:"drift_reasons"`
}

func readDriftVocabulary(t *testing.T) driftVocabulary {
	t.Helper()
	raw, err := os.ReadFile("../../../ui/src/lib/vocabulary.json")
	if err != nil {
		t.Fatalf("read vocabulary.json: %v", err)
	}
	var vocab driftVocabulary
	if err := json.Unmarshal(raw, &vocab); err != nil {
		t.Fatalf("parse vocabulary.json: %v", err)
	}
	return vocab
}

// TestVocabularyCoversEveryMarker pins the console's vocabulary.json
// drift_markers against KnownMarkers, so a marker added in Go cannot reach
// the screen before the console has plain copy for it.
func TestVocabularyCoversEveryMarker(t *testing.T) {
	have := map[string]bool{}
	for _, m := range readDriftVocabulary(t).Markers {
		have[m] = true
	}
	for _, m := range KnownMarkers {
		if !have[m] {
			t.Errorf("vocabulary.json drift_markers is missing %q", m)
		}
	}
}

// reasonLiteral finds each reason token a sweep emits: a Reason field set to
// a literal, a call of the design sweep's uncovered closure, or a call of
// staleSpecUncovered. The serve handler's own comparison_error reason is
// added by name because it lives outside this package.
var reasonLiteral = regexp.MustCompile(`(?:Reason:\s*|\buncovered\(|staleSpecUncovered\([^"\n]*)"([a-z_]+)"`)

// TestVocabularyCoversEveryReason pins drift_reasons against the reasons the
// sweeps emit, so a new reason fails the build until it has console copy.
func TestVocabularyCoversEveryReason(t *testing.T) {
	have := map[string]bool{}
	for _, r := range readDriftVocabulary(t).Reasons {
		have[r] = true
	}
	want := map[string]bool{"comparison_error": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reasonLiteral.FindAllStringSubmatch(string(src), -1) {
			want[m[1]] = true
		}
	}
	if len(want) < 10 {
		t.Fatalf("found only %d reasons in source, the scan is broken: %v", len(want), want)
	}
	for r := range want {
		if !have[r] {
			t.Errorf("vocabulary.json drift_reasons is missing %q", r)
		}
	}
	for r := range have {
		if !want[r] {
			t.Errorf("vocabulary.json drift_reasons lists %q, which no sweep emits", r)
		}
	}
}

// outOfPackageMarkers are markers emitted outside the drift package:
// serve/prose.go reports a failed design-file comparison under "design".
// They are not sweep markers, so they live in drift_extra_markers.
var outOfPackageMarkers = []string{"design"}

func TestVocabularyExtraMarkersAreExactlyTheOutOfPackageEmitters(t *testing.T) {
	got := readDriftVocabulary(t).ExtraMarkers
	if strings.Join(got, ",") != strings.Join(outOfPackageMarkers, ",") {
		t.Errorf("drift_extra_markers = %v, want %v", got, outOfPackageMarkers)
	}
}

func TestVocabularyMarkersAreExactlyKnownMarkers(t *testing.T) {
	got := readDriftVocabulary(t).Markers
	if strings.Join(got, ",") != strings.Join(KnownMarkers, ",") {
		t.Errorf("drift_markers = %v, want KnownMarkers %v", got, KnownMarkers)
	}
}

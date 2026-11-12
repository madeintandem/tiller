package events

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeSegmentWritesOneSortedLinePerEvent(t *testing.T) {
	later := Event{ID: "EV-bbbbbb", Type: "submit.task", At: at("2026-10-13T09:30:00.5Z"), Contributor: "lantern",
		Command: "submit task T-000001", Targets: []string{"T-000001"},
		Before: map[string]any{"status": "check"}, After: map[string]any{"status": "in_review", "pr": 167},
		Files: []FileChange{{Path: ".tiller/x/task.yml"}}}
	earlier := Event{ID: "EV-aaaaaa", Type: "add.task", At: at("2026-10-13T08:00:00Z"), Contributor: "jc",
		Command: "add task", Targets: []string{"T-000001"}}

	data, err := EncodeSegment([]Event{later, earlier})

	require.NoError(t, err)
	require.Equal(t, `{"id":"EV-aaaaaa","type":"add.task","at":"2026-10-13T08:00:00Z","contributor":"jc","command":"add task","targets":["T-000001"]}
{"id":"EV-bbbbbb","type":"submit.task","at":"2026-10-13T09:30:00.5Z","contributor":"lantern","command":"submit task T-000001","targets":["T-000001"],"before":{"status":"check"},"after":{"pr":167,"status":"in_review"},"files":[{"path":".tiller/x/task.yml"}]}
`, string(data))
}

func TestALooseEventSurvivesTheSegmentRoundTripByValue(t *testing.T) {
	loose, err := DecodeLoose("x.yml", []byte("id: EV-cccccc\ntype: submit.task\nat: 2026-10-08T04:43:03.534897Z\n"+
		"contributor: lantern\ncommand: submit task T-0z03r7\ntargets:\n    - T-0z03r7\n"+
		"after:\n    pr: 167\n    ratio: 1.5\n    seen: 2026-10-08T04:43:03Z\n    nested:\n        list: [a, 2]\n"+
		"files:\n    - path: a/task.yml\n      after:\n        kind: git_blob\n        value: git:abc1234\n        observed_at: 2026-10-08T04:43:03Z\n"))
	require.NoError(t, err)

	data, err := EncodeSegment([]Event{loose})
	require.NoError(t, err)
	back, err := DecodeSegment("2026/10/08-x.jsonl", data)
	require.NoError(t, err)

	require.Len(t, back, 1)
	require.True(t, Equal(loose, back[0]))
	require.Equal(t, 167, loose.After["pr"])
	require.Equal(t, 167, back[0].After["pr"])
	require.Equal(t, 1.5, back[0].After["ratio"])
	require.Equal(t, "2026-10-08T04:43:03Z", loose.After["seen"])
	require.Equal(t, "2026-10-08T04:43:03Z", back[0].After["seen"])
	require.Equal(t, []any{"a", 2}, back[0].After["nested"].(map[string]any)["list"])
}

func TestALegacyOwnerEncodesAsContributor(t *testing.T) {
	loose, err := DecodeLoose("x.yml", []byte("id: EV-dddddd\ntype: add.task\nat: 2026-10-08T04:43:03Z\nowner: jc\ncommand: add task\n"))
	require.NoError(t, err)

	data, err := EncodeSegment([]Event{loose})

	require.NoError(t, err)
	require.Contains(t, string(data), `"contributor":"jc"`)
	require.NotContains(t, string(data), `"owner"`)
}

func TestDecodeSegmentNamesTheFileAndLine(t *testing.T) {
	raw := []byte(`{"id":"EV-aaaaaa","at":"2026-10-13T08:00:00Z"}` + "\n{not json\n")

	_, err := DecodeSegment("2026/10/13-x.jsonl", raw)

	require.ErrorContains(t, err, "2026/10/13-x.jsonl line 2")
}

func TestDecodeSegmentRefusesABlankOrTruncatedLine(t *testing.T) {
	line := `{"id":"EV-aaaaaa","at":"2026-10-13T08:00:00Z"}`

	_, err := DecodeSegment("s.jsonl", []byte(line+"\n\n"))
	require.ErrorContains(t, err, "line 2")

	_, err = DecodeSegment("s.jsonl", []byte(line))
	require.ErrorContains(t, err, "line 1")
}

func TestSegmentRelPathIsNamedByDayAndContent(t *testing.T) {
	data := []byte("x\n")
	sum := sha256.Sum256(data)

	got := SegmentRelPath(at("2026-10-13T00:00:00Z"), data)

	require.Equal(t, filepath.Join(".tiller", "events", "2026", "10", "13-"+hex.EncodeToString(sum[:])[:12]+".jsonl"), got)
}

func TestSegmentDayReadsOnlySegmentNames(t *testing.T) {
	day, ok := SegmentDay("13-3f9a2c41d0aa.jsonl")
	require.True(t, ok)
	require.Equal(t, 13, day)

	for _, name := range []string{"EV-aaaaaa.yml", "13.jsonl", "xx-abc.jsonl"} {
		_, ok := SegmentDay(name)
		require.False(t, ok, name)
	}
}

func TestIsSegmentPathNamesOnlyASegmentInAMonthDirectory(t *testing.T) {
	require.True(t, IsSegmentPath(".tiller/events/2026/10/13-3f9a2c41d0aa.jsonl"))
	require.True(t, IsSegmentPath(filepath.Join(".tiller", "events", "2026", "10", "13-3f9a2c41d0aa.jsonl")))
	for _, rel := range []string{
		".tiller/events/notes.jsonl",
		".tiller/events/2026/notes.jsonl",
		".tiller/events/2026/10/notes.jsonl",
		".tiller/events/2026/10/13/13-3f9a2c41d0aa.jsonl",
		".tiller/events/2026/13-3f9a2c41d0aa.jsonl",
		".tiller/events/misc/10/13-3f9a2c41d0aa.jsonl",
		".tiller/events/2026/13/13-3f9a2c41d0aa.jsonl",
		".tiller/events/2026/10/EV-aaaaaa.yml",
		"other/2026/10/13-3f9a2c41d0aa.jsonl",
	} {
		require.False(t, IsSegmentPath(rel), rel)
	}
}

func TestDecodeLooseStrictRefusesAKeyTheShapeDoesNotDefine(t *testing.T) {
	head := "id: EV-eeeeee\ntype: add.task\nat: 2026-10-08T04:43:03Z\ncommand: add task\n"

	_, err := DecodeLooseStrict("x.yml", []byte(head+"surprise: 1\n"))
	require.Error(t, err)

	_, err = DecodeLooseStrict("x.yml", []byte(head+"files:\n    - path: a\n      surprise: 1\n"))
	require.Error(t, err)

	_, err = DecodeLooseStrict("x.yml", []byte(head+"files:\n    - path: a\n      after:\n        kind: git_blob\n        surprise: 1\n"))
	require.Error(t, err)

	_, err = DecodeLooseStrict("x.yml", []byte(head+"---\nid: EV-ffffff\n"))
	require.Error(t, err)

	e, err := DecodeLooseStrict("x.yml", []byte(head+"owner: jc\n"))
	require.NoError(t, err)
	require.Equal(t, "jc", e.Contributor)
}

func TestEncodeSegmentRefusesWhatJSONCannotCarry(t *testing.T) {
	e := Event{ID: "EV-gggggg", Type: "add.task", At: at("2026-10-13T08:00:00Z"),
		Before: map[string]any{"m": map[any]any{1: "x"}}}

	_, err := EncodeSegment([]Event{e})

	require.Error(t, err)
}

func TestEncodeSegmentRefusesAnEmptyIDAZeroTimeAndADuplicateID(t *testing.T) {
	ok := Event{ID: "EV-hhhhhh", At: at("2026-10-13T08:00:00Z")}

	_, err := EncodeSegment([]Event{{At: ok.At}})
	require.Error(t, err)
	_, err = EncodeSegment([]Event{{ID: "EV-hhhhhh"}})
	require.Error(t, err)
	_, err = EncodeSegment([]Event{ok, ok})
	require.Error(t, err)
}

func TestDecodeFilePicksTheDecoderByPath(t *testing.T) {
	ev := Event{ID: "EV-aaaaaa", Type: "add.task", At: at("2026-10-13T08:00:00Z"), Contributor: "lantern", Command: "add task"}
	data, err := EncodeSegment([]Event{ev})
	require.NoError(t, err)
	segment := SegmentRelPath(ev.At, data)
	require.True(t, IsEventFile(segment))
	got, err := DecodeFile(segment, data)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, Equal(ev, got[0]))

	loose := RelPath(ev)
	require.True(t, IsEventFile(loose))
	got, err = DecodeFile(loose, []byte("id: EV-aaaaaa\ntype: add.task\nat: 2026-10-13T08:00:00Z\ncontributor: lantern\ncommand: add task\n"))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, Equal(ev, got[0]))

	require.False(t, IsEventFile(filepath.Join(".tiller", "events", "notes.jsonl")))
	_, err = DecodeFile(loose, []byte("id: [\n"))
	require.ErrorContains(t, err, loose)
}

func TestFoldCountsAnEventOnceAndRefusesDifferingCopies(t *testing.T) {
	ev := Event{ID: "EV-aaaaaa", Type: "add.task", At: at("2026-10-13T08:00:00Z"), Contributor: "lantern"}
	other := Event{ID: "EV-bbbbbb", Type: "add.task", At: at("2026-10-13T07:00:00Z"), Contributor: "lantern"}
	loose := Located{Event: ev, Path: ".tiller/events/2026/10/13/EV-aaaaaa.yml"}
	segment := Located{Event: ev, Path: ".tiller/events/2026/10/13-0123456789ab.jsonl"}

	folded, err := Fold([]Located{loose, {Event: other, Path: "b.yml"}, segment})
	require.NoError(t, err)
	require.Equal(t, []Located{segment, {Event: other, Path: "b.yml"}}, folded, "first-seen order, a segment preferred as the home")

	changed := ev
	changed.Type = "start.task"
	_, err = Fold([]Located{segment, {Event: changed, Path: loose.Path}})
	require.EqualError(t, err, "events: EV-aaaaaa differs between "+segment.Path+" and "+loose.Path)
	require.NoError(t, Conflict(loose, segment))
}

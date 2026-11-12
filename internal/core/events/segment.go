package events

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// A segment is one UTC day of events compacted into a single JSON Lines file,
// .tiller/events/YYYY/MM/DD-<hash>.jsonl, one event per line in stream order.
// The wire structs below keep JSON tags off Event, FileChange, and
// store.RevisionRef, because other JSON surfaces embed those types.

type wireEvent struct {
	ID          string         `json:"id"`
	Type        string         `json:"type,omitempty"`
	At          time.Time      `json:"at"`
	Contributor string         `json:"contributor,omitempty"`
	Command     string         `json:"command,omitempty"`
	Targets     []string       `json:"targets,omitempty"`
	Before      map[string]any `json:"before,omitempty"`
	After       map[string]any `json:"after,omitempty"`
	Files       []wireFile     `json:"files,omitempty"`
}

type wireFile struct {
	Path   string       `json:"path"`
	Before wireRevision `json:"before,omitzero"`
	After  wireRevision `json:"after,omitzero"`
}

type wireRevision struct {
	Kind       string    `json:"kind,omitempty"`
	Value      string    `json:"value,omitempty"`
	Path       string    `json:"path,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitzero"`
}

// EncodeSegment serializes evs as one JSON object per line, sorted in stream
// order. It refuses an event with no id, a zero timestamp, or an id already
// seen, and any Before or After value JSON cannot carry.
func EncodeSegment(evs []Event) ([]byte, error) {
	sorted := append([]Event(nil), evs...)
	Sort(sorted)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	seen := make(map[string]bool, len(sorted))
	for _, e := range sorted {
		if e.ID == "" {
			return nil, errors.New("events: segment event has no id")
		}
		if e.At.IsZero() {
			return nil, fmt.Errorf("events: segment event %s has a zero timestamp", e.ID)
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("events: segment holds %s twice", e.ID)
		}
		seen[e.ID] = true
		if err := refuseUncarriable(e.Before); err != nil {
			return nil, fmt.Errorf("events: encode %s before: %w", e.ID, err)
		}
		if err := refuseUncarriable(e.After); err != nil {
			return nil, fmt.Errorf("events: encode %s after: %w", e.ID, err)
		}
		e.Before, e.After = normalizeMap(e.Before), normalizeMap(e.After)
		if err := enc.Encode(toWire(e)); err != nil {
			return nil, fmt.Errorf("events: encode %s: %w", e.ID, err)
		}
	}
	return buf.Bytes(), nil
}

// DecodeSegment parses a segment into events. A blank line, or a last line
// without its newline (a truncated write), is an error.
func DecodeSegment(relative string, raw []byte) ([]Event, error) {
	var out []Event
	rest := raw
	for n := 1; len(rest) > 0; n++ {
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			return nil, segmentError(relative, n, errors.New("line has no trailing newline"))
		}
		e, err := decodeSegmentLine(rest[:end])
		if err != nil {
			return nil, segmentError(relative, n, err)
		}
		out = append(out, e)
		rest = rest[end+1:]
	}
	return out, nil
}

func segmentError(relative string, line int, cause error) error {
	return fmt.Errorf("events: parse %s line %d: %w", relative, line, cause)
}

func decodeSegmentLine(line []byte) (Event, error) {
	if len(bytes.TrimSpace(line)) == 0 {
		return Event{}, errors.New("blank line")
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	var w wireEvent
	if err := dec.Decode(&w); err != nil {
		return Event{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return Event{}, errors.New("unexpected data after the event")
	}
	e := fromWire(w)
	normalize(&e)
	return e, nil
}

// SegmentRelPath is where a segment holding data for day lives, named by the
// day of month and the first 12 hex characters of the content hash.
func SegmentRelPath(day time.Time, data []byte) string {
	day = day.UTC()
	sum := sha256.Sum256(data)
	name := fmt.Sprintf("%02d-%s.jsonl", day.Day(), hex.EncodeToString(sum[:])[:12])
	return filepath.Join(".tiller", "events", fmt.Sprintf("%04d", day.Year()), fmt.Sprintf("%02d", int(day.Month())), name)
}

// IsSegmentPath reports whether rel, a project-relative path, names a segment:
// a .jsonl file directly in a YYYY/MM month directory under .tiller/events
// whose name SegmentDay accepts. It is the one definition every reader and
// compaction share; any other .jsonl under the events directory is not events.
func IsSegmentPath(rel string) bool {
	rest, ok := strings.CutPrefix(filepath.ToSlash(rel), ".tiller/events/")
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return false
	}
	month := parts[0] + "/" + parts[1]
	if parsed, err := time.Parse("2006/01", month); err != nil || parsed.Format("2006/01") != month {
		return false
	}
	_, ok = SegmentDay(parts[2])
	return ok
}

// SegmentDay reads the day of month from a segment file name, and reports
// false for any other name.
func SegmentDay(name string) (day int, ok bool) {
	stem, found := strings.CutSuffix(name, ".jsonl")
	if !found {
		return 0, false
	}
	dayText, hash, found := strings.Cut(stem, "-")
	if !found || len(dayText) != 2 || len(hash) != 12 {
		return 0, false
	}
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return 0, false
		}
	}
	day, err := strconv.Atoi(dayText)
	if err != nil || day < 1 || day > 31 {
		return 0, false
	}
	return day, true
}

// Equal reports whether two events mean the same thing. Timestamps compare by
// instant, so a zone or location difference does not matter.
func Equal(a, b Event) bool {
	if a.ID != b.ID || a.Type != b.Type || !a.At.Equal(b.At) || a.Contributor != b.Contributor || a.Command != b.Command {
		return false
	}
	if !sameList(a.Targets, b.Targets) || !sameMap(a.Before, b.Before) || !sameMap(a.After, b.After) || len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		fa, fb := a.Files[i], b.Files[i]
		if fa.Path != fb.Path || !fa.Before.SameAs(fb.Before) || !fa.After.SameAs(fb.After) {
			return false
		}
	}
	return true
}

func sameList(a, b []string) bool {
	return len(a) == len(b) && (len(a) == 0 || reflect.DeepEqual(a, b))
}

func sameMap(a, b map[string]any) bool {
	return len(a) == len(b) && (len(a) == 0 || reflect.DeepEqual(a, b))
}

// DecodeLooseStrict decodes a loose event file like DecodeLoose, and also refuses
// any key the event shape does not define and a second YAML document.
func DecodeLooseStrict(relative string, raw []byte) (Event, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return Event{}, fmt.Errorf("events: parse %s: %w", relative, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return Event{}, fmt.Errorf("events: parse %s: more than one YAML document", relative)
	}
	if len(doc.Content) == 1 {
		if err := refuseUnknownKeys(doc.Content[0]); err != nil {
			return Event{}, fmt.Errorf("events: parse %s: %w", relative, err)
		}
	}
	return DecodeLoose(relative, raw)
}

var (
	eventKeys    = []string{"id", "type", "at", "contributor", "owner", "command", "targets", "before", "after", "files"}
	fileKeys     = []string{"path", "before", "after"}
	revisionKeys = []string{"kind", "value", "path", "observed_at"}
)

func refuseUnknownKeys(event *yaml.Node) error {
	return checkKeys(event, "event", eventKeys, func(key string, value *yaml.Node) error {
		if key != "files" {
			return nil
		}
		for _, file := range value.Content {
			if err := checkFile(file); err != nil {
				return err
			}
		}
		return nil
	})
}

func checkFile(file *yaml.Node) error {
	return checkKeys(file, "file change", fileKeys, func(key string, value *yaml.Node) error {
		if key == "before" || key == "after" {
			return checkKeys(value, "revision", revisionKeys, nil)
		}
		return nil
	})
}

// checkKeys refuses a mapping key outside allowed, and hands each allowed
// key's value to descend when it is set.
func checkKeys(node *yaml.Node, what string, allowed []string, descend func(key string, value *yaml.Node) error) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !contains(allowed, key) {
			return fmt.Errorf("%s has unknown key %q", what, key)
		}
		if descend != nil {
			if err := descend(key, node.Content[i+1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func toWire(e Event) wireEvent {
	w := wireEvent{
		ID: e.ID, Type: e.Type, At: e.At.UTC(), Contributor: e.Contributor, Command: e.Command,
		Targets: e.Targets, Before: e.Before, After: e.After,
	}
	for _, f := range e.Files {
		w.Files = append(w.Files, wireFile{Path: f.Path, Before: toWireRevision(f.Before), After: toWireRevision(f.After)})
	}
	return w
}

func toWireRevision(r store.RevisionRef) wireRevision {
	rev := wireRevision{Kind: r.Kind, Value: r.Value, Path: r.Path}
	if !r.ObservedAt.IsZero() {
		rev.ObservedAt = r.ObservedAt.UTC()
	}
	return rev
}

func fromWire(w wireEvent) Event {
	e := Event{
		ID: w.ID, Type: w.Type, At: w.At, Contributor: w.Contributor, Command: w.Command,
		Targets: w.Targets, Before: w.Before, After: w.After,
	}
	for _, f := range w.Files {
		e.Files = append(e.Files, FileChange{Path: f.Path, Before: fromWireRevision(f.Before), After: fromWireRevision(f.After)})
	}
	return e
}

func fromWireRevision(r wireRevision) store.RevisionRef {
	return store.RevisionRef{Kind: r.Kind, Value: r.Value, Path: r.Path, ObservedAt: r.ObservedAt}
}

// normalize puts an event into the one shape both the YAML and the segment
// decoders agree on: UTC times, and Before and After values reduced to
// strings, int, float64, bool, nil, []any, and map[string]any.
func normalize(e *Event) {
	e.At = e.At.UTC()
	for i := range e.Files {
		e.Files[i].Before.ObservedAt = utcOrZero(e.Files[i].Before.ObservedAt)
		e.Files[i].After.ObservedAt = utcOrZero(e.Files[i].After.ObservedAt)
	}
	e.Before = normalizeMap(e.Before)
	e.After = normalizeMap(e.After)
}

// refuseUncarriable rejects a Before or After value the segment cannot hold
// faithfully: anything but scalars, lists, and string-keyed maps. JSON would
// silently turn a map with non-string keys into string keys.
func refuseUncarriable(v any) error {
	switch x := v.(type) {
	case nil, string, bool, time.Time, json.Number,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return nil
	case map[string]any:
		for k, item := range x {
			if err := refuseUncarriable(item); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		return nil
	case []any:
		for _, item := range x {
			if err := refuseUncarriable(item); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("%T cannot be written to a segment", v)
}

func utcOrZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC()
}

func normalizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case map[string]any:
		return normalizeMap(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeValue(item)
		}
		return out
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return int(i)
		}
		f, err := x.Float64()
		if err != nil {
			return x.String()
		}
		return normalizeFloat(f)
	case float64:
		return normalizeFloat(x)
	case float32:
		return normalizeFloat(float64(x))
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return normalizeUnsigned(uint64(x))
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return normalizeUnsigned(x)
	}
	return v
}

func normalizeUnsigned(u uint64) any {
	if u <= math.MaxInt64 {
		return int(u)
	}
	return float64(u)
}

// normalizeFloat turns a whole number that fits int64 into an int, so 2 and
// 2.0 read the same from either encoding.
func normalizeFloat(f float64) any {
	if f == math.Trunc(f) && f >= -(1<<63) && f < 1<<63 {
		return int(f)
	}
	return f
}

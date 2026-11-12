package execution

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/id"
)

// decodeEventFile returns the events one source file holds (events.DecodeFile),
// naming the source in an error.
func decodeEventFile(path string, data []byte) ([]events.Event, error) {
	evs, err := events.DecodeFile(path, data)
	if err != nil {
		return nil, fmt.Errorf("execution event %s: %w", path, err)
	}
	return evs, nil
}

// checkEvent refuses an event with no usable identity, timestamp, or type. A
// loose file must also be named for its event; a segment holds many.
func checkEvent(path string, event events.Event) error {
	if !id.Valid(event.ID) || !strings.HasPrefix(event.ID, "EV-") || event.At.IsZero() || event.Type == "" {
		return fmt.Errorf("execution event %s has invalid identity, timestamp, or type", path)
	}
	if !events.IsSegmentPath(path) && filepath.Base(path) != event.ID+".yml" {
		return fmt.Errorf("execution event %s has invalid identity, timestamp, or type", path)
	}
	return nil
}

// compactedHolds reports whether a segment of the event's day carries its id.
func (s *source) compactedHolds(event events.Event) (bool, error) {
	paths, err := s.daySegments(event)
	if err != nil || len(paths) == 0 {
		return false, err
	}
	contents, err := s.eventContents(context.Background(), paths)
	if err != nil {
		return false, err
	}
	for i, path := range paths {
		evs, err := decodeEventFile(path, contents[i])
		if err != nil {
			return false, err
		}
		if slices.ContainsFunc(evs, func(held events.Event) bool { return held.ID == event.ID }) {
			return true, nil
		}
	}
	return false, nil
}

// daySegments lists the segments in the source for the event's UTC day.
func (s *source) daySegments(event events.Event) ([]string, error) {
	day := event.At.UTC().Day()
	month := filepath.Dir(filepath.Dir(events.RelPath(event)))
	var names []string
	if s.kind != "working_tree" {
		prefix := s.gitPath(month) + "/"
		for path := range s.entries {
			name, ok := strings.CutPrefix(path, prefix)
			if !ok || strings.Contains(name, "/") {
				continue
			}
			if held, ok := events.SegmentDay(name); ok && held == day {
				names = append(names, name)
			}
		}
	} else {
		if err := s.checkWorktreePath(month); err != nil {
			return nil, err
		}
		entries, state, err := directory(s.worktreeRoot, s.gitPath(month))
		if err != nil {
			return nil, err
		}
		if expected, ok := s.directories[month]; ok && expected != state {
			return nil, fmt.Errorf("execution event directory %s changed", month)
		}
		s.directories[month] = state
		for _, entry := range entries {
			if held, ok := events.SegmentDay(entry.Name()); ok && held == day && !entry.IsDir() {
				names = append(names, entry.Name())
			}
		}
	}
	sort.Strings(names)
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(month, name)
	}
	return paths, nil
}

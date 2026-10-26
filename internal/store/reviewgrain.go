package store

import (
	"fmt"
	"strings"
)

// Review grain values: the grain at which a review PR opens
// (docs/spec/artifact-schema.md).
const (
	ReviewGrainTask    = "task"
	ReviewGrainFeature = "feature"
	ReviewGrainEpic    = "epic"
)

// ValidReviewGrain reports whether grain is a legal review-grain value for
// noun, one of NounEpic, NounFeature, or "project"
// (docs/spec/artifact-schema.md). All three grains may be explicit on a
// feature, including an epic-grain override retained by a reviewed feature
// move.
func ValidReviewGrain(noun, grain string) bool {
	switch noun {
	case NounEpic, NounFeature, "project":
		switch grain {
		case ReviewGrainTask, ReviewGrainFeature, ReviewGrainEpic:
			return true
		}
		return false
	default:
		return false
	}
}

func validateStoredReviewGrain(path, noun, grain string) error {
	if grain == "" || ValidReviewGrain(noun, grain) {
		return nil
	}
	field := noun + " review_grain"
	legal := "task, feature, or epic"
	if noun == "project" {
		field = "execution.review_grain"
	}
	return fmt.Errorf("parse %s: %s %q is invalid; want %s", path, field, grain, legal)
}

// validateStoredStartedGrain refuses a task.yml started_grain outside the
// three review grains, the same load-time posture review_grain takes.
func validateStoredStartedGrain(path, grain string) error {
	switch grain {
	case "", ReviewGrainTask, ReviewGrainFeature, ReviewGrainEpic:
		return nil
	}
	return fmt.Errorf("parse %s: task started_grain %q is invalid; want task, feature, or epic", path, grain)
}

// TaskReviewGrain is the review grain in force for task: the grain it started
// under when start recorded one (source "started"), else its parents'
// resolution (ResolvedReviewGrainWithSource). A task started at task grain
// keeps task grain through a later coarse setting. task may be
// nil, for work not yet started.
func TaskReviewGrain(m *Manifest, e *Epic, f *Feature, task *Task) (grain, source string) {
	if task != nil && task.StartedGrain != "" {
		return task.StartedGrain, "started"
	}
	return ResolvedReviewGrainWithSource(m, e, f)
}

// ResolvedReviewGrain returns the review grain in force for work under e and
// f: the feature's explicit setting if it has one, else the epic's, else the
// manifest's project default, else task
// (docs/spec/artifact-schema.md).
// m, e, and f may all be nil or zero; ResolvedReviewGrain performs no
// validation, the setters that write these fields own that.
//
// It delegates to ResolvedReviewGrainWithSource, keeping the grain half so
// this function's existing callers (which want only the grain, never the
// source) stay untouched. The two can never disagree, because there is only
// one precedence walk between them.
func ResolvedReviewGrain(m *Manifest, e *Epic, f *Feature) string {
	grain, _ := ResolvedReviewGrainWithSource(m, e, f)
	return grain
}

// ResolvedReviewGrainWithSource is ResolvedReviewGrain's own precedence walk,
// widened to also name which of the three homes
// (docs/spec/artifact-schema.md) actually
// produced the resolved value: "feature", "epic", "project" (the manifest's
// execution.review_grain), or "default" when none of the three is set and
// the value is the hardcoded task fallback. This is the ONE resolver both
// ResolvedReviewGrain and the console's node header payload
// (views.reviewGrainOf, internal/views/node.go) consume, so a future
// precedence change (a new home, a reordering) cannot land in one without the
// other silently falling out of step (the two used
// to be a parallel re-derivation, one switch computing the grain and a second
// switch in views guessing at the source from the same fields).
//
// Source names WHERE the value came from, not whether it happens to differ
// from the hardcoded fallback: an explicit feature-level "task" override
// under an epic-grain epic resolves grain "task" (the same value the
// fallback would also produce) but sources as "feature", the explicit
// setting that actually won, never "default"
// (TestResolvedReviewGrainWithSourceNamesTheWinningHome's own ambiguous
// case). m, e, and f may all be nil or zero; this function performs no
// validation, the setters that write these fields own that.
func ResolvedReviewGrainWithSource(m *Manifest, e *Epic, f *Feature) (grain string, source string) {
	switch {
	case f != nil && f.ReviewGrain != "":
		return f.ReviewGrain, "feature"
	case e != nil && e.ReviewGrain != "":
		return e.ReviewGrain, "epic"
	case m != nil && m.Execution.ReviewGrain != "":
		return m.Execution.ReviewGrain, "project"
	default:
		return ReviewGrainTask, "default"
	}
}

// RenderExecutionReviewGrain rewrites only the execution.review_grain setting
// of manifestData, leaving every other line, comment, and formatting byte
// intact, the same surgical posture RenderContributors and RenderSchemaVersion take
// with their own blocks: the manifest is human-owned. If an execution: block
// already exists, its review_grain line is rewritten in place, or inserted
// right after the block header if the block has no such line yet. If no
// execution: block exists, one is appended at the end.
func RenderExecutionReviewGrain(manifestData []byte, grain string) ([]byte, error) {
	lines := strings.Split(string(manifestData), "\n")

	start := -1
	for i, line := range lines {
		if line != strings.TrimLeft(line, " \t") || !strings.HasPrefix(line, "execution:") {
			continue
		}
		remainder := strings.TrimSpace(strings.TrimPrefix(line, "execution:"))
		if remainder != "" && !strings.HasPrefix(remainder, "#") {
			return nil, fmt.Errorf("manifest execution must use a block mapping before tiller can update review_grain safely")
		}
		start = i
		break
	}

	if start == -1 {
		out := strings.TrimRight(string(manifestData), "\n") + "\n"
		out += fmt.Sprintf("execution:\n  review_grain: %s\n", grain)
		return []byte(out), nil
	}

	end := start
	childIndent := ""
	for j := start + 1; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			end = j
			continue
		}
		if lines[j][0] != ' ' && lines[j][0] != '\t' {
			break
		}
		end = j
		content := strings.TrimLeft(lines[j], " \t")
		indent := lines[j][:len(lines[j])-len(content)]
		if childIndent == "" || len(indent) < len(childIndent) {
			childIndent = indent
		}
	}
	if childIndent == "" {
		childIndent = "  "
	}

	for j := start + 1; j <= end; j++ {
		trimmed := strings.TrimLeft(lines[j], " \t")
		indent := lines[j][:len(lines[j])-len(trimmed)]
		key, _, found := strings.Cut(trimmed, ":")
		if indent == childIndent && found && key == "review_grain" {
			lines[j] = childIndent + "review_grain: " + grain
			return []byte(strings.Join(lines, "\n")), nil
		}
	}

	// The block exists but has no direct review_grain child yet: insert it
	// right after the block header without altering any nested key.
	next := append([]string{}, lines[:start+1]...)
	next = append(next, childIndent+"review_grain: "+grain)
	next = append(next, lines[start+1:]...)
	return []byte(strings.Join(next, "\n")), nil
}

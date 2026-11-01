package mutate

import (
	"fmt"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

const eventGradeBug = "grade.bug"

// GradeInput names a bug and the severity to set on it.
type GradeInput struct {
	Noun     string
	Node     string
	Severity string
}

// GradeResult reports one severity change. Changed is false when the bug
// already carried the level.
type GradeResult struct {
	ID          string
	Key         string
	OldSeverity string
	Severity    string
	Changed     bool
}

// Grade sets or changes a non-terminal bug's severity. Any contributor may
// grade; severity never affects routing. A done or cancelled bug refuses,
// since its grade is part of the closed record. An equal level is a no-op
// that writes nothing and records no event.
func Grade(tree *store.Tree, deps Deps, in GradeInput) (GradeResult, error) {
	if in.Noun != store.NounBug {
		return GradeResult{}, &usageError{reason: "severity_not_bug", msg: "severity applies to bugs"}
	}
	if in.Severity == "" {
		return GradeResult{}, &usageError{reason: "invalid_severity", msg: "severity needs one of blocker, important, medium, or minor"}
	}
	if err := severityUsage(in.Noun, in.Severity); err != nil {
		return GradeResult{}, err
	}
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return GradeResult{}, err
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Node})
	if err != nil {
		return GradeResult{}, err
	}
	w, ok := store.AsWorkNode(node)
	if !ok {
		return GradeResult{}, fmt.Errorf("mutate: grade resolved %s to an unsupported node type", in.Noun)
	}
	doc, rel, snapshot, err := loadWorkItemFile(tree, w)
	if err != nil {
		return GradeResult{}, err
	}
	b := doc.Body()
	if b.Status == "done" || b.Status == "cancelled" {
		return GradeResult{}, &usageError{
			reason: "terminal_grade",
			msg:    fmt.Sprintf("bug %s is %s; a closed bug's grade is part of the record", b.ID, b.Status),
		}
	}
	result := GradeResult{ID: b.ID, Key: deref(b.Key), OldSeverity: b.Severity, Severity: in.Severity}
	if b.Severity == in.Severity {
		return result, nil
	}
	b.Severity = in.Severity
	data, err := yaml.Marshal(doc)
	if err != nil {
		return GradeResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventGradeBug, owner.ID,
		fmt.Sprintf("grade bug %s %s", b.ID, in.Severity), b.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"severity": result.OldSeverity}
	ev.After = map[string]any{"severity": in.Severity}
	ev.Files = []events.FileChange{{Path: rel}}

	if err := commitMutation(tree, "grade bug", []store.PlannedWrite{
		store.PlanWrite(filepath.Join(tree.Root, rel), snapshot, data),
	}, []events.Event{ev}); err != nil {
		return GradeResult{}, err
	}
	result.Changed = true
	return result, nil
}

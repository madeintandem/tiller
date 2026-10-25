// Package execution supplies request-local, read-only observations of task
// execution. Integration owns structure and completion; task branches contribute
// only their own execution fields, never a second project tree.
package execution

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Record identifies the source of one displayed task execution observation.
// Revision is an opaque, source-qualified read token, not a Git revision to run.
type Record struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Branch      string    `json:"branch"`
	Source      string    `json:"source"`
	Revision    string    `json:"revision"`
	ObservedAt  time.Time `json:"observed_at"`
	Uncommitted bool      `json:"uncommitted"`
	// Freshness names which related tip is behind when the local branch and
	// its tracking ref differ: local_branch_behind or remote_tracking_behind.
	Freshness         string `json:"freshness,omitempty"`
	Status            string `json:"status"`
	Owner             string `json:"owner"`
	IntegrationStatus string `json:"integration_status"`
}

type Diagnostic struct {
	ID      string `json:"id,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Snapshot never mutates Base. Tree retains Base's immutable structural values
// and indexes its own task copies wherever execution fields differ.
type Snapshot struct {
	Base         *store.Tree
	Tree         *store.Tree
	Observations map[string]Record
	Diagnostics  []Diagnostic
	repo         *repository
	sources      map[string]*source
	// integrationEvents is read at most once, only when an ownership
	// disagreement needs the newest-event tie-break.
	integrationEvents    []events.Event
	integrationEventsErr error
	integrationEventsSet bool
}

type Report struct {
	Observations []Record     `json:"observations"`
	Diagnostics  []Diagnostic `json:"diagnostics"`
}

func (s *Snapshot) Report() Report {
	out := Report{Observations: make([]Record, 0, len(s.Observations)), Diagnostics: make([]Diagnostic, len(s.Diagnostics))}
	for _, observation := range s.Observations {
		out.Observations = append(out.Observations, observation)
	}
	sort.Slice(out.Observations, func(i, j int) bool { return out.Observations[i].ID < out.Observations[j].ID })
	copy(out.Diagnostics, s.Diagnostics)
	sortDiagnostics(out.Diagnostics)
	return out
}

// Load discovers refs and worktrees once, then reads only matching task records.
// A rejected candidate contributes a diagnostic, not a failed integration read.
func Load(base *store.Tree) (*Snapshot, error) {
	if base == nil || base.Manifest == nil {
		return nil, errors.New("execution: integration tree and manifest are required")
	}
	if err := base.CheckStable(); err != nil {
		return nil, err
	}
	s := &Snapshot{Base: base, Tree: displayTree(base), Observations: map[string]Record{}, Diagnostics: []Diagnostic{}, sources: map[string]*source{}}
	repo, err := discover(base.Root)
	if err != nil {
		s.diagnostic("", "", "discovery_unavailable", err.Error())
		return s, nil
	}
	if repo == nil {
		return s, nil
	}
	s.repo = repo
	ids := make([]string, 0, len(base.Tasks))
	for id := range base.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	observedAt := time.Now().UTC()
	var candidates []candidate
	for _, id := range ids {
		task := base.Tasks[id]
		if !eligible(base, task) {
			continue
		}
		key, path, err := taskIdentity(base, task)
		if err != nil {
			s.diagnostic(id, task.Branch, "invalid_identity", err.Error())
			continue
		}
		branch := task.Branch
		if branch == "" {
			pattern := base.Manifest.Git.BranchPattern
			if strings.TrimSpace(pattern) == "" {
				pattern = "tiller/${node.key}"
			}
			if !strings.Contains(pattern, "${node.key}") {
				s.diagnostic(id, "", "branch_invalid", "git.branch_pattern must include ${node.key}")
				continue
			}
			branch = strings.ReplaceAll(pattern, "${node.key}", key)
		}
		if !validBranch(branch) {
			s.diagnostic(id, branch, "branch_invalid", "execution branch is not a valid local branch name")
			continue
		}
		candidates = append(candidates, candidate{id: id, key: key, path: path, branch: branch, task: task})
	}
	for i, selected := range repo.selectAll(candidates) {
		s.apply(candidates[i], selected, observedAt)
	}
	s.reindex()
	return s, nil
}

// apply records one candidate's selection: its diagnostics, and when its
// source holds a valid, active execution record, the observation. It runs
// serially in task id order, so ownership tie-breaks and every write to s
// behave exactly as when sources were selected one at a time.
func (s *Snapshot) apply(c candidate, selected selection, observedAt time.Time) {
	id, task, branch := c.id, c.task, c.branch
	for _, diagnostic := range selected.diagnostics {
		diagnostic.ID = id
		s.Diagnostics = append(s.Diagnostics, diagnostic)
	}
	if selected.err != nil {
		s.diagnostic(id, branch, "source_unavailable", selected.err.Error())
		return
	}
	src := selected.src
	if src == nil {
		if task.Branch != "" && len(selected.diagnostics) == 0 {
			s.diagnostic(id, branch, "branch_missing", "recorded execution branch cannot be resolved locally; it may be uncreated, deleted, or unavailable")
		}
		return
	}
	if selected.readErr != nil {
		s.diagnostic(id, branch, "source_unavailable", selected.readErr.Error())
		return
	}
	var record store.Task
	if err := yaml.Unmarshal(selected.data, &record); err != nil {
		s.diagnostic(id, branch, "invalid_candidate", fmt.Sprintf("parse execution task: %v", err))
		return
	}
	if err := validateCandidate(task, &record, branch); err != nil {
		s.diagnostic(id, branch, "invalid_candidate", err.Error())
		return
	}
	// A matching branch name by itself is not evidence of activity.
	if record.Owner == "" && !record.Started && task.Owner == "" && !task.Started {
		return
	}
	if record.Owner != task.Owner {
		newer, err := s.newerIntegrationOwnership(src, id)
		if err != nil {
			s.diagnostic(id, branch, "ownership_unresolved", "cannot compare ownership events; showing integration ownership: "+err.Error())
			return
		}
		if newer != nil {
			s.diagnostic(id, branch, "ownership_superseded", fmt.Sprintf("integration %s %s at %s is newer than the branch's ownership record; showing integration ownership", newer.Type, newer.ID, newer.At.Format(time.RFC3339)))
			return
		}
	}
	updated := *task
	updated.Status, updated.Owner, updated.Started = record.Status, record.Owner, record.Started
	updated.Branch, updated.PR, updated.ReviewRef = record.Branch, record.PR, record.ReviewRef
	updated.Gates, updated.Attestations, updated.Evidence = record.Gates, record.Attestations, record.Evidence
	s.Tree.Tasks[id] = &updated
	s.sources[id] = src
	s.Observations[id] = Record{ID: id, Key: c.key, Branch: branch, Source: src.kind, Revision: src.revision(id), ObservedAt: observedAt, Uncommitted: src.uncommitted, Freshness: src.freshness, Status: record.Status, Owner: record.Owner, IntegrationStatus: task.Status}
}

// reindex points FeatureTasks at the displayed task values and recomputes
// contributors, after observations are applied or dropped.
func (s *Snapshot) reindex() {
	for featureID, tasks := range s.Tree.FeatureTasks {
		for i, task := range tasks {
			tasks[i] = s.Tree.Tasks[task.ID]
		}
		s.Tree.FeatureTasks[featureID] = tasks
	}
	s.Tree.ProjectContributors()
	sortDiagnostics(s.Diagnostics)
}

// ownershipEventTypes are the events that set or clear a task's owner.
var ownershipEventTypes = map[string]bool{"claim.task": true, "release.task": true, "reassign.task": true, "start.task": true}

func newestOwnership(evs []events.Event, taskID string) *events.Event {
	var newest *events.Event
	for i := range evs {
		event := &evs[i]
		if ownershipEventTypes[event.Type] && slices.Contains(event.Targets, taskID) && (newest == nil || events.Less(*newest, *event)) {
			newest = event
		}
	}
	return newest
}

// newerIntegrationOwnership breaks an owner disagreement by the newest
// ownership event. It returns integration's newest ownership event when that
// event is newer than the branch's, and nil when the branch's record stands:
// integration has no ownership event for the task, the branch already carries
// integration's newest one, or the branch has a newer one of its own.
func (s *Snapshot) newerIntegrationOwnership(src *source, taskID string) (*events.Event, error) {
	if !s.integrationEventsSet {
		s.integrationEvents, s.integrationEventsErr = events.List(s.Base)
		s.integrationEventsSet = true
	}
	if s.integrationEventsErr != nil {
		return nil, s.integrationEventsErr
	}
	latest := newestOwnership(s.integrationEvents, taskID)
	if latest == nil {
		return nil, nil
	}
	seen, err := src.hasEvent(*latest)
	if err != nil || seen {
		return nil, err
	}
	branchEvents, err := src.events(taskID)
	if err != nil {
		return nil, err
	}
	if branchLatest := newestOwnership(branchEvents, taskID); branchLatest != nil && events.Less(*latest, *branchLatest) {
		return nil, nil
	}
	return latest, nil
}

func (s *Snapshot) diagnostic(id, branch, code, message string) {
	s.Diagnostics = append(s.Diagnostics, Diagnostic{ID: id, Branch: branch, Code: code, Message: message})
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		a, b := diagnostics[i], diagnostics[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Branch != b.Branch {
			return a.Branch < b.Branch
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}

func displayTree(base *store.Tree) *store.Tree {
	out := *base
	out.Tasks = maps.Clone(base.Tasks)
	out.FeatureTasks = make(map[string][]*store.Task, len(base.FeatureTasks))
	for id, tasks := range base.FeatureTasks {
		out.FeatureTasks[id] = append([]*store.Task(nil), tasks...)
	}
	return &out
}

func eligible(base *store.Tree, task *store.Task) bool {
	if task.Status == "done" || task.Status == "cancelled" || task.SupersededBy != nil {
		return false
	}
	feature := base.Features[task.FeatureID]
	if feature == nil || feature.SupersededBy != nil {
		return false
	}
	return gen.RetiredAncestor(base, task) == nil
}

func taskIdentity(base *store.Tree, task *store.Task) (string, string, error) {
	feature := base.Features[task.FeatureID]
	if task.Key == nil || feature == nil || feature.Key == nil {
		return "", "", fmt.Errorf("task %s has no canonical task identity", task.ID)
	}
	key := *task.Key
	parts := strings.Split(key, "/")
	featureParts := strings.Split(*feature.Key, "/")
	if len(parts) != 3 || len(featureParts) != 2 || parts[0] != feature.EpicKey || parts[0] != featureParts[0] || parts[1] != featureParts[1] {
		return "", "", fmt.Errorf("task %s key %q disagrees with its integration parent", task.ID, key)
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || strings.ContainsAny(part, "\\\x00") {
			return "", "", fmt.Errorf("task %s has unsafe key %q", task.ID, key)
		}
	}
	return key, filepath.Join(store.TaskRelDir(parts[0], parts[1], parts[2]), store.TaskStateFile), nil
}

func validateCandidate(base, candidate *store.Task, branch string) error {
	if candidate.ID != base.ID || candidate.Key == nil || base.Key == nil || *candidate.Key != *base.Key {
		return fmt.Errorf("execution record must match integration task ID %s and key %q", base.ID, *base.Key)
	}
	if (base.AcceptedAgainst == nil) != (candidate.AcceptedAgainst == nil) {
		return fmt.Errorf("task %s execution accepted basis does not match integration", base.ID)
	}
	if base.AcceptedAgainst != nil && (!base.AcceptedAgainst.Spec.SameAs(candidate.AcceptedAgainst.Spec) ||
		!base.AcceptedAgainst.Eval.SameAs(candidate.AcceptedAgainst.Eval) ||
		!base.AcceptedAgainst.Commit.SameAs(candidate.AcceptedAgainst.Commit)) {
		return fmt.Errorf("task %s execution accepted basis does not match integration", base.ID)
	}
	if strings.TrimSpace(candidate.Owner) != candidate.Owner {
		return fmt.Errorf("task %s execution owner must not contain surrounding whitespace", base.ID)
	}
	if candidate.Branch != "" && candidate.Branch != branch {
		return fmt.Errorf("task %s records branch %q, not selected branch %q", base.ID, candidate.Branch, branch)
	}
	if base.Branch != "" && candidate.Branch != base.Branch {
		return fmt.Errorf("task %s does not preserve its recorded execution branch %q", base.ID, base.Branch)
	}
	if candidate.Started && candidate.Branch != branch {
		return fmt.Errorf("started task %s has no matching recorded branch", base.ID)
	}
	if base.Started && (!candidate.Started || candidate.Owner != base.Owner) {
		return fmt.Errorf("task %s cannot replace its integration-recorded durable start or execution owner %q", base.ID, base.Owner)
	}
	if candidate.ReviewRef != nil && candidate.ReviewRef.Branch != branch {
		return fmt.Errorf("task %s review ref does not name its selected execution branch %q", base.ID, branch)
	}
	switch candidate.Status {
	case "ready", "proposed", "in_progress", "check", "in_review":
	default:
		return fmt.Errorf("task %s has invalid execution status %q; terminal state remains integration-authoritative", base.ID, candidate.Status)
	}
	if candidate.Status == "proposed" && candidate.Owner != "" {
		return fmt.Errorf("proposed task %s cannot carry an execution reservation", base.ID)
	}
	// Reuse ownership rules, but never validate or import the candidate's stale
	// replacement graph or siblings as if they were the approved integration plan.
	ownership := *candidate
	ownership.Supersedes, ownership.SupersededBy = nil, nil
	return (&store.Tree{Tasks: map[string]*store.Task{ownership.ID: &ownership}}).ValidateOwnership()
}

// unstable reports each selected source whose observation no longer holds.
// An error means the integration tree itself moved.
func (s *Snapshot) unstable() (map[string]error, error) {
	if err := s.Base.CheckStable(); err != nil {
		return nil, err
	}
	failures := map[string]error{}
	remaining := map[string]*source{}
	for id, src := range s.sources {
		if err := src.checkFiles(); err != nil {
			failures[id] = err
		} else {
			remaining[id] = src
		}
	}
	if len(remaining) == 0 {
		return failures, nil
	}
	identity, err := s.repo.checkIdentities(remaining)
	if err != nil {
		// Without a trustworthy ref inventory no source can be confirmed.
		for id := range remaining {
			failures[id] = err
		}
		return failures, nil
	}
	maps.Copy(failures, identity)
	return failures, nil
}

// CheckStable closes both integration and selected-source read boundaries,
// failing if any of them moved.
func (s *Snapshot) CheckStable() error {
	failures, err := s.unstable()
	if err != nil {
		return err
	}
	ids := slices.Sorted(maps.Keys(failures))
	if len(ids) > 0 {
		return fmt.Errorf("execution %s: %w", ids[0], failures[ids[0]])
	}
	return nil
}

// Settle closes the read boundary like CheckStable, but a source that moved
// costs only its own observation: the task falls back to its integration
// value and gains a source_unstable diagnostic. It reports whether anything
// was dropped, so the caller can recompute what it built from Tree. An error
// still means the integration tree itself moved.
func (s *Snapshot) Settle() (bool, error) {
	failures, err := s.unstable()
	if err != nil || len(failures) == 0 {
		return false, err
	}
	for id, failure := range failures {
		s.diagnostic(id, s.Observations[id].Branch, "source_unstable", failure.Error())
		delete(s.Observations, id)
		delete(s.sources, id)
		s.Tree.Tasks[id] = s.Base.Tasks[id]
	}
	s.reindex()
	return true, nil
}

// Served reports whether a task's observation is the served checkout itself:
// the checkout has that task's branch checked out, so the served tree already
// carries its execution and nothing is in flight anywhere else.
func (s *Snapshot) Served(id string) bool {
	src := s.sources[id]
	return src != nil && s.repo != nil && src.worktreeRoot != "" && samePath(src.worktreeRoot, s.repo.root)
}

// ReadFile cannot fall back to integration or another observation. Callers must
// present the token of the selected task source and use a visible relative path.
// A file larger than a positive limit is refused with ErrTooLarge before it is
// loaded; zero reads the whole file. Like every source read, its bytes are trustworthy only once
// CheckStable or Settle has confirmed the source afterwards.
func (s *Snapshot) ReadFile(id, revision, path string, limit int64) ([]byte, error) {
	src := s.sources[id]
	record, ok := s.Observations[id]
	if !ok || src == nil {
		return nil, fmt.Errorf("execution source for %s is unavailable", id)
	}
	if record.Revision != revision {
		return nil, errors.New("execution revision does not match selected source; refresh the task")
	}
	if err := evidencePath(path); err != nil {
		return nil, err
	}
	return src.readAtMost(path, limit)
}

// Events returns only events targeting this task from its selected source,
// confirmed by a later CheckStable or Settle like ReadFile.
func (s *Snapshot) Events(id string) ([]events.Event, error) {
	src := s.sources[id]
	if src == nil {
		return nil, fmt.Errorf("execution history for %s is unavailable", id)
	}
	return src.events(id)
}

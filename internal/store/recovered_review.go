package store

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/id"
	"gopkg.in/yaml.v3"
)

// RecoveredReview binds an aggregate review to immutable historical evidence.
// Head is the source tree for every task and policy blob, never the live branch.
type RecoveredReview struct {
	Branch            string                     `yaml:"branch"`
	Head              RevisionRef                `yaml:"head"`
	MergeCommit       RevisionRef                `yaml:"merge_commit"`
	IntegrationBranch string                     `yaml:"integration_branch"`
	Basis             string                     `yaml:"basis"`
	RecoveredBy       string                     `yaml:"recovered_by"`
	RecoveredAt       time.Time                  `yaml:"recovered_at"`
	Reason            string                     `yaml:"reason"`
	Tasks             []RecoveredTask            `yaml:"tasks"`
	PolicySnapshots   []HistoricalPolicySnapshot `yaml:"policy_snapshots"`
}

// RecoveredTask pins submitted state rather than copying mutable task fields.
type RecoveredTask struct {
	ID    string      `yaml:"id"`
	Path  string      `yaml:"path"`
	State RevisionRef `yaml:"state"`
}

// HistoricalPolicySnapshot pins the recorded policy inputs. Empty NodeID names
// the manifest; node observations are stored-state facts, not newly run checks.
type HistoricalPolicySnapshot struct {
	NodeID       string                        `yaml:"node_id,omitempty"`
	Path         string                        `yaml:"path"`
	Source       RevisionRef                   `yaml:"source"`
	Observations []HistoricalPolicyObservation `yaml:"observations,omitempty"`
}

type HistoricalPolicyObservation struct {
	Gate               string   `yaml:"gate"`
	Status             string   `yaml:"status"`
	UnresolvedCriteria []string `yaml:"unresolved_criteria,omitempty"`
	EvidenceRefs       []string `yaml:"evidence_refs,omitempty"`
}

// UnmarshalYAML is deliberately stricter than legacy node decoding. A typo or
// partial recovery record must refuse instead of selecting ordinary live refs.
func (r *RecoveredReview) UnmarshalYAML(node *yaml.Node) error {
	if err := recoveryShape(node); err != nil {
		return fmt.Errorf("recovered_review: %w", err)
	}
	type plain RecoveredReview
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return fmt.Errorf("recovered_review: %w", err)
	}
	*r = RecoveredReview(decoded)
	return r.Validate()
}

// Validate checks durable shape independent of Git availability. Evidence and
// pinned historical owner/scope checks belong to the recovery planner and sync.
func (r *RecoveredReview) Validate() error {
	bad := func(format string, args ...any) error { return fmt.Errorf("recovered_review: "+format, args...) }
	if r == nil {
		return bad("record must be a mapping")
	}
	for name, value := range map[string]string{"branch": r.Branch, "integration_branch": r.IntegrationBranch, "recovered_by": r.RecoveredBy, "reason": r.Reason} {
		if strings.TrimSpace(value) == "" {
			return bad("%s must not be blank", name)
		}
	}
	if r.Branch != strings.TrimSpace(r.Branch) || r.IntegrationBranch != strings.TrimSpace(r.IntegrationBranch) || r.RecoveredBy != strings.TrimSpace(r.RecoveredBy) {
		return bad("branch, integration_branch, and recovered_by must be canonical nonblank identities")
	}
	if r.RecoveredAt.IsZero() {
		return bad("recovered_at must record the actual recording time")
	}
	switch r.Basis {
	case "ancestry", "content", "contained":
	default:
		return bad("unsupported basis %q", r.Basis)
	}
	if err := canonicalRecoveryRevision(r.Head, KindGitCommit, ""); err != nil {
		return bad("head: %v", err)
	}
	if err := canonicalRecoveryRevision(r.MergeCommit, KindGitCommit, ""); err != nil {
		return bad("merge_commit: %v", err)
	}
	if len(r.Tasks) == 0 {
		return bad("tasks must contain at least one historical rider")
	}
	paths := make(map[string]bool, len(r.Tasks))
	for i, task := range r.Tasks {
		if !id.Valid(task.ID) || !strings.HasPrefix(task.ID, "T-") {
			return bad("task %q is not a task ID", task.ID)
		}
		if i > 0 && r.Tasks[i-1].ID >= task.ID {
			return bad("tasks must be sorted by ID and unique")
		}
		if !recoveryPath(task.Path) || path.Base(task.Path) != TaskStateFile || !strings.Contains(task.Path, "/tasks/") || paths[task.Path] {
			return bad("task %s has invalid or duplicate historical path %q", task.ID, task.Path)
		}
		paths[task.Path] = true
		if err := canonicalRecoveryRevision(task.State, KindGitBlob, task.Path); err != nil {
			return bad("task %s state: %v", task.ID, err)
		}
	}
	if len(r.PolicySnapshots) == 0 || r.PolicySnapshots[0].NodeID != "" || r.PolicySnapshots[0].Path != ".tiller/tiller.yml" {
		return bad("policy_snapshots must begin with the historical manifest")
	}
	snapshots := make(map[string]HistoricalPolicySnapshot, len(r.PolicySnapshots))
	for i, snapshot := range r.PolicySnapshots {
		if i > 0 {
			prior := r.PolicySnapshots[i-1]
			if snapshot.NodeID == "" || prior.NodeID > snapshot.NodeID || (prior.NodeID == snapshot.NodeID && prior.Path >= snapshot.Path) {
				return bad("policy_snapshots must be sorted by node ID and path and unique")
			}
			if !id.Valid(snapshot.NodeID) || (!strings.HasPrefix(snapshot.NodeID, "F-") && !strings.HasPrefix(snapshot.NodeID, "T-")) {
				return bad("policy snapshot %q must name a task or feature", snapshot.NodeID)
			}
		}
		if !recoveryPath(snapshot.Path) || snapshots[snapshot.Path].Path != "" {
			return bad("invalid or duplicate policy snapshot path %q", snapshot.Path)
		}
		snapshots[snapshot.Path] = snapshot
		if err := canonicalRecoveryRevision(snapshot.Source, KindGitBlob, snapshot.Path); err != nil {
			return bad("policy snapshot %s: %v", snapshot.Path, err)
		}
		for j, observation := range snapshot.Observations {
			if err := validateHistoricalObservation(observation); err != nil {
				return bad("policy snapshot %s: %v", snapshot.Path, err)
			}
			if j > 0 && snapshot.Observations[j-1].Gate >= observation.Gate {
				return bad("policy observations must be sorted by gate and unique")
			}
		}
	}
	for _, task := range r.Tasks {
		snapshot, exists := snapshots[task.Path]
		if !exists || snapshot.NodeID != task.ID || snapshot.Source.Kind != task.State.Kind || snapshot.Source.Value != task.State.Value {
			return bad("task %s policy snapshot must pin its historical task state", task.ID)
		}
		featurePath := path.Join(path.Dir(path.Dir(path.Dir(task.Path))), "feature.yml")
		feature, exists := snapshots[featurePath]
		if !exists || !strings.HasPrefix(feature.NodeID, "F-") {
			return bad("task %s requires its historical feature policy snapshot", task.ID)
		}
	}
	return nil
}

func validateHistoricalObservation(o HistoricalPolicyObservation) error {
	if strings.TrimSpace(o.Gate) == "" || o.Gate != strings.TrimSpace(o.Gate) {
		return fmt.Errorf("historical gate must be a nonblank canonical name")
	}
	switch o.Status {
	case GateStatusFail, GateStatusPending, GateStatusSkipped, GateStatusStale, CriterionStatusNeedsAttestation, "unknown":
	default:
		return fmt.Errorf("historical gate %s has invalid nonpassing status %q", o.Gate, o.Status)
	}
	if err := sortedRecoveryStrings("unresolved_criteria", o.UnresolvedCriteria); err != nil {
		return err
	}
	return sortedRecoveryStrings("evidence_refs", o.EvidenceRefs)
}

func sortedRecoveryStrings(name string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || (i > 0 && values[i-1] >= value) {
			return fmt.Errorf("%s must contain sorted, unique, nonblank values", name)
		}
	}
	return nil
}

func recoveryPath(p string) bool {
	return strings.HasPrefix(p, ".tiller/") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00\r\n")
}

func canonicalRecoveryRevision(ref RevisionRef, kind, sourcePath string) error {
	if ref.Kind != kind || ref.Path != sourcePath || !strings.HasPrefix(ref.Value, "git:") {
		return fmt.Errorf("expected %s immutable revision with path %q", kind, sourcePath)
	}
	oid := strings.TrimPrefix(ref.Value, "git:")
	if len(oid) != 40 && len(oid) != 64 {
		return fmt.Errorf("revision must contain a full Git object ID")
	}
	for _, ch := range oid {
		if !(ch >= '0' && ch <= '9') && !(ch >= 'a' && ch <= 'f') {
			return fmt.Errorf("revision must contain a canonical lowercase Git object ID")
		}
	}
	return nil
}

func validateRecoveryOwner(record *RecoveredReview, branch string, pr *int, review *ReviewRef) error {
	if record == nil {
		return nil
	}
	if branch != record.Branch {
		return fmt.Errorf("recovered_review: branch does not match owning aggregate branch")
	}
	if (pr == nil) == (review == nil) {
		return fmt.Errorf("recovered_review: owning aggregate must have exactly one PR or review_ref identity")
	}
	if pr != nil && *pr <= 0 {
		return fmt.Errorf("recovered_review: owning PR must be positive")
	}
	if review != nil && (review.Branch != record.Branch || review.Head.Kind != record.Head.Kind || review.Head.Value != record.Head.Value || review.Head.Path != "") {
		return fmt.Errorf("recovered_review: owning review_ref must match historical branch and head")
	}
	return nil
}

// validateRecoveredReviewGrains refuses a live recovered aggregate whose
// resolved grain moved off the recorded one, since sync and every reader would
// disagree about who owns the round. A closed aggregate's record is history
// that ReviewOwnerOf reads regardless of grain, so a later grain change never
// refuses the whole tree over it.
func (t *Tree) validateRecoveredReviewGrains() error {
	for _, epic := range t.Epics {
		if epic.RecoveredReview == nil || recoveredAggregateClosed(epic.Status) {
			continue
		}
		if ResolvedReviewGrain(t.Manifest, epic, nil) != ReviewGrainEpic {
			return fmt.Errorf("%s recovered_review: owning aggregate no longer has epic review grain; restore review_grain: epic on the epic or as execution.review_grain in tiller.yml", epic.ID)
		}
	}
	for _, feature := range t.Features {
		if feature.RecoveredReview == nil || recoveredAggregateClosed(feature.Status) {
			continue
		}
		var parent *Epic
		for _, epic := range t.Epics {
			if epic.Key != nil && *epic.Key == feature.EpicKey {
				parent = epic
				break
			}
		}
		if ResolvedReviewGrain(t.Manifest, parent, feature) != ReviewGrainFeature {
			return fmt.Errorf("%s recovered_review: owning aggregate no longer has feature review grain; restore review_grain: feature on the feature, its epic, or as execution.review_grain in tiller.yml", feature.ID)
		}
	}
	return nil
}

func recoveredAggregateClosed(status string) bool {
	return status == "done" || status == "cancelled" || status == "archived"
}

// Presence is checked separately because yaml.v3 skips pointer unmarshalling
// for null. Explicit null is corruption, not absence of a recovery record.
func validateRecoveryPresence(node *yaml.Node) error {
	hasMerge := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "recovered_review" && node.Content[i+1].Kind != yaml.MappingNode {
			return fmt.Errorf("recovered_review: record must be a mapping")
		}
		hasMerge = hasMerge || node.Content[i].Tag == "!!merge"
	}
	if hasMerge {
		// Decode effective keys using yaml.v3's merge precedence. Inspecting
		// only direct keys misses an inherited null, whose pointer decoder
		// skips RecoveredReview.UnmarshalYAML entirely.
		var fields map[string]yaml.Node
		if err := node.Decode(&fields); err != nil {
			return err
		}
		if record, exists := fields["recovered_review"]; exists && record.Kind != yaml.MappingNode {
			return fmt.Errorf("recovered_review: record must be a mapping")
		}
	}
	return nil
}

func (f *Feature) UnmarshalYAML(node *yaml.Node) error {
	if err := validateRecoveryPresence(node); err != nil {
		return err
	}
	type plain Feature
	if err := node.Decode((*plain)(f)); err != nil {
		return err
	}
	return validateRecoveryOwner(f.RecoveredReview, f.Branch, f.PR, f.ReviewRef)
}

func (e *Epic) UnmarshalYAML(node *yaml.Node) error {
	if err := validateRecoveryPresence(node); err != nil {
		return err
	}
	type plain Epic
	if err := node.Decode((*plain)(e)); err != nil {
		return err
	}
	return validateRecoveryOwner(e.RecoveredReview, e.Branch, e.PR, e.ReviewRef)
}

func recoveryShape(node *yaml.Node) error {
	fields, err := recoveryMapping(node, "branch head merge_commit integration_branch basis recovered_by recovered_at reason tasks policy_snapshots", "branch head merge_commit integration_branch basis recovered_by recovered_at reason tasks policy_snapshots")
	if err != nil {
		return err
	}
	for _, name := range []string{"branch", "integration_branch", "basis", "recovered_by", "reason"} {
		if fields[name].Kind != yaml.ScalarNode || fields[name].Tag != "!!str" {
			return fmt.Errorf("%s must be a string", name)
		}
	}
	for _, name := range []string{"head", "merge_commit"} {
		if _, err := recoveryMapping(fields[name], "kind value path observed_at", "kind value"); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, name := range []string{"tasks", "policy_snapshots"} {
		if fields[name].Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence", name)
		}
		for _, item := range fields[name].Content {
			allowed, required, revision := "id path state", "id path state", "state"
			if name == "policy_snapshots" {
				allowed, required, revision = "node_id path source observations", "path source", "source"
			}
			values, err := recoveryMapping(item, allowed, required)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if _, err := recoveryMapping(values[revision], "kind value path observed_at", "kind value path"); err != nil {
				return fmt.Errorf("%s %s: %w", name, revision, err)
			}
			if observations, exists := values["observations"]; exists {
				if observations.Kind != yaml.SequenceNode {
					return fmt.Errorf("observations must be a sequence")
				}
				for _, observation := range observations.Content {
					if _, err := recoveryMapping(observation, "gate status unresolved_criteria evidence_refs", "gate status"); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func recoveryMapping(node *yaml.Node, allowed, required string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag != "!!str" || !strings.Contains(" "+allowed+" ", " "+key.Value+" ") || fields[key.Value] != nil {
			return nil, fmt.Errorf("unknown or duplicate field %q", key.Value)
		}
		if value.Tag == "!!null" || value.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("%s must not be null or an alias", key.Value)
		}
		switch key.Value {
		case "branch", "integration_branch", "basis", "recovered_by", "reason", "kind", "value", "path", "id", "node_id", "gate", "status":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return nil, fmt.Errorf("%s must be a string", key.Value)
			}
		case "unresolved_criteria", "evidence_refs":
			if value.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("%s must be a sequence", key.Value)
			}
			for _, item := range value.Content {
				if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
					return nil, fmt.Errorf("%s must contain strings", key.Value)
				}
			}
		}
		fields[key.Value] = value
	}
	for _, key := range strings.Fields(required) {
		if fields[key] == nil {
			return nil, fmt.Errorf("missing required field %s", key)
		}
	}
	return fields, nil
}

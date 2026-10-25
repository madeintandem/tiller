package mutate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// OwnershipReview is the human-reviewed, snapshot-bound schema-6 conversion.
// Only ConfirmedOwner may be edited; every started unfinished execution unit
// must be explicitly confirmed, including assignments inherited in schema 5.
type OwnershipReview struct {
	SourceSchema int                   `json:"source_schema" yaml:"source_schema"`
	TargetSchema int                   `json:"target_schema" yaml:"target_schema"`
	Snapshot     string                `json:"snapshot" yaml:"snapshot"`
	Sources      []OwnershipSource     `json:"sources" yaml:"sources"`
	Active       []OwnershipAssignment `json:"active" yaml:"active"`
}

type OwnershipSource struct {
	Path   string `json:"path" yaml:"path"`
	SHA256 string `json:"sha256" yaml:"sha256"`
}

type OwnershipAssignment struct {
	ID             string `json:"id" yaml:"id"`
	Noun           string `json:"noun" yaml:"noun"`
	Ref            string `json:"ref" yaml:"ref"`
	ObservedOwner  string `json:"observed_owner" yaml:"observed_owner"`
	ConfirmedOwner string `json:"confirmed_owner" yaml:"confirmed_owner"`
}

type ownershipInput struct {
	path                                      string
	data                                      []byte
	snapshot                                  store.FileSnapshot
	doc                                       yaml.Node
	noun                                      string
	id, key, status, owner, branch, doneBasis string
	started                                   bool
	pr                                        *int
	review                                    *store.ReviewRef
}

type ownershipConversion struct {
	review       *OwnershipReview
	writes       []store.PlannedWrite
	changed      []string
	blockers     []MigrationBlocker
	notes        []string
	legacyOwners map[string]string
	corrections  []OwnershipAssignment
}

func ownershipReviewError(reason, detail string) error {
	return &store.EnvError{Code: 3, Reason: reason, Msg: detail + "; run `tiller migrate --dry-run --json`, save the report outside .tiller/, have a human confirm every active owner, then run `tiller migrate --ownership-review <file>` or `tiller upgrade --project-only --ownership-review <file>`"}
}

// RequireOwnershipReview validates offline approval without writing. Maintenance
// calls this during candidate preflight, before replacing any executable.
func RequireOwnershipReview(expected, reviewed *OwnershipReview, manifest *store.Manifest) error {
	if expected == nil {
		return nil
	}
	if reviewed == nil {
		if len(expected.Active) == 0 {
			return nil
		}
		return ownershipReviewError(store.ReasonOwnershipReviewRequired, "started unfinished work requires explicit ownership review before schema 6")
	}
	if expected.SourceSchema != reviewed.SourceSchema || expected.TargetSchema != reviewed.TargetSchema || expected.Snapshot != reviewed.Snapshot || !reflect.DeepEqual(expected.Sources, reviewed.Sources) {
		return ownershipReviewError(store.ReasonOwnershipReviewInvalid, "ownership review does not match the manifest, roster, node inventory, or source snapshots")
	}
	if len(expected.Active) != len(reviewed.Active) {
		return ownershipReviewError(store.ReasonOwnershipReviewInvalid, "ownership review must include every active started execution ID exactly once")
	}
	owners := map[string]bool{}
	for _, owner := range manifest.Contributors {
		owners[owner.ID] = true
	}
	byID := map[string]OwnershipAssignment{}
	for _, assignment := range reviewed.Active {
		if _, duplicate := byID[assignment.ID]; duplicate {
			return ownershipReviewError(store.ReasonOwnershipReviewInvalid, "duplicate execution ID "+assignment.ID)
		}
		byID[assignment.ID] = assignment
	}
	for _, expectedAssignment := range expected.Active {
		actual, exists := byID[expectedAssignment.ID]
		if !exists || actual.Noun != expectedAssignment.Noun || actual.Ref != expectedAssignment.Ref || actual.ObservedOwner != expectedAssignment.ObservedOwner || !owners[actual.ConfirmedOwner] {
			return ownershipReviewError(store.ReasonOwnershipReviewInvalid, "missing, conflicting, or unregistered confirmed ownership for "+expectedAssignment.ID)
		}
	}
	return nil
}

func hashOwnershipBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// Inventory all relevant directories, not only loaded nodes. This refuses
// symlinks/nonregular entries and guards new state and optional event files.
func ownershipInputs(root string) ([]ownershipInput, []store.PlannedWrite, []OwnershipSource, error) {
	var inputs []ownershipInput
	var guards []store.PlannedWrite
	sources := []OwnershipSource{}
	var walk func(string, bool) error
	walk = func(path string, eventDir bool) error {
		if _, err := store.PathExistsNoSymlinks(root, path); err != nil {
			return err
		}
		snapshot, err := store.ReadDirectorySnapshot(root, path)
		if err != nil {
			return err
		}
		guards = append(guards, store.PlanDirectoryGuard(path, snapshot))
		entries, err := readMutationDirectory(root, path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("unsafe migration input %s: symlinks are not supported", child)
			}
			if entry.IsDir() {
				if err := walk(child, eventDir); err != nil {
					return err
				}
				continue
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("unsafe migration input %s: expected regular file", child)
			}
			rel, err := filepath.Rel(root, child)
			if err != nil {
				return err
			}
			noun := migrationStateNoun(filepath.ToSlash(rel))
			if noun == "" && !strings.HasSuffix(entry.Name(), ".yml") {
				continue
			}
			if noun == "" {
				noun = "opaque"
			}
			data, fileSnapshot, err := store.ReadRegularFileSnapshot(root, child)
			if err != nil {
				return err
			}
			if !fileSnapshot.Exists() {
				return fmt.Errorf("migration input disappeared: %s", child)
			}
			if eventDir {
				noun = "event"
			}
			sources = append(sources, OwnershipSource{filepath.ToSlash(rel), hashOwnershipBytes(data)})
			inputs = append(inputs, ownershipInput{path: child, data: data, snapshot: fileSnapshot, noun: noun})
		}
		return nil
	}
	dirs := append(append([]string{"epics"}, shelfDirs()...), "events", "decisions", "amendments")
	for _, dir := range dirs {
		if err := walk(filepath.Join(root, ".tiller", dir), dir == "events"); err != nil {
			return nil, nil, nil, err
		}
	}
	return inputs, guards, sources, nil
}

func migrationStateNoun(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 4 && parts[1] == "epics" && parts[3] == "epic.yml" {
		return store.NounEpic
	}
	if len(parts) == 6 && parts[1] == "epics" && parts[3] == "features" && parts[5] == "feature.yml" {
		return store.NounFeature
	}
	if len(parts) == 8 && parts[1] == "epics" && parts[3] == "features" && parts[5] == "tasks" && parts[7] == "task.yml" {
		return store.NounTask
	}
	if len(parts) == 4 {
		for _, noun := range store.ShelfNouns() {
			if parts[1] == store.ShelfDir(noun) && parts[3] == store.ShelfStateFile(noun) {
				return noun
			}
		}
	}
	return ""
}

func planOwnershipMigration(root string, manifest *store.Manifest, manifestData []byte, reviewed *OwnershipReview, requireReview bool) (ownershipConversion, error) {
	out := ownershipConversion{legacyOwners: map[string]string{}}
	inputs, guards, sources, err := ownershipInputs(root)
	if err != nil {
		return out, err
	}
	out.writes = guards
	sources = append(sources, OwnershipSource{".tiller/tiller.yml", hashOwnershipBytes(manifestData)})
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	// Optional history enriches start inference only. A malformed event remains
	// byte-for-byte untouched and does not invalidate otherwise safe live state.
	starts := map[string]bool{}
	for _, input := range inputs {
		if input.noun != "event" {
			continue
		}
		var event events.Event
		if yaml.Unmarshal(input.data, &event) == nil && (event.Type == "start.task" || event.Type == "start.bug" || event.Type == "start.chore") {
			for _, id := range event.Targets {
				starts[id] = true
			}
		}
	}
	report := &OwnershipReview{SourceSchema: manifest.SchemaVersion, TargetSchema: store.CurrentSchemaVersion, Sources: sources, Active: []OwnershipAssignment{}}
	encodedSources, _ := json.Marshal(sources)
	report.Snapshot = hashOwnershipBytes(encodedSources)
	out.review = report
	readyParents := map[string]bool{}
	for i := range inputs {
		input := &inputs[i]
		if input.noun == "event" || input.noun == "opaque" {
			continue
		}
		var state struct {
			ID        string           `yaml:"id"`
			Key       string           `yaml:"key"`
			Status    string           `yaml:"status"`
			Owner     string           `yaml:"owner"`
			Branch    string           `yaml:"branch"`
			DoneBasis string           `yaml:"done_basis"`
			Started   bool             `yaml:"started"`
			PR        *int             `yaml:"pr"`
			ReviewRef *store.ReviewRef `yaml:"review_ref"`
		}
		if err := yaml.Unmarshal(input.data, &state); err != nil {
			return out, fmt.Errorf("parse %s: %w", input.path, err)
		}
		if err := yaml.Unmarshal(input.data, &input.doc); err != nil {
			return out, err
		}
		if len(input.doc.Content) != 1 || input.doc.Content[0].Kind != yaml.MappingNode {
			return out, fmt.Errorf("parse %s: expected state mapping", input.path)
		}
		input.id, input.key, input.status, input.owner, input.branch, input.doneBasis = state.ID, state.Key, state.Status, state.Owner, state.Branch, state.DoneBasis
		input.pr, input.review = state.PR, state.ReviewRef
		terminal, drainBlocked := migrationNodeState(input.noun, state.Status, state.PR, state.ReviewRef)
		if manifest.SchemaVersion < 5 && !store.IsWorkNoun(input.noun) && drainBlocked {
			rel, _ := filepath.Rel(root, input.path)
			out.blockers = append(out.blockers, MigrationBlocker{Noun: input.noun, ID: state.ID, Ref: state.Key, Path: filepath.ToSlash(rel), Status: state.Status, Branch: state.Branch, PR: state.PR, ReviewRef: state.ReviewRef})
		}
		if input.noun == store.NounEpic || input.noun == store.NounFeature {
			if state.Owner != "" {
				out.legacyOwners[state.ID] = state.Owner
			}
			continue
		}
		switch state.Status {
		case "proposed", "ready", "in_progress", "check", "in_review", "done", "cancelled":
		default:
			return out, fmt.Errorf("unsafe live execution state %s: unknown status %q", input.path, state.Status)
		}
		input.started = state.Started || starts[state.ID] || state.Branch != "" || state.PR != nil || state.ReviewRef != nil || state.Status == "in_progress" || state.Status == "check" || state.Status == "in_review" || (state.Status == "done" && (input.noun == store.NounTask || state.DoneBasis != "carried"))
		if input.started && (state.Status == "proposed" || state.Status == "ready") {
			return out, fmt.Errorf("unsafe execution state %s: pre-start status %q has reliable start evidence; reconcile the lifecycle with a compatible binary before migration", input.path, state.Status)
		}
		if input.started && !terminal {
			report.Active = append(report.Active, OwnershipAssignment{ID: state.ID, Noun: input.noun, Ref: state.Key, ObservedOwner: state.Owner})
		}
		if input.noun == store.NounTask && input.started {
			readyParents[filepath.Dir(filepath.Dir(filepath.Dir(input.path)))] = true
		}
	}
	sort.Slice(report.Active, func(i, j int) bool { return report.Active[i].ID < report.Active[j].ID })
	if len(out.blockers) == 0 && (reviewed != nil || requireReview) {
		if err := RequireOwnershipReview(report, reviewed, manifest); err != nil {
			return out, err
		}
	}
	confirmed := map[string]string{}
	if reviewed != nil {
		for _, assignment := range reviewed.Active {
			confirmed[assignment.ID] = assignment.ConfirmedOwner
		}
	}
	for _, input := range inputs {
		if input.noun == "event" || input.noun == "opaque" {
			out.writes = append(out.writes, store.PlanReadGuard(input.path, input.snapshot))
			continue
		}
		mapping := input.doc.Content[0]
		changed := false
		if input.noun == store.NounEpic || input.noun == store.NounFeature {
			changed = removeMigrationField(mapping, "owner")
			terminal, blocked := migrationNodeState(input.noun, input.status, input.pr, input.review)
			if manifest.SchemaVersion < 5 && !terminal && !blocked {
				changed = removeMigrationField(mapping, "branch") || changed
			}
			if input.noun == store.NounFeature && input.status == "ready" && readyParents[filepath.Dir(input.path)] {
				setMigrationField(mapping, "status", "!!str", "active")
				changed = true
				out.notes = append(out.notes, fmt.Sprintf("reconcile feature %s (%s) from ready to active because a descendant task has started", input.key, input.id))
			}
		} else {
			setMigrationField(mapping, "started", "!!bool", fmt.Sprint(input.started))
			changed = true
			if owner, ok := confirmed[input.id]; ok {
				setMigrationField(mapping, "owner", "!!str", owner)
				if owner != input.owner {
					out.corrections = append(out.corrections, OwnershipAssignment{ID: input.id, Noun: input.noun, Ref: input.key, ObservedOwner: input.owner, ConfirmedOwner: owner})
				}
			}
		}
		if !changed {
			out.writes = append(out.writes, store.PlanReadGuard(input.path, input.snapshot))
			continue
		}
		rendered, err := yaml.Marshal(&input.doc)
		if err != nil {
			return out, err
		}
		out.writes = append(out.writes, store.PlanWrite(input.path, input.snapshot, rendered))
		rel, _ := filepath.Rel(root, input.path)
		out.changed = append(out.changed, filepath.ToSlash(rel))
	}
	return out, nil
}

func removeMigrationField(mapping *yaml.Node, key string) bool {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			preserveMigrationAliases(mapping, mapping.Content[i+1])
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}

func setMigrationField(mapping *yaml.Node, key, tag, value string) {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			preserveMigrationAliases(mapping, mapping.Content[i+1])
			mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
}

// Unknown historical fields may alias a scalar we remove or correct. Expand
// those references to their original value, not the new ownership assignment.
func preserveMigrationAliases(mapping, target *yaml.Node) {
	if target.Anchor == "" {
		return
	}
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.AliasNode && node.Alias == target {
			*node = *target
			node.Anchor = ""
			return
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(mapping)
}

// Remove only the retired manifest block, retaining all unrelated bytes,
// comments, and unknown human-owned configuration.
func renderOwnershipManifest(data []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	mapping := doc.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != "claiming" {
			continue
		}
		if mapping.Style&yaml.FlowStyle != 0 {
			return nil, fmt.Errorf("rewrite the manifest as block-form YAML before migrating obsolete claiming configuration")
		}
		lines := bytes.SplitAfter(data, []byte("\n"))
		start := mapping.Content[i].Line - 1
		end := len(lines)
		if i+2 < len(mapping.Content) {
			end = mapping.Content[i+2].Line - 1
		}
		for end > start+1 {
			line := bytes.TrimSpace(lines[end-1])
			if len(line) != 0 && !bytes.HasPrefix(line, []byte("#")) {
				break
			}
			end--
		}
		data = bytes.Join(append(lines[:start:start], lines[end:]...), nil)
		break
	}
	return store.RenderSchemaVersion(data, 6)
}

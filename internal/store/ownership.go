package store

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

func rejectRetiredOwnership(path string, data []byte, field string) error {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if _, exists := fields[field]; exists {
		return &EnvError{Code: 3, Reason: "obsolete_ownership", Msg: fmt.Sprintf("%s contains obsolete %s in schema 6; migrate the original legacy snapshot with ownership review, or remove this obsolete field after reviewing the schema-6 state", path, field)}
	}
	return nil
}

func (t *Tree) readNodeYAML(path string, out any) error {
	data, err := t.stateFile(path)
	if err != nil {
		return err
	}
	if t.Manifest.SchemaVersion >= 6 {
		if err := rejectRetiredOwnership(path, data, "owner"); err != nil {
			return err
		}
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// ProjectContributors builds the shared read projection. Load calls it once;
// callers constructing an in-memory tree may call it after filling that tree.
func (t *Tree) ProjectContributors() {
	t.Contributors = make(map[string][]Contributor, len(t.Epics)+len(t.Features))
	names := map[string]string{}
	if t.Manifest != nil {
		for _, owner := range t.Manifest.Contributors {
			names[owner.ID] = owner.Name
		}
	}
	epics := make(map[string]string, len(t.Epics))
	sets := make(map[string]map[string]bool, len(t.Epics)+len(t.Features))
	for id, epic := range t.Epics {
		sets[id] = map[string]bool{}
		if epic.Key != nil {
			epics[*epic.Key] = id
		}
	}
	for id := range t.Features {
		sets[id] = map[string]bool{}
	}
	for _, task := range t.Tasks {
		if !task.Started || task.Owner == "" {
			continue
		}
		feature := t.Features[task.FeatureID]
		if feature == nil {
			continue
		}
		sets[feature.ID][task.Owner] = true
		if epicID, ok := epics[feature.EpicKey]; ok {
			sets[epicID][task.Owner] = true
		}
	}
	for id, set := range sets {
		owners := make([]Contributor, 0, len(set))
		for ownerID := range set {
			name := names[ownerID]
			if name == "" {
				name = ownerID
			}
			owners = append(owners, Contributor{ID: ownerID, Name: name})
		}
		sort.Slice(owners, func(i, j int) bool { return owners[i].ID < owners[j].ID })
		t.Contributors[id] = owners
	}
}

func validateExecutionOwnership(id, status, owner, branch, doneBasis string, started bool, pr *int, review *ReviewRef) error {
	active := status == "in_progress" || status == "check" || status == "in_review"
	terminal := status == "done" || status == "cancelled"
	if (active || branch != "" || pr != nil || review != nil || (status == "done" && doneBasis != "carried")) && !started {
		return fmt.Errorf("%s has execution history without started: true; restore the durable start marker or migrate the original legacy snapshot", id)
	}
	if started && !terminal && owner == "" {
		return fmt.Errorf("%s is started but has no owner; restore its reviewed execution owner", id)
	}
	if started && (status == "ready" || status == "proposed") {
		return fmt.Errorf("%s is started but has pre-start status %q", id, status)
	}
	return nil
}

// ValidateOwnership checks the schema-6 execution and replacement invariants.
// Migration uses it on its planned snapshot before committing any conversion.
func (t *Tree) ValidateOwnership() error {
	type edge struct {
		noun   string
		before []string
		after  *string
	}
	lineage := map[string]edge{}
	for id, task := range t.Tasks {
		if err := validateExecutionOwnership(id, task.Status, task.Owner, task.Branch, "", task.Started, task.PR, task.ReviewRef); err != nil {
			return err
		}
		lineage[id] = edge{NounTask, task.Supersedes, task.SupersededBy}
	}
	for _, noun := range ShelfNouns() {
		for _, node := range t.WorkNodesOf(noun) {
			w := node.Body()
			if err := validateExecutionOwnership(w.ID, w.Status, w.Owner, w.Branch, w.DoneBasis, w.Started, w.PR, w.ReviewRef); err != nil {
				return err
			}
			if w.Resolution == ResolutionTransferred && (w.Status != "cancelled" || !w.Started || w.SupersededBy == nil || w.DuplicateOf != "") {
				return fmt.Errorf("%s has invalid transferred resolution: require started cancelled work and reciprocal replacement lineage, not duplicate_of", w.ID)
			}
			lineage[w.ID] = edge{noun, w.Supersedes, w.SupersededBy}
		}
	}
	for id, e := range lineage {
		seen := map[string]bool{}
		for _, previous := range e.before {
			p, ok := lineage[previous]
			if seen[previous] || !ok || p.noun != e.noun || p.after == nil || *p.after != id {
				return fmt.Errorf("%s has nonreciprocal, duplicate, or cross-noun supersedes edge to %s", id, previous)
			}
			seen[previous] = true
		}
		if e.after == nil {
			continue
		}
		next, ok := lineage[*e.after]
		if !ok || next.noun != e.noun {
			return fmt.Errorf("%s has missing or cross-noun successor %s", id, *e.after)
		}
		reciprocal := false
		for _, previous := range next.before {
			if previous == id {
				reciprocal = true
			}
		}
		if !reciprocal {
			return fmt.Errorf("%s has nonreciprocal successor %s", id, *e.after)
		}
	}
	states := make(map[string]uint8, len(lineage))
	var visit func(string) error
	visit = func(id string) error {
		if states[id] == 1 {
			return fmt.Errorf("%s has cyclic replacement lineage", id)
		}
		if states[id] == 2 {
			return nil
		}
		states[id] = 1
		if next := lineage[id].after; next != nil {
			if err := visit(*next); err != nil {
				return err
			}
		}
		states[id] = 2
		return nil
	}
	for id := range lineage {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

package mutate

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

type recoveryHistory struct {
	manifest       *store.Manifest
	manifestSource store.RevisionRef
	features       map[string]*store.Feature
	epics          map[string]*store.Epic
	tasks          map[string]*store.Task
	paths          map[string]string
	sources        map[string]store.RevisionRef
}

func readRecoveryHistory(rev vcs.HistoricalRevisions, head, root string) (*recoveryHistory, error) {
	h := &recoveryHistory{features: map[string]*store.Feature{}, epics: map[string]*store.Epic{}, tasks: map[string]*store.Task{}, paths: map[string]string{}, sources: map[string]store.RevisionRef{}}
	data, source, err := rev.ReadFileAt(head, ".tiller/tiller.yml")
	if err != nil {
		return nil, recoveryReadError("historical_policy_unavailable", ".tiller/tiller.yml", "fetch or restore the regular historical manifest", err)
	}
	// Historical schema 9 remains valid after additive adoption into schema 10.
	h.manifest, err = store.ParseManifestUnchecked(root, data)
	if err != nil {
		return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", err.Error())
	}
	h.manifestSource = source
	if h.manifest.SchemaVersion < 6 || h.manifest.SchemaVersion > store.CurrentSchemaVersion {
		return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", "restore a supported historical execution-ownership schema")
	}
	// Validate gate configuration explicitly: unchecked schema parsing is not a
	// declaration that a missing or malformed policy means all-clear.
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	gateNode, present := fields["gates"]
	if !present {
		return nil, recoveryRefusal("historical_policy_missing", ".tiller/tiller.yml", "restore the historical gates configuration; absence is not proof of no policy")
	}
	if err := validateHistoricalGateShape(&gateNode); err != nil {
		return nil, err
	}
	for name, spec := range h.manifest.Gates.Required {
		if strings.TrimSpace(name) == "" || (spec.Kind != store.GateKindCommand && spec.Kind != store.GateKindAttestation) || len(spec.On) == 0 {
			return nil, recoveryRefusal("historical_policy_invalid", name, "restore a valid required historical gate kind and node scope")
		}
		if spec.Kind == store.GateKindCommand && strings.TrimSpace(spec.Run) == "" {
			return nil, recoveryRefusal("historical_policy_invalid", name, "restore the required historical command")
		}
		for _, noun := range spec.On {
			if noun != store.NounFeature && noun != store.NounTask {
				return nil, recoveryRefusal("historical_policy_invalid", name, "restore supported historical feature/task gate scopes")
			}
		}
	}
	files, err := rev.FilesAt(head, ".tiller/epics")
	if err != nil {
		return nil, recoveryReadError("historical_state_unavailable", head, "fetch or restore regular historical state files", err)
	}
	featureDirs := map[string]string{}
	for _, path := range files {
		base := filepath.Base(path)
		if base != "epic.yml" && base != "feature.yml" {
			continue
		}
		data, source, err := rev.ReadFileAt(head, path)
		if err != nil {
			return nil, recoveryReadError("historical_state_unavailable", path, "fetch or restore the regular historical state blob", err)
		}
		var id string
		if base == "epic.yml" {
			var e store.Epic
			if err := yaml.Unmarshal(data, &e); err != nil {
				return nil, recoveryRefusal("historical_state_invalid", path, err.Error())
			}
			id = e.ID
			h.epics[id] = &e
			parts := strings.Split(filepath.ToSlash(path), "/")
			if len(parts) != 4 || deref(e.Key) != parts[2] {
				return nil, recoveryRefusal("historical_identity_conflict", path, "restore the historical epic's canonical key and layout")
			}
		} else {
			var f store.Feature
			if err := yaml.Unmarshal(data, &f); err != nil {
				return nil, recoveryRefusal("historical_state_invalid", path, err.Error())
			}
			id = f.ID
			parts := strings.Split(filepath.ToSlash(path), "/")
			if len(parts) != 6 || parts[3] != "features" || deref(f.Key) != parts[2]+"/"+parts[4] {
				return nil, recoveryRefusal("historical_path_invalid", path, "restore canonical historical feature identity and layout")
			}
			f.EpicKey = parts[2]
			h.features[id] = &f
			featureDirs[filepath.Dir(path)] = id
		}
		if id == "" || h.paths[id] != "" {
			return nil, recoveryRefusal("historical_identity_conflict", path, "restore unique nonblank historical node IDs")
		}
		h.paths[id] = path
		h.sources[id] = source
	}
	for _, path := range files {
		if filepath.Base(path) != store.TaskStateFile {
			continue
		}
		data, source, err := rev.ReadFileAt(head, path)
		if err != nil {
			return nil, recoveryReadError("historical_state_unavailable", path, "fetch or restore the regular historical task state blob", err)
		}
		var task store.Task
		if err := yaml.Unmarshal(data, &task); err != nil {
			return nil, recoveryRefusal("historical_state_invalid", path, err.Error())
		}
		parts := strings.Split(filepath.ToSlash(path), "/")
		if len(parts) != 8 || parts[3] != "features" || parts[5] != store.TaskDirName || deref(task.Key) != parts[2]+"/"+parts[4]+"/"+parts[6] {
			return nil, recoveryRefusal("historical_identity_conflict", path, "restore the historical task's canonical key and owner layout")
		}
		featureID := featureDirs[filepath.Dir(filepath.Dir(filepath.Dir(path)))]
		if featureID == "" {
			return nil, recoveryRefusal("historical_owner_missing", path, "restore the historical owning feature document")
		}
		task.FeatureID = featureID
		if task.ID == "" || h.paths[task.ID] != "" {
			return nil, recoveryRefusal("historical_identity_conflict", path, "restore unique historical task IDs")
		}
		h.paths[task.ID] = path
		h.sources[task.ID] = source
		h.tasks[task.ID] = &task
	}
	return h, nil
}

func (h *recoveryHistory) matchAggregate(tree *store.Tree, noun, id, path, branch string, requestedPR *int, requestedReview *store.ReviewRef, rev vcs.HistoricalRevisions) error {
	if h.paths[id] != path {
		return recoveryRefusal("historical_identity_conflict", path, "restore the matching historical aggregate ID and canonical path")
	}
	var historicalBranch, historicalKey string
	var grain string
	var historicalPR *int
	var historicalReview *store.ReviewRef
	if noun == store.NounFeature {
		f := h.features[id]
		if f == nil {
			return recoveryRefusal("historical_identity_conflict", path, "restore the historical feature identity")
		}
		e := (*store.Epic)(nil)
		for _, candidate := range h.epics {
			if deref(candidate.Key) == f.EpicKey {
				e = candidate
				break
			}
		}
		grain = store.ResolvedReviewGrain(h.manifest, e, f)
		historicalBranch = f.Branch
		historicalKey = deref(f.Key)
		historicalPR = f.PR
		historicalReview = f.ReviewRef
		if deref(tree.Features[id].Key) != historicalKey || e == nil || epicByKey(tree, tree.Features[id].EpicKey) == nil || epicByKey(tree, tree.Features[id].EpicKey).ID != e.ID {
			return recoveryRefusal("historical_owner_mismatch", path, "restore the aggregate's historical canonical key and parent identity")
		}
	} else {
		e := h.epics[id]
		if e == nil {
			return recoveryRefusal("historical_identity_conflict", path, "restore the historical epic identity")
		}
		grain = store.ResolvedReviewGrain(h.manifest, e, nil)
		historicalBranch = e.Branch
		historicalKey = deref(e.Key)
		historicalPR = e.PR
		historicalReview = e.ReviewRef
		if deref(tree.Epics[id].Key) != historicalKey {
			return recoveryRefusal("historical_owner_mismatch", path, "restore the aggregate's historical canonical key")
		}
	}
	if historicalBranch != branch || grain != noun || historicalKey == "" {
		return recoveryRefusal("historical_round_mismatch", branch, "restore the aggregate branch and review grain belonging to the historical reviewed round")
	}
	if historicalPR != nil && !samePR(historicalPR, requestedPR) {
		return recoveryRefusal("historical_review_conflict", branch, "retain the PR identity recorded in the historical review round")
	}
	if historicalReview != nil {
		historicalHead, err := rev.ResolveCommit(recoveryOID(historicalReview.Head))
		if err != nil {
			return recoveryReadError("object_unavailable", historicalReview.Head.Value, "restore the historical no-remote review identity", err)
		}
		if requestedReview == nil || historicalReview.Branch != requestedReview.Branch || historicalHead != recoveryOID(requestedReview.Head) {
			return recoveryRefusal("historical_review_conflict", branch, "preserve historical review identity rather than absorbing another round")
		}
	}
	return nil
}

func (h *recoveryHistory) bindTask(tree *store.Tree, rev vcs.Revisions, history vcs.HistoricalRevisions, head string, current *store.Task, refPins map[string]string) (store.RecoveredTask, error) {
	fail := func(reason, ref, remedy string) (store.RecoveredTask, error) {
		e := recoveryRefusal(reason, ref, remedy)
		e.TaskID = current.ID
		return store.RecoveredTask{}, e
	}
	historical := h.tasks[current.ID]
	path := filepath.Join(taskDir(tree, current), store.TaskStateFile)
	if historical == nil || h.paths[current.ID] != path {
		return fail("historical_rider_missing", path, "restore the submitted task.yml in the reviewed tree; implementation ancestry alone is insufficient")
	}
	if historical.Status != "in_review" || historical.PR != nil || !recoveredTaskIdentityMatches(current, historical) {
		return fail("historical_rider_mismatch", path, "restore the same owner, branch, immutable submission head, and approved task scope from this historical round")
	}
	if !validWorkingBranch(historical.Branch) || historical.ReviewRef.Branch != historical.Branch || historical.ReviewRef.Head.Kind != store.KindGitCommit {
		return fail("historical_rider_mismatch", path, "restore the historical task's recorded local branch and typed submission head")
	}
	hf := h.features[historical.FeatureID]
	he := (*store.Epic)(nil)
	for _, candidate := range h.epics {
		if deref(candidate.Key) == hf.EpicKey {
			he = candidate
			break
		}
	}
	cf := tree.Features[current.FeatureID]
	ce := epicByKey(tree, cf.EpicKey)
	if store.ResolvedReviewGrain(h.manifest, he, hf) != store.ResolvedReviewGrain(tree.Manifest, ce, cf) {
		return fail("historical_grain_mismatch", path, "retain the historical review-grain owner; independent review overrides cannot ride")
	}
	submitted, err := history.ResolveCommit(recoveryOID(historical.ReviewRef.Head))
	if err != nil {
		problem := recoveryReadError("object_unavailable", historical.ReviewRef.Head.Value, "fetch or restore the immutable task submission head", err)
		if refusal, ok := problem.(*RecoverReviewError); ok {
			refusal.TaskID = current.ID
		}
		return store.RecoveredTask{}, problem
	}
	included, err := rev.IsMergedInto(submitted, head)
	if err != nil {
		return store.RecoveredTask{}, err
	}
	if !included {
		return fail("submitted_head_not_included", submitted, "restore the historical aggregate tree containing this rider's submitted head")
	}
	specPath := filepath.Join(filepath.Dir(path), "task.md")
	_, specRef, err := history.ReadFileAt(head, specPath)
	if err != nil {
		problem := recoveryReadError("historical_scope_unavailable", specPath, "restore the regular approved task specification in the reviewed tree", err)
		if refusal, ok := problem.(*RecoverReviewError); ok {
			refusal.TaskID = current.ID
		}
		return store.RecoveredTask{}, problem
	}
	// Older bases may use abbreviated blob OIDs. Compare their immutable prefix
	// against the full historical blob, never against today's feature contract.
	accepted := historical.AcceptedAgainst.Spec
	if accepted.Kind != store.KindGitBlob || accepted.Path != specPath || !strings.HasPrefix(specRef.Value, accepted.Value) || !strings.HasPrefix(accepted.Value, "git:") || len(accepted.Value) < 11 {
		return fail("historical_scope_mismatch", specPath, "restore the historical task specification matching its accepted basis")
	}
	ref := branchRef(current.Branch)
	exists, err := rev.RefExists(ref)
	if err != nil {
		return store.RecoveredTask{}, err
	}
	if exists {
		tip, err := rev.MergeBase(ref, ref)
		if err != nil {
			return store.RecoveredTask{}, err
		}
		refPins[ref] = tip
		included, err := rev.IsMergedInto(tip, head)
		if err != nil {
			return store.RecoveredTask{}, err
		}
		if !included {
			return fail("retained_task_tip_not_included", ref, "integrate or preserve later task work separately; do not complete it using the historical review")
		}
	} else {
		refPins[ref] = ""
	}
	return store.RecoveredTask{ID: current.ID, Path: path, State: h.sources[current.ID]}, nil
}

func (h *recoveryHistory) policySnapshots(rev vcs.HistoricalRevisions, head string, features []*store.Feature, tasks []*store.Task) ([]store.HistoricalPolicySnapshot, error) {
	snapshots := []store.HistoricalPolicySnapshot{{Path: ".tiller/tiller.yml", Source: h.manifestSource}}
	contracts := make(map[string]*store.EvalFile, len(features))
	for _, current := range features {
		historical := h.features[current.ID]
		if historical == nil {
			return nil, recoveryRefusal("historical_policy_missing", current.ID, "restore the historical feature gate-state document")
		}
		evalPath := filepath.Join(filepath.Dir(h.paths[current.ID]), "feature.eval.md")
		data, source, err := rev.ReadFileAt(head, evalPath)
		if err != nil {
			return nil, recoveryReadError("historical_policy_unavailable", evalPath, "restore the regular historical acceptance contract", err)
		}
		contract, err := store.ParseEvalFile(data)
		if err != nil {
			return nil, recoveryRefusal("historical_policy_invalid", evalPath, err.Error())
		}
		if err := contract.ValidateContract(); err != nil {
			return nil, recoveryRefusal("historical_policy_invalid", evalPath, err.Error())
		}
		if contract.Node != current.ID {
			return nil, recoveryRefusal("historical_policy_invalid", evalPath, "restore the historical acceptance contract for the owning feature")
		}
		contracts[current.ID] = contract
		observations, err := historicalPolicyObservations(h.manifest, store.NounFeature, &historical.GatesBlock, contract, evalPath)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, store.HistoricalPolicySnapshot{NodeID: current.ID, Path: h.paths[current.ID], Source: h.sources[current.ID], Observations: observations}, store.HistoricalPolicySnapshot{NodeID: current.ID, Path: evalPath, Source: source})
	}
	for _, task := range tasks {
		historical := h.tasks[task.ID]
		evalPath := filepath.Join(filepath.Dir(h.paths[historical.FeatureID]), "feature.eval.md")
		observations, err := historicalPolicyObservations(h.manifest, store.NounTask, &historical.GatesBlock, contracts[historical.FeatureID], evalPath)
		if err != nil {
			if refusal, ok := err.(*RecoverReviewError); ok {
				refusal.TaskID = task.ID
			}
			return nil, err
		}
		snapshots = append(snapshots, store.HistoricalPolicySnapshot{NodeID: task.ID, Path: h.paths[task.ID], Source: h.sources[task.ID], Observations: observations})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].NodeID != snapshots[j].NodeID {
			return snapshots[i].NodeID < snapshots[j].NodeID
		}
		return snapshots[i].Path < snapshots[j].Path
	})
	return snapshots, nil
}

func historicalGateObservations(manifest *store.Manifest, noun string, block *store.GatesBlock) []store.HistoricalPolicyObservation {
	var observations []store.HistoricalPolicyObservation
	for _, config := range requiredGateConfigs(manifest, noun) {
		gate, found := block.Gates[config.Name]
		if found && gate.Status == store.GateStatusPass {
			continue
		}
		status := gate.Status
		if !found || status == "" {
			status = store.GateStatusPending
		}
		switch status {
		case store.GateStatusFail, store.GateStatusPending, store.GateStatusSkipped, store.GateStatusStale:
		default:
			status = "unknown"
		}
		var evidence []string
		for _, item := range gate.Evidence {
			evidence = append(evidence, item.Ref)
		}
		observations = append(observations, store.HistoricalPolicyObservation{Gate: config.Name, Status: status, EvidenceRefs: sortedRecoveryStrings(evidence)})
	}
	return observations
}

func sortedRecoveryStrings(values []string) []string {
	sort.Strings(values)
	var result []string
	for _, value := range values {
		if value != "" && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}

func historicalPolicyObservations(manifest *store.Manifest, noun string, block *store.GatesBlock, contract *store.EvalFile, evalPath string) ([]store.HistoricalPolicyObservation, error) {
	observations := historicalGateObservations(manifest, noun, block)
	config, required := manifest.Gates.Required["evals"]
	if !required || !onIncludes(config.On, noun) {
		return observations, nil
	}
	if contract == nil {
		return nil, recoveryRefusal("historical_policy_missing", evalPath, "restore the owning historical acceptance contract for the required evals gate")
	}
	gate := block.Gates["evals"]
	var unresolved, evidence []string
	for _, criterion := range contract.Criteria {
		if !criterion.Required {
			continue
		}
		state := gate.Criteria[criterion.ID]
		switch state.Status {
		case "", store.CriterionStatusPass, store.CriterionStatusFail, store.CriterionStatusPending, store.CriterionStatusNeedsAttestation, store.CriterionStatusStale:
		default:
			return nil, recoveryRefusal("historical_policy_invalid", evalPath, "restore the recorded historical criterion status for "+criterion.ID)
		}
		if state.Status != store.CriterionStatusPass {
			unresolved = append(unresolved, criterion.ID)
		}
		for _, item := range state.Evidence {
			evidence = append(evidence, item.Ref)
		}
	}
	found := false
	for i := range observations {
		if observations[i].Gate == "evals" {
			observations[i].UnresolvedCriteria = sortedRecoveryStrings(unresolved)
			observations[i].EvidenceRefs = sortedRecoveryStrings(append(observations[i].EvidenceRefs, evidence...))
			found = true
		}
	}
	if !found && len(unresolved) > 0 {
		observations = append(observations, store.HistoricalPolicyObservation{Gate: "evals", Status: store.GateStatusPending, UnresolvedCriteria: sortedRecoveryStrings(unresolved), EvidenceRefs: sortedRecoveryStrings(evidence)})
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].Gate < observations[j].Gate })
	return observations, nil
}

func validateHistoricalGateShape(node *yaml.Node) error {
	sections, err := historicalPolicyMapping(node, "required optional", "gates")
	if err != nil {
		return err
	}
	for _, section := range []string{"required", "optional"} {
		node, present := sections[section]
		if !present {
			continue
		}
		specs, err := historicalPolicyMapping(&node, "", "gates."+section)
		if err != nil {
			return err
		}
		for name, spec := range specs {
			if strings.TrimSpace(name) == "" {
				return recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", "restore nonblank historical gate names")
			}
			if _, err := historicalPolicyMapping(&spec, "kind run on", "gates."+section+"."+name); err != nil {
				return err
			}
		}
	}
	return nil
}

func historicalPolicyMapping(node *yaml.Node, allowed, context string) (map[string]yaml.Node, error) {
	for depth := 0; node != nil && node.Kind == yaml.AliasNode; depth++ {
		if depth == 32 {
			return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", "restore a finite historical "+context+" mapping")
		}
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", "restore historical "+context+" as an explicit policy mapping; null or missing policy is not all-clear")
	}
	var fields map[string]yaml.Node
	if err := node.Decode(&fields); err != nil {
		return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", err.Error())
	}
	for name := range fields {
		if allowed == "" {
			continue
		}
		recognized := false
		for _, field := range strings.Fields(allowed) {
			if name == field {
				recognized = true
				break
			}
		}
		if !recognized {
			return nil, recoveryRefusal("historical_policy_invalid", ".tiller/tiller.yml", "restore historical "+context+"; unsupported policy field "+name+" must not be silently discarded")
		}
	}
	return fields, nil
}

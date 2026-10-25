package mutate

// Schema migrations run as one guarded source transaction. Only schemas 2–4
// cross the shared-branch drain boundary; schema 5 supports live owner review.

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

// MigrateOutcome distinguishes why Migrate did or did not change the tree: a
// real stamp-and-commit, or a no-op because the tree already reads at
// store.CurrentSchemaVersion.
type MigrateOutcome string

const (
	MigrateOutcomeMigrated MigrateOutcome = "migrated"
	MigrateOutcomeCurrent  MigrateOutcome = "current"
)

// MigrateResult reports the observed source schema and transaction paths,
// including a newly minted event. Outcome is set only after success. On error,
// Files and Notes describe the attempted plan, not confirmed writes, and To
// retains From. A recovery-required error makes durability unknown; callers
// must not interpret To as a successful rollback.
type MigrateResult struct {
	From        int                `json:"from"`
	To          int                `json:"to"`
	Outcome     MigrateOutcome     `json:"outcome"`
	Files       []string           `json:"files,omitempty"`
	Notes       []string           `json:"notes,omitempty"`
	Blockers    []MigrationBlocker `json:"blockers,omitempty"`
	Contributor string             `json:"contributor,omitempty"`
}

// MigrationBlocker identifies unfinished legacy work without changing it.
type MigrationBlocker struct {
	Noun      string           `json:"noun"`
	ID        string           `json:"id"`
	Ref       string           `json:"ref"`
	Path      string           `json:"path"`
	Status    string           `json:"status"`
	Branch    string           `json:"branch,omitempty"`
	PR        *int             `json:"pr,omitempty"`
	ReviewRef *store.ReviewRef `json:"review_ref,omitempty"`
}

// MigrationInspection describes the source migration without creating an event,
// generation token, cache, or identity. To is the target, not an applied stamp.
// Files names existing transformation inputs; the eventual event path cannot be
// known until apply mints its identity.
type MigrationInspection struct {
	From            int                `json:"from"`
	To              int                `json:"to"`
	Files           []string           `json:"files,omitempty"`
	Notes           []string           `json:"notes,omitempty"`
	Blockers        []MigrationBlocker `json:"blockers,omitempty"`
	Contributor     string             `json:"contributor,omitempty"`
	OwnershipReview *OwnershipReview   `json:"ownership_review,omitempty"`
}

type migrationPlan struct {
	inspection MigrationInspection
	tree       *store.Tree
	writes     []store.PlannedWrite
	ownership  ownershipConversion
	gitInputs  []string
}

// schemaMigration carries one boundary: from's tree becomes from+1's.
type schemaMigration struct {
	from  int
	note  string
	apply func(manifestData []byte) ([]byte, error)
}

var schemaMigrations = []schemaMigration{
	{
		from:  2,
		note:  "stamp schema_version 3; Review Grain's fields are additive, no data transforms",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 3) },
	},
	{
		from:  3,
		note:  "stamp schema_version 4; the bugs/ and chores/ shelves are additive, no data transforms",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 4) },
	},
	{
		from:  4,
		note:  "stamp schema_version 5 after draining shared-branch execution and reviews; clear unsubmitted legacy ancestor branch fields, preserve terminal history and Git refs",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 5) },
	},
	{
		from:  5,
		note:  "stamp schema_version 6; freeze reviewed execution ownership, derive contributors, and retire ancestor claims",
		apply: renderOwnershipManifest,
	},
	{
		from:  6,
		note:  "stamp schema_version 7; reviewed feature moves are additive, preserve existing work and amendment history",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 7) },
	},
	{
		from:  7,
		note:  "stamp schema_version 8; enable the tweaks/ shelf without rewriting existing work",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 8) },
	},
	{
		from:  8,
		note:  "stamp schema_version 9; rename roster and local contributor identity, preserving work claims, history, branches, and epoch without drain",
		apply: store.RenderContributorMigration,
	},
	{
		from:  9,
		note:  "stamp schema_version 10; immutable aggregate recovery and historical policy provenance are additive; preserve work, events, and active reviews without drain",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 10) },
	},
	{
		from:  10,
		note:  "stamp schema_version 11; task started_grain is additive; preserve work, events, and active reviews without drain",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 11) },
	},
	{
		from:  11,
		note:  "stamp schema_version 12; question cancel_reason is additive; preserve work, events, and active reviews without drain",
		apply: func(d []byte) ([]byte, error) { return store.RenderSchemaVersion(d, 12) },
	},
}

// Migrate brings root's .tiller/tiller.yml forward from its stamped
// schema_version to store.CurrentSchemaVersion, one registered migration at a
// time, and commits the result as a single mutation with one migrate.schema
// event. A tree already at CurrentSchemaVersion is a no-op: MigrateResult's
// Outcome reports which case ran. A tree stamped below
// store.MinMigratableSchemaVersion or above store.CurrentSchemaVersion
// refuses with an *store.EnvError naming the actual remedy (re-init or
// upgrade tiller); like every other mutation, this one attributes to an
// contributor, so unset or unknown deps.Contributor refuses like any other
// mutation does.
func Migrate(root string, deps Deps) (MigrateResult, error) {
	return MigrateWithCommand(root, deps, "migrate")
}

// InspectMigration uses exactly the planner that apply reruns. Inspection is
// not a reservation: file, lineage, generation, and Git guards still apply.
func InspectMigration(root string, deps Deps, reviewed ...*OwnershipReview) (MigrationInspection, error) {
	plan, err := planMigration(root, deps, firstOwnershipReview(reviewed), false)
	return plan.inspection, err
}

// MigrateWithCommand preserves the actual invoking maintenance command in the
// event and transaction provenance.
func MigrateWithCommand(root string, deps Deps, command string, reviewed ...*OwnershipReview) (MigrateResult, error) {
	plan, err := planMigration(root, deps, firstOwnershipReview(reviewed), true)
	inspection := plan.inspection
	result := MigrateResult{From: inspection.From, To: inspection.From,
		Files: inspection.Files, Notes: inspection.Notes, Blockers: inspection.Blockers, Contributor: inspection.Contributor}
	if err != nil {
		return result, err
	}
	if inspection.From == inspection.To {
		result.Outcome = MigrateOutcomeCurrent
		return result, nil
	}

	ev := events.NewEvent(deps.clock(), "migrate.schema", inspection.Contributor, command)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"schema_version": inspection.From}
	ev.After = map[string]any{"schema_version": inspection.To}
	if len(plan.ownership.legacyOwners) != 0 {
		ev.Before["ancestor_owners"] = plan.ownership.legacyOwners
	}
	if len(plan.ownership.corrections) != 0 {
		ev.After["owner_corrections"] = plan.ownership.corrections
	}
	if plan.ownership.review != nil && len(plan.ownership.review.Active) != 0 {
		ev.After["ownership_review"] = firstOwnershipReview(reviewed)
	}
	if len(plan.ownership.notes) != 0 {
		ev.After["reconciliations"] = plan.ownership.notes
	}
	for _, path := range inspection.Files {
		ev.Files = append(ev.Files, events.FileChange{Path: path})
	}
	appended, err := eventWrites(plan.tree, []events.Event{ev})
	if err != nil {
		return result, err
	}
	result.Files = append([]string(nil), inspection.Files...)
	for _, write := range appended {
		rel, err := filepath.Rel(root, write.Path)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, filepath.ToSlash(rel))
		for _, guarded := range plan.writes {
			if guarded.Path == write.Path {
				return result, &store.EnvError{Code: 3, Reason: store.ReasonMutationStale,
					Msg: fmt.Sprintf("migration event %s already exists; historical events were not overwritten; reload and retry", filepath.ToSlash(rel))}
			}
		}
	}
	// Newly minted event paths have absence guards, never committed-baseline
	// requirements. Recheck only the existing transformation inputs.
	git := vcs.New(root)
	if err := plan.tree.CommitChecked(command, append(plan.writes, appended...), func() error {
		return git.CheckMigrationInputs(plan.gitInputs)
	}); err != nil {
		return result, err
	}
	result.To = inspection.To
	result.Outcome = MigrateOutcomeMigrated
	return result, nil
}

func planMigration(root string, deps Deps, reviewed *OwnershipReview, applying bool) (migrationPlan, error) {
	plan := migrationPlan{}
	manifestRel := ".tiller/tiller.yml"
	manifestPath := filepath.Join(root, manifestRel)
	data, snapshot, err := store.ReadRegularFileSnapshot(root, manifestPath)
	if err != nil {
		return plan, err
	}
	if !snapshot.Exists() {
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonNotARepo,
			Msg: fmt.Sprintf("%s does not exist; run tiller from a Tiller project", manifestPath)}
	}
	version, refusal := store.SchemaVersionOf(manifestPath, data)
	if refusal != nil {
		return plan, refusal
	}
	plan.inspection = MigrationInspection{From: version, To: store.CurrentSchemaVersion}
	switch {
	case version == store.CurrentSchemaVersion:
		_, err := store.ParseManifest(root, data)
		return plan, err
	case version > store.CurrentSchemaVersion:
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonSchemaVersionUnsupported,
			Msg: fmt.Sprintf("%s is stamped schema_version %d; this tiller writes %d: the tree already moved past this binary, so upgrade tiller rather than migrate", manifestPath, version, store.CurrentSchemaVersion)}
	case version < store.MinMigratableSchemaVersion:
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonSchemaVersionUnsupported,
			Msg: fmt.Sprintf("%s is stamped schema_version %d, below the migratable floor (%d): remove .tiller/ and re-run `tiller init`, the settled remedy for the pre-migration boundary", manifestPath, version, store.MinMigratableSchemaVersion)}
	}

	manifest, err := store.ParseManifestUnchecked(root, data)
	if err != nil {
		return plan, err
	}
	plan.tree, err = store.UncheckedTree(root, manifest)
	if err != nil {
		return plan, err
	}
	if err := refuseReservedEpicKeys(plan.tree, version); err != nil {
		return plan, err
	}
	plan.inspection.Files = []string{manifestRel}
	rendered := data
	for v := version; v < store.CurrentSchemaVersion; v++ {
		mig, ok := migrationFrom(v)
		if !ok {
			return plan, &store.EnvError{Code: 3, Reason: store.ReasonSchemaVersionUnsupported,
				Msg: fmt.Sprintf("no migration registered from schema_version %d; this binary cannot carry the tree forward", v)}
		}
		if rendered, err = mig.apply(rendered); err != nil {
			return plan, err
		}
		plan.inspection.Notes = append(plan.inspection.Notes, mig.note)
	}
	var conversion ownershipConversion
	if version < 6 {
		conversion, err = planOwnershipMigration(root, manifest, data, reviewed, applying)
	}
	if _, err := store.ParseManifest(root, rendered); err != nil {
		return plan, err
	}
	plan.ownership = conversion
	plan.inspection.OwnershipReview = conversion.review
	plan.inspection.Files = append(plan.inspection.Files, conversion.changed...)
	plan.inspection.Notes = append(plan.inspection.Notes, conversion.notes...)
	plan.inspection.Blockers = conversion.blockers
	if err != nil {
		return plan, err
	}
	plan.writes = append(conversion.writes, store.PlanWrite(manifestPath, snapshot, rendered))
	plan.gitInputs = append([]string(nil), plan.inspection.Files...)
	identityRel := ".tiller/identity.yml"
	identityPath := filepath.Join(root, identityRel)
	identityData, identitySnapshot, identityErr := store.ReadRegularFileSnapshot(root, identityPath)
	if identityErr != nil {
		return plan, identityErr
	}
	if identitySnapshot.Exists() {
		identityRendered, err := store.RenderLocalIdentityMigration(identityData)
		if err != nil {
			return plan, fmt.Errorf("migrate %s: %w", identityRel, err)
		}
		if string(identityRendered) != string(identityData) {
			plan.writes = append(plan.writes, store.PlanWrite(identityPath, identitySnapshot, identityRendered))
			plan.inspection.Files = append(plan.inspection.Files, identityRel)
		} else {
			plan.writes = append(plan.writes, store.PlanReadGuard(identityPath, identitySnapshot))
		}
	} else {
		plan.writes = append(plan.writes, store.PlanReadGuard(identityPath, identitySnapshot))
	}
	if err := plan.tree.CheckStable(); err != nil {
		return plan, err
	}
	if len(conversion.blockers) > 0 {
		refs := make([]string, 0, len(conversion.blockers))
		for _, blocker := range conversion.blockers {
			refs = append(refs, fmt.Sprintf("%s (%s, status %q)", blocker.Ref, blocker.Path, blocker.Status))
		}
		return plan, &store.EnvError{Code: 3, Reason: store.ReasonMigrationWorkInFlight,
			Msg: fmt.Sprintf("cannot migrate to schema_version 5: unfinished shared-branch work: %s; use a compatible older tiller binary to finish the authorized work and run `tiller sync` before retrying maintenance; cancellation is a separate human decision; no files or Git refs were changed", strings.Join(refs, "; "))}
	}
	for _, write := range conversion.writes {
		if len(write.Data) == 0 {
			continue
		}
		var identity struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(write.Data, &identity); err != nil {
			return plan, err
		}
		if task := plan.tree.Tasks[identity.ID]; task != nil {
			if err := yaml.Unmarshal(write.Data, task); err != nil {
				return plan, err
			}
		} else if node, ok := plan.tree.WorkNode(identity.ID); ok {
			if err := yaml.Unmarshal(write.Data, node); err != nil {
				return plan, err
			}
		}
	}
	if applying || reviewed != nil || conversion.review == nil || len(conversion.review.Active) == 0 {
		if err := plan.tree.ValidateOwnership(); err != nil {
			return plan, err
		}
	}
	if applying || deps.Contributor != "" {
		owner, err := store.ResolveContributor(manifest, deps.Contributor)
		if err != nil {
			return plan, err
		}
		plan.inspection.Contributor = owner.ID
	}
	if err := vcs.New(root).CheckMigrationInputs(plan.gitInputs); err != nil {
		return plan, err
	}
	return plan, nil
}

// refuseReservedEpicKeys stops a migration that would carry an epic keyed by
// a shelf folder (bugs, chores, tweaks) into a schema that reserves it: its
// feature refs would read as shelf refs and collide with shelf keys. An older
// binary could create one before its shelf existed. Only that binary can
// still load the tree to rename the epic, and only while it is draft.
func refuseReservedEpicKeys(tree *store.Tree, version int) error {
	ids := make([]string, 0, len(tree.Epics))
	for id := range tree.Epics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		epic := tree.Epics[id]
		key := deref(epic.Key)
		if !store.IsReservedEpicKey(key) {
			continue
		}
		remedy := fmt.Sprintf("with an older tiller binary that still reads schema_version %d, run `tiller rename epic %s --key <new-key>`, commit, then retry the migration", version, key)
		if epic.Status != statusDraft {
			remedy = fmt.Sprintf("the epic is %s and no tiller command can rename an epic past draft, so keep this project on a tiller binary that reads schema_version %d", epic.Status, version)
		}
		return &store.EnvError{Code: 3, Reason: store.ReasonMigrationReservedEpicKey,
			Msg: fmt.Sprintf("cannot migrate: epic %s (%s) uses the reserved shelf folder name %q, which schema_version %d gives to the .tiller/%s/ shelf; %s; no files were changed", key, id, key, store.CurrentSchemaVersion, key, remedy)}
	}
	return nil
}

func migrationFrom(v int) (schemaMigration, bool) {
	for _, m := range schemaMigrations {
		if m.from == v {
			return m, true
		}
	}
	return schemaMigration{}, false
}

func migrationNodeState(noun, status string, pr *int, review *store.ReviewRef) (terminal, blocked bool) {
	terminal = status == statusDone || status == statusCancelled || (noun == store.NounEpic && status == statusArchived)
	// Features remain active between tasks. The migration inventories every
	// task and ancestor review separately before allowing any source writes.
	active := status == "in_progress" || status == "check" || status == "in_review" || (status == "active" && noun != store.NounFeature)
	return terminal, !terminal && (active || pr != nil || review != nil)
}

// MigrationExecutionWarnings diagnoses predictable task-start collisions using
// the normal branch guard and local refs only. It never fetches or changes refs.
func MigrationExecutionWarnings(root string) ([]string, error) {
	data, _, err := store.ReadRegularFileSnapshot(root, filepath.Join(root, ".tiller", "tiller.yml"))
	if err != nil {
		return nil, err
	}
	manifest, err := store.ParseManifestUnchecked(root, data)
	if err != nil {
		return nil, err
	}
	tree, err := store.UncheckedTree(root, manifest)
	if err != nil {
		return nil, err
	}
	// Inspect the post-migration namespace: unsubmitted legacy ancestor branch
	// fields disappear, while their Git refs and terminal review history remain.
	if manifest.SchemaVersion < 5 {
		for _, epic := range tree.Epics {
			terminal, blocked := migrationNodeState(store.NounEpic, epic.Status, epic.PR, epic.ReviewRef)
			if !terminal && !blocked {
				epic.Branch = ""
			}
		}
		for _, feature := range tree.Features {
			terminal, blocked := migrationNodeState(store.NounFeature, feature.Status, feature.PR, feature.ReviewRef)
			if !terminal && !blocked {
				feature.Branch = ""
			}
		}
	}
	ids := make([]string, 0, len(tree.Tasks))
	for id := range tree.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var warnings []string
	git := vcs.New(root)
	for _, id := range ids {
		task := tree.Tasks[id]
		if task.Branch != "" || task.Key == nil || (task.Status != "ready" && task.Status != "proposed") {
			continue
		}
		if err := guardNewBranch(tree, git, store.NounTask, id, branchFor(manifest, *task.Key)); err != nil {
			var unsafe *BranchSafetyError
			if !errors.As(err, &unsafe) {
				return warnings, err
			}
			warnings = append(warnings, fmt.Sprintf("task %s (%s): %s; only local refs were inspected", *task.Key, id, unsafe.Error()))
		}
	}
	return warnings, tree.CheckStable()
}

func firstOwnershipReview(reviews []*OwnershipReview) *OwnershipReview {
	if len(reviews) == 0 {
		return nil
	}
	return reviews[0]
}

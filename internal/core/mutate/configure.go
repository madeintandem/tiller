package mutate

// Configure is the record-only setter for Review Grain
// (docs/plans/2026-08-26-review-grain-design.md, "The setting" and
// "Authorization"). It writes exactly one field, review_grain, at one of its
// three homes: the manifest's execution block (the project default), an
// epic.yml, or a feature.yml. It never touches the lifecycle engine, never
// moves a node's status, and never records a PR or a review ref; it is a
// sibling to the approve verbs' own --review-grain flag, not a replacement
// for them, and shares their guards.
//
// Authorization ("Authorization"): setting a grain is a gate decision, the
// human's decision rather than the human's keyboard. This function trusts its
// caller to have that instruction in hand; it validates the value, records
// the event, and reads back what it recorded, but it does not itself ask
// permission. `configure project`, `configure epic <key>`, and
// `configure feature <key>` are the only three legal targets; a caller must
// always name one, since the CLI grammar layer is the one that refuses a
// bare, targetless call.
//
// Changeability ("Changeability"): the grain on a node may change only until
// that node's own review surface exists. Once `submit` has recorded a PR or a
// review ref on the node itself, or the node has reached a terminal status,
// configureNode refuses: a change governs only work not yet submitted, and a
// concluded node has none left to govern.
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/store"
	"gopkg.in/yaml.v3"
)

// Event type discriminators for configure, one per home review_grain lives
// at (docs/plans/2026-08-26-review-grain-design.md, "The setting").
const (
	eventConfigureProject = "configure.project"
	eventConfigureEpic    = "configure.epic"
	eventConfigureFeature = "configure.feature"
)

// ConfigureInput is a request to record grain at one of Review Grain's three
// homes. Noun is "project", store.NounEpic, or store.NounFeature; Key is the
// node's id or canonical key and is empty for "project", which has none.
type ConfigureInput struct {
	Noun        string
	Key         string
	ReviewGrain string
}

// ConfigureResult is the outcome: the resolved noun and node identity (ID and
// Key stay empty for "project"), the review grain now on record (whether or
// not this call changed it), and whether this call actually wrote anything.
// A no-op, whether because the value already matched or because the request
// was refused, is never the caller's job to distinguish from Changed alone;
// a refusal returns an error instead.
type ConfigureResult struct {
	Noun        string
	ID          string
	Key         string
	ReviewGrain string
	Changed     bool
}

// Configure resolves the acting contributor, then dispatches on Noun. It is the
// only writer of review_grain outside the approve verbs' own
// --review-grain flag (roster.go's AddContributor follows the same named-writer
// convention for the contributors block rather than a general manifest mutation).
func Configure(tree *store.Tree, deps Deps, in ConfigureInput) (ConfigureResult, error) {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return ConfigureResult{}, err
	}
	switch in.Noun {
	case "project":
		return configureProject(tree, deps, owner, in.ReviewGrain)
	case store.NounEpic, store.NounFeature:
		return configureNode(tree, deps, owner, in)
	default:
		return ConfigureResult{}, &usageError{
			reason: "unsupported_noun",
			msg:    fmt.Sprintf("mutate: configure records review_grain on project, epic, or feature, not %q", in.Noun),
		}
	}
}

// configureProject writes execution.review_grain into the manifest,
// following the roster precedent AddContributor already set: a manifest snapshot
// read, a surgical render (store.RenderExecutionReviewGrain), one event, one
// commitMutation. The manifest stays human-owned; a hand edit remains legal
// (design doc, "Authorization").
func configureProject(tree *store.Tree, deps Deps, owner store.Contributor, grain string) (ConfigureResult, error) {
	if !store.ValidReviewGrain("project", grain) {
		return ConfigureResult{}, &usageError{
			reason: "invalid_review_grain",
			msg:    fmt.Sprintf("mutate: %q is not a legal review grain for the project; task, feature, or epic", grain),
		}
	}

	manifestRelPath := filepath.Join(".tiller", "tiller.yml")
	manifestPath := filepath.Join(tree.Root, manifestRelPath)
	manifestData, manifestSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, manifestPath)
	if err != nil {
		return ConfigureResult{}, err
	}
	if !manifestSnapshot.Exists() {
		return ConfigureResult{}, fmt.Errorf("read %s: %w", manifestPath, os.ErrNotExist)
	}
	rendered, err := store.RenderExecutionReviewGrain(manifestData, grain)
	if err != nil {
		return ConfigureResult{}, &usageError{
			reason: "unsupported_manifest_shape",
			msg:    "mutate: " + err.Error(),
		}
	}
	manifest, err := store.ParseManifest(tree.Root, manifestData)
	if err != nil {
		return ConfigureResult{}, err
	}
	guardTree := *tree
	guardTree.Manifest = manifest
	guards, err := guardReviewGrainChange(&guardTree, reviewGrainChange{
		Noun: "project", Desired: grain,
	})
	if err != nil {
		return ConfigureResult{}, err
	}

	before := manifest.Execution.ReviewGrain
	if before == grain {
		return ConfigureResult{Noun: "project", ReviewGrain: before, Changed: false}, nil
	}
	if _, err := store.ParseManifest(tree.Root, rendered); err != nil {
		return ConfigureResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventConfigureProject, owner.ID, "configure project --review-grain "+grain)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"review_grain": before}
	ev.After = map[string]any{"review_grain": grain}
	ev.Files = []events.FileChange{{Path: manifestRelPath}}

	writes := append(guards, store.PlanWrite(manifestPath, manifestSnapshot, rendered))
	if err := commitMutation(tree, "configure project", writes, []events.Event{ev}); err != nil {
		return ConfigureResult{}, err
	}

	return ConfigureResult{Noun: "project", ReviewGrain: grain, Changed: true}, nil
}

// configureNode resolves in.Key against the tree (any addressable epic or
// feature, by id or by canonical key, the same resolve.Resolver every other
// keyed command uses) and dispatches to the epic or feature arm.
func configureNode(tree *store.Tree, deps Deps, owner store.Contributor, in ConfigureInput) (ConfigureResult, error) {
	if !store.ValidReviewGrain(in.Noun, in.ReviewGrain) {
		return ConfigureResult{}, &usageError{
			reason: "invalid_review_grain",
			msg:    fmt.Sprintf("mutate: %q is not a legal review grain for a %s", in.ReviewGrain, in.Noun),
		}
	}
	node, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: in.Noun, Raw: in.Key})
	if err != nil {
		return ConfigureResult{}, err
	}
	switch n := node.(type) {
	case *store.Epic:
		return configureEpic(tree, deps, owner, n, in.ReviewGrain)
	case *store.Feature:
		return configureFeature(tree, deps, owner, n, in.ReviewGrain)
	default:
		// Unreachable: in.Noun was already checked against store.NounEpic and
		// store.NounFeature above, and the resolver refuses a noun mismatch
		// before returning, so it never hands back a third concrete type here.
		return ConfigureResult{}, fmt.Errorf("mutate: configure resolved %s to an unsupported node type", in.Noun)
	}
}

// configureEpic reads epic.yml fresh, refuses on a terminal epic or one that
// already carries its own recorded PR or review ref (Changeability), no-ops
// on an equal value, and otherwise sets ReviewGrain, marshals, and commits one
// write plus one configure.epic event. It never calls into the lifecycle
// engine: this is a plain field write, not a transition.
func configureEpic(tree *store.Tree, deps Deps, owner store.Contributor, treeEpic *store.Epic, grain string) (ConfigureResult, error) {
	key := deref(treeEpic.Key)
	relYml := filepath.Join(epicRelDir(treeEpic), "epic.yml")

	var doc store.Epic
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if err != nil {
		return ConfigureResult{}, err
	}

	terminal := doc.Status == statusCancelled || doc.Status == statusArchived
	if err := refuseReviewSurfaceLocked(store.NounEpic, doc.ID, key, doc.Status, doc.PR, doc.ReviewRef, terminal); err != nil {
		return ConfigureResult{}, err
	}
	guards, err := guardReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounEpic, Epic: &doc, Desired: grain,
	})
	if err != nil {
		return ConfigureResult{}, err
	}

	before := doc.ReviewGrain
	if before == grain {
		return ConfigureResult{Noun: store.NounEpic, ID: doc.ID, Key: key, ReviewGrain: before, Changed: false}, nil
	}
	doc.ReviewGrain = grain

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return ConfigureResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventConfigureEpic, owner.ID, fmt.Sprintf("configure epic %s --review-grain %s", key, grain), doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"review_grain": before}
	ev.After = map[string]any{"review_grain": grain}
	ev.Files = []events.FileChange{{Path: relYml}}

	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, ydata))
	if err := commitMutation(tree, "configure epic", writes, []events.Event{ev}); err != nil {
		return ConfigureResult{}, err
	}

	return ConfigureResult{Noun: store.NounEpic, ID: doc.ID, Key: key, ReviewGrain: grain, Changed: true}, nil
}

// configureFeature is configureEpic's feature-grain twin: it reads
// feature.yml fresh, refuses on a terminal feature or one that already
// carries its own recorded PR or review ref, no-ops on an equal value, and
// otherwise sets ReviewGrain, marshals, and commits one write plus one
// configure.feature event.
func configureFeature(tree *store.Tree, deps Deps, owner store.Contributor, treeFeat *store.Feature, grain string) (ConfigureResult, error) {
	relYml := filepath.Join(featureRelDir(treeFeat), "feature.yml")

	var doc store.Feature
	snapshot, err := readYAMLSnapshot(tree.Root, filepath.Join(tree.Root, relYml), &doc)
	if err != nil {
		return ConfigureResult{}, err
	}
	doc.EpicKey = treeFeat.EpicKey

	key := deref(doc.Key)
	terminal := doc.Status == statusDone || doc.Status == statusCancelled
	if err := refuseReviewSurfaceLocked(store.NounFeature, doc.ID, key, doc.Status, doc.PR, doc.ReviewRef, terminal); err != nil {
		return ConfigureResult{}, err
	}
	guards, err := guardReviewGrainChange(tree, reviewGrainChange{
		Noun: store.NounFeature, Epic: epicByKey(tree, doc.EpicKey), Feature: &doc, Desired: grain,
	})
	if err != nil {
		return ConfigureResult{}, err
	}

	before := doc.ReviewGrain
	if before == grain {
		return ConfigureResult{Noun: store.NounFeature, ID: doc.ID, Key: key, ReviewGrain: before, Changed: false}, nil
	}
	doc.ReviewGrain = grain

	ydata, err := yaml.Marshal(doc)
	if err != nil {
		return ConfigureResult{}, err
	}

	ev := events.NewEvent(deps.clock(), eventConfigureFeature, owner.ID, fmt.Sprintf("configure feature %s --review-grain %s", key, grain), doc.ID)
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"review_grain": before}
	ev.After = map[string]any{"review_grain": grain}
	ev.Files = []events.FileChange{{Path: relYml}}

	writes := append(guards, store.PlanWrite(filepath.Join(tree.Root, relYml), snapshot, ydata))
	if err := commitMutation(tree, "configure feature", writes, []events.Event{ev}); err != nil {
		return ConfigureResult{}, err
	}

	return ConfigureResult{Noun: store.NounFeature, ID: doc.ID, Key: key, ReviewGrain: grain, Changed: true}, nil
}

// refuseReviewSurfaceLocked is the Changeability guard configureEpic and
// configureFeature share: a node's grain may change only until its own
// review surface exists, either because submit already recorded a PR or a
// review ref on it, or because the node itself is terminal and has nothing
// left to submit (docs/plans/2026-08-26-review-grain-design.md,
// "Changeability"). It returns nil when neither condition holds.
func refuseReviewSurfaceLocked(noun, id, key, status string, pr *int, ref *store.ReviewRef, terminal bool) error {
	switch {
	case pr != nil:
		return &ReviewGrainLockedError{Noun: noun, ID: id, Key: key, PR: pr}
	case ref != nil:
		return &ReviewGrainLockedError{Noun: noun, ID: id, Key: key, ReviewRef: ref}
	case terminal:
		return &ReviewGrainLockedError{Noun: noun, ID: id, Key: key, Status: status}
	default:
		return nil
	}
}

// ReviewGrainLockedError refuses `configure epic` or `configure feature`
// because the node's own review surface already exists
// (docs/plans/2026-08-26-review-grain-design.md, "Changeability": "The grain
// on a node may change until that node's own review surface exists"). Like
// the other blocked answers in this package it is exit 1, rendered through
// the CLI's emitReport as an ok:false data block, and it deliberately does
// not implement ExitCode, because the report sets that.
type ReviewGrainLockedError struct {
	Noun      string
	ID        string
	Key       string
	Status    string
	PR        *int
	ReviewRef *store.ReviewRef
}

func (e *ReviewGrainLockedError) ident() string {
	if e.Key != "" {
		return e.Key
	}
	return e.ID
}

func (e *ReviewGrainLockedError) Error() string {
	switch {
	case e.PR != nil:
		return fmt.Sprintf("mutate: %s %s already has a review surface recorded (pr #%d); review grain cannot change once its own review surface exists",
			e.Noun, e.ident(), *e.PR)
	case e.ReviewRef != nil:
		return fmt.Sprintf("mutate: %s %s already has a review surface recorded (review_ref branch %s); review grain cannot change once its own review surface exists",
			e.Noun, e.ident(), e.ReviewRef.Branch)
	default:
		return fmt.Sprintf("mutate: %s %s is terminal (%s); review grain cannot change once its own review surface exists",
			e.Noun, e.ident(), e.Status)
	}
}

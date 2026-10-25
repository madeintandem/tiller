package mutate

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"gopkg.in/yaml.v3"
)

type DesignLinkInput struct {
	Doc       string
	Describes []string
	From      []string
}

type DesignResult struct {
	Doc     store.BaselineDoc
	Changed bool
}

type DesignApprovalResult struct {
	Docs []DesignResult
}

type designState struct {
	ledger   store.BaselineLedger
	manifest *store.Manifest
	owner    store.Contributor
	snapshot store.FileSnapshot
	guards   []store.PlannedWrite
}

func readDesignState(tree *store.Tree, deps Deps) (*designState, error) {
	if err := tree.CheckStable(); err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(tree.Root, ".tiller", "tiller.yml")
	data, manifestSnapshot, err := store.ReadRegularFileSnapshot(tree.Root, manifestPath)
	if err != nil {
		return nil, err
	}
	if !manifestSnapshot.Exists() {
		return nil, fmt.Errorf("read %s: manifest is missing", manifestPath)
	}
	manifest, err := store.ParseManifest(tree.Root, data)
	if err != nil {
		return nil, err
	}
	owner, err := store.ResolveContributor(manifest, deps.Contributor)
	if err != nil {
		return nil, err
	}
	data, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, filepath.Join(tree.Root, store.BaselineRelPath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", store.BaselineRelPath, err)
	}
	ledger := store.BaselineLedger{Docs: []store.BaselineDoc{}}
	if snapshot.Exists() {
		ledger, err = store.ParseBaseline(data)
		if err != nil {
			return nil, err
		}
	}
	if err := tree.CheckStable(); err != nil {
		return nil, err
	}
	return &designState{ledger: ledger, manifest: manifest, owner: owner, snapshot: snapshot, guards: []store.PlannedWrite{store.PlanReadGuard(manifestPath, manifestSnapshot)}}, nil
}

func (state *designState) guardDoc(tree *store.Tree, doc string) error {
	p := filepath.Join(tree.Root, ".tiller", "design", doc)
	_, snapshot, err := store.ReadRegularFileSnapshot(tree.Root, p)
	if err != nil {
		return designPathError("design_path", "cannot read design document %q: %v", doc, err)
	}
	if !snapshot.Exists() {
		return designPathError("design_missing", "design document %q is missing", doc)
	}
	state.guards = append(state.guards, store.PlanReadGuard(p, snapshot))
	return nil
}

func (state *designState) index(doc string) int {
	return slices.IndexFunc(state.ledger.Docs, func(entry store.BaselineDoc) bool { return entry.Path == doc })
}

func (state *designState) commit(tree *store.Tree, operation string, evs []events.Event) error {
	if len(evs) == 0 {
		// A no-op still validates the fresh preconditions under the transaction
		// lock, without changing generation or producing an event.
		guards := append(state.guards, store.PlanReadGuard(filepath.Join(tree.Root, store.BaselineRelPath), state.snapshot))
		return tree.Commit(operation, guards)
	}
	data, err := yaml.Marshal(state.ledger)
	if err != nil {
		return err
	}
	if _, err := store.ParseBaseline(data); err != nil {
		return err
	}
	writes := append(state.guards, store.PlanWrite(filepath.Join(tree.Root, store.BaselineRelPath), state.snapshot, data))
	return commitMutation(tree, operation, writes, evs)
}

type designClock struct{ at time.Time }

func (clock designClock) Now() time.Time { return clock.at }

func designEvent(deps Deps, owner store.Contributor, action string, doc store.BaselineDoc, at time.Time, data map[string]any) events.Event {
	ev := events.NewEvent(designClock{at}, action+".design", owner.ID, action+" design "+doc.Path, ".tiller/design/"+doc.Path)
	ev.ID = deps.mintRaw("EV")
	ev.Files = []events.FileChange{{Path: store.BaselineRelPath}}
	data["doc"] = doc.Path
	ev.After = data
	return ev
}

// LinkDesign unions declared coverage and upserts recoverable project-HEAD
// provenance. Neither operation writes the design prose or the manifest.
func LinkDesign(tree *store.Tree, deps Deps, input DesignLinkInput) (DesignResult, error) {
	docPath, err := designDocPath(input.Doc)
	if err != nil {
		return DesignResult{}, err
	}
	if len(input.Describes) == 0 && len(input.From) == 0 {
		return DesignResult{}, designPathError("design_link", "link design needs --describes or --from")
	}
	state, err := readDesignState(tree, deps)
	if err != nil {
		return DesignResult{}, err
	}
	if err := state.guardDoc(tree, docPath); err != nil {
		return DesignResult{}, err
	}
	index := state.index(docPath)
	doc := store.BaselineDoc{Path: docPath}
	if index >= 0 {
		doc = state.ledger.Docs[index]
	}
	if len(doc.Describes) == 0 && len(input.Describes) == 0 {
		return DesignResult{}, designPathError("design_unlinked", "link design %s with --describes before recording provenance", docPath)
	}
	var added []string
	for _, raw := range input.Describes {
		p, err := resolveDesignSurface(tree.Root, state.manifest, raw)
		if err != nil {
			return DesignResult{}, err
		}
		if !slices.Contains(doc.Describes, p.recorded) {
			doc.Describes = append(doc.Describes, p.recorded)
			added = append(added, p.recorded)
		}
	}
	if len(added) > 0 {
		doc.Confirmed = nil
	}
	at := deps.clock().Now().UTC()
	var fromAdded, fromUpdated []string
	if len(input.From) > 0 {
		head, err := deps.rev(tree.Root).CurrentCommit()
		if err != nil {
			return DesignResult{}, err
		}
		for _, raw := range input.From {
			p, err := designSourcePath(tree.Root, raw)
			if err != nil {
				return DesignResult{}, err
			}
			ref, err := vcs.BaselineSource(tree.Root, head, p, at)
			if err != nil {
				return DesignResult{}, err
			}
			i := slices.IndexFunc(doc.DerivedFrom, func(old store.RevisionRef) bool { return old.Path == p })
			if i < 0 {
				doc.DerivedFrom = append(doc.DerivedFrom, ref)
				fromAdded = append(fromAdded, p)
			} else if doc.DerivedFrom[i].Value != ref.Value || doc.DerivedFrom[i].Kind != ref.Kind {
				doc.DerivedFrom[i] = ref
				fromUpdated = append(fromUpdated, p)
			}
		}
	}
	changed := len(added)+len(fromAdded)+len(fromUpdated) > 0
	var evs []events.Event
	if changed {
		if index < 0 {
			state.ledger.Docs = append(state.ledger.Docs, doc)
		} else {
			state.ledger.Docs[index] = doc
		}
		data := map[string]any{"describes": doc.Describes, "derived_from": doc.DerivedFrom}
		if len(added) > 0 {
			data["describes_added"] = added
		}
		if len(fromAdded) > 0 {
			data["from_added"] = fromAdded
		}
		if len(fromUpdated) > 0 {
			data["from_updated"] = fromUpdated
		}
		evs = append(evs, designEvent(deps, state.owner, "link", doc, at, data))
	}
	err = state.commit(tree, "link.design", evs)
	return DesignResult{Doc: doc, Changed: changed}, err
}

// UnlinkDesign intentionally does not inspect prose, Git, or code paths: it is
// also the repair path after those files or declarations have disappeared.
func UnlinkDesign(tree *store.Tree, deps Deps, input DesignLinkInput) (DesignResult, error) {
	docPath, err := designDocPath(input.Doc)
	if err != nil {
		return DesignResult{}, err
	}
	if len(input.Describes) == 0 && len(input.From) == 0 {
		return DesignResult{}, designPathError("design_unlink", "unlink design needs --describes or --from")
	}
	state, err := readDesignState(tree, deps)
	if err != nil {
		return DesignResult{}, err
	}
	paths, err := designRemovalPaths(tree.Root, input.Describes, false)
	if err != nil {
		return DesignResult{}, err
	}
	paths = designRemovalAliases(tree.Root, state.manifest, paths)
	sources, err := designRemovalPaths(tree.Root, input.From, true)
	if err != nil {
		return DesignResult{}, err
	}
	index := state.index(docPath)
	doc := store.BaselineDoc{Path: docPath}
	var removed, fromRemoved []string
	if index >= 0 {
		doc = state.ledger.Docs[index]
		doc.Describes = slices.DeleteFunc(doc.Describes, func(p string) bool {
			if slices.Contains(paths, p) {
				removed = append(removed, p)
				return true
			}
			return false
		})
		doc.DerivedFrom = slices.DeleteFunc(doc.DerivedFrom, func(ref store.RevisionRef) bool {
			if slices.Contains(sources, ref.Path) {
				fromRemoved = append(fromRemoved, ref.Path)
				return true
			}
			return false
		})
	}
	changed := len(removed)+len(fromRemoved) > 0
	var evs []events.Event
	if changed {
		if len(removed) > 0 {
			doc.Confirmed = nil
		}
		if len(doc.Describes) == 0 {
			for _, ref := range doc.DerivedFrom {
				fromRemoved = append(fromRemoved, ref.Path)
			}
			doc.DerivedFrom = nil
			state.ledger.Docs = slices.Delete(state.ledger.Docs, index, index+1)
		} else {
			state.ledger.Docs[index] = doc
		}
		data := map[string]any{"describes": doc.Describes, "derived_from": doc.DerivedFrom}
		if len(removed) > 0 {
			data["describes_removed"] = removed
		}
		if len(fromRemoved) > 0 {
			data["from_removed"] = fromRemoved
		}
		evs = append(evs, designEvent(deps, state.owner, "unlink", doc, deps.clock().Now().UTC(), data))
	}
	err = state.commit(tree, "unlink.design", evs)
	return DesignResult{Doc: doc, Changed: changed}, err
}

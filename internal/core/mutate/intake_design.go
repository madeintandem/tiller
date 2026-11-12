package mutate

import (
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// eventDesignIntake records a founding design landing under .tiller/design/
// with no amendment opened (`tiller intake --no-amendment`).
const eventDesignIntake = "design_intake"

// RecordDesignIntake writes the one event that says a founding design landed:
// destRel is where it landed (repo-relative) and files how many documents it
// holds. The event names no file of its own, since the landed documents are
// written by intake outside the transaction and no node changes.
func RecordDesignIntake(tree *store.Tree, deps Deps, destRel string, files int) error {
	owner, err := store.ResolveContributor(tree.Manifest, deps.Contributor)
	if err != nil {
		return err
	}
	ev := events.NewEvent(deps.clock(), eventDesignIntake, owner.ID, "intake design "+destRel)
	ev.ID = deps.mintRaw("EV")
	ev.After = map[string]any{"path": destRel, "files": files}
	return commitMutation(tree, "intake.design", nil, []events.Event{ev})
}

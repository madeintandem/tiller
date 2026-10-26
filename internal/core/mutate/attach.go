package mutate

import (
	"fmt"
	"strings"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
)

// Attachment is a shelf node's attached_to (docs/spec/cli-surface.md). It is
// provenance, so it is legal at every state, including done and cancelled, and
// it never runs the dependency-change lock; the target is an epic, a feature,
// or a task. Link sets it and Unlink clears it, each in the same planned batch
// as any dependency change the call also requested, so one refused axis leaves
// the other unwritten.

// requireAttachmentNoun refuses --attach or --detach on a noun that carries no
// attachment. It runs before the node resolves, so the refusal names the
// misuse rather than a lookup.
func requireAttachmentNoun(noun string) error {
	if !store.IsWorkNoun(noun) {
		return &usageError{reason: "attach_noun", msg: fmt.Sprintf("--attach applies to a shelf work node, not a %s", noun)}
	}
	return nil
}

// resolveAttachRequest resolves a requested --attach target to the stored id.
// A blank target is a usage error, never a silent detach.
func resolveAttachRequest(tree *store.Tree, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", &usageError{reason: "attach_target", msg: "--attach needs an epic, feature, or task"}
	}
	return resolveAttachTarget(tree, raw)
}

// attachmentEvent records one attached_to change. verb is "link" or "unlink".
func attachmentEvent(deps Deps, owner store.Contributor, node store.Node, verb, command, relYml, before, after string) events.Event {
	ev := events.NewEvent(deps.clock(), verb+".attach."+node.Noun(), owner.ID, command, node.NodeID())
	ev.ID = deps.mintRaw("EV")
	ev.Before = map[string]any{"attached_to": before}
	ev.After = map[string]any{"attached_to": after}
	ev.Files = []events.FileChange{{Path: relYml}}
	return ev
}

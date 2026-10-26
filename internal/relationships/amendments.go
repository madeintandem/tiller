// Package relationships retains historical amendment identities without resolving
// old keys through the current tree. Presentation belongs to the consuming view.
package relationships

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/id"
	"github.com/madeintandem/tiller/internal/store"
)

// AmendmentEndpoint distinguishes a proven immutable identity from a raw,
// unresolved historical coordinate. Only an ID may be looked up in today's tree.
// Pending marks an unresolved key that this proposed amendment stages itself
// and has not yet applied: its node is expected not to exist before approval.
type AmendmentEndpoint struct {
	ID         string
	Raw        string
	Unresolved bool
	Pending    bool
}

// AmendmentOperation contains identity facts only; lifecycle and source checks
// remain with the amendment's existing owners.
type AmendmentOperation struct {
	Target, By, SupersededBy, Created, Parent *AmendmentEndpoint
	Blocks                                    []AmendmentEndpoint
	Notes                                     []string
}

type AmendmentEffect struct {
	OperationIndex *int
	Operation      string
	Role           string
	Scope          string
	Node           *AmendmentEndpoint
	Reference      string
	Note           string
	State          string
	Evidence       []string
}

type AmendmentRelation struct {
	AmendmentID string
	Effects     []AmendmentEffect
	ProposedAt  string
	ApprovedAt  string
	WithdrawnAt string
}

// AmendmentIndex is the shared, neutral projection used by both the graph and
// the existing forward/backward amendment pages. Effects retains unresolved
// direct endpoints too; Relations contains identity-safe page summaries.
type AmendmentIndex struct {
	Operations map[string][]AmendmentOperation
	Relations  map[string][]AmendmentRelation
	Effects    map[string][]AmendmentEffect
	Warning    string
}

func amendmentNodeKey(n store.Node) string {
	if key := n.NodeKey(); key != nil {
		return *key
	}
	return ""
}

func amendmentLess(as, ak, ai, bs, bk, bi string) bool {
	if (as == store.AmendmentStatusProposed) != (bs == store.AmendmentStatusProposed) {
		return as == store.AmendmentStatusProposed
	}
	if ak != bk {
		return ak < bk
	}
	return ai < bi
}

// AmendmentIdentities does no I/O and never resolves a historical key through the
// current resolver. Both navigation directions consume these same endpoints.
func AmendmentIdentities(t *store.Tree, evs []events.Event) *AmendmentIndex {
	out := &AmendmentIndex{Operations: make(map[string][]AmendmentOperation), Relations: make(map[string][]AmendmentRelation), Effects: make(map[string][]AmendmentEffect)}
	if len(t.Amendments) == 0 {
		return out
	}
	history := amendmentKeyHistoryOf(t, evs)
	byAmendment := make(map[string][]events.Event)
	for _, ev := range evs {
		seen := make(map[string]bool)
		for _, target := range appendEventAmendment(ev) {
			if t.Amendments[target] != nil && !seen[target] {
				byAmendment[target] = append(byAmendment[target], ev)
				seen[target] = true
			}
		}
	}
	amendments := make([]*store.Amendment, 0, len(t.Amendments))
	for _, a := range t.Amendments {
		amendments = append(amendments, a)
	}
	sort.Slice(amendments, func(i, j int) bool {
		a, b := amendments[i], amendments[j]
		return amendmentLess(a.Status, amendmentNodeKey(a), a.ID, b.Status, amendmentNodeKey(b), b.ID)
	})
	for _, a := range amendments {
		projectAmendmentIdentity(t, a, byAmendment[a.ID], history, out)
	}
	return out
}

func appendEventAmendment(ev events.Event) []string {
	if a := eventString(ev.After, "amendment"); a != "" {
		return []string{a}
	}
	return ev.Targets
}

func projectAmendmentIdentity(t *store.Tree, a *store.Amendment, evs []events.Event, history amendmentKeyHistory, out *AmendmentIndex) {
	row := AmendmentRelation{AmendmentID: a.ID}
	for _, ev := range evs {
		switch ev.Type {
		case "amendment_opened":
			row.ProposedAt = earlierEventDate(row.ProposedAt, ev.At)
		case "amendment_approved":
			row.ApprovedAt = earlierEventDate(row.ApprovedAt, ev.At)
		case "amendment_withdrawn":
			row.WithdrawnAt = earlierEventDate(row.WithdrawnAt, ev.At)
		}
	}
	warn := func() {
		out.Warning = "Some amendment identities could not be established from the retained history; known links are shown and relationships may be incomplete."
	}
	if len(evs) == 0 || row.ProposedAt == "" || (a.Status == store.AmendmentStatusApproved && row.ApprovedAt == "") {
		warn()
	}
	ops := make([]AmendmentOperation, len(a.Operations))
	// Exact keyed-add joins stand independently of lifecycle status: an
	// interrupted application can leave its amendment proposed.
	created := make(map[string]string)
	creationEvidence := make(map[string][]string)
	ambiguous := make(map[string]bool)
	for _, ev := range evs {
		if !strings.HasPrefix(ev.Type, "add.") || eventString(ev.After, "amendment") != a.ID || len(ev.Targets) == 0 {
			continue
		}
		key := eventString(ev.After, "key")
		if key == "" {
			continue
		}
		join := strings.TrimPrefix(ev.Type, "add.") + "\x00" + key
		if prior := created[join]; prior != "" && prior != ev.Targets[0] {
			ambiguous[join] = true
		}
		created[join] = ev.Targets[0]
		creationEvidence[ev.Targets[0]] = mergeAmendmentEvidence(creationEvidence[ev.Targets[0]], amendmentEventEvidence([]events.Event{ev}))
	}
	staged := make(map[string][]int)
	for i, op := range a.Operations {
		if op.Key != "" && strings.HasPrefix(op.Op, "add-") {
			staged[op.Key] = append(staged[op.Key], i)
		}
	}
	applications := make([][]events.Event, len(a.Operations))
	staging := make([][]events.Event, len(a.Operations))
	var unjoined []events.Event
	for _, ev := range evs {
		if ev.Type == "amendment_staged" {
			idx, ok := eventIndex(ev.After, "index")
			if !ok || idx >= len(a.Operations) {
				continue
			}
			op := a.Operations[idx]
			if eventString(ev.After, "op") == op.Op && (eventString(ev.After, "key") == "" || eventString(ev.After, "key") == op.Key) {
				staging[idx] = append(staging[idx], ev)
			}
		} else if idx := amendmentApplicationIndex(ev, a); idx >= 0 {
			applications[idx] = append(applications[idx], ev)
		} else {
			unjoined = append(unjoined, ev)
		}
	}
	resolveRaw := func(raw string) (*AmendmentEndpoint, string) {
		if raw == "" {
			return nil, ""
		}
		if id.Valid(raw) {
			return amendmentIDNode(t, raw)
		}
		if indices := staged[raw]; len(indices) == 1 {
			op := a.Operations[indices[0]]
			join := strings.TrimPrefix(op.Op, "add-") + "\x00" + raw
			if nodeID := created[join]; nodeID != "" && !ambiguous[join] {
				return amendmentIDNode(t, nodeID)
			}
			if ambiguous[join] {
				return unresolvedAmendmentRef(raw), "Conflicting creation events for staged key " + raw
			}
			// Before approval applies the staging operation, its node is
			// expected not to exist; that is not a defect to report.
			if a.Status == store.AmendmentStatusProposed && len(applications[indices[0]]) == 0 {
				return &AmendmentEndpoint{Raw: raw, Unresolved: true, Pending: true}, ""
			}
			return unresolvedAmendmentRef(raw), "Staged reference " + raw + " has no proven materialized identity"
		}
		return unresolvedAmendmentRef(raw), "Historical reference " + raw + " cannot be resolved by today's key holder"
	}
	// Store effects by page, deduplicating only identical endpoints/roles/ops.
	pageEffects := make(map[string][]AmendmentEffect)
	addEffect := func(effect AmendmentEffect) {
		if effect.Node == nil {
			return
		}
		out.Effects[a.ID] = appendUniqueAmendmentEffect(out.Effects[a.ID], effect)
		if effect.Node.ID == "" {
			return
		}
		pageEffects[effect.Node.ID] = appendUniqueAmendmentEffect(pageEffects[effect.Node.ID], effect)
		for _, ancestor := range amendmentAncestors(t, effect.Node.ID) {
			rolled := effect
			if rolled.Scope != "historical_parent" {
				rolled.Scope = "descendant"
			}
			pageEffects[ancestor] = appendUniqueAmendmentEffect(pageEffects[ancestor], rolled)
		}
	}
	for i, op := range a.Operations {
		identity := &ops[i]
		application, stages := applications[i], staging[i]
		evidence := mergeAmendmentEvidence(amendmentEventEvidence(stages), amendmentEventEvidence(application))
		state := "proposed"
		if len(application) > 0 {
			state = "applied"
		}
		replacement, conflict := uniqueEventIdentity(application, "superseded_by")
		note := func(s string) {
			if s == "" {
				return
			}
			for _, prior := range identity.Notes {
				if prior == s {
					return
				}
			}
			identity.Notes = append(identity.Notes, s)
		}
		endpoint := func(raw string) *AmendmentEndpoint {
			n, why := resolveRaw(raw)
			note(why)
			if why != "" && (id.Valid(raw) || len(staged[raw]) != 1 || ambiguous[strings.TrimPrefix(a.Operations[staged[raw][0]].Op, "add-")+"\x00"+raw]) {
				warn()
			}
			return n
		}
		identity.Target = endpoint(op.Target)
		if replacement == "" && !conflict {
			identity.By = endpoint(op.By)
			identity.SupersededBy = endpoint(op.SupersededBy)
		}
		for _, raw := range op.Blocks {
			if block := endpoint(raw); block != nil {
				identity.Blocks = append(identity.Blocks, *block)
			}
		}
		if len(stages) == 0 {
			note("Operation staging history is unavailable")
			warn()
		}
		if op.Key != "" && strings.HasPrefix(op.Op, "add-") {
			join := strings.TrimPrefix(op.Op, "add-") + "\x00" + op.Key
			if ambiguous[join] || len(staged[op.Key]) != 1 {
				note("Conflicting creation evidence for " + op.Key)
				warn()
			} else if nodeID := created[join]; nodeID != "" {
				var why string
				identity.Created, why = amendmentIDNode(t, nodeID)
				note(why)
				if why != "" {
					warn()
				}
			} else if a.Status == store.AmendmentStatusApproved {
				note("No creation event establishes the identity added at " + op.Key)
				warn()
			}
		}
		// Replacement IDs are recorded in After, not necessarily Targets (cancel).
		if conflict {
			note("Conflicting replacement identities in application history")
			warn()
			if op.By != "" {
				identity.By = unresolvedAmendmentRef(op.By)
			}
			if op.SupersededBy != "" {
				identity.SupersededBy = unresolvedAmendmentRef(op.SupersededBy)
			}
		}
		if replacement != "" {
			n, why := amendmentIDNode(t, replacement)
			note(why)
			if why != "" {
				warn()
			}
			if op.By != "" {
				identity.By = n
			}
			if op.SupersededBy != "" {
				identity.SupersededBy = n
			}
		}
		if op.Op == store.AmendmentOpAddQuestion {
			var questionAdds []events.Event
			for _, ev := range application {
				if ev.Type == "add.question" && eventString(ev.After, "outcome") == "created" {
					questionAdds = append(questionAdds, ev)
				}
			}
			qID, conflict := uniqueEventTarget(questionAdds)
			if qID != "" && !conflict {
				var why string
				identity.Created, why = amendmentIDNode(t, qID)
				note(why)
				if why != "" {
					warn()
				}
			} else {
				note("No creation event proves a new question; this operation may be unexecuted or folded into an existing question")
			}
		}
		parentKey := ""
		if op.Op == store.AmendmentOpAddFeature || op.Op == store.AmendmentOpAddTask {
			if slash := strings.LastIndex(op.Key, "/"); slash > 0 {
				parentKey = op.Key[:slash]
			}
		}
		if parentKey != "" {
			field := "epic"
			if op.Op == store.AmendmentOpAddTask {
				field = "feature"
			}
			parentID, conflict := uniqueEventIdentity(application, field)
			if parentID == "" && !conflict {
				parentID, conflict = uniqueEventIdentity(stages, "parent")
			}
			if conflict {
				note("Conflicting addition-parent identities")
				warn()
			} else if parentID != "" {
				var why string
				identity.Parent, why = amendmentIDNode(t, parentID)
				note(why)
				if why != "" {
					warn()
				}
			} else if len(staged[parentKey]) == 1 {
				identity.Parent = endpoint(parentKey)
			} else {
				// Legacy parent keys require a bounded identity history at the
				// staging/application instant, never a live key lookup.
				when := time.Time{}
				if len(stages) == 1 {
					when = stages[0].At
				} else if len(application) == 1 {
					when = application[0].At
				}
				if parentID = history.resolve(parentKey, when); parentID != "" {
					evidence = mergeAmendmentEvidence(evidence, history.evidence[parentID])
					var why string
					identity.Parent, why = amendmentIDNode(t, parentID)
					note(why)
					if why != "" {
						warn()
					}
				} else {
					identity.Parent = unresolvedAmendmentRef(parentKey)
					note("Original addition parent " + parentKey + " is unresolved")
					warn()
				}
			}
		}
		number := i + 1
		if op.Op == store.AmendmentOpMoveFeature && op.Move != nil {
			destination := op.Move.DestinationEpicID
			if destination == "" {
				if parentID, conflict := uniqueEventIdentity(application, "epic"); !conflict {
					destination = parentID
				}
			}
			if destination == "" {
				destination = op.Move.DestinationEpicKey
			}
			identity.Parent = endpoint(destination)
			for _, parent := range []struct{ role, raw string }{
				{"move_source_parent", op.Move.SourceEpicID}, {"move_destination_parent", destination},
			} {
				scope := "direct"
				if len(application) > 0 || a.Status == store.AmendmentStatusApproved {
					scope = "historical_parent"
				}
				addEffect(AmendmentEffect{OperationIndex: &number, Operation: op.Op, Role: parent.role, Scope: scope, Node: endpoint(parent.raw), Reference: op.Key, State: state, Evidence: evidence})
			}
		}
		for _, pair := range []struct {
			role string
			node *AmendmentEndpoint
		}{
			{"target", identity.Target}, {"replacement", identity.By}, {"replacement", identity.SupersededBy}, {"created", identity.Created}, {"addition_parent", identity.Parent},
		} {
			if pair.role == "addition_parent" && op.Op == store.AmendmentOpMoveFeature {
				continue
			}
			if pair.node == nil {
				continue
			}
			scope := "direct"
			if pair.role == "addition_parent" && (identity.Created != nil || len(application) > 0 || a.Status == store.AmendmentStatusApproved) {
				scope = "historical_parent"
			}
			addEffect(AmendmentEffect{OperationIndex: &number, Operation: op.Op, Role: pair.role, Scope: scope, Node: pair.node, Reference: op.Key, Note: strings.Join(identity.Notes, "; "), State: state, Evidence: mergeAmendmentEvidence(evidence, creationEvidence[pair.node.ID])})
		}
		for j := range identity.Blocks {
			addEffect(AmendmentEffect{OperationIndex: &number, Operation: op.Op, Role: "question_block", Scope: "direct", Node: &identity.Blocks[j], Note: "Retained question-block intent; a folded question operation may not have changed its blocks", State: "proposed", Evidence: evidence})
		}
	}
	// An event may survive after its operation was hand-edited or removed. Keep
	// its proven endpoints without inventing an operation-number join.
	for _, ev := range unjoined {
		if eventString(ev.After, "amendment") != a.ID || len(ev.Targets) == 0 {
			continue
		}
		op, role := "", "target"
		switch {
		case strings.HasPrefix(ev.Type, "add."):
			op, role = strings.Replace(ev.Type, ".", "-", 1), "created"
			if ev.Type == "add.question" && eventString(ev.After, "outcome") != "created" {
				role = "target"
			}
		case strings.HasPrefix(ev.Type, "cancel."):
			op = "cancel"
		case strings.HasPrefix(ev.Type, "revise."):
			op = "revise"
		case strings.HasPrefix(ev.Type, "correct-eval."):
			op = store.AmendmentOpCorrectEval
		case ev.Type == "superseded":
			op = "supersede"
		case ev.Type == "move.feature":
			op = store.AmendmentOpMoveFeature
		}
		if op == "" {
			continue
		}
		warn()
		n, _ := amendmentIDNode(t, ev.Targets[0])
		addEffect(AmendmentEffect{Operation: op, Role: role, Scope: "direct", Node: n, Note: "Application event cannot be joined to one retained operation", State: "applied", Evidence: amendmentEventEvidence([]events.Event{ev})})
		if replacement := eventString(ev.After, "superseded_by"); replacement != "" {
			n, _ = amendmentIDNode(t, replacement)
			addEffect(AmendmentEffect{Operation: op, Role: "replacement", Scope: "direct", Node: n, Note: "Application event cannot be joined to one retained operation", State: "applied", Evidence: amendmentEventEvidence([]events.Event{ev})})
		}
		for _, field := range []string{"feature", "epic"} {
			if parentID := eventString(ev.After, field); parentID != "" && strings.HasPrefix(ev.Type, "add.") {
				parent, _ := amendmentIDNode(t, parentID)
				addEffect(AmendmentEffect{Operation: op, Role: "addition_parent", Scope: "historical_parent", Node: parent, Reference: eventString(ev.After, "key"), Note: "Application event cannot be joined to one retained operation", State: "applied", Evidence: amendmentEventEvidence([]events.Event{ev})})
			}
		}
	}
	out.Operations[a.ID] = ops
	for nodeID, effects := range pageEffects {
		related := row
		related.Effects = effects
		out.Relations[nodeID] = append(out.Relations[nodeID], related)
	}
}

func amendmentApplicationMatches(ev events.Event, amendmentID string, index int, op store.AmendmentOp) bool {
	if eventString(ev.After, "amendment") != amendmentID || len(ev.Targets) == 0 {
		return false
	}
	if recorded, ok := eventIndex(ev.After, "operation_index"); ok && recorded != index {
		return false
	}
	if replacement := eventString(ev.After, "superseded_by"); replacement != "" {
		if id.Valid(op.By) && op.By != replacement || id.Valid(op.SupersededBy) && op.SupersededBy != replacement {
			return false
		}
	}
	switch op.Op {
	case store.AmendmentOpMoveFeature:
		return ev.Type == "move.feature" && ev.Targets[0] == op.Target && eventString(ev.After, "key") == op.Key
	case store.AmendmentOpAddEpic, store.AmendmentOpAddFeature, store.AmendmentOpAddTask:
		return ev.Type == "add."+strings.TrimPrefix(op.Op, "add-") && eventString(ev.After, "key") == op.Key
	case store.AmendmentOpAddQuestion:
		_, indexed := eventIndex(ev.After, "operation_index")
		return indexed && ev.Type == "add.question"
	case store.AmendmentOpCancel:
		return strings.HasPrefix(ev.Type, "cancel.") && ev.Targets[0] == op.Target
	case store.AmendmentOpRevise:
		return strings.HasPrefix(ev.Type, "revise.") && ev.Targets[0] == op.Target
	case store.AmendmentOpSupersede:
		return ev.Type == "superseded" && ev.Targets[0] == op.Target
	case store.AmendmentOpCorrectEval:
		return ev.Type == "correct-eval.feature" && ev.Targets[0] == op.Target
	}
	return false
}

func amendmentIDNode(t *store.Tree, raw string) (*AmendmentEndpoint, string) {
	if !id.Valid(raw) {
		return unresolvedAmendmentRef(raw), "Recorded identity " + raw + " is invalid"
	}
	if n, err := resolve.NewResolver(t).Resolve(resolve.Ref{Raw: raw}); err == nil {
		return &AmendmentEndpoint{ID: n.NodeID(), Raw: raw}, ""
	}
	return &AmendmentEndpoint{ID: raw, Raw: raw, Unresolved: true}, "Recorded node " + raw + " is no longer available"
}

func unresolvedAmendmentRef(raw string) *AmendmentEndpoint {
	return &AmendmentEndpoint{Raw: raw, Unresolved: true}
}

func amendmentAncestors(t *store.Tree, nodeID string) []string {
	var out []string
	feature := t.Features[nodeID]
	if task := t.Tasks[nodeID]; task != nil {
		feature = t.Features[task.FeatureID]
		if feature != nil {
			out = append(out, feature.ID)
		}
	}
	if feature != nil {
		for _, epic := range t.Epics {
			if amendmentNodeKey(epic) == feature.EpicKey {
				out = append(out, epic.ID)
				break
			}
		}
	}
	return out
}

func appendUniqueAmendmentEffect(effects []AmendmentEffect, e AmendmentEffect) []AmendmentEffect {
	for i, prior := range effects {
		sameIndex := prior.OperationIndex == nil && e.OperationIndex == nil || prior.OperationIndex != nil && e.OperationIndex != nil && *prior.OperationIndex == *e.OperationIndex
		if sameIndex && prior.Operation == e.Operation && prior.Role == e.Role && prior.Scope == e.Scope && prior.Node.ID == e.Node.ID && prior.Node.Raw == e.Node.Raw && prior.State == e.State {
			effects[i].Evidence = mergeAmendmentEvidence(prior.Evidence, e.Evidence)
			return effects
		}
	}
	return append(effects, e)
}

func earlierEventDate(prior string, at time.Time) string {
	if at.IsZero() {
		return prior
	}
	next := at.UTC().Format(time.RFC3339)
	if prior == "" || next < prior {
		return next
	}
	return prior
}

func eventString(m map[string]any, key string) string { value, _ := m[key].(string); return value }
func eventIndex(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, v >= 0
	case int64:
		return int(v), v >= 0
	case uint64:
		return int(v), v <= uint64(^uint(0)>>1)
	case float64:
		return int(v), v >= 0 && float64(int(v)) == v
	}
	return 0, false
}

func uniqueEventIdentity(evs []events.Event, key string) (string, bool) {
	var found string
	for _, ev := range evs {
		value := eventString(ev.After, key)
		if value == "" {
			continue
		}
		if found != "" && found != value {
			return "", true
		}
		found = value
	}
	return found, false
}
func uniqueEventTarget(evs []events.Event) (string, bool) {
	var found string
	for _, ev := range evs {
		if len(ev.Targets) == 0 {
			continue
		}
		if found != "" && found != ev.Targets[0] {
			return "", true
		}
		found = ev.Targets[0]
	}
	return found, false
}

// This is deliberately an identity history, not lifecycle replay. Only add,
// key-rename and remove facts participate. Equal-time facts have no causal
// order: a random EV identifier cannot resolve conflicting key ownership.
type amendmentKeyFact struct {
	at            time.Time
	before, after string
}
type amendmentKeyHistory struct {
	facts    map[string][]amendmentKeyFact
	live     map[string]string
	evidence map[string][]string
}

func amendmentKeyHistoryOf(t *store.Tree, evs []events.Event) amendmentKeyHistory {
	h := amendmentKeyHistory{facts: make(map[string][]amendmentKeyFact), live: make(map[string]string), evidence: make(map[string][]string)}
	for _, n := range t.Epics {
		h.live[n.ID] = amendmentNodeKey(n)
	}
	for _, n := range t.Features {
		h.live[n.ID] = amendmentNodeKey(n)
	}
	for _, n := range t.Tasks {
		h.live[n.ID] = amendmentNodeKey(n)
	}
	var epicRenames []events.Event
	for _, ev := range evs {
		if len(ev.Targets) == 0 {
			continue
		}
		switch {
		case ev.Type == "add.epic" || ev.Type == "add.feature" || ev.Type == "add.task":
			h.evidence[ev.Targets[0]] = mergeAmendmentEvidence(h.evidence[ev.Targets[0]], amendmentEventEvidence([]events.Event{ev}))
			h.facts[ev.Targets[0]] = append(h.facts[ev.Targets[0]], amendmentKeyFact{at: ev.At, after: eventString(ev.After, "key")})
		case strings.HasPrefix(ev.Type, "rename.key.") || ev.Type == "move.feature":
			h.evidence[ev.Targets[0]] = mergeAmendmentEvidence(h.evidence[ev.Targets[0]], amendmentEventEvidence([]events.Event{ev}))
			h.facts[ev.Targets[0]] = append(h.facts[ev.Targets[0]], amendmentKeyFact{ev.At, eventString(ev.Before, "key"), eventString(ev.After, "key")})
			if ev.Type == "rename.key.epic" {
				epicRenames = append(epicRenames, ev)
			}
			if ev.Type == "move.feature" {
				before, after := eventStringMap(ev.Before["task_keys"]), eventStringMap(ev.After["task_keys"])
				for taskID, oldKey := range before {
					if newKey := after[taskID]; newKey != "" {
						h.facts[taskID] = append(h.facts[taskID], amendmentKeyFact{ev.At, oldKey, newKey})
						h.evidence[taskID] = mergeAmendmentEvidence(h.evidence[taskID], amendmentEventEvidence([]events.Event{ev}))
					}
				}
			}
		case strings.HasPrefix(ev.Type, "remove."):
			for _, removed := range removedIdentityRows(ev.Before["removed"]) {
				h.evidence[removed["id"]] = mergeAmendmentEvidence(h.evidence[removed["id"]], amendmentEventEvidence([]events.Event{ev}))
				h.facts[removed["id"]] = append(h.facts[removed["id"]], amendmentKeyFact{at: ev.At, before: removed["key"]})
			}
		}
	}
	// An epic rename also names every child whose canonical key it rewrote.
	// The local feature segment is invariant in that move; use a separate
	// feature identity fact to establish it, never infer it from today's key.
	for _, ev := range epicRenames {
		for _, child := range ev.Targets[1:] {
			if !strings.HasPrefix(child, "F-") {
				continue
			}
			local := ""
			for _, fact := range h.facts[child] {
				for _, key := range []string{fact.before, fact.after} {
					if key != "" {
						candidate := key[strings.LastIndex(key, "/")+1:]
						if local != "" && local != candidate {
							local = "?"
							break
						}
						local = candidate
					}
				}
				if local == "?" {
					break
				}
			}
			if local != "" && local != "?" {
				h.evidence[child] = mergeAmendmentEvidence(h.evidence[child], amendmentEventEvidence([]events.Event{ev}))
				h.facts[child] = append(h.facts[child], amendmentKeyFact{ev.At, eventString(ev.Before, "key") + "/" + local, eventString(ev.After, "key") + "/" + local})
			}
		}
	}
	for nodeID, facts := range h.facts {
		sort.Slice(facts, func(i, j int) bool { return facts[i].at.Before(facts[j].at) })
		h.facts[nodeID] = facts
	}
	return h
}

func (h amendmentKeyHistory) resolve(key string, when time.Time) string {
	if when.IsZero() {
		return ""
	}
	found := ""
	for nodeID, facts := range h.facts {
		current, atQuery, valid := "", "", true
		for i, fact := range facts {
			if fact.at.IsZero() || i > 0 && fact.at.Equal(facts[i-1].at) || fact.before != current {
				valid = false
				break
			}
			if fact.at.Equal(when) && (fact.before == key || fact.after == key) {
				return ""
			}
			current = fact.after
			if fact.at.Before(when) {
				atQuery = current
			}
		}
		if !valid || current != h.live[nodeID] {
			// Contradictory evidence touching this key makes it unknowable,
			// not permission to choose another claimant with a cleaner log.
			for _, fact := range facts {
				if fact.before == key || fact.after == key {
					return ""
				}
			}
			continue
		}
		if atQuery != key {
			continue
		}
		if found != "" && found != nodeID {
			return ""
		}
		found = nodeID
	}
	return found
}

func removedIdentityRows(raw any) []map[string]string {
	var rows []map[string]string
	switch values := raw.(type) {
	case []map[string]string:
		return values
	case []any:
		for _, value := range values {
			row := make(map[string]string)
			switch fields := value.(type) {
			case map[string]any:
				for key, value := range fields {
					if text, ok := value.(string); ok {
						row[key] = text
					}
				}
			case map[string]string:
				row = fields
			}
			if row["id"] != "" && row["key"] != "" {
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func amendmentApplicationIndex(ev events.Event, a *store.Amendment) int {
	found := -1
	for i, op := range a.Operations {
		if !amendmentApplicationMatches(ev, a.ID, i, op) {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

// AddAmendments enriches the same request-local graph with proven identities,
// operation intent, and application evidence. It never synthesizes supersession
// from a proposal; recorded lineage remains the structural extractor's fact.
func AddAmendments(t *store.Tree, index *Index, evs []events.Event, historyError string) {
	facts := AmendmentIdentities(t, evs)
	index.Amendments = facts
	if historyError != "" {
		facts.Warning = "Amendment history could not be loaded; known identity links are shown, but relationships may be incomplete. " + historyError
	}
	if facts.Warning != "" {
		index.AddDiagnostic(Diagnostic{Code: "amendment_history_incomplete", Message: facts.Warning, Facet: "amendments"}, true)
	}
	add := func(amendmentID, targetID string, effect AmendmentEffect) {
		amendment := t.Amendments[amendmentID]
		file, _ := NodePath(t, amendment, "yml")
		source := Source{File: file, Field: "operations", Basis: effect.Operation, Role: effect.Role, State: effect.State, Scope: effect.Scope, Evidence: effect.Evidence, Raw: effect.Node.Raw}
		if effect.OperationIndex != nil {
			source.Operation = *effect.OperationIndex
			source.Field = fmt.Sprintf("operations[%d]", *effect.OperationIndex-1)
		}
		if targetID == "" {
			targetID = effect.Node.ID
			if targetID == "" {
				// Source scope prevents the same reused key in two amendments
				// from claiming a common historical identity.
				targetID = "unresolved:" + amendmentID + ":" + url.PathEscape(effect.Node.Raw)
			}
		}
		if _, exists := index.Vertices[targetID]; !exists {
			kind := referenceNoun(effect.Node.ID)
			if kind == "" {
				kind = "reference"
			}
			index.AddVertex(Vertex{ID: targetID, Kind: kind, Raw: effect.Node.Raw, Unresolved: true})
		}
		if effect.Node.Unresolved && !effect.Node.Pending {
			message := effect.Note
			if message == "" {
				message = "Amendment endpoint cannot be resolved: " + effect.Node.Raw
			}
			index.AddDiagnostic(Diagnostic{Code: "unresolved_amendment_endpoint", Message: message, Facet: "amendments", Source: &source, Subjects: []string{amendmentID, targetID}}, false)
		}
		index.AddEdge("amends", amendmentID, targetID, source)
	}
	for amendmentID, effects := range facts.Effects {
		for _, effect := range effects {
			add(amendmentID, "", effect)
		}
	}
	for nodeID, relations := range facts.Relations {
		for _, relation := range relations {
			for _, effect := range relation.Effects {
				if effect.Node.ID != nodeID {
					add(relation.AmendmentID, nodeID, effect)
				}
			}
		}
	}
}

func amendmentEventEvidence(evs []events.Event) []string {
	var evidence []string
	for _, ev := range evs {
		if ev.ID != "" {
			evidence = append(evidence, ev.ID)
		}
	}
	return mergeAmendmentEvidence(nil, evidence)
}

func mergeAmendmentEvidence(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	evidence := make([]string, 0, len(a)+len(b))
	evidence = append(evidence, a...)
	evidence = append(evidence, b...)
	sort.Strings(evidence)
	unique := evidence[:0]
	for _, value := range evidence {
		if len(unique) == 0 || value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func eventStringMap(raw any) map[string]string {
	out := map[string]string{}
	switch values := raw.(type) {
	case map[string]string:
		return values
	case map[string]any:
		for key, value := range values {
			if text, ok := value.(string); ok {
				out[key] = text
			}
		}
	}
	return out
}

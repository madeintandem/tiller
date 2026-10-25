package views

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
)

type WorkItemsBoard struct {
	Open        []WorkItemCard `json:"open"`
	Closed      []WorkItemCard `json:"closed"`
	Attachments []RelatedNode  `json:"attachments"`
}

type WorkItemCard struct {
	ListItem
	Noun          string       `json:"noun"`
	Ref           string       `json:"ref"`
	Attached      *RelatedNode `json:"attached,omitempty"`
	AttachmentIDs []string     `json:"attachment_ids,omitempty"`
	ClosedAt      string       `json:"closed_at,omitempty"`
	Resolution    string       `json:"resolution,omitempty"`
	Supersedes    []string     `json:"supersedes,omitempty"`
	SupersededBy  string       `json:"superseded_by,omitempty"`
}

type terminalStamp struct {
	At      time.Time
	EventID string
}

func WorkItemsBoardOf(tree *store.Tree, evs []events.Event) WorkItemsBoard {
	board := WorkItemsBoard{
		Open:        []WorkItemCard{},
		Closed:      []WorkItemCard{},
		Attachments: []RelatedNode{},
	}
	stamps := terminalStamps(evs)
	cards := make([]WorkItemCard, 0, len(tree.Bugs)+len(tree.Chores)+len(tree.Tweaks))

	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(tree, noun) {
			card := workItemCardOf(tree, work)
			cards = append(cards, card)
			if !isTerminalWorkStatus(card.Status) {
				board.Open = append(board.Open, card)
				continue
			}
			if stamp, ok := stamps[terminalStampKey(card.Noun, card.ID)]; ok {
				card.ClosedAt = formatTime(stamp.At)
			}
			board.Closed = append(board.Closed, card)
		}
	}

	sort.SliceStable(board.Closed, func(i, j int) bool {
		left, leftOK := stamps[terminalStampKey(board.Closed[i].Noun, board.Closed[i].ID)]
		right, rightOK := stamps[terminalStampKey(board.Closed[j].Noun, board.Closed[j].ID)]
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return false
		}
		if left.At.Equal(right.At) {
			return left.EventID > right.EventID
		}
		return left.At.After(right.At)
	})
	board.Attachments = attachmentOptions(tree, cards)
	return board
}

func AttachedWorkOf(tree *store.Tree, targetID string) []RelatedNode {
	out := []RelatedNode{}
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(tree, noun) {
			if cardMatchesAttachment(workItemCardOf(tree, work), targetID) {
				out = append(out, relatedOf(tree, work))
			}
		}
	}
	return out
}

func OpenAttachedWorkCount(tree *store.Tree, targetID string) int {
	count := 0
	for _, noun := range store.ShelfNouns() {
		for _, work := range orderedShelfNodes(tree, noun) {
			if isTerminalWorkStatus(work.NodeStatus()) {
				continue
			}
			if cardMatchesAttachment(workItemCardOf(tree, work), targetID) {
				count++
			}
		}
	}
	return count
}

func workItemCardOf(tree *store.Tree, work store.WorkNode) WorkItemCard {
	body := work.Body()
	card := WorkItemCard{
		ListItem:      Item(tree, work),
		Noun:          work.Noun(),
		Ref:           gen.Ref(tree, work),
		AttachmentIDs: attachmentIDs(tree, body.AttachedTo),
		Resolution:    body.Resolution,
		Supersedes:    body.Supersedes,
		SupersededBy:  deref(body.SupersededBy),
	}
	if body.AttachedTo != "" {
		attached := relatedByIDs(tree, []string{body.AttachedTo})[0]
		card.Attached = &attached
	}
	return card
}

func attachmentIDs(tree *store.Tree, attachedTo string) []string {
	if attachedTo == "" {
		return nil
	}
	if epic, ok := tree.Epics[attachedTo]; ok {
		return []string{epic.ID}
	}
	if feature, ok := tree.Features[attachedTo]; ok {
		if epic := owningEpic(tree, feature.EpicKey); epic != nil {
			return []string{epic.ID, feature.ID}
		}
		return []string{feature.ID}
	}
	if task, ok := tree.Tasks[attachedTo]; ok {
		if feature, found := tree.Features[task.FeatureID]; found {
			if epic := owningEpic(tree, feature.EpicKey); epic != nil {
				return []string{epic.ID, feature.ID, task.ID}
			}
			return []string{feature.ID, task.ID}
		}
		return []string{task.ID}
	}
	return []string{attachedTo}
}

func attachmentOptions(tree *store.Tree, cards []WorkItemCard) []RelatedNode {
	used := make(map[string]bool)
	for _, card := range cards {
		for _, id := range card.AttachmentIDs {
			used[id] = true
		}
	}

	options := []RelatedNode{}
	order := gen.DependencyOrder(tree)
	for _, epicID := range order.Epics() {
		epic := tree.Epics[epicID]
		if epic == nil {
			continue
		}
		if used[epic.ID] {
			options = append(options, relatedOf(tree, epic))
		}
		for _, featureID := range order.Features(Key(epic)) {
			feature := tree.Features[featureID]
			if feature == nil {
				continue
			}
			if used[feature.ID] {
				options = append(options, relatedOf(tree, feature))
			}
			for _, task := range order.Tasks(feature.ID) {
				if used[task.ID] {
					options = append(options, relatedOf(tree, task))
				}
			}
		}
	}
	return options
}

// shelfTerminalEvent splits an event type that closes shelf work, such as
// sync.bug, cancel.chore, or transfer.tweak, into its verb and shelf noun.
func shelfTerminalEvent(eventType string) (verb, noun string, ok bool) {
	verb, noun, found := strings.Cut(eventType, ".")
	if !found || !store.IsWorkNoun(noun) {
		return "", "", false
	}
	switch verb {
	case "sync", "cancel", "transfer":
		return verb, noun, true
	}
	return "", "", false
}

func terminalStamps(evs []events.Event) map[string]terminalStamp {
	terminal := make([]events.Event, 0, len(evs))
	for _, event := range evs {
		if _, _, ok := shelfTerminalEvent(event.Type); ok {
			terminal = append(terminal, event)
		}
	}
	events.Sort(terminal)

	stamps := make(map[string]terminalStamp)
	for _, event := range terminal {
		verb, noun, _ := shelfTerminalEvent(event.Type)
		if verb == "transfer" {
			// A transfer targets both records, but only the original closes.
			if original, ok := event.After["id"].(string); ok && original != "" {
				stamps[terminalStampKey(noun, original)] = terminalStamp{At: event.At, EventID: event.ID}
			}
			continue
		}
		for _, target := range event.Targets {
			stamps[terminalStampKey(noun, target)] = terminalStamp{At: event.At, EventID: event.ID}
		}
	}
	return stamps
}

func terminalStampKey(noun, targetID string) string {
	return noun + "\x00" + targetID
}

func isTerminalWorkStatus(status string) bool {
	return status == "done" || status == "cancelled"
}

func cardMatchesAttachment(card WorkItemCard, targetID string) bool {
	return slices.Contains(card.AttachmentIDs, targetID)
}

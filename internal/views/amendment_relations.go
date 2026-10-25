package views

import (
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// AmendmentRelation is one retained proposal, not one row per changed child.
type AmendmentRelation struct {
	Amendment   RelatedNode       `json:"amendment"`
	Effects     []AmendmentEffect `json:"effects"`
	ProposedAt  string            `json:"proposed_at,omitempty"`
	ApprovedAt  string            `json:"approved_at,omitempty"`
	WithdrawnAt string            `json:"withdrawn_at,omitempty"`
}

// OperationIndex is the human-facing, one-based operation number. It is absent
// for application evidence that cannot be joined to exactly one retained op.
type AmendmentEffect struct {
	OperationIndex *int         `json:"operation_index,omitempty"`
	Operation      string       `json:"operation"`
	Role           string       `json:"role"`
	Scope          string       `json:"scope"`
	Node           *RelatedNode `json:"node,omitempty"`
	Reference      string       `json:"reference,omitempty"`
	Note           string       `json:"note,omitempty"`
}

type amendmentOperationIdentity struct {
	target, by, supersededBy, created, parent *RelatedNode
	blocks                                    []RelatedNode
	notes                                     []string
}

type amendmentIdentityIndex struct {
	operations map[string][]amendmentOperationIdentity
	relations  map[string][]AmendmentRelation
	warning    string
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

// amendmentViewsOf performs presentation only. Both navigation directions use
// the identities already proven by the shared relationship index.
func amendmentViewsOf(t *store.Tree, facts *relationships.AmendmentIndex) amendmentIdentityIndex {
	out := amendmentIdentityIndex{
		operations: make(map[string][]amendmentOperationIdentity),
		relations:  make(map[string][]AmendmentRelation),
	}
	if facts == nil {
		return out
	}
	out.warning = facts.Warning
	for amendmentID, operations := range facts.Operations {
		rows := make([]amendmentOperationIdentity, len(operations))
		for i, op := range operations {
			row := &rows[i]
			row.target = amendmentEndpointOf(t, op.Target)
			row.by = amendmentEndpointOf(t, op.By)
			row.supersededBy = amendmentEndpointOf(t, op.SupersededBy)
			row.created = amendmentEndpointOf(t, op.Created)
			row.parent = amendmentEndpointOf(t, op.Parent)
			row.notes = op.Notes
			for j := range op.Blocks {
				row.blocks = append(row.blocks, *amendmentEndpointOf(t, &op.Blocks[j]))
			}
		}
		out.operations[amendmentID] = rows
	}
	for nodeID, relations := range facts.Relations {
		rows := make([]AmendmentRelation, 0, len(relations))
		for _, relation := range relations {
			amendment := t.Amendments[relation.AmendmentID]
			row := AmendmentRelation{
				Amendment: relatedOf(t, amendment), ProposedAt: relation.ProposedAt,
				ApprovedAt: relation.ApprovedAt, WithdrawnAt: relation.WithdrawnAt,
			}
			for _, effect := range relation.Effects {
				row.Effects = append(row.Effects, AmendmentEffect{
					OperationIndex: effect.OperationIndex, Operation: effect.Operation,
					Role: effect.Role, Scope: effect.Scope, Node: amendmentEndpointOf(t, effect.Node),
					Reference: effect.Reference, Note: effect.Note,
				})
			}
			rows = append(rows, row)
		}
		out.relations[nodeID] = rows
	}
	return out
}

func amendmentEndpointOf(t *store.Tree, endpoint *relationships.AmendmentEndpoint) *RelatedNode {
	if endpoint == nil {
		return nil
	}
	if endpoint.ID == "" {
		return &RelatedNode{Key: endpoint.Raw, Ref: endpoint.Raw}
	}
	if !endpoint.Unresolved {
		if n, err := resolve.NewResolver(t).Resolve(resolve.Ref{Raw: endpoint.ID}); err == nil {
			row := relatedOf(t, n)
			return &row
		}
	}
	return &RelatedNode{ID: endpoint.ID, Ref: endpoint.ID}
}

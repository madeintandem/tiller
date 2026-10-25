package serve

import (
	"errors"
	"testing"
)

// TestJoinAmendmentValidityErrorsDedupesIdenticalMessages pins
// joinAmendmentValidityErrors directly, the same way
// cli/amendment_cmds_test.go's TestJoinLiveReadErrorsDedupesIdenticalMessages
// pins its own twin joinLiveReadErrors: EvaluateAmendmentPreconditions and
// EvaluateAmendmentSource both open with the SAME loadAmendmentDoc read
// (amendment_validity.go), so a doc-load failure (a corrupt or hand-mangled
// amendment yml) surfaces from both evaluators as the exact same error text.
//
// Reproducing that exact pairing through the real load-then-evaluate flow
// (fillAmendmentValidity, node.go) is not possible the same way the CLI's
// own doc comment on TestJoinLiveReadErrorsDedupesIdenticalMessages already
// states it is not possible there: the tree loads once, at the top of the
// request, through the identical store.Load a hand-mangled doc.id would
// already break — so a fresh loadAmendmentDoc re-read returning a
// MISMATCHED id (the one error text both evaluators can independently
// produce) never has a window to fire, since the node fails to resolve
// before fillAmendmentValidity ever runs. This is the pure join function
// pinned directly instead, unit-level, the identical scope the CLI's own
// test settled for.
func TestJoinAmendmentValidityErrorsDedupesIdenticalMessages(t *testing.T) {
	same := "mutate: A-aaaaaa-x.yml holds amendment A-bbbbbb, not the addressed A-aaaaaa"
	if got := joinAmendmentValidityErrors(errors.New(same), errors.New(same)); got != same {
		t.Errorf("joinAmendmentValidityErrors(same, same) = %q, want the identical failure named once, not twice: %q", got, same)
	}

	if got := joinAmendmentValidityErrors(nil, nil); got != "" {
		t.Errorf("joinAmendmentValidityErrors(nil, nil) = %q, want empty", got)
	}
	if got := joinAmendmentValidityErrors(errors.New("only preconditions failed"), nil); got != "only preconditions failed" {
		t.Errorf("joinAmendmentValidityErrors(err, nil) = %q, want the lone message", got)
	}
	if got := joinAmendmentValidityErrors(nil, errors.New("only source failed")); got != "only source failed" {
		t.Errorf("joinAmendmentValidityErrors(nil, err) = %q, want the lone message", got)
	}
	want := "preconditions broke; source broke"
	if got := joinAmendmentValidityErrors(errors.New("preconditions broke"), errors.New("source broke")); got != want {
		t.Errorf("joinAmendmentValidityErrors(distinct, distinct) = %q, want %q (preconditions first)", got, want)
	}
}

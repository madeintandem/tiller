package e2e

// This file is the soft order's end-to-end arc test (Task 11,
// docs/spec/cli-surface.md): the same built-binary,
// subprocess harness smoke_test.go and concurrency_test.go already use
// (harness, requireOK, buildTiller), driving `tiller order` through render
// (`tiller tree`) and routing (`tiller guide`, `tiller readout orient`) at
// the CLI surface a human or an agent actually runs, rather than
// re-deriving gen.DependencyOrder and views.NextOf in-process the way the
// unit suites already do (gen/order_test.go, views/readout_test.go,
// cli/order_cmd_test.go, cli/order_read_test.go). Its job is not to
// re-prove the precedence stack those pin piecewise; it pins that the built
// binary wires add, order, link, tree, guide, and readout together into the
// one arc a developer or an agent actually runs, end to end.
//
// The fixture is two epics, two features each, no edges at first
// (docs/spec/cli-surface.md). Four states are asserted in sequence, each
// against the exact text or sequence the unit suites already proved in
// isolation:
//
//  1. Baseline: no order recorded, so the tree and the route both read plain
//     key order (epic-a leading).
//  2. `tiller order epic epic-b epic-a` inverts key order: the tree renders
//     epic-b first, and routing follows it to epic-b's own first feature,
//     not the key-first one.
//  3. `tiller order epic --clear` erases the list: the tree and the route
//     both fall back to key order, the contrast case task-7-brief.md's own
//     brief names ("without the soft order the same fixture routes to the
//     key-first one").
//  4. A recorded dependency (epic-a depends on epic-b) plus a soft order
//     that agrees with key order but contradicts that edge: the dependency
//     layer outranks the soft order (docs/spec/cli-surface.md), so the tree and
//     the route both stay on epic-b, and the readout's "Dependency order" line
//     names the contradiction by exact text rather than staying quiet about it.
import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	arcEpicIDRe    = regexp.MustCompile(`E-[a-z0-9]{6}`)
	arcFeatureIDRe = regexp.MustCompile(`F-[a-z0-9]{6}`)
)

// arcTreeNode is one node of `tiller tree --json`'s Nodes/Children
// (gen.TreeNode), narrowed to the fields this test reads.
type arcTreeNode struct {
	Key      string        `json:"key"`
	ID       string        `json:"id"`
	Children []arcTreeNode `json:"children"`
}

// arcTreeEnvelope is `tiller tree --json`'s envelope, narrowed to Nodes.
type arcTreeEnvelope struct {
	Data struct {
		Nodes []arcTreeNode `json:"nodes"`
	} `json:"data"`
}

// arcGuideEnvelope is bare `tiller guide --json`'s envelope: the seam it
// routed to (cli/guide.go's SuggestView). Its Reason is a fixed, generic
// sentence per seam (guide.go's suggestSkill switch), not one naming the
// specific feature Route chose, so this test reads Skill off it for the
// seam-name confirmation and reads the ref-specific claim off the readout
// below instead, which carries Route's own reason verbatim.
type arcGuideEnvelope struct {
	Data struct {
		Skill  string `json:"skill"`
		Reason string `json:"reason"`
	} `json:"data"`
}

// arcReadoutEnvelope is `tiller readout <seam> --json`'s envelope: the
// rendered block, the same text a harness reproduces verbatim
// (cli/readout.go's readoutData). It carries the "Dependency order" line
// (views/readout.go's dependencyOrderCheck) that guide's terse skill/reason
// pair does not.
type arcReadoutEnvelope struct {
	Data struct {
		Readout string `json:"readout"`
	} `json:"data"`
}

func TestOrderArcDrivesTreeRoutingAndOverride(t *testing.T) {
	h := &harness{
		t:    t,
		bin:  buildTiller(t),
		dir:  t.TempDir(),
		home: t.TempDir(),
	}

	h.git("init", "-b", "main")
	requireOK(t, h.tiller("init", "--name", "Order Arc", "--key", "orderarc", "--kind", "greenfield"), "init")
	requireOK(t, h.joinAsOwner("jc", "JC Grubbs"), "join")
	// The readouts below rank for pat, who has recorded nothing, so the list
	// reads plain plan order: jc's own adds would rank by recency instead.
	requireOK(t, h.tiller("add", "contributor", "pat", "--name", "Pat"), "add contributor pat")
	h.git("add", "-A")
	h.git("commit", "-m", "init project")

	addEpicA := h.tiller("add", "epic", "epic-a", "--title", "Epic A", "--contributor", "jc", "--json")
	requireOK(t, addEpicA, "add epic epic-a")
	epicA := arcID(t, arcEpicIDRe, addEpicA.stdout)

	addEpicB := h.tiller("add", "epic", "epic-b", "--title", "Epic B", "--contributor", "jc", "--json")
	requireOK(t, addEpicB, "add epic epic-b")
	epicB := arcID(t, arcEpicIDRe, addEpicB.stdout)

	addFeature := func(key, title string) string {
		res := h.tiller("add", "feature", key, "--title", title, "--contributor", "jc", "--json")
		requireOK(t, res, "add feature "+key)
		return arcID(t, arcFeatureIDRe, res.stdout)
	}
	featureA1 := addFeature("epic-a/one", "Epic A, feature one")
	addFeature("epic-a/two", "Epic A, feature two")
	featureB1 := addFeature("epic-b/one", "Epic B, feature one")
	addFeature("epic-b/two", "Epic B, feature two")

	// 0. Every epic is still draft, so the route's first answer is the
	// decomposition gate, not the frontier (issue #93): no agent move is legal,
	// so bare guide names orient, and every readout lists both approvals in
	// key order, the same list on every moment.
	draftGuide := h.tiller("guide", "--json")
	requireOK(t, draftGuide, "guide all-draft")
	require.Equal(t, "orient", arcGuide(t, draftGuide.stdout).Data.Skill)
	require.Equal(t, "No agent move is legal right now; the moves above are yours.", arcGuide(t, draftGuide.stdout).Data.Reason)

	draftDecompose := h.tillerAs("pat", "readout", "decompose", "--json")
	requireOK(t, draftDecompose, "readout decompose all-draft")
	draftNext := arcNext(t, arcReadout(t, draftDecompose.stdout).Data.Readout)
	require.True(t, strings.HasPrefix(draftNext,
		fmt.Sprintf("1. You: approve epic epic-a (%s).", epicA)), draftNext)
	require.Contains(t, draftNext, fmt.Sprintf("\n2. You: approve epic epic-b (%s).", epicB))

	draftOrient := h.tillerAs("pat", "readout", "orient", "--json")
	requireOK(t, draftOrient, "readout orient all-draft")
	require.Equal(t, draftNext, arcNext(t, arcReadout(t, draftOrient.stdout).Data.Readout))

	// The human approves both decompositions; every state below reads the
	// frontier under approved epics.
	requireOK(t, h.tiller("approve", "epic", "epic-a", "epic-b", "--contributor", "jc"), "approve epics")

	// 1. Baseline: no order recorded anywhere, so the tree and the route both
	// read plain key order, epic-a leading.
	baseline := h.tiller("tree", "--json")
	requireOK(t, baseline, "tree baseline")
	baselineTree := arcTree(t, baseline.stdout)
	require.Equal(t, []string{"epic-a", "epic-b"}, arcTopKeys(baselineTree))
	require.Equal(t, []string{"epic-a/one", "epic-a/two"}, arcChildKeys(t, baselineTree, "epic-a"))
	require.Equal(t, []string{"epic-b/one", "epic-b/two"}, arcChildKeys(t, baselineTree, "epic-b"))

	baseGuide := h.tiller("guide", "--json")
	requireOK(t, baseGuide, "guide baseline")
	require.Equal(t, "specify", arcGuide(t, baseGuide.stdout).Data.Skill)

	baseReadout := h.tillerAs("pat", "readout", "orient", "--json")
	requireOK(t, baseReadout, "readout orient baseline")
	require.True(t, strings.HasPrefix(arcNext(t, arcReadout(t, baseReadout.stdout).Data.Readout),
		fmt.Sprintf("1. Agent: specify epic-a/one (%s).", featureA1)),
		"with no soft order recorded, the list reads plain key order, epic-a's feature first")

	// 2. Invert key order: epic-b leads.
	requireOK(t, h.tiller("order", "epic", "epic-b", "epic-a", "--contributor", "jc"), "order epic invert")

	inverted := h.tiller("tree", "--json")
	requireOK(t, inverted, "tree inverted")
	require.Equal(t, []string{"epic-b", "epic-a"}, arcTopKeys(arcTree(t, inverted.stdout)),
		"the soft order inverts the render, epic-b leading")

	invertedHuman := h.tiller("tree")
	requireOK(t, invertedHuman, "tree inverted human")
	bPos := strings.Index(invertedHuman.stdout, "epic-b (")
	aPos := strings.Index(invertedHuman.stdout, "epic-a (")
	require.True(t, bPos >= 0 && aPos >= 0 && bPos < aPos,
		"tiller tree renders epic-b ahead of epic-a once the soft order inverts key order:\n%s", invertedHuman.stdout)

	invertedGuide := h.tiller("guide", "--json")
	requireOK(t, invertedGuide, "guide inverted")
	require.Equal(t, "specify", arcGuide(t, invertedGuide.stdout).Data.Skill)

	invertedReadout := h.tillerAs("pat", "readout", "orient", "--json")
	requireOK(t, invertedReadout, "readout orient inverted")
	ir := arcReadout(t, invertedReadout.stdout)
	require.NotContains(t, ir.Data.Readout, "- Route:", "orient carries no Route check; Next is the answer")
	require.True(t, strings.HasPrefix(arcNext(t, ir.Data.Readout),
		fmt.Sprintf("1. Agent: specify epic-b/one (%s).", featureB1)))
	require.NotContains(t, ir.Data.Readout, "is overridden by recorded dependencies",
		"the render matches the recorded soft order exactly, so nothing is overridden")

	// 3. --clear erases the recorded list; key order returns, the contrast
	// case task-7-brief.md's own brief names.
	requireOK(t, h.tiller("order", "epic", "--clear", "--contributor", "jc"), "order epic clear")

	cleared := h.tiller("tree", "--json")
	requireOK(t, cleared, "tree cleared")
	require.Equal(t, []string{"epic-a", "epic-b"}, arcTopKeys(arcTree(t, cleared.stdout)),
		"clearing the recorded order restores key order")

	clearedGuide := h.tiller("guide", "--json")
	requireOK(t, clearedGuide, "guide cleared")
	require.Equal(t, "specify", arcGuide(t, clearedGuide.stdout).Data.Skill)

	clearedReadout := h.tillerAs("pat", "readout", "orient", "--json")
	requireOK(t, clearedReadout, "readout orient cleared")
	require.True(t, strings.HasPrefix(arcNext(t, arcReadout(t, clearedReadout.stdout).Data.Readout),
		fmt.Sprintf("1. Agent: specify epic-a/one (%s).", featureA1)),
		"with the soft order cleared, the list falls back to key order, epic-a's feature first")

	// 4. Record a dependency (epic-a depends on epic-b), then re-record a
	// soft order that agrees with key order but contradicts that edge: the
	// layer the edge builds always wins, so the render and the route both
	// stay on epic-b, and the readout names the contradiction.
	requireOK(t, h.tiller("link", "epic", "epic-a", "--depends-on", "epic-b", "--contributor", "jc"),
		"link epic-a depends-on epic-b")
	requireOK(t, h.tiller("order", "epic", "epic-a", "epic-b", "--contributor", "jc"),
		"order epic contradicting the recorded edge")

	overriddenTree := h.tiller("tree", "--json")
	requireOK(t, overriddenTree, "tree overridden")
	require.Equal(t, []string{"epic-b", "epic-a"}, arcTopKeys(arcTree(t, overriddenTree.stdout)),
		"the recorded edge forces epic-b first regardless of what the soft order now asks for")

	overriddenGuide := h.tiller("guide", "--json")
	requireOK(t, overriddenGuide, "guide overridden")
	require.Equal(t, "specify", arcGuide(t, overriddenGuide.stdout).Data.Skill)

	overriddenReadout := h.tillerAs("pat", "readout", "orient", "--json")
	requireOK(t, overriddenReadout, "readout orient overridden")
	or := arcReadout(t, overriddenReadout.stdout)
	require.Contains(t, or.Data.Readout,
		fmt.Sprintf("- Dependency order: soft order for epic-a (%s) and epic-b (%s) is overridden by recorded dependencies",
			epicA, epicB),
		"the readout names the override rather than staying quiet about it")
	// The list still honors the recorded edge, not the soft order that now
	// contradicts it: it still leads with epic-b's feature, unchanged from
	// state 2's soft-order-driven answer.
	require.True(t, strings.HasPrefix(arcNext(t, or.Data.Readout),
		fmt.Sprintf("1. Agent: specify epic-b/one (%s).", featureB1)))
}

// arcNext is a rendered block's ranked list, everything under **Next:**.
func arcNext(t *testing.T, readout string) string {
	t.Helper()
	_, next, found := strings.Cut(readout, "**Next:**\n")
	require.True(t, found, readout)
	return next
}

// arcID extracts the one id re matches out of a command's --json output,
// failing the test if none is found.
func arcID(t *testing.T, re *regexp.Regexp, out string) string {
	t.Helper()
	id := re.FindString(out)
	require.NotEmptyf(t, id, "no id matching %s in: %s", re, out)
	return id
}

// arcTree decodes `tiller tree --json`'s envelope.
func arcTree(t *testing.T, jsonText string) arcTreeEnvelope {
	t.Helper()
	var env arcTreeEnvelope
	require.NoError(t, json.Unmarshal([]byte(jsonText), &env), "tree --json must parse")
	return env
}

// arcTopKeys reads the top-level node keys off an already-decoded tree, in
// the rendered sequence.
func arcTopKeys(env arcTreeEnvelope) []string {
	keys := make([]string, len(env.Data.Nodes))
	for i, n := range env.Data.Nodes {
		keys[i] = n.Key
	}
	return keys
}

// arcChildKeys reads one top-level node's children keys, in the rendered
// sequence, failing the test if parentKey is not a top-level node.
func arcChildKeys(t *testing.T, env arcTreeEnvelope, parentKey string) []string {
	t.Helper()
	for _, n := range env.Data.Nodes {
		if n.Key != parentKey {
			continue
		}
		keys := make([]string, len(n.Children))
		for i, c := range n.Children {
			keys[i] = c.Key
		}
		return keys
	}
	require.Failf(t, "parent not found", "key %q not among the top-level tree nodes", parentKey)
	return nil
}

// arcGuide decodes bare `tiller guide --json`'s envelope.
func arcGuide(t *testing.T, jsonText string) arcGuideEnvelope {
	t.Helper()
	var env arcGuideEnvelope
	require.NoError(t, json.Unmarshal([]byte(jsonText), &env), "guide --json must parse")
	return env
}

// arcReadout decodes `tiller readout <seam> --json`'s envelope.
func arcReadout(t *testing.T, jsonText string) arcReadoutEnvelope {
	t.Helper()
	var env arcReadoutEnvelope
	require.NoError(t, json.Unmarshal([]byte(jsonText), &env), "readout --json must parse")
	return env
}

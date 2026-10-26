package views_test

// These tests pin the action queue (ui-surface.md, "The action queue"; plan
// 2026-07-05, serve pass 1 lane B): every kind fires from a fixture driven
// through the real mutations, the payload is the exact JSON shape the console
// consumes, and the fixed group order plus within-group ordering hold. The one
// deliberate shortcut: needs_attestation criteria are written directly into
// the node yml (a feature.yml or a task.yml gates block) because
// driving them through eval/check needs a git repo and a rubric run, which is
// the eval machinery's own test surface, not the queue's.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const queueManifest = `schema_version: 12
schema_epoch: initial
project:
  name: Queue Test
  key: queuetest
  kind: greenfield
contributors:
  - { id: jc, name: JC Grubbs }
gates:
  optional:
    code_review:          { kind: attestation, on: pr }
    security_review:      { kind: attestation, on: pr }
    performance_review:   { kind: attestation, on: pr }
    accessibility_review: { kind: attestation, on: pr }
`

// queueClock is a fixed clock so every recorded event is deterministic.
type queueClock struct{ t time.Time }

func (c queueClock) Now() time.Time { return c.t }

// queueMint mints sequential per-prefix ids (E-aaaaaa, E-bbbbbb, ...) so the
// fixture's ids, and therefore the queue's refs and commands, are pinnable.
func queueMint() func(string) string {
	counts := map[string]int{}
	return func(prefix string) string {
		i := counts[prefix]
		counts[prefix]++
		return prefix + "-" + strings.Repeat(string(rune('a'+i)), 6)
	}
}

func queueDeps(mint func(string) string) mutate.Deps {
	return mutate.Deps{
		Contributor: "jc",
		Clock:       queueClock{t: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)},
		Mint:        mint,
	}
}

func queueLoad(t *testing.T, root string) *store.Tree {
	t.Helper()
	tree, err := store.Load(root)
	require.NoError(t, err)
	return tree
}

func queueResolve(t *testing.T, tree *store.Tree, noun, raw string) store.Node {
	t.Helper()
	n, err := resolve.NewResolver(tree).Resolve(resolve.Ref{Noun: noun, Raw: raw})
	require.NoError(t, err)
	return n
}

// queueWriteRealSpec replaces a feature's stub feature.md and writes a valid
// feature.eval.md so `spec feature` validates.
func queueWriteRealSpec(t *testing.T, root, epicKey, featKey, featureID string) {
	t.Helper()
	dir := filepath.Join(root, ".tiller", "epics", epicKey, "features", featKey)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.md"),
		[]byte("# Feature\n\nReal spec prose for the queue fixture.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.eval.md"),
		[]byte("---\nnode: "+featureID+"\ncriteria:\n  - id: C1\n    statement: It works\n    polarity: introduce\n    binding: { type: test, ref: spec/works_spec.rb }\n---\n\n# Acceptance\n"), 0o644))
}

// queueWriteTaskSpec writes a real implementation spec beside a task.yml, the
// non-stub task.md `spec feature` requires of the explosion.
func queueWriteTaskSpec(t *testing.T, taskYML string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(taskYML), store.TaskProseFile),
		[]byte("# Task\n\n## Approach\n\nReal implementation spec for the queue fixture.\n"), 0o644))
}

// queueSetGates writes a gates block carrying evals criteria straight into a
// node yml through a generic yaml round-trip, preserving every other field.
func queueSetGates(t *testing.T, path string, criteria map[string]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	crit := map[string]any{}
	for id, status := range criteria {
		crit[id] = map[string]any{"status": status}
	}
	doc["gates"] = map[string]any{
		"evals": map[string]any{"required": true, "kind": "command", "status": "fail", "criteria": crit},
	}
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// queueTaskYML is one task's task.yml under a feature, the file the task-level
// shortcuts below rewrite.
func queueTaskYML(root, epicKey, featureKey, taskKey string) string {
	return filepath.Join(root, store.TaskRelDir(epicKey, featureKey, taskKey), store.TaskStateFile)
}

// queueReadTask and queueWriteTask are the read/modify/write pair every
// task-level shortcut below shares.
func queueReadTask(t *testing.T, path string) *store.Task {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tk store.Task
	require.NoError(t, yaml.Unmarshal(data, &tk))
	return &tk
}

func queueWriteTask(t *testing.T, path string, tk *store.Task) {
	t.Helper()
	if tk.Status == "in_progress" || tk.Status == "check" || tk.Status == "in_review" || tk.Status == "done" {
		tk.Started = true
		if tk.Owner == "" {
			tk.Owner = "jc"
		}
	}
	out, err := yaml.Marshal(tk)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

// queueSetTaskGates writes evals criteria onto one task's task.yml.
func queueSetTaskGates(t *testing.T, path string, criteria map[string]string) {
	t.Helper()
	tk := queueReadTask(t, path)
	crit := map[string]store.Criterion{}
	for id, status := range criteria {
		crit[id] = store.Criterion{Status: status}
	}
	tk.Gates = map[string]store.Gate{
		"evals": {Required: true, Kind: "command", Status: "fail", Criteria: crit},
	}
	queueWriteTask(t, path, tk)
}

// queueSetTaskPRReview rewrites one task to the in_review state with a PR
// number, the shortcut for staging a PR-backed task at the review gate without
// driving the submit path (which needs a git remote).
func queueSetTaskPRReview(t *testing.T, path string, pr int) {
	t.Helper()
	tk := queueReadTask(t, path)
	tk.Status = "in_review"
	tk.PR = &pr
	queueWriteTask(t, path, tk)
}

// queueSetTaskInReview rewrites one task to the in_review state on an offline
// review_ref (a branch and head commit, the no-remote fallback), the shortcut
// for staging a task at the review gate.
func queueSetTaskInReview(t *testing.T, path string) {
	t.Helper()
	tk := queueReadTask(t, path)
	tk.Status = "in_review"
	tk.ReviewRef = &store.ReviewRef{
		Branch: "tiller/schema-engine",
		Head:   store.RevisionRef{Kind: "git_commit", Value: "git:abc1234"},
	}
	queueWriteTask(t, path, tk)
}

// queueSetTaskGateStatuses writes stored optional-gate statuses onto one task,
// the documented shortcut standing in for a recorded `tiller check` run so the
// queue can be pinned across the pass/fail/skipped/stale statuses without a git
// repo.
func queueSetTaskGateStatuses(t *testing.T, path string, statuses map[string]string) {
	t.Helper()
	tk := queueReadTask(t, path)
	gates := map[string]store.Gate{}
	for name, status := range statuses {
		gates[name] = store.Gate{Kind: store.GateKindAttestation, Status: status}
	}
	tk.Gates = gates
	queueWriteTask(t, path, tk)
}

// queueSetStatusOnDisk rewrites a node yml's status line, the cheap way to
// stage a done child feature without driving the merge-observed sync path.
func queueSetStatusOnDisk(t *testing.T, path, from, to string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := strings.Replace(string(data), "status: "+from, "status: "+to, 1)
	require.NotEqual(t, string(data), out, "expected a status: %s line to rewrite in %s", from, path)
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// queueFixture drives a repo through the real mutations until every queue kind
// fires (two for answer_question and attest_criterion, eight for attest_gate,
// whose task-before-shelf and within-node ordering need more than one entry):
//
//	approve_epic      form-engine left draft (checkout is approved to ready, and
//	                  a ready epic queues nothing: its done is a rollup)
//	approve_feature   form-engine/schema-engine spec'd to specced
//	approve_tasks     two proposed tasks added under form-engine/review-me AFTER
//	                  it passed its plan gate, batched into one feature-grain
//	                  entry (no per-task rows). schema-engine's own three tasks
//	                  are the explosion that rides its feature's approve, so they
//	                  queue nothing of their own.
//	approve_bug       one proposed bug awaiting triage
//	approve_chore     one proposed chore awaiting triage
//	answer_question   one undated question added first, one dated added second
//	carrier_cancelled one ready bug whose recorded carrier is absent
//	attest_gate       an in-review task and chore, each carrying four optional
//	                  on:pr gates, task first and gate-sorted within each node
//	attest_criterion  needs_attestation written into audit-export's feature.yml
//	                  and the task's ledger entry (see the file comment)
func queueFixture(t *testing.T) *store.Tree {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(queueManifest), 0o644))
	deps := queueDeps(queueMint())

	// approve_feature: E-aaaaaa, F-aaaaaa spec'd to specced.
	_, err := mutate.AddEpic(queueLoad(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)
	queueWriteRealSpec(t, root, "form-engine", "schema-engine", "F-aaaaaa")

	// The task explosion is part of the stage-2 output `spec feature` validates,
	// so schema-engine's three tasks (T-aaaaaa, T-bbbbbb, T-cccccc) go down
	// before the gate. They ride the feature's own approve, so they queue no row
	// of their own; the approve_tasks row below comes from a mid-flight addition.
	// T-aaaaaa additionally carries a needs_attestation criterion, so it
	// double-serves attest_criterion.
	for _, tk := range []struct{ local, title string }{
		{"schema-loader", "Wire the schema loader"},
		{"field-parser", "Add the field parser"},
		{"edge-cases", "Cover the edge cases"},
	} {
		_, err = mutate.AddTask(queueLoad(t, root), deps, mutate.TaskInput{
			Key: "form-engine/schema-engine/" + tk.local, Title: tk.title,
		})
		require.NoError(t, err)
		queueWriteTaskSpec(t, queueTaskYML(root, "form-engine", "schema-engine", tk.local))
	}

	tree := queueLoad(t, root)
	_, err = mutate.Transition(tree, deps, queueResolve(t, tree, store.NounFeature, "form-engine/schema-engine"), mutate.TransitionInput{Verb: "spec"})
	require.NoError(t, err)

	// attest_criterion: F-bbbbbb carries one needs_attestation criterion (and
	// one already passed, which must not queue); the task carries another.
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "form-engine/audit-export", Title: "Audit export", ChangeType: "create"})
	require.NoError(t, err)
	queueSetGates(t, filepath.Join(root, ".tiller", "epics", "form-engine", "features", "audit-export", "feature.yml"),
		map[string]string{"C1": store.CriterionStatusNeedsAttestation, "C2": store.CriterionStatusPass})
	queueSetTaskGates(t, queueTaskYML(root, "form-engine", "schema-engine", "schema-loader"),
		map[string]string{"TC1": store.CriterionStatusNeedsAttestation})

	// A ready epic whose rollup is complete: checkout approved to ready, cart
	// done on disk and promo cancelled on disk. It queues NOTHING, and the
	// golden below is where that is proven: epic done is a rollup sync records,
	// so this epic needs the human for nothing (artifact-schema.md, Epics
	// States). Before the rollup landed this was the fixture's accept_epic row.
	_, err = mutate.AddEpic(queueLoad(t, root), deps, mutate.EpicInput{Key: "checkout", Title: "Checkout"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "checkout/cart", Title: "Cart", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "checkout/promo", Title: "Promo codes", ChangeType: "create"})
	require.NoError(t, err)
	queueSetStatusOnDisk(t, filepath.Join(root, ".tiller", "epics", "checkout", "features", "cart", "feature.yml"), "draft", "done")
	queueSetStatusOnDisk(t, filepath.Join(root, ".tiller", "epics", "checkout", "features", "promo", "feature.yml"), "draft", "cancelled")
	tree = queueLoad(t, root)
	_, err = mutate.Transition(tree, deps, queueResolve(t, tree, store.NounEpic, "checkout"), mutate.TransitionInput{Verb: "approve"})
	require.NoError(t, err)

	// attest_gate: T-dddddd, a PR-backed task under form-engine/review-me, driven
	// to in_review on disk. The manifest's four optional attestation gates are all
	// on:pr, which is the task grain now that a PR always represents a task, so
	// every one applies; none is recorded, so all four queue pending, sorted by
	// gate name. The submit path that records a real PR needs a git remote, so the
	// review state is written straight to disk (the same shortcut the stored-gate
	// fixtures use).
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "form-engine/review-me", Title: "Review me", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(queueLoad(t, root), deps, mutate.TaskInput{Key: "form-engine/review-me/review-me", Title: "Review me"})
	require.NoError(t, err)
	queueSetTaskPRReview(t, queueTaskYML(root, "form-engine", "review-me", "review-me"), 42)

	// approve_tasks: review-me is past its plan gate (active on disk), so the
	// tasks added under it afterwards are the human's own decision and the queue
	// offers the feature-grain batch command once (cli-surface.md, "Specifying a
	// feature": the initial set rides the feature's approve, a mid-flight
	// addition is approved individually). TWO of them, so the golden below shows
	// one entry per feature rather than one per task, which is this group's
	// defining property.
	queueSetStatusOnDisk(t, filepath.Join(root, ".tiller", "epics", "form-engine", "features", "review-me", "feature.yml"), "draft", "active")
	for _, local := range []string{"late-idea", "later-idea"} {
		_, err = mutate.AddTask(queueLoad(t, root), deps, mutate.TaskInput{
			Key: "form-engine/review-me/" + local, Title: "Idea " + local,
		})
		require.NoError(t, err)
	}

	// answer_question: the undated question minted first (Q-aaaaaa), the dated
	// one second (Q-bbbbbb), so due-first ordering visibly beats id order.
	_, err = mutate.AddQuestion(queueLoad(t, root), deps, mutate.QuestionInput{Title: "Currency rounding", Statement: "Which currency rounding applies?"})
	require.NoError(t, err)
	_, err = mutate.AddQuestion(queueLoad(t, root), deps, mutate.QuestionInput{
		Title:     "Canonical schema version",
		Statement: "Which schema version is canonical?",
		Due:       store.QuestionDue{Date: "2026-07-10"},
	})
	require.NoError(t, err)

	// Shelf groups: one proposed bug and chore for triage, one ready bug whose
	// carrier is gone, and one in-review chore whose on:pr gates must follow the
	// task's gates. The status and carrier shortcuts are projection facts, like
	// the gate-status shortcuts above; add itself still goes through mutation.
	_, err = mutate.AddWorkItem(queueLoad(t, root), deps, mutate.WorkItemInput{
		Noun: store.NounBug, Key: "triage-bug", Title: "Triage bug",
	})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(queueLoad(t, root), deps, mutate.WorkItemInput{
		Noun: store.NounChore, Key: "triage-chore", Title: "Triage chore",
	})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(queueLoad(t, root), deps, mutate.WorkItemInput{
		Noun: store.NounBug, Key: "lost-carrier", Title: "Lost carrier",
	})
	require.NoError(t, err)
	_, err = mutate.AddWorkItem(queueLoad(t, root), deps, mutate.WorkItemInput{
		Noun: store.NounChore, Key: "review-chore", Title: "Review chore",
	})
	require.NoError(t, err)

	tree = queueLoad(t, root)
	tree.Bugs["B-bbbbbb"].Status = "ready"
	tree.Bugs["B-bbbbbb"].ResolvedBy = "F-gone00"
	tree.Chores["CH-bbbbbb"].Status = "in_review"
	return tree

}

// TestQueueOfPinsEveryKindAndOrder is the payload golden: every kind fires,
// the groups come in fixed order, including bug before chore approval,
// carrier-cancelled after questions, and shelf attestations after task
// attestations. Each group remains stable within itself. The ready checkout
// epic with its rollup complete contributes no row at all: there is no
// epic-acceptance kind.
func TestQueueOfPinsEveryKindAndOrder(t *testing.T) {
	tree := queueFixture(t)

	got, err := json.Marshal(views.QueueOf(tree))
	require.NoError(t, err)

	want := `{
	  "entries": [
	    {"kind":"approve_epic","noun":"epic","id":"E-aaaaaa","key":"form-engine","ref":"form-engine (E-aaaaaa)","prose":"Form Engine","command":"tiller approve epic form-engine"},
	    {"kind":"approve_feature","noun":"feature","id":"F-aaaaaa","key":"form-engine/schema-engine","ref":"form-engine/schema-engine (F-aaaaaa)","prose":"Schema engine","command":"tiller approve feature form-engine/schema-engine"},
	    {"kind":"approve_tasks","noun":"feature","id":"F-eeeeee","key":"form-engine/review-me","ref":"form-engine/review-me (F-eeeeee)","prose":"Review me","command":"tiller approve task --feature form-engine/review-me"},
	    {"kind":"approve_bug","noun":"bug","id":"B-aaaaaa","key":"bugs/triage-bug","ref":"bugs/triage-bug (B-aaaaaa)","prose":"Triage bug","command":"tiller approve bug bugs/triage-bug"},
	    {"kind":"approve_chore","noun":"chore","id":"CH-aaaaaa","key":"chores/triage-chore","ref":"chores/triage-chore (CH-aaaaaa)","prose":"Triage chore","command":"tiller approve chore chores/triage-chore"},
	    {"kind":"answer_question","noun":"question","id":"Q-bbbbbb","ref":"Q-bbbbbb","prose":"Canonical schema version","command":"tiller answer question Q-bbbbbb --answer \"...\"","due":"2026-07-10"},
	    {"kind":"answer_question","noun":"question","id":"Q-aaaaaa","ref":"Q-aaaaaa","prose":"Currency rounding","command":"tiller answer question Q-aaaaaa --answer \"...\""},
	    {"kind":"carrier_cancelled","noun":"bug","id":"B-bbbbbb","key":"bugs/lost-carrier","ref":"bugs/lost-carrier (B-bbbbbb)","prose":"Lost carrier","command":"tiller resolve bug bugs/lost-carrier --clear"},
	    {"kind":"attest_gate","noun":"task","id":"T-dddddd","key":"form-engine/review-me/review-me","ref":"form-engine/review-me/review-me (T-dddddd)","prose":"Review me","command":"tiller attest task form-engine/review-me/review-me --gate accessibility_review --status pass","gate":"accessibility_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"task","id":"T-dddddd","key":"form-engine/review-me/review-me","ref":"form-engine/review-me/review-me (T-dddddd)","prose":"Review me","command":"tiller attest task form-engine/review-me/review-me --gate code_review --status pass","gate":"code_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"task","id":"T-dddddd","key":"form-engine/review-me/review-me","ref":"form-engine/review-me/review-me (T-dddddd)","prose":"Review me","command":"tiller attest task form-engine/review-me/review-me --gate performance_review --status pass","gate":"performance_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"task","id":"T-dddddd","key":"form-engine/review-me/review-me","ref":"form-engine/review-me/review-me (T-dddddd)","prose":"Review me","command":"tiller attest task form-engine/review-me/review-me --gate security_review --status pass","gate":"security_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"chore","id":"CH-bbbbbb","key":"chores/review-chore","ref":"chores/review-chore (CH-bbbbbb)","prose":"Review chore","command":"tiller attest chore chores/review-chore --gate accessibility_review --status pass","gate":"accessibility_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"chore","id":"CH-bbbbbb","key":"chores/review-chore","ref":"chores/review-chore (CH-bbbbbb)","prose":"Review chore","command":"tiller attest chore chores/review-chore --gate code_review --status pass","gate":"code_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"chore","id":"CH-bbbbbb","key":"chores/review-chore","ref":"chores/review-chore (CH-bbbbbb)","prose":"Review chore","command":"tiller attest chore chores/review-chore --gate performance_review --status pass","gate":"performance_review","gate_status":"pending"},
	    {"kind":"attest_gate","noun":"chore","id":"CH-bbbbbb","key":"chores/review-chore","ref":"chores/review-chore (CH-bbbbbb)","prose":"Review chore","command":"tiller attest chore chores/review-chore --gate security_review --status pass","gate":"security_review","gate_status":"pending"},
	    {"kind":"attest_criterion","noun":"feature","id":"F-bbbbbb","key":"form-engine/audit-export","ref":"form-engine/audit-export (F-bbbbbb)","prose":"C1","command":"tiller attest feature form-engine/audit-export --criterion C1 --status pass"},
	    {"kind":"attest_criterion","noun":"task","id":"T-aaaaaa","key":"form-engine/schema-engine/schema-loader","ref":"form-engine/schema-engine/schema-loader (T-aaaaaa)","prose":"TC1","command":"tiller attest task form-engine/schema-engine/schema-loader --criterion TC1 --status pass"}
	  ]
	}`
	require.JSONEq(t, want, string(got))

	// The contract change: per-task approve rows are gone; a feature with
	// proposed tasks batches into one approve_tasks entry, and no approve_task
	// kind survives anywhere in the payload.
	entries := views.QueueOf(tree).Entries
	for _, e := range entries {
		require.NotEqual(t, "approve_task", e.Kind, "per-task approve_task rows were replaced by feature-grain approve_tasks")
		require.NotEqual(t, "accept_epic", e.Kind, "epic done is a rollup sync records; no human waits on it")
	}
	// The defining property of that group, stated over the fixture that can show
	// it: review-me carries TWO proposed tasks and contributes ONE row.
	require.Len(t, queueEntryKeys(entries, "approve_tasks"), 1,
		"one approve_tasks entry per feature, not one per proposed task")
	proposed := 0
	for _, tk := range tree.Tasks {
		if tk.Status == "proposed" && tk.FeatureID == featureIDByKeyQ(t, tree, "form-engine/review-me") {
			proposed++
		}
	}
	require.Equal(t, 2, proposed, "the one row stands for both of review-me's proposed tasks")
}

func TestQueueQuestionsUsePersistedAndBoundedLegacyTitles(t *testing.T) {
	titledStatement := strings.Repeat("This is the full titled question statement. ", 6)
	legacyStatement := strings.Repeat("界", 81)
	tree := &store.Tree{Questions: map[string]*store.Question{
		"Q-titled": {
			ID: "Q-titled", Title: "Canonical schema", Statement: titledStatement, Status: "open",
		},
		"Q-legacy": {
			ID: "Q-legacy", Statement: legacyStatement, Status: "open",
		},
	}}

	entries := entriesOfKind(views.QueueOf(tree), "answer_question")
	require.Len(t, entries, 2)
	byID := map[string]views.QueueEntry{}
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	require.Equal(t, "Canonical schema", byID["Q-titled"].Prose)
	require.Equal(t, strings.Repeat("界", 79)+"…", byID["Q-legacy"].Prose)
	require.NotContains(t, byID["Q-titled"].Prose, titledStatement)
	require.NotContains(t, byID["Q-legacy"].Prose, legacyStatement)
}

// featureIDByKeyQ resolves a feature id from its canonical key in an in-memory
// tree, so an assertion can name the feature it means without the minted id.
func featureIDByKeyQ(t *testing.T, tree *store.Tree, key string) string {
	t.Helper()
	for id, f := range tree.Features {
		if f.Key != nil && *f.Key == key {
			return id
		}
	}
	t.Fatalf("no feature keyed %s", key)
	return ""
}

// TestQueueIsAlwaysAnArray pins the empty shape: no gates waiting renders an
// empty entries array, never null.
func TestQueueIsAlwaysAnArray(t *testing.T) {
	tree := &store.Tree{Manifest: &store.Manifest{}}
	got, err := json.Marshal(views.QueueOf(tree))
	require.NoError(t, err)
	require.JSONEq(t, `{"entries":[]}`, string(got))
}

// TestQueueApproveAmendmentsListsOnlyProposedNoSuppression pins the new
// approve_amendment group: every proposed amendment
// queues, id-ascending (an amendment carries no dependency edges among
// itself and any other amendment to rank by), and nothing suppresses a row
// the way a retired ancestor suppresses approve_feature and
// approve_tasks: an amendment has no ancestor at all to be held under.
func TestQueueApproveAmendmentsListsOnlyProposedNoSuppression(t *testing.T) {
	keyLater, keyFirst := "later-reshape", "iphone-only"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Amendments: map[string]*store.Amendment{
			"A-bbbbbb": {ID: "A-bbbbbb", Key: &keyLater, Title: "Later reshape", Status: store.AmendmentStatusProposed},
			"A-aaaaaa": {ID: "A-aaaaaa", Key: &keyFirst, Title: "Remove iPad", Status: store.AmendmentStatusProposed},
			"A-cccccc": {ID: "A-cccccc", Status: store.AmendmentStatusWithdrawn},
			"A-dddddd": {ID: "A-dddddd", Status: store.AmendmentStatusApproved},
		},
	}

	var got []views.QueueEntry
	for _, e := range views.QueueOf(tree).Entries {
		if e.Kind == "approve_amendment" {
			got = append(got, e)
		}
	}

	require.Len(t, got, 2, "only the two proposed amendments queue")
	require.Equal(t, "A-aaaaaa", got[0].ID, "id order, not key order")
	require.Equal(t, "A-bbbbbb", got[1].ID)
	require.Equal(t, store.NounAmendment, got[0].Noun)
	require.Equal(t, "iphone-only (A-aaaaaa)", got[0].Ref)
	require.Equal(t, "Remove iPad", got[0].Prose)
	require.Equal(t, "tiller approve amendment iphone-only", got[0].Command)
}

// TestQueueApproveAmendmentsLeadTheGroupOrder pins where the new group sits
// among the others: first, mirroring Route's own priority (a proposed
// amendment outranks even the plan gate there), so the
// queue's group order does not disagree with it.
func TestQueueApproveAmendmentsLeadTheGroupOrder(t *testing.T) {
	tree := queueOrderEpicsTree()
	key := "iphone-only"
	tree.Amendments = map[string]*store.Amendment{
		"A-aaaaaa": {ID: "A-aaaaaa", Key: &key, Status: store.AmendmentStatusProposed},
	}

	entries := views.QueueOf(tree).Entries
	require.NotEmpty(t, entries)
	require.Equal(t, "approve_amendment", entries[0].Kind)
}

// These tests pin Task A1 on the queue: within a kind, epics and
// features order by DependencyOrder rather than by reference, and that row
// order is what a caller batching several rows into one CLI invocation
// (`tiller approve epic <a> <b> <c>...`, cli-surface.md's high-repetition
// batch verbs) must join addresses in for the batch to apply dependencies
// before what rests on them.

// queueEntryKeys projects the entries of one kind onto their Key field, in
// payload order, so a test can assert on row order without restating the rest
// of each entry.
func queueEntryKeys(entries []views.QueueEntry, kind string) []string {
	var out []string
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e.Key)
		}
	}
	return out
}

// queueOrderEpicsTree lays three draft epics where dependency order and
// alphabetical order disagree: "alpha" depends on "zeta", so alpha would sort
// first alphabetically but must sort last; "foundation" and "zeta" (both
// dependency-free) sort key-ascending between themselves.
func queueOrderEpicsTree() *store.Tree {
	foundKey, zetaKey, alphaKey := "foundation", "zeta", "alpha"
	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-foundation": {ID: "E-foundation", Key: &foundKey, Title: "Foundation", Status: "draft"},
			"E-zeta":       {ID: "E-zeta", Key: &zetaKey, Title: "Zeta", Status: "draft"},
			"E-alpha":      {ID: "E-alpha", Key: &alphaKey, Title: "Alpha", Status: "draft", DependsOn: []string{"E-zeta"}},
		},
	}
}

// TestQueueApproveEpicsFollowDependencyOrderNotRef pins the approve_epic
// group's row order.
func TestQueueApproveEpicsFollowDependencyOrderNotRef(t *testing.T) {
	keys := queueEntryKeys(views.QueueOf(queueOrderEpicsTree()).Entries, "approve_epic")
	require.Equal(t, []string{"foundation", "zeta", "alpha"}, keys,
		"foundation and zeta (no deps) lead, alpha (depends on zeta) trails, though alpha sorts first alphabetically")
}

// TestQueueBatchArgumentOrderMatchesDependencyOrder pins the "batch command
// argument order" half of Task A1: a caller assembling one batch invocation
// out of a kind's rows (Home's future batch footer, plan
// 2026-07-23-console-chart-table-pass's Task C1) joins addresses in the
// queue's own row order, so the assembled command applies a dependency
// before what rests on it for free, with no separate sort of its own.
func TestQueueBatchArgumentOrderMatchesDependencyOrder(t *testing.T) {
	addrs := queueEntryKeys(views.QueueOf(queueOrderEpicsTree()).Entries, "approve_epic")
	batchCommand := "tiller approve epic " + strings.Join(addrs, " ")
	require.Equal(t, "tiller approve epic foundation zeta alpha", batchCommand)
}

// queueOrderFeaturesTree lays two specced features under one epic with the
// same alphabet-versus-dependency disagreement: "widgets/alpha" depends on
// "widgets/zeta".
func queueOrderFeaturesTree() *store.Tree {
	epicKey, zetaKey, alphaKey := "widgets", "widgets/zeta", "widgets/alpha"
	return &store.Tree{
		Manifest: &store.Manifest{},
		Epics: map[string]*store.Epic{
			"E-widgets": {ID: "E-widgets", Key: &epicKey, Title: "Widgets", Status: "ready"},
		},
		Features: map[string]*store.Feature{
			"F-zeta":  {ID: "F-zeta", Key: &zetaKey, Title: "Zeta", Status: "specced", EpicKey: "widgets"},
			"F-alpha": {ID: "F-alpha", Key: &alphaKey, Title: "Alpha", Status: "specced", EpicKey: "widgets", DependsOn: []string{"F-zeta"}},
		},
	}
}

// TestQueueApproveFeaturesFollowDependencyOrderNotRef pins the
// approve_feature group's row order.
func TestQueueApproveFeaturesFollowDependencyOrderNotRef(t *testing.T) {
	keys := queueEntryKeys(views.QueueOf(queueOrderFeaturesTree()).Entries, "approve_feature")
	require.Equal(t, []string{"widgets/zeta", "widgets/alpha"}, keys)
}

// TestQueueApproveFeaturesSuppressUnderRetiredAncestor extends
// queueFeatureChecks' suppress-not-exempt rule (gen.RetiredAncestor) to this
// neighboring group: a specced feature under a cancelled epic offers no
// approve_feature row, because `tiller approve feature` would now refuse with
// ParentEpicRetiredError, and a queue whose commands fail when
// copied is worse than silence. A live epic ancestor is the positive sibling:
// the same feature still queues normally.
func TestQueueApproveFeaturesSuppressUnderRetiredAncestor(t *testing.T) {
	epicKey, featKey := "foundation", "foundation/widgets"
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "cancelled"}},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &featKey, Status: "specced", EpicKey: epicKey},
		},
	}
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_feature"),
		"a specced feature under a cancelled epic offers no row (#37)")

	tree.Epics["E-aaaaaa"].Status = "ready"
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_feature"), 1,
		"a live epic ancestor still queues the row")
}

// TestQueueApproveTaskBatchesFollowDependencyOrderNotRef pins the
// approve_tasks group's row order: one entry per feature carrying a proposed
// task, dependency-ordered by the owning feature.
func TestQueueApproveTaskBatchesFollowDependencyOrderNotRef(t *testing.T) {
	tree := queueOrderFeaturesTree()
	// The row is about tasks added after the plan gate, so both features are
	// past it: before it, the initial set rides `approve feature` instead.
	for _, f := range tree.Features {
		f.Status = "active"
	}
	zetaTaskKey, alphaTaskKey := "widgets/zeta/one", "widgets/alpha/one"
	zetaTask := &store.Task{ID: "T-zeta1", Key: &zetaTaskKey, Status: "proposed", FeatureID: "F-zeta"}
	alphaTask := &store.Task{ID: "T-alpha1", Key: &alphaTaskKey, Status: "proposed", FeatureID: "F-alpha"}
	tree.Tasks = map[string]*store.Task{"T-zeta1": zetaTask, "T-alpha1": alphaTask}
	tree.FeatureTasks = map[string][]*store.Task{"F-zeta": {zetaTask}, "F-alpha": {alphaTask}}

	keys := queueEntryKeys(views.QueueOf(tree).Entries, "approve_tasks")
	require.Equal(t, []string{"widgets/zeta", "widgets/alpha"}, keys)
}

// TestQueueApproveTaskBatchesSuppressUnderRetiredAncestor is approve_tasks'
// half of the same suppress-not-exempt rule: proposed tasks under a ready
// feature under a cancelled epic offer no approve_tasks row, because `tiller
// approve task --feature` would refuse the same way at the first task it
// reached (ParentEpicRetiredError). A live epic ancestor is the
// positive sibling.
func TestQueueApproveTaskBatchesSuppressUnderRetiredAncestor(t *testing.T) {
	epicKey, featKey, taskKey := "foundation", "foundation/widgets", "foundation/widgets/one"
	feat := &store.Feature{ID: "F-aaaaaa", Key: &featKey, Status: "ready", EpicKey: epicKey}
	task := &store.Task{ID: "T-aaaaaa", Key: &taskKey, Status: "proposed", FeatureID: feat.ID}
	tree := &store.Tree{
		Manifest:     &store.Manifest{},
		Epics:        map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "cancelled"}},
		Features:     map[string]*store.Feature{feat.ID: feat},
		Tasks:        map[string]*store.Task{task.ID: task},
		FeatureTasks: map[string][]*store.Task{feat.ID: {task}},
	}
	require.Empty(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"),
		"proposed tasks under a ready feature under a cancelled epic offer no row (#37)")

	tree.Epics["E-aaaaaa"].Status = "ready"
	require.Len(t, entriesOfKind(views.QueueOf(tree), "approve_tasks"), 1,
		"a live epic ancestor still queues the row")
}

// TestQueueAttestGatesFollowDependencyOrderForTaskPairs restores the
// dependency-order pin this group lost when in_review became a task-only
// state: the group holds tasks alone, and a task now ranks by its recorded
// depends_on (artifact-schema.md, Tasks: Ordering). alfa follows zulu and
// leads it by both key and id, so only the recorded edge produces this order.
func TestQueueAttestGatesFollowDependencyOrderForTaskPairs(t *testing.T) {
	featKey := "widgets/loader"
	alfaKey, zuluKey := "widgets/loader/alfa", "widgets/loader/zulu"
	zulu := &store.Task{ID: "T-zulu00", Key: &zuluKey, Status: "in_review", FeatureID: "F-loader"}
	alfa := &store.Task{ID: "T-alfa00", Key: &alfaKey, Status: "in_review", FeatureID: "F-loader",
		DependsOn: []string{"T-zulu00"}}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
			"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		}}},
		Features:     map[string]*store.Feature{"F-loader": {ID: "F-loader", Key: &featKey, Status: "active"}},
		Tasks:        map[string]*store.Task{"T-zulu00": zulu, "T-alfa00": alfa},
		FeatureTasks: map[string][]*store.Task{"F-loader": {alfa, zulu}},
	}

	keys := queueEntryKeys(views.QueueOf(tree).Entries, "attest_gate")
	require.Equal(t, []string{"widgets/loader/zulu", "widgets/loader/alfa"}, keys,
		"zulu leads the alfa that depends on it, though alfa sorts first by key and by id")
}

// TestQueueAttestCriteriaFollowDependencyOrderForFeaturePairs pins the
// attest_criterion group's row order across two features: alpha depends on
// zeta, each carrying one needs_attestation criterion. It reads features
// rather than epics because an epic carries no gate block to record a
// criterion in (artifact-schema.md, Epics), which is also why
// queueAttestCriteria no longer scans them.
func TestQueueAttestCriteriaFollowDependencyOrderForFeaturePairs(t *testing.T) {
	zetaKey, alphaKey := "widgets/zeta", "widgets/alpha"
	needsAttestation := func() store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{
			"evals": {Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusNeedsAttestation}}},
		}}
	}
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Features: map[string]*store.Feature{
			"F-zeta":  {ID: "F-zeta", Key: &zetaKey, Status: "active", GatesBlock: needsAttestation()},
			"F-alpha": {ID: "F-alpha", Key: &alphaKey, Status: "active", DependsOn: []string{"F-zeta"}, GatesBlock: needsAttestation()},
		},
	}
	keys := queueEntryKeys(views.QueueOf(tree).Entries, "attest_criterion")
	require.Equal(t, []string{"widgets/zeta", "widgets/alpha"}, keys)
}

// TestQueueAttestCriteriaLeadWithGrain pins the leading term of the
// attest_criterion comparator, the group that mixes features and tasks. A
// feature key that sorts after a task's key path is ordinary, since a task
// reference leads with its own feature's key, so without the grain term the
// group falls back to comparing references across grains and the task leads:
//
//	grain-first:  zeta/f (F), alpha/g/t (T)
//	by reference: alpha/g/t (T), zeta/f (F)
//
// The term is not cosmetic. It is what stops rank and reference from each
// deciding some pairs of the same list, which is not transitive.
func TestQueueAttestCriteriaLeadWithGrain(t *testing.T) {
	featKey, taskKey := "zeta/f", "alpha/g/t"
	needsAttestation := func() store.GatesBlock {
		return store.GatesBlock{Gates: map[string]store.Gate{
			"evals": {Criteria: map[string]store.Criterion{"C1": {Status: store.CriterionStatusNeedsAttestation}}},
		}}
	}
	task := &store.Task{ID: "T-000001", Key: &taskKey, Status: "in_review", FeatureID: "F-000002",
		GatesBlock: needsAttestation()}
	tree := &store.Tree{
		Manifest: &store.Manifest{},
		Features: map[string]*store.Feature{
			"F-000001": {ID: "F-000001", Key: &featKey, Status: "active", EpicKey: "zeta",
				GatesBlock: needsAttestation()},
		},
		Tasks:        map[string]*store.Task{"T-000001": task},
		FeatureTasks: map[string][]*store.Task{"F-000002": {task}},
	}

	keys := queueEntryKeys(views.QueueOf(tree).Entries, "attest_criterion")
	require.Equal(t, []string{"zeta/f", "alpha/g/t"}, keys,
		"the feature leads the task, not the reference order that would lead with alpha/g/t")
}

// TestQueueNeverAsksTheHumanToAcceptAnEpic pins the removal from the side that
// matters: whatever the features under a ready epic look like, including the
// completed rollup that used to raise an accept_epic row, the queue asks the
// human for nothing at this grain. Epic done is a computed fact sync records
// (artifact-schema.md, Epics States), and the human's own judgment moment,
// archive, is a deliberate close rather than a queued ask.
func TestQueueNeverAsksTheHumanToAcceptAnEpic(t *testing.T) {
	epicKey := "checkout"
	cases := []struct {
		name     string
		statuses []string
	}{
		{"all done, the completed rollup", []string{"done", "done"}},
		{"done and cancelled mix", []string{"done", "cancelled"}},
		{"one still pending", []string{"done", "active"}},
		{"all cancelled", []string{"cancelled", "cancelled"}},
		{"zero features", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			features := map[string]*store.Feature{}
			for i, status := range tc.statuses {
				id := "F-" + strings.Repeat(string(rune('a'+i)), 6)
				key := epicKey + "/" + string(rune('a'+i))
				features[id] = &store.Feature{ID: id, Key: &key, Status: status, EpicKey: epicKey}
			}
			tree := &store.Tree{
				Manifest: &store.Manifest{},
				Epics:    map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: &epicKey, Status: "ready", Title: "Checkout"}},
				Features: features,
			}
			require.Empty(t, views.QueueOf(tree).Entries,
				"a ready epic raises no queue entry whatever its features are doing")
		})
	}

	// A draft epic still queues its approve, the one gate an epic does have.
	draftKey, featKey := "checkout", "checkout/cart"
	draft := &store.Tree{
		Manifest: &store.Manifest{},
		Epics:    map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: &draftKey, Status: "draft"}},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &featKey, Status: "done", EpicKey: "checkout"},
		},
	}
	require.Equal(t, []string{"approve_epic"}, queueEntryKinds(views.QueueOf(draft).Entries),
		"a draft epic sits at the approve gate, the epic's one queued gate")
}

// queueEntryKinds projects a queue payload onto its kinds in order.
func queueEntryKinds(entries []views.QueueEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Kind)
	}
	return out
}

// queueGatesOnly keeps just the attest_gate entries, so a focused test can pin
// the gate derivation without restating the rest of the payload.
func queueGatesOnly(entries []views.QueueEntry) []views.QueueEntry {
	var out []views.QueueEntry
	for _, e := range entries {
		if e.Kind == "attest_gate" {
			out = append(out, e)
		}
	}
	return out
}

// queueGateNames projects attest_gate entries onto their gate names in order,
// the within-group ordering (by gate) the derivation promises.
func queueGateNames(entries []views.QueueEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Gate)
	}
	return out
}

// queueReviewTaskFixture stages one in_review task on an offline review_ref
// under form-engine/schema-engine, the offline (no-remote) node path for the
// attest_gate derivation. It returns the repo root and the task's task.yml path
// so a test can record stored gate statuses straight into it.
func queueReviewTaskFixture(t *testing.T) (root, taskPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".tiller"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", "tiller.yml"), []byte(queueManifest), 0o644))
	deps := queueDeps(queueMint())
	_, err := mutate.AddEpic(queueLoad(t, root), deps, mutate.EpicInput{Key: "form-engine", Title: "Form Engine"})
	require.NoError(t, err)
	_, err = mutate.AddFeature(queueLoad(t, root), deps, mutate.FeatureInput{Key: "form-engine/schema-engine", Title: "Schema engine", ChangeType: "create"})
	require.NoError(t, err)
	_, err = mutate.AddTask(queueLoad(t, root), deps, mutate.TaskInput{Key: "form-engine/schema-engine/schema-loader", Title: "Wire the schema loader"})
	require.NoError(t, err)
	taskPath = queueTaskYML(root, "form-engine", "schema-engine", "schema-loader")
	queueSetTaskInReview(t, taskPath)
	return root, taskPath
}

// TestQueueAttestGateStoredStatuses pins the attest_gate derivation over an
// offline in_review task: absent gates queue pending, gate-sorted, then a
// recorded pass drops out while fail, skipped, and stale pass through as
// gate_status. The command always names --status pass (the affordance that
// clears the gate); gate_status carries the current state.
func TestQueueAttestGateStoredStatuses(t *testing.T) {
	root, taskPath := queueReviewTaskFixture(t)

	gates := queueGatesOnly(views.QueueOf(queueLoad(t, root)).Entries)
	require.Equal(t, []string{"accessibility_review", "code_review", "performance_review", "security_review"}, queueGateNames(gates),
		"every on:pr gate applies to a task, none recorded, so all four queue")
	for _, g := range gates {
		require.Equal(t, "task", g.Noun)
		require.Equal(t, "T-aaaaaa", g.ID)
		require.Equal(t, "form-engine/schema-engine/schema-loader (T-aaaaaa)", g.Ref)
		require.Equal(t, "pending", g.GateStatus, "an absent gate is pending")
		require.Equal(t, "tiller attest task form-engine/schema-engine/schema-loader --gate "+g.Gate+" --status pass", g.Command)
	}

	queueSetTaskGateStatuses(t, taskPath, map[string]string{
		"code_review":          store.GateStatusPass,
		"security_review":      store.GateStatusFail,
		"performance_review":   store.GateStatusSkipped,
		"accessibility_review": store.GateStatusStale,
	})

	gates = queueGatesOnly(views.QueueOf(queueLoad(t, root)).Entries)
	require.Equal(t, []string{"accessibility_review", "performance_review", "security_review"}, queueGateNames(gates),
		"the passed gate drops; the rest remain, gate-sorted")
	byGate := map[string]string{}
	for _, g := range gates {
		byGate[g.Gate] = g.GateStatus
	}
	require.Equal(t, map[string]string{
		"accessibility_review": "stale",
		"performance_review":   "skipped",
		"security_review":      "fail",
	}, byGate, "fail, skipped, and stale pass through verbatim")
}

// TestQueueAttestGateUsesTaskAndShelfPRGrains pins where an on:pr review gate
// lands: task, bug, and chore use the in_review lifecycle and queue in that
// fixed grain order. An ordinary feature remains excluded because it does not
// use that lifecycle.
func TestQueueAttestGateUsesTaskAndShelfPRGrains(t *testing.T) {
	featKey := "form-engine/schema-engine"
	taskKey := "form-engine/schema-engine/schema-loader"
	bugKey := "bugs/blank-on-reload"
	choreKey := "chores/bump-go"
	tree := &store.Tree{
		Manifest: &store.Manifest{
			Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
				"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
			}},
		},
		Features: map[string]*store.Feature{
			"F-aaaaaa": {ID: "F-aaaaaa", Key: &featKey, Title: "Schema engine", Status: "in_review"},
		},
		Tasks: map[string]*store.Task{
			"T-aaaaaa": {ID: "T-aaaaaa", Key: &taskKey, Status: "in_review", FeatureID: "F-aaaaaa", Title: "Wire it"},
		},
		Bugs: map[string]*store.Bug{
			"B-aaaaaa": {WorkItem: store.WorkItem{ID: "B-aaaaaa", Key: &bugKey, Status: "in_review", Title: "Blank on reload"}},
		},
		Chores: map[string]*store.Chore{
			"CH-aaaaaa": {WorkItem: store.WorkItem{ID: "CH-aaaaaa", Key: &choreKey, Status: "in_review", Title: "Bump Go"}},
		},
	}

	gates := queueGatesOnly(views.QueueOf(tree).Entries)
	require.Len(t, gates, 3)
	require.Equal(t, []string{store.NounTask, store.NounBug, store.NounChore}, []string{
		gates[0].Noun, gates[1].Noun, gates[2].Noun,
	})
	require.Equal(t, []string{"T-aaaaaa", "B-aaaaaa", "CH-aaaaaa"}, []string{
		gates[0].ID, gates[1].ID, gates[2].ID,
	})
	for _, gate := range gates {
		require.Equal(t, "code_review", gate.Gate)
		require.Equal(t, "pending", gate.GateStatus)
	}
}

func TestQueueAttestGateExcludesCarriedShelfWithoutChangingTaskGates(t *testing.T) {
	carrierKey := "form-engine/carrier"
	taskKey := "form-engine/carrier/review"
	bugKey := "bugs/carried"
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Optional: map[string]store.GateSpec{
			"code_review": {Kind: store.GateKindAttestation, On: store.StringList{"pr"}},
		}}},
		Features: map[string]*store.Feature{
			"F-live000": {ID: "F-live000", Key: &carrierKey, Status: "active"},
		},
		Tasks: map[string]*store.Task{
			"T-review0": {ID: "T-review0", Key: &taskKey, Status: "in_review", FeatureID: "F-live000", Title: "Review task"},
		},
		Bugs: map[string]*store.Bug{
			"B-carried": {WorkItem: store.WorkItem{
				ID: "B-carried", Key: &bugKey, Status: "in_review", Title: "Carried bug",
				ResolvedBy: "F-live000",
			}},
		},
	}

	require.Equal(t, gen.ActivityCarried, gen.WorkActivity(tree, tree.Bugs["B-carried"]))
	gates := queueGatesOnly(views.QueueOf(tree).Entries)
	require.Len(t, gates, 1)
	require.Equal(t, store.NounTask, gates[0].Noun)
	require.Equal(t, "T-review0", gates[0].ID)
}

// awaitingCheckTree builds a one-feature tree parked in the awaiting-check
// phase: active, one done task, the evals gate required at feature grain and
// unrecorded.
func awaitingCheckTree() (*store.Tree, *store.Feature) {
	key := "foundation/repo-tooling"
	f := &store.Feature{ID: "F-aaaaaa", Key: &key, Status: "active", EpicKey: "foundation",
		Title: "Repository and Development Tooling"}
	tree := &store.Tree{
		Manifest: &store.Manifest{Gates: store.GatesConfig{Required: map[string]store.GateSpec{
			"evals": {Kind: "command", On: store.StringList{"feature"}},
		}}},
		Epics:    map[string]*store.Epic{"E-aaaaaa": {ID: "E-aaaaaa", Key: strPtr("foundation"), Status: "ready"}},
		Features: map[string]*store.Feature{f.ID: f},
		Tasks: map[string]*store.Task{
			"T-aaaaaa": {ID: "T-aaaaaa", Status: "done", FeatureID: f.ID},
		},
		Questions: map[string]*store.Question{},
	}
	tree.FeatureTasks = map[string][]*store.Task{f.ID: {tree.Tasks["T-aaaaaa"]}}
	return tree, f
}

func entriesOfKind(q views.Queue, kind string) []views.QueueEntry {
	var out []views.QueueEntry
	for _, e := range q.Entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestQueueSharedCloseoutTracksWorkflowPhasesOnce(t *testing.T) {
	for _, grain := range []string{store.ReviewGrainTask, store.ReviewGrainFeature, store.ReviewGrainEpic} {
		t.Run(grain, func(t *testing.T) {
			tree, f, in := workflowFeature(t, grain)
			reviewID := f.ID
			if grain == store.ReviewGrainEpic {
				f.ReviewGrain = ""
				tree.Epics["E-aaaaa0"].ReviewGrain = grain
				reviewID = "E-aaaaa0"
			}
			queue := func() []views.QueueEntry {
				return views.QueueOfInput(tree, views.QueueInput{Workflow: in}).Entries
			}
			assertPhase := func(phase, actor string) {
				t.Helper()
				rows := queue()
				require.Len(t, rows, 1, "shared review work is not duplicated for owners or individual criteria")
				require.Equal(t, reviewID, rows[0].ID)
				require.Equal(t, phase, rows[0].Phase)
				require.Equal(t, actor, rows[0].Actor)
			}
			if grain != store.ReviewGrainTask {
				in.Assembly = map[string][]string{f.ID: {"T-aaaaa0"}}
				assertPhase("assembly", "agent")
				delete(in.Assembly, f.ID)
			}
			assertPhase("checks", "agent")
			completeMachine(f)
			assertPhase("rubric", "agent")
			f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: store.CriterionStatusNeedsAttestation, Evidence: []store.Evidence{{Kind: "rubric_output", Ref: "grade.json"}}}
			assertPhase("acceptance", "human")
			f.Gates["evals"].Criteria["manual"] = store.Criterion{Status: "pass"}
			f.Gates["evals"].Criteria["rubric"] = store.Criterion{Status: "pass"}
			gate := f.Gates["evals"]
			gate.Status = "pass"
			f.Gates["evals"] = gate
			if grain == store.ReviewGrainTask {
				assertPhase("sync", "agent")
				f.Status = "done"
			} else {
				assertPhase("submission", "agent")
				pr := 42
				if grain == store.ReviewGrainEpic {
					tree.Epics[reviewID].PR = &pr
				} else {
					f.PR = &pr
				}
				assertPhase("review", "human")
				// An unfinished replacement makes the old aggregate unmergeable.
				replacement := &store.Task{ID: "T-bbbbb0", Status: "ready", Owner: "recipient", FeatureID: f.ID}
				tree.Tasks[replacement.ID] = replacement
				tree.FeatureTasks[f.ID] = append(tree.FeatureTasks[f.ID], replacement)
				require.Empty(t, queue())
				replacement.Status = "cancelled"
				if grain == store.ReviewGrainEpic {
					tree.Epics[reviewID].Status = "archived"
				} else {
					f.Status = "cancelled"
				}
			}
			require.Empty(t, queue(), "terminal review units never retain a closeout ask")
		})
	}
}

func TestQueueSharedCloseoutPreservesHolds(t *testing.T) {
	for _, hold := range []string{"basis", "question", "manual", "retired"} {
		t.Run(hold, func(t *testing.T) {
			tree, f, in := workflowFeature(t, store.ReviewGrainTask)
			require.Len(t, views.QueueOfInput(tree, views.QueueInput{Workflow: in}).Entries, 1)
			switch hold {
			case "basis":
				in.Eligibility = map[string]string{f.ID: "accepted basis changed"}
			case "question":
				holdClaimableNode(tree, "Q-aaaaa0", f.ID)
			case "manual":
				f.Blocked = true
			case "retired":
				tree.Epics["E-aaaaa0"].Status = "cancelled"
			}
			queue := views.QueueOfInput(tree, views.QueueInput{Workflow: in})
			require.Empty(t, entriesOfKind(queue, "feature_check"))
			require.Empty(t, entriesOfKind(queue, "workflow_closeout"))
		})
	}
}

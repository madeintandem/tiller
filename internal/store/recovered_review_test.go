package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func recoveryStoreFixture(t *testing.T, noun string) (string, string, *RecoveredReview) {
	t.Helper()
	root := writeManifestFixture(t, fmt.Sprintf("schema_version: %d\ncontributors: [{id: ada, name: Ada}]\n", CurrentSchemaVersion))
	record := &RecoveredReview{
		Branch: "review/work/build", Head: RevisionRef{Kind: KindGitCommit, Value: "git:" + strings.Repeat("a", 40)},
		MergeCommit:       RevisionRef{Kind: KindGitCommit, Value: "git:" + strings.Repeat("b", 40)},
		IntegrationBranch: "main", Basis: "ancestry", RecoveredBy: "ada",
		RecoveredAt: time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC), Reason: "Recover the already merged review",
		Tasks:           []RecoveredTask{{ID: "T-aaaaaa", Path: ".tiller/epics/work/features/build/tasks/send/task.yml", State: RevisionRef{Kind: KindGitBlob, Value: "git:" + strings.Repeat("c", 40), Path: ".tiller/epics/work/features/build/tasks/send/task.yml"}}},
		PolicySnapshots: []HistoricalPolicySnapshot{{Path: ".tiller/tiller.yml", Source: RevisionRef{Kind: KindGitBlob, Value: "git:" + strings.Repeat("d", 40), Path: ".tiller/tiller.yml"}}, {NodeID: "F-aaaaaa", Path: ".tiller/epics/work/features/build/feature.yml", Source: RevisionRef{Kind: KindGitBlob, Value: "git:" + strings.Repeat("e", 40), Path: ".tiller/epics/work/features/build/feature.yml"}, Observations: []HistoricalPolicyObservation{{Gate: "evals", Status: "pending", UnresolvedCriteria: []string{"C8"}, EvidenceRefs: []string{"evidence/evals.json"}}}}},
	}
	record.PolicySnapshots = append(record.PolicySnapshots, HistoricalPolicySnapshot{NodeID: record.Tasks[0].ID, Path: record.Tasks[0].Path, Source: record.Tasks[0].State})
	writeFile(t, filepath.Join(root, ".tiller/epics/work/epic.yml"), "id: E-aaaaaa\nkey: work\nstatus: ready\n")
	writeFile(t, filepath.Join(root, ".tiller/epics/work/features/build/feature.yml"), "id: F-aaaaaa\nkey: work/build\nstatus: active\n")
	path := filepath.Join(root, ".tiller/epics/work/features/build/feature.yml")
	var node any = &Feature{ID: "F-aaaaaa", Key: new("work/build"), Status: "active", ReviewGrain: ReviewGrainFeature, Branch: record.Branch, ReviewRef: &ReviewRef{Branch: record.Branch, Head: record.Head}, RecoveredReview: record}
	if noun == NounEpic {
		path = filepath.Join(root, ".tiller/epics/work/epic.yml")
		node = &Epic{ID: "E-aaaaaa", Key: new("work"), Status: "ready", ReviewGrain: ReviewGrainEpic, Branch: record.Branch, ReviewRef: &ReviewRef{Branch: record.Branch, Head: record.Head}, RecoveredReview: record}
	}
	data, err := yaml.Marshal(node)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return root, path, record
}

func TestRecoveredReviewSurvivesLoadAndSourceMutation(t *testing.T) {
	for _, noun := range []string{NounFeature, NounEpic} {
		t.Run(noun, func(t *testing.T) {
			root, path, expected := recoveryStoreFixture(t, noun)
			tree, err := Load(root)
			require.NoError(t, err)
			var node any
			if noun == NounFeature {
				f := tree.Features["F-aaaaaa"]
				require.Equal(t, expected, f.RecoveredReview)
				f.Title = "Updated title"
				f.PolicyViolations = []PolicyViolation{{Gate: "evals", Status: "pending", Observed: expected.MergeCommit, SourceRevision: expected.PolicySnapshots[1].Source, UnresolvedCriteria: []string{"C8"}, EvidenceRefs: []string{"evidence/evals.json"}}}
				node = f
			} else {
				e := tree.Epics["E-aaaaaa"]
				require.Equal(t, expected, e.RecoveredReview)
				e.Title = "Updated title"
				node = e
			}
			_, snapshot, err := ReadRegularFileSnapshot(root, path)
			require.NoError(t, err)
			data, err := yaml.Marshal(node)
			require.NoError(t, err)
			require.NoError(t, tree.Commit("update title", []PlannedWrite{PlanWrite(path, snapshot, data)}))
			reloaded, err := Load(root)
			require.NoError(t, err)
			if noun == NounFeature {
				require.Equal(t, expected, reloaded.Features["F-aaaaaa"].RecoveredReview)
				require.Equal(t, []string{"C8"}, reloaded.Features["F-aaaaaa"].PolicyViolations[0].UnresolvedCriteria)
				require.Equal(t, expected.PolicySnapshots[1].Source, reloaded.Features["F-aaaaaa"].PolicyViolations[0].SourceRevision)
			} else {
				require.Equal(t, expected, reloaded.Epics["E-aaaaaa"].RecoveredReview)
			}
		})
	}
}

func TestLoadRefusesMalformedRecoveredProvenance(t *testing.T) {
	cases := map[string]func(*RecoveredReview){
		"abbreviated head":           func(r *RecoveredReview) { r.Head.Value = "git:abcdef0" },
		"uppercase head":             func(r *RecoveredReview) { r.Head.Value = "git:" + strings.Repeat("A", 40) },
		"wrong revision kind":        func(r *RecoveredReview) { r.Head.Kind = KindGitBlob },
		"commit path":                func(r *RecoveredReview) { r.Head.Path = "unexpected" },
		"unsupported basis":          func(r *RecoveredReview) { r.Basis = "human_attested" },
		"blank actor":                func(r *RecoveredReview) { r.RecoveredBy = " " },
		"missing time":               func(r *RecoveredReview) { r.RecoveredAt = time.Time{} },
		"blank reason":               func(r *RecoveredReview) { r.Reason = " " },
		"missing integration":        func(r *RecoveredReview) { r.IntegrationBranch = "" },
		"wrong owning branch":        func(r *RecoveredReview) { r.Branch = "another/round" },
		"no riders":                  func(r *RecoveredReview) { r.Tasks = nil },
		"duplicate riders":           func(r *RecoveredReview) { r.Tasks = append(r.Tasks, r.Tasks[0]) },
		"escaping path":              func(r *RecoveredReview) { r.Tasks[0].Path = "../task.yml" },
		"wrong blob path":            func(r *RecoveredReview) { r.Tasks[0].State.Path = ".tiller/other.yml" },
		"missing policy":             func(r *RecoveredReview) { r.PolicySnapshots = nil },
		"missing task policy":        func(r *RecoveredReview) { r.PolicySnapshots = r.PolicySnapshots[:2] },
		"missing feature policy":     func(r *RecoveredReview) { r.PolicySnapshots = append(r.PolicySnapshots[:1], r.PolicySnapshots[2]) },
		"divergent task policy blob": func(r *RecoveredReview) { r.PolicySnapshots[2].Source.Value = "git:" + strings.Repeat("f", 40) },
		"duplicate snapshots":        func(r *RecoveredReview) { r.PolicySnapshots = append(r.PolicySnapshots, r.PolicySnapshots[1]) },
		"unsorted snapshots": func(r *RecoveredReview) {
			r.PolicySnapshots[0], r.PolicySnapshots[1] = r.PolicySnapshots[1], r.PolicySnapshots[0]
		},
		"passing historical warning": func(r *RecoveredReview) { r.PolicySnapshots[1].Observations[0].Status = "pass" },
		"duplicate criteria": func(r *RecoveredReview) {
			r.PolicySnapshots[1].Observations[0].UnresolvedCriteria = []string{"C8", "C8"}
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			root, path, record := recoveryStoreFixture(t, NounFeature)
			damage(record)
			data, err := yaml.Marshal(&Feature{ID: "F-aaaaaa", Status: "active", ReviewGrain: ReviewGrainFeature, Branch: "review/work/build", ReviewRef: &ReviewRef{Branch: "review/work/build", Head: RevisionRef{Kind: KindGitCommit, Value: "git:" + strings.Repeat("a", 40)}}, RecoveredReview: record})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			_, err = Load(root)
			require.ErrorContains(t, err, "recovered_review")
		})
	}
}

func TestLoadRefusesRecoveryShapeAndIdentityLoss(t *testing.T) {
	for name, change := range map[string]func(string) string{
		"null": func(s string) string { return strings.Split(s, "recovered_review:")[0] + "recovered_review: null\n" },
		"unknown field": func(s string) string {
			return strings.Replace(s, "recovered_review:\n", "recovered_review:\n    misspelled_head: ignored\n", 1)
		},
		"wrong head identity": func(s string) string {
			return strings.Replace(s, "value: git:"+strings.Repeat("a", 40), "value: git:"+strings.Repeat("f", 40), 1)
		},
		"missing review identity": func(s string) string {
			start := strings.Index(s, "review_ref:")
			end := strings.Index(s, "recovered_review:")
			return s[:start] + s[end:]
		},
		"nonpositive PR": func(s string) string {
			start := strings.Index(s, "review_ref:")
			end := strings.Index(s, "recovered_review:")
			return s[:start] + "pr: 0\n" + s[end:]
		},
		"both identities":   func(s string) string { return "pr: 42\n" + s },
		"wrong owner grain": func(s string) string { return strings.Replace(s, "review_grain: feature", "review_grain: task", 1) },
	} {
		t.Run(name, func(t *testing.T) {
			root, path, _ := recoveryStoreFixture(t, NounFeature)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(change(string(data))), 0o644))
			_, err = Load(root)
			require.ErrorContains(t, err, "recovered_review")
		})
	}
}

func TestRecoveredReviewSupportsPRIdentityWithoutChangingAbsentRecords(t *testing.T) {
	root, path, record := recoveryStoreFixture(t, NounFeature)
	node := &Feature{ID: "F-aaaaaa", Status: "active", ReviewGrain: ReviewGrainFeature, Branch: record.Branch, PR: new(42), RecoveredReview: record}
	data, err := yaml.Marshal(node)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, 42, *tree.Features[node.ID].PR)
	require.Nil(t, tree.Features[node.ID].ReviewRef)
	require.Equal(t, record, tree.Features[node.ID].RecoveredReview)
	node.RecoveredReview = nil
	data, err = yaml.Marshal(node)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	tree, err = Load(root)
	require.NoError(t, err)
	require.Nil(t, tree.Features[node.ID].RecoveredReview)
	require.Equal(t, 42, *tree.Features[node.ID].PR)
}

func TestLoadRefusesMalformedNestedRecoveryShapes(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"unknown revision field": func(r map[string]any) { r["head"].(map[string]any)["misspelled_revision"] = "ignored" },
		"null head":              func(r map[string]any) { r["head"] = nil },
		"numeric actor":          func(r map[string]any) { r["recovered_by"] = 7 },
		"numeric criterion": func(r map[string]any) {
			snapshot := r["policy_snapshots"].([]any)[1].(map[string]any)
			snapshot["observations"].([]any)[0].(map[string]any)["unresolved_criteria"] = []any{8}
		},
		"unknown observation field": func(r map[string]any) {
			snapshot := r["policy_snapshots"].([]any)[1].(map[string]any)
			snapshot["observations"].([]any)[0].(map[string]any)["ignored_evidence"] = "omitted"
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, path, _ := recoveryStoreFixture(t, NounFeature)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var document map[string]any
			require.NoError(t, yaml.Unmarshal(data, &document))
			change(document["recovered_review"].(map[string]any))
			data, err = yaml.Marshal(document)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			_, err = Load(root)
			require.ErrorContains(t, err, "recovered_review")
		})
	}
}

func TestHistoricalPolicySourceCannotBeDroppedOnLoad(t *testing.T) {
	for name, source := range map[string]string{
		"null":                   "source_revision: null",
		"abbreviated":            "source_revision: {kind: git_blob, value: 'git:abc', path: .tiller/tiller.yml}",
		"wrong kind":             "source_revision: {kind: git_commit, value: 'git:" + strings.Repeat("a", 40) + "', path: .tiller/tiller.yml}",
		"unknown revision field": "source_revision: {kind: git_blob, value: 'git:" + strings.Repeat("a", 40) + "', path: .tiller/tiller.yml, overlooked: bad}",
		"missing source":         "unresolved_criteria: [C8]",
	} {
		t.Run(name, func(t *testing.T) {
			root, path, _ := recoveryStoreFixture(t, NounFeature)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			data = append(data, []byte("policy_violations:\n    - gate: evals\n      status: pending\n      "+source+"\n")...)
			require.NoError(t, os.WriteFile(path, data, 0o644))
			_, err = Load(root)
			require.ErrorContains(t, err, "policy_violations")
		})
	}
}

func TestLoadRefusesOlderOrNewerSchemaBeforeTouchingRecovery(t *testing.T) {
	for _, version := range []int{9, CurrentSchemaVersion + 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			root, path, _ := recoveryStoreFixture(t, NounFeature)
			manifestPath := filepath.Join(root, ".tiller/tiller.yml")
			data, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			data, err = RenderSchemaVersion(data, version)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(manifestPath, data, 0o644))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = Load(root)
			var refusal *EnvError
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, ReasonSchemaVersionUnsupported, refusal.Reason)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestRecoveryRecordSurvivesRefusedStaleTransaction(t *testing.T) {
	root, path, original := recoveryStoreFixture(t, NounFeature)
	tree, err := Load(root)
	require.NoError(t, err)
	_, snapshot, err := ReadRegularFileSnapshot(root, path)
	require.NoError(t, err)
	feature := tree.Features["F-aaaaaa"]
	feature.Title = "Planned title"
	planned, err := yaml.Marshal(feature)
	require.NoError(t, err)
	feature.Title = "Concurrent title"
	concurrent, err := yaml.Marshal(feature)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, concurrent, 0o644))
	err = tree.Commit("update title", []PlannedWrite{PlanWrite(path, snapshot, planned)})
	var refusal *EnvError
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, ReasonMutationStale, refusal.Reason)
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, concurrent, actual)
	reloaded, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, "Concurrent title", reloaded.Features[feature.ID].Title)
	require.Equal(t, original, reloaded.Features[feature.ID].RecoveredReview)
}

func TestLoadRefusesNullRecoveryInheritedThroughYAMLMerge(t *testing.T) {
	for _, noun := range []string{NounFeature, NounEpic} {
		for name, merge := range map[string]string{
			"inline":       "<<: {recovered_review: null}\n",
			"alias":        "defaults: &round {recovered_review: null}\n<<: *round\n",
			"nested alias": "base: &base {recovered_review: null}\ndefaults: &round {<<: *base}\n<<: *round\n",
			"sequence":     "defaults: &round {recovered_review: null}\n<<: [*round, {title: Merged}]\n",
		} {
			t.Run(noun+"/"+name, func(t *testing.T) {
				root, statePath, _ := recoveryStoreFixture(t, noun)
				identity := "id: F-aaaaaa\nkey: work/build\nstatus: active\nreview_grain: feature\n"
				if noun == NounEpic {
					identity = "id: E-aaaaaa\nkey: work\nstatus: ready\nreview_grain: epic\n"
				}
				state := merge + identity + "branch: review/work/build\npr: 42\n"
				require.NoError(t, os.WriteFile(statePath, []byte(state), 0o644))
				_, err := Load(root)
				require.ErrorContains(t, err, "recovered_review")
			})
		}
	}
}

func TestLoadHonorsExplicitRecoveryOverMergedNullDefault(t *testing.T) {
	root, statePath, record := recoveryStoreFixture(t, NounFeature)
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	data = append([]byte("defaults: &round {recovered_review: null}\n<<: *round\n"), data...)
	require.NoError(t, os.WriteFile(statePath, data, 0o644))
	tree, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, record, tree.Features["F-aaaaaa"].RecoveredReview)
}

func TestRecoveredGrainCheckSkipsClosedAggregates(t *testing.T) {
	for _, c := range []struct{ noun, live, closed string }{
		{NounFeature, "active", "done"},
		{NounEpic, "ready", "done"},
		{NounEpic, "ready", "archived"},
	} {
		t.Run(c.noun+"/"+c.closed, func(t *testing.T) {
			root, path, record := recoveryStoreFixture(t, c.noun)
			write := func(status string) {
				t.Helper()
				ref := &ReviewRef{Branch: record.Branch, Head: record.Head}
				var node any = &Feature{ID: "F-aaaaaa", Key: new("work/build"), Status: status, ReviewGrain: ReviewGrainTask, Branch: record.Branch, ReviewRef: ref, RecoveredReview: record}
				if c.noun == NounEpic {
					node = &Epic{ID: "E-aaaaaa", Key: new("work"), Status: status, ReviewGrain: ReviewGrainTask, Branch: record.Branch, ReviewRef: ref, RecoveredReview: record}
				}
				data, err := yaml.Marshal(node)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, data, 0o644))
			}

			write(c.live)
			_, err := Load(root)
			require.ErrorContains(t, err, "no longer has "+c.noun+" review grain")
			require.ErrorContains(t, err, "execution.review_grain", "the refusal names where to restore the grain")

			write(c.closed)
			tree, err := Load(root)
			require.NoError(t, err, "a closed aggregate's recovery is history, not a live ownership claim")
			if c.noun == NounFeature {
				require.Equal(t, record, tree.Features["F-aaaaaa"].RecoveredReview)
			} else {
				require.Equal(t, record, tree.Epics["E-aaaaaa"].RecoveredReview)
			}
		})
	}
}

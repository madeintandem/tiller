package gen

import (
	"testing"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func TestEligibleTaskAcceptsSubmittedPrerequisiteFeatureOnTheSameEpicAggregate(t *testing.T) {
	tree := candidateTree()
	task := tree.Tasks["T-aaaa01"]
	dependent, prerequisite, epic := tree.Features["F-loadng"], tree.Features["F-export"], tree.Epics["E-formen"]
	submitted := tree.Tasks["T-cccc03"]
	for _, id := range []string{"T-cccc03", "T-dddd04"} {
		tree.Tasks[id].Status = "in_review"
	}
	dependent.DependsOn = []string{prerequisite.ID}
	require.False(t, EligibleTask(tree, task), "at task grain the prerequisite must be done")

	epic.ReviewGrain = store.ReviewGrainEpic
	require.True(t, EligibleTask(tree, task))
	require.True(t, FeaturePrerequisiteSubmitted(tree, dependent, prerequisite))

	for name, change := range map[string]func() func(){
		"prerequisite takes its own review": func() func() {
			prerequisite.ReviewGrain = store.ReviewGrainFeature
			return func() { prerequisite.ReviewGrain = "" }
		},
		"dependent takes its own review": func() func() {
			dependent.ReviewGrain = store.ReviewGrainFeature
			return func() { dependent.ReviewGrain = "" }
		},
		"prerequisite task not submitted": func() func() {
			submitted.Status = "in_progress"
			return func() { submitted.Status = "in_review" }
		},
		"prerequisite task started at task grain": func() func() {
			submitted.StartedGrain = store.ReviewGrainTask
			return func() { submitted.StartedGrain = "" }
		},
		"prerequisite has no live task": func() func() {
			tasks := tree.FeatureTasks[prerequisite.ID]
			tree.FeatureTasks[prerequisite.ID] = nil
			return func() { tree.FeatureTasks[prerequisite.ID] = tasks }
		},
		"epic records the prerequisite": func() func() {
			epic.DependsOn = []string{prerequisite.ID}
			return func() { epic.DependsOn = nil }
		},
	} {
		restore := change()
		require.False(t, EligibleTask(tree, task), name)
		restore()
		require.True(t, EligibleTask(tree, task), name)
	}
}

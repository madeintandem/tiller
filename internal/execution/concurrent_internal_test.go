package execution

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestForEachConcurrentlyRunsEveryIndexWithinTheBound(t *testing.T) {
	var mu sync.Mutex
	running, peak := 0, 0
	seen := make([]bool, 20)
	forEachConcurrently(len(seen), func(i int) {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		running--
		seen[i] = true
		mu.Unlock()
	})
	require.LessOrEqual(t, peak, observationConcurrency)
	for i, ok := range seen {
		require.True(t, ok, "index %d ran", i)
	}
}

func TestForEachConcurrentlyRepanicsOnTheCaller(t *testing.T) {
	var mu sync.Mutex
	ran := make([]bool, 12)
	sentinel := "work item exploded"
	require.PanicsWithValue(t, sentinel, func() {
		forEachConcurrently(len(ran), func(i int) {
			mu.Lock()
			ran[i] = true
			mu.Unlock()
			if i == 3 {
				panic(sentinel)
			}
		})
	})
	for i, ok := range ran {
		require.True(t, ok, "index %d ran despite the panic", i)
	}
}

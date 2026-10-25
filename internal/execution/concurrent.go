package execution

import (
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/madeintandem/tiller/internal/store"
)

// observationConcurrency bounds the git processes one observation runs at
// once. Each task source costs a few short git reads; running them side by
// side turns their sum into roughly their slowest, without flooding the
// machine when a project has many worktrees.
const observationConcurrency = 4

// forEachConcurrently calls work for every index in [0, n), at most
// observationConcurrency at a time, and returns when all have finished. A
// panic in any work item is re-raised on the caller's goroutine once every
// item has finished.
func forEachConcurrently(n int, work func(i int)) {
	slots := make(chan struct{}, observationConcurrency)
	var wg sync.WaitGroup
	var once sync.Once
	var panicked any
	var didPanic bool
	for i := 0; i < n; i++ {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			// A goroutine panic would kill the server; hand the first one back
			// to the caller so net/http recovers the request, as it did before
			// this work left the request goroutine.
			defer func() {
				if r := recover(); r != nil {
					once.Do(func() { panicked, didPanic = r, true })
				}
			}()
			work(i)
		}()
	}
	wg.Wait()
	if didPanic {
		panic(panicked)
	}
}

// candidate is one eligible task whose execution branch is worth selecting.
type candidate struct {
	id, key, path, branch string
	task                  *store.Task
}

// selection is what one candidate's concurrent phase produced.
type selection struct {
	src         *source
	diagnostics []Diagnostic
	data        []byte
	err         error
	readErr     error
}

// selectAll selects and reads every candidate's source concurrently. It only
// reads the repository discovered before it, so sources never share writes.
func (r *repository) selectAll(candidates []candidate) []selection {
	results := make([]selection, len(candidates))
	forEachConcurrently(len(candidates), func(i int) {
		c := candidates[i]
		src, diagnostics, err := r.selectSource(c.branch, c.path)
		result := selection{src: src, diagnostics: diagnostics, err: err}
		if err == nil && src != nil {
			result.data, result.readErr = src.read(c.path)
		}
		results[i] = result
	})
	return results
}

// checkIdentities confirms that the served checkout, every watched ref, and
// every selected worktree still match discovery. Refs are read in one call for
// all sources, then each source's own checks run concurrently. A failure that
// is not source-specific is returned as err.
func (r *repository) checkIdentities(sources map[string]*source) (map[string]error, error) {
	failures := map[string]error{}
	if len(sources) == 0 {
		return failures, nil
	}
	common, err := commonDir(r.root)
	if err != nil {
		return nil, err
	}
	if common != r.common {
		return nil, errors.New("served checkout Git identity changed")
	}
	var names []string
	for _, src := range sources {
		names = append(names, src.watchedRefs()...)
	}
	oids, err := refOIDs(r.root, names)
	if err != nil {
		return nil, err
	}
	ids := slices.Sorted(maps.Keys(sources))
	results := make([]error, len(ids))
	forEachConcurrently(len(ids), func(i int) {
		src := sources[ids[i]]
		if err := src.checkRefs(oids); err != nil {
			results[i] = err
		} else if src.kind == "working_tree" {
			results[i] = src.checkWorktreeIdentity()
		}
	})
	for i, err := range results {
		if err != nil {
			failures[ids[i]] = err
		}
	}
	return failures, nil
}

package serve

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/stretchr/testify/require"
)

func blockingGroup(release <-chan struct{}, started chan<- struct{}, result error) (*loadGroup, *atomic.Int32) {
	var calls atomic.Int32
	return &loadGroup{inflight: map[loadKey]*loadCall{}, load: func(root string, observe bool) (*store.Tree, *execution.Snapshot, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return &store.Tree{Root: root}, nil, result
	}}, &calls
}

func waitForWaiters(t *testing.T, group *loadGroup, key loadKey, n int32) {
	t.Helper()
	require.Eventually(t, func() bool {
		group.mu.Lock()
		defer group.mu.Unlock()
		call := group.inflight[key]
		return call != nil && call.waiters.Load() >= n
	}, time.Second, time.Millisecond)
}

func TestConcurrentReadsShareOneLoad(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{}, 4)
	group, calls := blockingGroup(release, started, nil)
	trees := make(chan *store.Tree, 4)
	go func() { tree, _, _, _ := group.do("/repo", true); trees <- tree }()
	<-started
	var joined sync.WaitGroup
	for i := 0; i < 3; i++ {
		joined.Add(1)
		go func() { defer joined.Done(); tree, _, _, _ := group.do("/repo", true); trees <- tree }()
	}
	// The leader's call is registered before its load starts, so every do()
	// issued after started has joined it.
	waitForWaiters(t, group, loadKey{"/repo", true}, 3)
	close(release)
	joined.Wait()
	first := <-trees
	for i := 0; i < 3; i++ {
		require.Same(t, first, <-trees)
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestReadAfterALoadFinishesLoadsFresh(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{}, 2)
	close(release)
	group, calls := blockingGroup(release, started, nil)
	_, _, _, err := group.do("/repo", false)
	require.NoError(t, err)
	_, _, _, err = group.do("/repo", false)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load(), "nothing is kept once a load finishes")
}

// An observed read needs a snapshot and an integration read has none, so the
// two must never share a load. Both loads are held in flight at once: if the
// key ignored observe, the second read would join the first and never start.
func TestObservedAndIntegrationReadsDoNotShare(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{}, 2)
	group, calls := blockingGroup(release, started, nil)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _, _, _, _ = group.do("/repo", true) }()
	<-started
	wg.Add(1)
	go func() { defer wg.Done(); _, _, _, _ = group.do("/repo", false) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		wg.Wait()
		t.Fatal("integration read joined the observed load")
	}
	close(release)
	wg.Wait()
	require.EqualValues(t, 2, calls.Load())
}

func TestASharedLoadErrorReachesEveryWaiter(t *testing.T) {
	release, started := make(chan struct{}), make(chan struct{}, 2)
	failure := errors.New("repo unreadable")
	group, _ := blockingGroup(release, started, failure)
	errs := make(chan error, 2)
	go func() { _, _, _, err := group.do("/repo", true); errs <- err }()
	<-started
	go func() { _, _, _, err := group.do("/repo", true); errs <- err }()
	waitForWaiters(t, group, loadKey{"/repo", true}, 1)
	close(release)
	require.ErrorIs(t, <-errs, failure)
	require.ErrorIs(t, <-errs, failure)
}

func TestAnAbandonedLoadReleasesItsWaiters(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{})
	group := &loadGroup{inflight: map[loadKey]*loadCall{}, load: func(string, bool) (*store.Tree, *execution.Snapshot, error) {
		started <- struct{}{}
		<-release
		panic("projection bug")
	}}
	go func() {
		defer func() { _ = recover() }()
		_, _, _, _ = group.do("/repo", true)
	}()
	<-started
	errs := make(chan error, 1)
	go func() { _, _, _, err := group.do("/repo", true); errs <- err }()
	waitForWaiters(t, group, loadKey{"/repo", true}, 1)
	close(release)
	require.ErrorIs(t, <-errs, errLoadAbandoned)
}

// movingGenerationRepo commits a manifest and seeds the generation file, so a
// test can move the generation by rewriting that file.
func movingGenerationRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	movingGit(t, root, "init", "-q", "-b", "main")
	movingFile(t, root, ".tiller/tiller.yml", "schema_version: 12\nschema_epoch: initial\nproject: {name: Moving, key: moving, kind: greenfield}\ncontributors:\n  - {id: jc, name: JC}\ngit: {integration_branch: main}\n")
	movingFile(t, root, ".tiller/.cache/generation", "one")
	movingGit(t, root, "add", ".tiller/tiller.yml")
	movingGit(t, root, "commit", "-qm", "manifest")
	return root
}

// sharedResponse is one response from sharedStatusReads.
type sharedResponse struct {
	code int
	body string
}

// sharedStatusReads issues two concurrent /api/status reads through api(),
// returning their responses once both finish. The second joins the
// first's load; between join and release, move runs.
func sharedStatusReads(t *testing.T, root string, loaded <-chan struct{}, release chan<- struct{}, move func()) []sharedResponse {
	t.Helper()
	handler := api(root, func(*store.Tree, *http.Request) (any, error) { return map[string]string{}, nil })
	responses := make(chan sharedResponse, 2)
	for i := 0; i < 2; i++ {
		go func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
			responses <- sharedResponse{recorder.Code, recorder.Body.String()}
		}()
	}
	<-loaded
	waitForWaiters(t, sharedLoads, loadKey{root, true}, 1)
	move()
	close(release)
	return []sharedResponse{<-responses, <-responses}
}

func TestAJoinedReadRetriesOnceWhenTheGenerationMovedUnderItsSharedLoad(t *testing.T) {
	root := movingGenerationRepo(t)
	var calls atomic.Int32
	loaded, release := make(chan struct{}), make(chan struct{})
	previous := sharedLoads
	sharedLoads = &loadGroup{inflight: map[loadKey]*loadCall{}, load: func(root string, observe bool) (*store.Tree, *execution.Snapshot, error) {
		tree, snapshot, err := loadForRead(root, observe)
		if calls.Add(1) == 1 {
			close(loaded)
			<-release
		}
		return tree, snapshot, err
	}}
	t.Cleanup(func() { sharedLoads = previous })

	responses := sharedStatusReads(t, root, loaded, release, func() {
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tiller", ".cache", "generation"), []byte("two"), 0o644))
	})
	codes := []int{responses[0].code, responses[1].code}
	require.ElementsMatch(t, []int{http.StatusInternalServerError, http.StatusOK}, codes,
		"the leader's own load moved under it; the joined read reloads and answers")
	for _, r := range responses {
		if r.code == http.StatusInternalServerError {
			require.Contains(t, r.body, "Tiller state changed while it was being read")
		}
	}
	require.EqualValues(t, 2, calls.Load())
}

func TestAJoinedReadRetriesOnlyOnce(t *testing.T) {
	root := movingGenerationRepo(t)
	var calls atomic.Int32
	loaded, release := make(chan struct{}), make(chan struct{})
	previous := sharedLoads
	sharedLoads = &loadGroup{inflight: map[loadKey]*loadCall{}, load: func(root string, observe bool) (*store.Tree, *execution.Snapshot, error) {
		tree, snapshot, err := loadForRead(root, observe)
		n := calls.Add(1)
		if n == 1 {
			close(loaded)
			<-release
		}
		// Every load is overtaken by a Tiller mutation before its read projects.
		// This runs on a handler goroutine, so report with t.Error, not require.
		if werr := os.WriteFile(filepath.Join(root, ".tiller", ".cache", "generation"), []byte(fmt.Sprintf("moved-%d", n)), 0o644); werr != nil {
			t.Error(werr)
		}
		return tree, snapshot, err
	}}
	t.Cleanup(func() { sharedLoads = previous })

	responses := sharedStatusReads(t, root, loaded, release, func() {})
	for _, r := range responses {
		require.Equal(t, http.StatusInternalServerError, r.code)
		require.Contains(t, r.body, "Tiller state changed while it was being read")
	}
	require.EqualValues(t, 2, calls.Load(), "the joined read retries exactly once")
}

// Every endpoint at once, over one shared load, with an observed worktree and
// a recorded event. Run under -race: projections must only read the shared
// tree, snapshot, and cached events.
func TestConcurrentEndpointReadsAreRaceFree(t *testing.T) {
	root := t.TempDir()
	movingGit(t, root, "init", "-q", "-b", "main")
	movingFile(t, root, ".tiller/tiller.yml", "schema_version: 12\nschema_epoch: initial\nproject: {name: Moving, key: moving, kind: greenfield}\ncontributors:\n  - {id: jc, name: JC}\ngit: {integration_branch: main}\n")
	movingFile(t, root, ".tiller/epics/work/epic.yml", "id: E-aaaaaa\nkey: work\nstatus: ready\n")
	movingFile(t, root, ".tiller/epics/work/features/build/feature.yml", "id: F-bbbbbb\nkey: work/build\nstatus: ready\n")
	movingFile(t, root, movingTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: ready\n")
	movingFile(t, root, ".tiller/events/2026/10/01/EV-aaaaaa.yml", "id: EV-aaaaaa\ntype: claim.task\nat: 2026-10-01T10:00:00Z\ncontributor: jc\ncommand: claim\ntargets: [T-cccccc]\n")
	movingGit(t, root, "add", ".")
	movingGit(t, root, "commit", "-qm", "approved plan")
	worktree := filepath.Join(t.TempDir(), "execution")
	movingGit(t, root, "worktree", "add", "-q", "-b", "tiller/work/build/one", worktree)
	movingFile(t, worktree, movingTaskPath, "id: T-cccccc\nkey: work/build/one\nstatus: in_progress\nstarted: true\nowner: jc\nbranch: tiller/work/build/one\n")

	srv := httptest.NewServer(Handler(root, fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}, "test", DefaultAuthority()))
	t.Cleanup(srv.Close)
	paths := []string{"/api/status", "/api/board", "/api/tree", "/api/map", "/api/ls?noun=task", "/api/build", "/api/queue", "/api/next", "/api/work-items", "/api/events", "/api/execution", "/api/drift", "/api/node/task/work/build/one", "/api/recap/peek"}
	var wg sync.WaitGroup
	for round := 0; round < 3; round++ {
		for _, path := range paths {
			wg.Add(1)
			go func() {
				defer wg.Done()
				response, err := http.Get(srv.URL + path)
				if err != nil {
					t.Error(err)
					return
				}
				response.Body.Close()
				if response.StatusCode >= 500 {
					t.Errorf("%s answered %d", path, response.StatusCode)
				}
			}()
		}
	}
	wg.Wait()
}

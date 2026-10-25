package serve

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
)

// A console page mounts several reads at once, and each used to load the
// tree, list the event stream, and walk every task worktree on its own. A read
// that arrives while an identical load is in flight waits for that load
// instead of starting another. Nothing is kept once the load finishes, so the
// next read always loads fresh. Each read projects over its own Fork of the
// shared snapshot and closes its own stability boundary. A read that joined a
// load and finds a Tiller mutation under it reloads once, because that load
// may have started before the read arrived; the read that started the load,
// or a retry that moves again, still refuses.

type loadKey struct {
	root    string
	observe bool
}

type loadCall struct {
	done     chan struct{}
	waiters  atomic.Int32
	tree     *store.Tree
	snapshot *execution.Snapshot
	err      error
}

type loadGroup struct {
	mu       sync.Mutex
	inflight map[loadKey]*loadCall
	load     func(root string, observe bool) (*store.Tree, *execution.Snapshot, error)
}

// errLoadAbandoned is what waiters receive when the load they joined panicked.
var errLoadAbandoned = errors.New("console read: the shared load did not finish; retry")

var sharedLoads = &loadGroup{inflight: map[loadKey]*loadCall{}, load: loadForRead}

// loadForRead is one read's load: the tree, and for observed reads the task
// execution observation over it.
func loadForRead(root string, observe bool) (*store.Tree, *execution.Snapshot, error) {
	tree, err := store.Load(root)
	if err != nil || !observe {
		return tree, nil, err
	}
	snapshot, err := execution.Load(tree)
	return tree, snapshot, err
}

// do returns the in-flight load for (root, observe), starting one when none is
// running. joined reports that this call waited on a load another read
// started, which may predate this call. Callers must not modify the returned
// tree or snapshot; observed reads project over snapshot.Fork().
func (g *loadGroup) do(root string, observe bool) (*store.Tree, *execution.Snapshot, bool, error) {
	key := loadKey{root: root, observe: observe}
	g.mu.Lock()
	if call, ok := g.inflight[key]; ok {
		call.waiters.Add(1)
		g.mu.Unlock()
		<-call.done
		return call.tree, call.snapshot, true, call.err
	}
	call := &loadCall{done: make(chan struct{}), err: errLoadAbandoned}
	g.inflight[key] = call
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.inflight, key)
		g.mu.Unlock()
		close(call.done)
	}()
	call.tree, call.snapshot, call.err = g.load(root, observe)
	return call.tree, call.snapshot, false, call.err
}

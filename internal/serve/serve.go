// Package serve is the watching console's HTTP layer (ui-surface.md; plan
// 2026-07-05, serve pass 1): a read-only local server over the working tree.
// Handlers are thin shells the way the CLI's commands are: load the tree,
// project it through internal/views, write JSON. The console reads the same
// files as the CLI, with cancelled epic subtrees omitted from browsing but
// retained for search, filters, and detail. The embedded UI is served with an
// SPA fallback so client-side routes deep-link.
package serve

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/madeintandem/tiller/internal/core/drift"
	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/gen"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
	"github.com/madeintandem/tiller/internal/views"
)

// Handler builds the console's http.Handler over one repo root: /api/* for the
// read API, everything else the embedded app. It is side-effect-free at build
// time, touching no disk until a request arrives, and each API request loads
// the working tree fresh, so the picture is live local files. The only reuse
// across requests is of immutable inputs (see ui-surface.md, Liveness).
// The console's one kind of disk write, the last-visit memory the recap reads,
// happens lazily inside the visit-window recap handler, never here (see
// visit.go); /api/recap/peek deliberately reads that same horizon without
// moving it, so Home's recap signal cannot spend a recap nobody opened.
// Every route is guarded by the console's request boundary (authority.go
// and boundary.go) before it reads project files, visit memory, or embedded
// assets: the Authority's Host set, plus Origin and fetch-metadata checks,
// plus the Referer check and framing headers on network binds.
func Handler(root string, assets fs.FS, version string, authz *Authority) http.Handler {
	// The console is the one long-lived reader of the event stream.
	events.EnableProcessCache()
	return newMux(root, assets, newVisitState(), version, authz)
}

// newMux wires the routes over an explicit visit state, the seam the visit
// tests drive with an injected clock, and an explicit Authority, the seam
// the boundary tests drive with non-loopback policies. Handler is the
// production entry point that hands it the real-clock state. A missing
// Authority fails closed: the boundary refuses everything rather than
// disabling the checks.
func newMux(root string, assets fs.FS, visits *visitState, version string, authz *Authority) http.Handler {
	mux := http.NewServeMux()
	build := struct {
		Version string `json:"version"`
	}{Version: version}
	// Binary metadata stays available even when the working tree cannot load.
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "the console is read-only; act through the tiller CLI")
			return
		}
		writeJSON(w, http.StatusOK, &build)
	})
	mux.HandleFunc("/api/status", api(root, func(t *store.Tree, _ *http.Request) (any, error) { return views.Summary(t), nil }))
	mux.HandleFunc("/api/board", api(root, func(t *store.Tree, _ *http.Request) (any, error) {
		return views.StreamsOfInput(t, readinput.Workflow(t)), nil
	}))
	mux.HandleFunc("/api/execution", api(root, func(_ *store.Tree, r *http.Request) (any, error) {
		return executionSnapshot(r).Report(), nil
	}))
	mux.HandleFunc("/api/tree", api(root, func(t *store.Tree, _ *http.Request) (any, error) { return gen.BrowseTree(t), nil }))
	mux.HandleFunc("/api/map", MapHandler(root))
	mux.HandleFunc("/api/ls", ListHandler(root))
	mux.Handle("/api/node/", NodeHandler(root))
	mux.HandleFunc("/api/events", EventsHandler(root))
	mux.HandleFunc("/api/queue", QueueHandler(root))
	mux.HandleFunc("/api/next", NextHandler(root))
	mux.HandleFunc("/api/build", BuildHandler(root))
	mux.HandleFunc("/api/work-items", WorkItemsHandler(root))
	mux.HandleFunc("/api/recap", recapHandler(root, visits))
	// Go's ServeMux exact-matches the pattern above, so the peek needs its own
	// line or it falls to the /api/ catch-all's 404.
	mux.HandleFunc("/api/recap/peek", recapPeekHandler(root, visits))
	mux.HandleFunc("/api/report", ReportHandler(root))
	mux.HandleFunc("/api/search", SearchHandler(root))
	mux.HandleFunc("/api/query", QueryHandler(root))
	mux.HandleFunc("/api/prose", ProseHandler(root))
	mux.HandleFunc("/api/evidence", EvidenceHandler(root))
	// The sweep runs fresh per request, the same live-tree posture every
	// other handler here takes (this file's own doc comment), rather than a
	// cache: drift.Run reads git and stats the working tree, which is
	// deliberately the most expensive read this API makes, and that cost is
	// accepted for v1. Do NOT "optimize" this into a cached sweep: the
	// liveness pass (P4) owns caching, and a cache here would go stale
	// silently the moment a file changed underneath a running console, which
	// is exactly the failure this read-only "watching console" must not have.
	// drift.Run's one error (an unknown drift.checks entry) is deliberately
	// not wrapped in an *httpError: it rides api()'s existing default path,
	// the same 500 JSON error shape every other uncaught projection error
	// already gets, so this route needed no new error-mapping arm.
	mux.HandleFunc("/api/drift", apiIntegration(root, func(t *store.Tree, _ *http.Request) (any, error) {
		rep, err := drift.Run(t, vcs.NewHeadSnapshot(vcs.New(root)))
		if err != nil {
			return nil, err
		}
		return views.DriftReportOf(t, rep), nil
	}))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "unknown API route")
	})
	mux.Handle("/", spa(assets))
	return requestBoundary(authz)(mux)
}

// httpError is a projection's typed refusal: the status to answer and the
// message to carry in the JSON error body. A handler returns one instead of
// writing the response itself, so the method guard, the tree load, and the
// error-to-status mapping live in api() once rather than pasted down every
// handler (the pattern search.go's header used to apologize for).
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// rawText is a projection's signal that its payload is not JSON at all: the
// read API's one non-JSON body, the captured command and rubric output the
// evidence endpoint serves verbatim (evidence.go). api() answers it with an
// explicit text/plain Content-Type and X-Content-Type-Options: nosniff so
// captured output a build command wrote can never be sniffed into HTML and
// executed in a browser tab. Every other projection returns any other value
// and rides the ordinary JSON envelope.
type rawText []byte

type postStableResult struct {
	value any
	after func()
}

func afterStability(value any, after func()) postStableResult {
	return postStableResult{value: value, after: after}
}

// api is the one read-only endpoint wrapper every /api/* handler rides. It
// holds the pass-1 posture (GET and HEAD only; the console is read-only), loads
// the working tree fresh (or shares the load of an identical read already in
// flight; a joined read that finds a mutation under the shared load reloads
// once) so the picture is live local files, and renders the
// projection's value as JSON, unless the value is rawText, the one carve-out
// for evidence content that must never render as HTML. A projection signals a
// client error by returning an *httpError with the status it wants: bad input
// is a 400, a node that does not resolve is a 404. Any other error is a 500,
// including store.Load failing, which means the repo went unreadable under a
// running console.
//
// api also loads the request's task execution observations and projects over
// the observed tree. A task source that moves while the request runs costs only
// its own observation: Settle drops it and the projection runs again, so an
// agent committing in its worktree never fails the read. Only movement in the
// served checkout itself is a 500.
func api(root string, project func(*store.Tree, *http.Request) (any, error)) http.HandlerFunc {
	return read(root, true, project)
}

// apiIntegration is api for projections that read only the served checkout:
// queue, route-free guidance, recap, drift, and global history. It skips the
// execution observation reads entirely.
func apiIntegration(root string, project func(*store.Tree, *http.Request) (any, error)) http.HandlerFunc {
	return read(root, false, project)
}

func read(root string, observe bool, project func(*store.Tree, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "the pass-1 console is read-only; act through the tiller CLI")
			return
		}
		v, err := loadAndProject(root, observe, r, project, true)
		var unstable *unstableError
		if errors.As(err, &unstable) {
			writeError(w, http.StatusInternalServerError, unstable.Error())
			return
		}
		if err != nil {
			var he *httpError
			if errors.As(err, &he) {
				writeError(w, he.status, he.msg)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if deferred, ok := v.(postStableResult); ok {
			deferred.after()
			v = deferred.value
		}
		if raw, ok := v.(rawText); ok {
			writeRawText(w, raw)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

// loadAndProject answers one read over a shared load. A read that joined
// another request's load and then sees a Tiller mutation under it reloads
// once: that load may have started before this request arrived, and an
// unshared read would have loaded after the mutation.
func loadAndProject(root string, observe bool, r *http.Request, project func(*store.Tree, *http.Request) (any, error), retry bool) (any, error) {
	tree, snapshot, joined, err := sharedLoads.do(root, observe)
	if err != nil {
		return nil, err
	}
	var v any
	if observe {
		v, err = projectObserved(snapshot.Fork(), r, project)
	} else {
		v, err = project(tree, r)
		if stableErr := tree.CheckStable(); stableErr != nil {
			err = &unstableError{stableErr}
		}
	}
	var unstable *unstableError
	if retry && joined && errors.As(err, &unstable) {
		return loadAndProject(root, observe, r, project, false)
	}
	return v, err
}

// unstableError marks a served checkout that moved under the read; it outranks
// whatever the projection itself returned.
type unstableError struct{ err error }

func (e *unstableError) Error() string { return e.err.Error() }

// projectObserved runs project over the observed tree until every remaining
// observation is confirmed. Each pass that drops a moved source reruns the
// projection, and each drop removes at least one source, so it terminates.
func projectObserved(snapshot *execution.Snapshot, r *http.Request, project func(*store.Tree, *http.Request) (any, error)) (any, error) {
	r = withExecutionSnapshot(r, snapshot)
	for {
		v, err := project(snapshot.Tree, r)
		dropped, stableErr := snapshot.Settle()
		if stableErr != nil {
			return nil, &unstableError{stableErr}
		}
		if !dropped {
			return v, err
		}
	}
}

// spa serves the embedded app: a real file when the path names one, and
// index.html for every other route, so the client router owns the URL space.
//
// Every answer carries an explicit Cache-Control (cacheControlFor), which is
// the fix for a real stale-app bug: with no header at all, a browser's own
// caching heuristic kept serving a prior session's index.html and its
// hashed bundle straight through a binary upgrade, so the console never saw
// the new build. index.html (and anything else outside assets/ or fonts/)
// answers no-cache, not no-store, so a conditional GET can still 304 off a
// validator; Vite's content-hashed assets/* are immutable for a year because
// a hash change is a new URL; fonts/* is stable but unhashed, so it gets a
// short-lived cache instead of "forever".
func spa(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := assets.Open(path); err == nil {
				f.Close()
				w.Header().Set("Cache-Control", cacheControlFor(path))
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "console assets missing; build the UI with `make ui`", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", cacheControlFor("index.html"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

// cacheControlFor picks the embedded UI's Cache-Control by directory
// convention: assets/ is Vite's content-hashed output (a changed file is a
// new URL, so the old one can be cached forever), fonts/ is stable but not
// hashed (a short cache, not immutable), and everything else - index.html
// above all - always revalidates.
func cacheControlFor(path string) string {
	switch {
	case strings.HasPrefix(path, "assets/"):
		return "public, max-age=31536000, immutable"
	case strings.HasPrefix(path, "fonts/"):
		return "public, max-age=86400"
	default:
		return "no-cache"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Every /api/* answer is a fresh read of the live working tree (this
	// file's own header comment), never something safe to serve stale, so it
	// opts out of the browser's own heuristic caching outright rather than
	// leaning on cacheControlFor's revalidate-first default.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// writeRawText answers a rawText payload verbatim: an explicit text/plain
// charset and X-Content-Type-Options: nosniff, so a browser navigating
// straight to the evidence URL (Task C3's evidence link) renders captured
// command output as inert text and never sniffs it into executable HTML.
func writeRawText(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

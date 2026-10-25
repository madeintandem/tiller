package serve

// This file serves recorded gate-evidence content, GET
// /api/evidence?ref=<evidence_ref> (ui-surface.md; plan 2026-07-23
// console-chart-table pass, Task A3): the plain-text twin of the captured
// command output and rubric evidence a node's GateView.Evidence entries point
// at (internal/core/gates writes it under artifacts/tiller/gates/ and
// artifacts/tiller/evals/, a sibling of .tiller/, not inside it). It rides the
// shared api() wrapper like every other read endpoint for the method guard,
// the tree load, and the httpError mapping, but the payload itself is never
// JSON: it is inert captured output that must render as plain text, not HTML,
// so a poisoned build log can never execute in a browser tab when Task C3's
// evidence link opens this URL directly in a new one. It serves only refs the
// loaded tree records (treeRecordsEvidence), never an arbitrary checkout file.

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/madeintandem/tiller/internal/execution"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

// EvidenceHandler answers GET /api/evidence?ref=<path>: the raw bytes of the
// evidence file ref names, read fresh off disk. ref is the repo-relative path
// exactly as a GateView.Evidence[].Ref carries it. An empty ref, an absolute
// one, one that climbs outside the project tree via .., or one entering a
// dot-prefixed path segment is a 400. A well-formed ref that no node in the
// loaded tree records is a 404, decided before disk is touched
// (treeRecordsEvidence), and so is a recorded ref that names no file on disk.
// A recorded ref with a symlink at any segment is a 400 whether the link
// leaves the repo or not: the console follows the store's confinement and
// serves only the regular file a ref names, never an alias's target. A file
// over maxServedFileBytes is a 413. The target is statted before it is read so
// a non-regular object such as a FIFO cannot block the server.
func EvidenceHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, r *http.Request) (any, error) {
		ref := r.URL.Query().Get("ref")
		taskID, revision, digest := r.URL.Query().Get("task"), r.URL.Query().Get("revision"), r.URL.Query().Get("digest")
		if taskID != "" || revision != "" || digest != "" {
			if taskID == "" || revision == "" || len(digest) != sha256.Size*2 {
				return nil, &httpError{http.StatusBadRequest, "execution evidence requires task, revision, and SHA-256 digest"}
			}
			name, err := evidenceRelativePath(ref)
			if err != nil {
				return nil, err
			}
			snapshot := executionSnapshot(r)
			task := snapshot.Tree.Tasks[taskID]
			record, ok := snapshot.Observations[taskID]
			if !ok || task == nil || record.Revision != revision {
				return nil, &httpError{http.StatusConflict, "execution source changed or is unavailable; refresh task detail"}
			}
			if !nodeEvidenceRef(views.Node(snapshot.Tree, task), ref) {
				return nil, &httpError{http.StatusBadRequest, "ref is not recorded evidence for this task"}
			}
			data, err := snapshot.ReadFile(taskID, revision, name, maxServedFileBytes)
			if errors.Is(err, execution.ErrTooLarge) {
				return nil, tooLarge(ref)
			}
			if err != nil {
				return nil, &httpError{http.StatusNotFound, "execution-source evidence unavailable: " + err.Error()}
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
				return nil, &httpError{http.StatusConflict, "execution evidence changed; refresh task detail"}
			}
			return rawText(data), nil
		}
		name, err := evidenceRelativePath(ref)
		if err != nil {
			return nil, err
		}
		if !treeRecordsEvidence(t, ref) {
			return nil, &httpError{http.StatusNotFound, fmt.Sprintf("no recorded evidence at %s", ref)}
		}
		projectRoot, err := os.OpenRoot(root)
		if err != nil {
			return nil, err
		}
		defer projectRoot.Close()
		data, err := readRegularFromRoot(projectRoot, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, &httpError{http.StatusNotFound, fmt.Sprintf("no evidence file at %s", ref)}
			}
			if errors.Is(err, errUnsafeServedPath) {
				return nil, &httpError{http.StatusBadRequest, "ref must not pass through a symlink"}
			}
			if errors.Is(err, errFileTooLarge) {
				return nil, tooLarge(ref)
			}
			return nil, err
		}
		return rawText(data), nil
	})
}

// treeRecordsEvidence reports whether some node in the loaded tree records
// ref, verbatim, as an evidence pointer: a gate's or a criterion's evidence,
// a node's own evidence list, an attestation's evidence_ref, or a policy
// violation's evidence_ref. This is the endpoint's scope. The console only
// links evidence refs the tree records (GateCard and CriterionRow), so
// nothing legitimate asks for anything else, while a server reachable over
// the network must not read arbitrary checkout files such as a gitignored
// secret. Refusing with 404 rather than 403 answers the same way whether or
// not a file exists at the path, so the refusal reveals nothing about disk.
func treeRecordsEvidence(t *store.Tree, ref string) bool {
	for _, f := range t.Features {
		if gatesRecordEvidence(f.GatesBlock, ref) {
			return true
		}
	}
	for _, task := range t.Tasks {
		if evidenceListHas(task.Evidence, ref) || gatesRecordEvidence(task.GatesBlock, ref) {
			return true
		}
	}
	for _, noun := range store.ShelfNouns() {
		for _, n := range t.WorkNodesOf(noun) {
			if workItemRecordsEvidence(n.Body(), ref) {
				return true
			}
		}
	}
	return false
}

func workItemRecordsEvidence(item *store.WorkItem, ref string) bool {
	return evidenceListHas(item.Evidence, ref) || gatesRecordEvidence(item.GatesBlock, ref)
}

func gatesRecordEvidence(b store.GatesBlock, ref string) bool {
	for _, g := range b.Gates {
		if evidenceListHas(g.Evidence, ref) {
			return true
		}
		for _, c := range g.Criteria {
			if evidenceListHas(c.Evidence, ref) {
				return true
			}
		}
	}
	for _, a := range b.Attestations {
		if a.EvidenceRef == ref {
			return true
		}
	}
	for _, v := range b.PolicyViolations {
		if v.EvidenceRef == ref {
			return true
		}
	}
	return false
}

func evidenceListHas(list []store.Evidence, ref string) bool {
	for _, e := range list {
		if e.Ref == ref {
			return true
		}
	}
	return false
}

// evidenceRelativePath applies evidence's established lexical ref rules and
// returns the cleaned ref. The lexical checks reject what
// a client should never be able to name before it reaches disk, rather than
// trusting filepath.Join to make an unsafe ref safe. An empty ref, an
// absolute one, any ref whose cleaned form climbs outside root via .., and
// any ref entering a dot-prefixed path segment are each refused with the
// read API's misuse status. The dot rule is the scope's teeth (plan
// 2026-07-23, A3 as amended): no legitimate ref names a hidden path,
// evidence living under artifacts/tiller/ and test-binding refs naming bare
// repo paths like spec/..., while .git/ and .tiller/ hold repo internals
// and the gitignored identity file that a pass-1 local server with no
// origin or auth checks must never serve. The handler runs these rules before
// the recorded-ref check, so a malformed ref stays a 400 whether or not the
// ledger records it: a record narrows what is served, it never widens it.
func evidenceRelativePath(ref string) (string, error) {
	if ref == "" {
		return "", &httpError{http.StatusBadRequest, "ref is required"}
	}
	if filepath.IsAbs(ref) {
		return "", &httpError{http.StatusBadRequest, "ref must be a relative path"}
	}
	clean := filepath.Clean(ref)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", &httpError{http.StatusBadRequest, "ref must resolve inside the project tree"}
	}
	for _, seg := range strings.Split(clean, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			return "", &httpError{http.StatusBadRequest, "ref must not name dot-prefixed path segments"}
		}
	}
	return clean, nil
}

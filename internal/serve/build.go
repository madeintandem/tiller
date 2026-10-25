package serve

// This file serves GET /api/build: the Build view's board (views.BuildBoard,
// internal/views/build.go), epics and their own features and tasks already
// in the shared dependency order, so the client filters the served sequence
// into columns and rows and never sorts anything itself (task-12-brief.md,
// phase 3 acceptance: "no client-side ordering code exists"). Queue and
// drift both ride their own endpoints and are deliberately absent from this
// payload; the client overlays both by node id, the same posture /api/board
// (StreamsOf) already takes toward queue data. See build.go's own package
// comment for the drift-mechanism cost reasoning.

import (
	"net/http"

	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

// BuildHandler answers GET /api/build with the Build view's board.
func BuildHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, _ *http.Request) (any, error) {
		return views.BuildBoardOf(t), nil
	})
}

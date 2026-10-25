package serve

import (
	"net/http"

	"github.com/madeintandem/tiller/internal/core/events"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

func WorkItemsHandler(root string) http.HandlerFunc {
	return api(root, func(tree *store.Tree, _ *http.Request) (any, error) {
		evs, err := events.List(tree)
		if err != nil {
			return nil, err
		}
		return views.WorkItemsBoardOf(tree, evs), nil
	})
}

package serve

import (
	"net/http"

	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/views"
)

func MapHandler(root string) http.HandlerFunc {
	return api(root, func(t *store.Tree, r *http.Request) (any, error) {
		index, err := readinput.Relationships(r.Context(), t, readinput.RelationshipOptions{AllowIncompleteCorpus: true})
		if err != nil {
			return nil, err
		}
		return views.MapOf(t, index), nil
	})
}

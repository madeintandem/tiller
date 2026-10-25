package serve

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/madeintandem/tiller/internal/core/resolve"
	"github.com/madeintandem/tiller/internal/readinput"
	"github.com/madeintandem/tiller/internal/relationships"
	"github.com/madeintandem/tiller/internal/store"
)

// QueryHandler answers GET /api/query with exactly the data carried by the CLI
// query envelope. api supplies GET/HEAD-only policy, a fresh tree, and the final
// stability check; all parsing and graph semantics are shared with the CLI.
func QueryHandler(root string) http.HandlerFunc {
	return api(root, func(tree *store.Tree, request *http.Request) (any, error) {
		values, err := url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			return nil, &httpError{http.StatusBadRequest, err.Error()}
		}
		for name := range values {
			switch name {
			case "from", "follow", "depth", "where":
			default:
				return nil, &httpError{http.StatusBadRequest, "unknown query parameter: " + name}
			}
		}
		input, err := relationships.ParseQuery(values["from"], values["follow"], values["depth"], values["where"])
		if err != nil {
			return nil, &httpError{http.StatusBadRequest, err.Error()}
		}
		index, err := readinput.Relationships(request.Context(), tree, readinput.RelationshipOptions{})
		if err != nil {
			return nil, err
		}
		result, err := relationships.Query(request.Context(), index, input)
		if err != nil {
			var missing *resolve.ErrNotFound
			var ambiguous *resolve.ErrAmbiguousKey
			var mismatch *resolve.ErrNounMismatch
			var misuse *relationships.QueryError
			switch {
			case errors.As(err, &missing), errors.As(err, &ambiguous), errors.As(err, &mismatch):
				return nil, &httpError{http.StatusNotFound, err.Error()}
			case errors.As(err, &misuse):
				return nil, &httpError{http.StatusBadRequest, err.Error()}
			default:
				return nil, err
			}
		}
		return result, nil
	})
}

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/madeintandem/tiller/internal/core/mutate"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/spf13/cobra"
)

func migrationReview(cmd *cobra.Command) (*mutate.OwnershipReview, error) {
	path, _ := cmd.Flags().GetString("ownership-review")
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ownership review %s: %w", path, err)
	}
	// Accept the whole saved dry-run envelope, its data, or the review itself.
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, invalidReviewFile(path, err)
	}
	if payload, ok := wrapper["data"]; ok {
		var success bool
		if err := json.Unmarshal(wrapper["ok"], &success); err != nil || !success {
			return nil, invalidReviewFile(path, fmt.Errorf("expected a successful dry-run report"))
		}
		wrapper = nil
		data = payload
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, invalidReviewFile(path, err)
		}
	}
	if payload, ok := wrapper["ownership_review"]; ok {
		data = payload
	}
	var review mutate.OwnershipReview
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&review); err != nil {
		return nil, invalidReviewFile(path, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, invalidReviewFile(path, fmt.Errorf("expected one review document"))
	}
	return &review, nil
}

func invalidReviewFile(path string, err error) error {
	return &store.EnvError{Code: 3, Reason: store.ReasonOwnershipReviewInvalid, Msg: fmt.Sprintf("invalid ownership review %s: %v; save `tiller migrate --dry-run --json` and have a human fill every confirmed_owner", path, err)}
}

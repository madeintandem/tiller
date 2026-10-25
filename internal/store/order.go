package store

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProjectOrder is the machine-owned root soft order (.tiller/order.yml): the
// recorded epic sequence. The file is absent until tiller order first writes
// it, and hand-editing it is the same machine-owned-state violation as any
// other .yml edit. See docs/plans/2026-08-08-soft-order-design.md.
type ProjectOrder struct {
	Epics []string `yaml:"epics,omitempty"`
}

// OrderRelPath is the root soft order file, relative to the project root.
const OrderRelPath = ".tiller/order.yml"

// LoadProjectOrder reads the root soft order. An absent file is no recorded
// order, not an error.
func LoadProjectOrder(root string) (ProjectOrder, error) {
	var po ProjectOrder
	data, err := ReadRepositoryFile(root, OrderRelPath)
	if errors.Is(err, os.ErrNotExist) {
		return po, nil
	}
	if err != nil {
		return po, fmt.Errorf("read %s: %w", OrderRelPath, err)
	}
	if err := yaml.Unmarshal(data, &po); err != nil {
		return po, fmt.Errorf("parse %s: %w", OrderRelPath, err)
	}
	return po, nil
}

package cli

import (
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/internal/vcs"
)

// onIntegrationBranch says whether the checkout is on the branch a repo-wide
// housekeeping write (usage capture, event compaction) may happen on: the
// declared integration branch, or any branch when none is declared.
func onIntegrationBranch(tree *store.Tree) (ok bool, current, integration string, err error) {
	integration = tree.Manifest.Git.IntegrationBranch
	if integration == "" {
		return true, "", "", nil
	}
	current, err = vcs.New(tree.Root).Branch()
	if err != nil {
		return false, "", integration, err
	}
	return current == integration, current, integration, nil
}

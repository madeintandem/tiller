package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Only the History page's Mark caught up button moves the recap's horizon;
// viewing History never does (docs/tiller-commands.md, the serve section).
func TestServeHelpNamesTheMarkCaughtUpButtonAsTheOnlyVisitMark(t *testing.T) {
	long := newServeCmd().Long
	require.Contains(t, long, "Only the console's Mark caught up button on History\nadvances the recap's last-visit mark;")
	require.NotContains(t, long, "History view advances")
}

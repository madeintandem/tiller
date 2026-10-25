package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransferCommandReturnsRunnableRecipientContinuation(t *testing.T) {
	originalID := layTaskReadyToStart(t)
	mustRun(t, "claim", "task", originalID, "--contributor", "jc")
	mustRun(t, "start", "task", originalID, "--contributor", "jc")
	out, stderr, code := execCLI(t, "transfer", "task", originalID, "--to", "pat", "--reason", "human-directed handoff", "--contributor", "jc", "--json")
	require.Equal(t, 0, code, stderr+out)
	data := jsonData(t, out)
	original := data["original"].(map[string]any)
	replacement := data["replacement"].(map[string]any)
	require.Equal(t, originalID, original["id"])
	require.Equal(t, "jc", original["owner"])
	require.Equal(t, "pat", replacement["owner"])
	require.NotEqual(t, original["key"], replacement["key"])
	nextID := replacement["id"].(string)
	tree := loadCwdTree(t)
	require.Equal(t, "cancelled", tree.Tasks[originalID].Status)
	require.Equal(t, "ready", tree.Tasks[nextID].Status)
	require.False(t, tree.Tasks[nextID].Started)
	command := strings.Fields(data["start_command"].(string))
	require.Equal(t, "tiller", command[0])
	mustRun(t, command[1:]...)
	tree = loadCwdTree(t)
	require.True(t, tree.Tasks[nextID].Started)
	require.Equal(t, "pat", tree.Tasks[nextID].Owner)
	out, _, code = execCLI(t, "transfer", "task", originalID, "--to", "pat", "--reason", "repeat", "--contributor", "jc", "--json")
	require.Equal(t, 1, code, out)
	require.Equal(t, "transfer_already_replaced", jsonData(t, out)["reason"])
	require.Equal(t, nextID, jsonData(t, out)["successor"])
	require.Len(t, loadCwdTree(t).Tasks, 2)
}

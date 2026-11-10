package serve_test

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestListOpenQuestionCarriesItsCommands pins the console-only commands an
// open question row carries: the server composes them, the UI only shows them.
func TestListOpenQuestionCarriesItsCommands(t *testing.T) {
	srv := lsServer(t)
	var got struct {
		Nodes []struct {
			ID       string `json:"id"`
			Commands *struct {
				Answer string   `json:"answer"`
				Cancel string   `json:"cancel"`
				Unlink []string `json:"unlink"`
			} `json:"commands"`
		} `json:"nodes"`
	}
	resp, err := srv.Client().Get(srv.URL + "/api/ls?noun=questions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got.Nodes, 1)
	require.Equal(t, "Q-000001", got.Nodes[0].ID)
	require.NotNil(t, got.Nodes[0].Commands)
	require.Equal(t, `tiller answer question Q-000001 --answer "..."`, got.Nodes[0].Commands.Answer)
	require.Equal(t, `tiller cancel question Q-000001 --reason "..."`, got.Nodes[0].Commands.Cancel)
	require.Equal(t, []string{"tiller unlink question Q-000001 --blocks F-000001"}, got.Nodes[0].Commands.Unlink)
}

// TestListOtherNounsCarryNoCommands pins that commands is a question-only field.
func TestListOtherNounsCarryNoCommands(t *testing.T) {
	srv := lsServer(t)
	resp, err := srv.Client().Get(srv.URL + "/api/ls?noun=epics")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), `"commands"`)
}

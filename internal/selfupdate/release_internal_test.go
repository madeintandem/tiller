package selfupdate

// This one white-box test reaches httpClient() directly, in package
// selfupdate rather than selfupdate_test, because the method and the
// timeout it falls back to are both unexported. The black-box tests in
// release_test.go build a Client{} literal for six of their seven cases
// (release_test.go's own header explains why), so httpClient()'s fallback
// runs on every request those tests make; if it ever silently reverted to
// http.DefaultClient's no-timeout zero value, a hung server would stall
// those tests (and any real `tiller upgrade` run built the same way)
// forever instead of failing fast.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPClientFallsBackToATimeoutBearingClient(t *testing.T) {
	c := &Client{}
	got := c.httpClient()
	require.NotNil(t, got)
	require.Equal(t, 30*time.Second, got.Timeout)
}

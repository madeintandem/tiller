package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runServeInFreshRepo runs `tiller serve` with args on port 0 in a freshly scaffolded
// repo, recording every browser open the command attempts, and returns the
// command's stdout, exit code, and the URLs opened. expectOpen says which
// signal ends the run: the recorded open (whose address the helper then proves
// is served) or the serving line alone. The opener runs before the accept
// loop starts, so once the command returns the recorder is complete either way.
func runServeInFreshRepo(t *testing.T, expectOpen bool, args ...string) (stdout string, code int, opened []string) {
	t.Helper()
	initRepo(t)
	return runServeInCurrentRepo(t, expectOpen, args...)
}

// runServeInCurrentRepo is runServeInFreshRepo for a repo the test has already
// scaffolded, so it can lay a .tiller/local.yml down first.
func runServeInCurrentRepo(t *testing.T, expectOpen bool, args ...string) (stdout string, code int, opened []string) {
	t.Helper()
	stdout, _, code, opened = runServeCapturingStderr(t, expectOpen, args...)
	return stdout, code, opened
}

// runServeCapturingStderr runs the same serve scenario as runServeInCurrentRepo
// but keeps stderr too: the network-mode exposure warnings and the human
// failure lines land there, and the host tests must observe them separately
// from the stdout serving lines.
func runServeCapturingStderr(t *testing.T, expectOpen bool, args ...string) (stdout, stderr string, code int, opened []string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out lockedBuffer
	var errb lockedBuffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(append([]string{"serve", "--port", "0"}, args...))

	previous := openBrowser
	t.Cleanup(func() { openBrowser = previous })
	openedURL := make(chan string, 1)
	openBrowser = func(url string) error {
		openedURL <- url
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	if expectOpen {
		select {
		case url := <-openedURL:
			opened = append(opened, url)
			// Prove the address handed to the browser is the one being served.
			resp, err := http.Get(url + "/api/status")
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		case err := <-done:
			require.Fail(t, "serve returned before opening a browser", "%v\n%s%s", err, out.String(), errb.String())
		case <-time.After(5 * time.Second):
			require.Fail(t, "serve never opened a browser", "%s%s", out.String(), errb.String())
		}
	} else {
		require.Eventually(t, func() bool {
			select {
			case err := <-done:
				done <- err
				return true
			default:
				return strings.Contains(out.String(), "serving")
			}
		}, 5*time.Second, 10*time.Millisecond, "serve neither served nor returned")
	}
	cancel()
	err := <-done
	code = 0
	if err != nil {
		var ee *exitError
		require.True(t, errors.As(err, &ee), "serve failed: %v\n%s", err, errb.String())
		code = ee.code
	}
	select {
	case url := <-openedURL:
		opened = append(opened, url)
	default:
	}
	return out.String(), errb.String(), code, opened
}

func writeLocalSettingsFile(t *testing.T, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(".tiller", "local.yml"), []byte(body), 0o644))
}

func TestServeOpensTheConsoleByDefault(t *testing.T) {
	t.Setenv("BROWSER", "")
	out, code, opened := runServeInFreshRepo(t, true)
	require.Equal(t, 0, code, out)
	require.Len(t, opened, 1)
	require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
	require.NotEqual(t, "http://127.0.0.1:0", opened[0], "the opened address carries the bound port, not the requested 0")
	require.Contains(t, out, "serving")
	require.Contains(t, out, opened[0], "the printed address and the opened one agree")
}

func TestServeOpenFalseSuppressesTheBrowser(t *testing.T) {
	t.Setenv("BROWSER", "")
	out, code, opened := runServeInFreshRepo(t, false, "--open=false")
	require.Equal(t, 0, code, out)
	require.Empty(t, opened)
	require.Contains(t, out, "serving")
}

func TestServeHonorsLocalSettingsConsoleOpenFalse(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	writeLocalSettingsFile(t, "console:\n  open: false\n")
	out, code, opened := runServeInCurrentRepo(t, false)
	require.Equal(t, 0, code, out)
	require.Empty(t, opened)
	require.Contains(t, out, "serving")
}

func TestServeHonorsBrowserNone(t *testing.T) {
	t.Setenv("BROWSER", "none")
	out, code, opened := runServeInFreshRepo(t, false)
	require.Equal(t, 0, code, out)
	require.Empty(t, opened)
	require.Contains(t, out, "serving")
}

// The flag is the most specific word: an explicit --open beats a local.yml
// that says no, and local.yml beats the environment.
func TestServeOpenPrecedence(t *testing.T) {
	t.Run("flag beats local.yml", func(t *testing.T) {
		t.Setenv("BROWSER", "")
		initRepo(t)
		writeLocalSettingsFile(t, "console:\n  open: false\n")
		_, code, opened := runServeInCurrentRepo(t, true, "--open")
		require.Equal(t, 0, code)
		require.Len(t, opened, 1)
	})
	t.Run("local.yml beats BROWSER=none", func(t *testing.T) {
		t.Setenv("BROWSER", "none")
		initRepo(t)
		writeLocalSettingsFile(t, "console:\n  open: true\n")
		_, code, opened := runServeInCurrentRepo(t, true)
		require.Equal(t, 0, code)
		require.Len(t, opened, 1)
	})
}

// A malformed local.yml is a real refusal before the port is bound, the same
// posture a corrupt identity file gets: silently opening (or not) would hide
// the typo that made the preference stop applying.
func TestServeRefusesAMalformedLocalSettingsFile(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	writeLocalSettingsFile(t, "console: [\n")
	out, code, opened := runServeInCurrentRepo(t, false, "--json")
	require.Equal(t, 3, code)
	require.Empty(t, opened)
	require.NotContains(t, out, "serving")
	require.Contains(t, out, `"local_settings_unreadable"`, "the refusal names the file's own reason")
	require.Contains(t, out, "local.yml", "the refusal names the file")
}

// Opening is best effort: a machine with no browser keeps serving and says so.
func TestServeKeepsServingWhenTheBrowserCannotOpen(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	previous := openBrowser
	t.Cleanup(func() { openBrowser = previous })
	openBrowser = func(string) error { return errors.New("exec: \"xdg-open\": executable file not found in $PATH") }

	ctx, cancel := context.WithCancel(context.Background())
	var out lockedBuffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"serve", "--port", "0"})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "could not open a browser") }, 5*time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.Contains(t, out.String(), "serving")
	require.Contains(t, out.String(), "open the address above yourself")
}

// An invalid --host is argument misuse: exit 2 with the stable reason, the
// supported-literal guidance, and no port bound and no browser opened.
func TestServeRefusesAnInvalidHost(t *testing.T) {
	for _, host := range []string{
		"",
		"localhost",
		"mitbox.local",
		"[::1]",
		"[192.168.1.5]",
		"192.168.1.5:4400",
		"fe80::1%eth0",
		"::ffff:192.168.1.5",
		"224.0.0.1",
		"169.254.1.1",
		"255.255.255.255",
		"not-an-ip",
	} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("BROWSER", "")
			initRepo(t)
			out, stderr, code, opened := runServeCapturingStderr(t, false, "--host", host, "--json")
			require.Equal(t, 2, code, out+stderr)
			require.Empty(t, opened)
			require.Contains(t, out, `"host_invalid"`, "the refusal names its own reason")
		})
	}
}

// The explicit default is the default: the same serving line, the same opened
// URL, and the same no-warning posture as omitting --host.
func TestServeExplicitDefaultHostMatchesTheDefault(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	stdout, stderr, code, opened := runServeCapturingStderr(t, true, "--host", "127.0.0.1")
	require.Equal(t, 0, code, stdout)
	require.Len(t, opened, 1)
	require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
	require.Contains(t, stdout, opened[0])
	require.Empty(t, stderr, "loopback mode warns nothing")
}

// A second loopback literal is still loopback mode: it serves and opens the
// bound literal, not 127.0.0.1, and warns nothing.
func TestServeExplicitLoopbackBindServesItsOwnLiteral(t *testing.T) {
	t.Setenv("BROWSER", "")
	probe, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Skipf("127.0.0.2 cannot be bound on this OS: %v (the pure-policy regression for explicit loopback still runs in internal/serve)", err)
	}
	probe.Close()
	initRepo(t)
	stdout, stderr, code, opened := runServeCapturingStderr(t, true, "--host", "127.0.0.2")
	require.Equal(t, 0, code, stdout)
	require.Len(t, opened, 1)
	require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.2:"), opened[0])
	require.Contains(t, stdout, opened[0])
	require.Empty(t, stderr, "a second loopback literal does not expose the console")
}

// --host ::1 is IPv6 loopback mode: the bracketed URL is what gets printed and
// opened, and the opened address serves an API read.
func TestServeIPv6LoopbackBind(t *testing.T) {
	t.Setenv("BROWSER", "")
	probe, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 is unavailable on this machine: %v", err)
	}
	probe.Close()
	initRepo(t)
	stdout, stderr, code, opened := runServeCapturingStderr(t, true, "--host", "::1")
	require.Equal(t, 0, code, stdout)
	require.Len(t, opened, 1)
	require.True(t, strings.HasPrefix(opened[0], "http://[::1]:"), opened[0])
	require.Contains(t, stdout, opened[0])
	require.Empty(t, stderr, "an IPv6 loopback bind does not expose the console")
}

// A wildcard bind advertises the listening socket and the admitted
// destinations as separate things, opens the standard loopback, and warns
// about the exposure it creates.
func TestServeWildcardBindPrintsListeningAndDestinations(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	stdout, stderr, code, opened := runServeCapturingStderr(t, true, "--host", "0.0.0.0")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, stdout, "serving")
	require.Contains(t, stdout, "Listening on 0.0.0.0:", "the listen socket is printed as a listen address")
	require.Len(t, opened, 1)
	require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
	require.Contains(t, stdout, opened[0], "the opened URL is a printed destination")
	require.NotContains(t, stdout, "http://0.0.0.0:", "the wildcard literal is not an HTTP destination")
	require.NotContains(t, stdout, "http://[", "a v4 wildcard never advertises the other family")
	require.Contains(t, stderr, "warning:", "a network bind warns before the browser opens")
	require.Contains(t, stderr, "authentication or TLS")
	require.Contains(t, stderr, "every interface")
}

func TestServeBareHostUsesIPv4Wildcard(t *testing.T) {
	for _, args := range [][]string{
		{"--host"},
		{"--host", "--open=true"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("BROWSER", "")
			initRepo(t)
			stdout, stderr, code, opened := runServeCapturingStderr(t, true, args...)
			require.Equal(t, 0, code, stdout+stderr)
			require.Contains(t, stdout, "Listening on 0.0.0.0:")
			require.Len(t, opened, 1)
			require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
			require.Contains(t, stderr, "without authentication or TLS")
			require.Contains(t, stderr, "every interface")
		})
	}
}

func TestServeHostLastOccurrenceWins(t *testing.T) {
	for _, args := range [][]string{
		{"--host", "--host", "127.0.0.1"},
		{"--host=0.0.0.0", "--host=127.0.0.1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("BROWSER", "")
			initRepo(t)
			stdout, stderr, code, opened := runServeCapturingStderr(t, true, args...)
			require.Equal(t, 0, code, stdout+stderr)
			require.Len(t, opened, 1)
			require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
			require.NotContains(t, stdout, "Listening on 0.0.0.0:")
			require.Empty(t, stderr)
		})
	}
}

func TestServeRejectsUnexpectedArgumentsWithBareHost(t *testing.T) {
	for _, args := range [][]string{
		{"--host", "--", "127.0.0.1"},
		{"--host", "--unknown"},
		{"--host", "127.0.0.1", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRootCmd()
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"serve"}, args...))
			require.Error(t, root.Execute())
			require.NotContains(t, out.String(), "serving")
		})
	}
}

// An address the machine has not assigned to an interface is an environment
// refusal, not argument misuse: exit 3, the attempted address, and a hint that
// names the --host remedy, not just the port.
func TestServeRefusesAnUnassignedAddress(t *testing.T) {
	t.Setenv("BROWSER", "")
	host := unassignedHost(t)
	initRepo(t)
	stdout, _, code, opened := runServeCapturingStderr(t, false, "--host", host, "--json")
	require.Equal(t, 3, code, stdout)
	require.Empty(t, opened)
	require.NotContains(t, stdout, "serving")
	require.Contains(t, stdout, "cannot bind "+net.JoinHostPort(host, "0"), "the refusal names the full attempted address")
	require.Contains(t, stdout, "port_unavailable")
	require.Contains(t, stdout, "choose a --host the machine has assigned")
}

// unassignedHost picks a 192.168.x.y literal the machine cannot bind, so the
// test exercises the refusal instead of assuming an address is free. It skips
// with a reason rather than fail if every candidate happens to be assigned.
func unassignedHost(t *testing.T) string {
	t.Helper()
	for _, host := range []string{"192.168.99.99", "192.168.77.123", "192.168.44.201"} {
		probe, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
		if err != nil {
			return host
		}
		probe.Close()
	}
	t.Skip("every candidate 192.168.x.y address is locally assigned; no unassigned-address refusal to exercise")
	return ""
}

// A held port is still the port_unavailable environment error, with the
// hint now covering the --host remedy as well.
func TestServeHeldPortKeepsTheEnvironmentRefusal(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { held.Close() })
	port := held.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out lockedBuffer
	var errb lockedBuffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"serve", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--json"})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	select {
	case err := <-done:
		require.Error(t, err, out.String()+errb.String())
		var exit *exitError
		require.ErrorAs(t, err, &exit)
		require.Equal(t, 3, exit.code)
	case <-time.After(5 * time.Second):
		require.Fail(t, "serve never refused the held port", out.String()+errb.String())
	}
	require.Contains(t, out.String(), `"port_unavailable"`)
	require.Contains(t, out.String(), net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "the refusal carries the full attempted address")
	require.Contains(t, out.String(), "choose a --host the machine has assigned")
}

// The open precedence is unchanged by --host: in network mode the flag still
// beats a local.yml that says no, and a refused open never stops the serve.
func TestServeNetworkModeKeepsTheOpenPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		browser  string
		settings string
		args     []string
		open     bool
	}{
		{name: "flag beats file", settings: "console:\n  open: false\n", args: []string{"--open"}, open: true},
		{name: "flag disables file", settings: "console:\n  open: true\n", args: []string{"--open=false"}},
		{name: "file beats environment", browser: "none", settings: "console:\n  open: true\n", open: true},
		{name: "file disables default", settings: "console:\n  open: false\n"},
		{name: "environment disables default", browser: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BROWSER", tc.browser)
			initRepo(t)
			if tc.settings != "" {
				writeLocalSettingsFile(t, tc.settings)
			}
			args := append([]string{"--host", "0.0.0.0"}, tc.args...)
			_, stderr, code, opened := runServeCapturingStderr(t, tc.open, args...)
			require.Equal(t, 0, code)
			require.Contains(t, stderr, "without authentication or TLS")
			// The warning names exactly what is readable, not a broader
			// "entire corpus" the endpoints no longer serve.
			require.Contains(t, stderr, "the .tiller/ Markdown corpus, Markdown documents the ledger cites, and gate evidence files the ledger records")
			require.NotContains(t, stderr, "entire project corpus")
			if tc.open {
				require.Equal(t, 1, len(opened))
				require.True(t, strings.HasPrefix(opened[0], "http://127.0.0.1:"), opened[0])
			} else {
				require.Empty(t, opened)
			}
		})
	}
}

// TestConsoleServerBoundsSlowClients pins the console's http.Server limits.
// A network bind accepts connections from anyone who can reach the port, so a
// client that trickles headers, never finishes a request, never reads a
// response, or parks an idle keep-alive connection must be cut off rather
// than holding a goroutine and socket forever. No console endpoint streams,
// so a write deadline is safe too.
func TestConsoleServerBoundsSlowClients(t *testing.T) {
	handler := http.NotFoundHandler()
	srv := consoleServer(handler)
	require.NotNil(t, srv.Handler)
	require.Equal(t, 10*time.Second, srv.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, srv.ReadTimeout)
	require.Equal(t, 2*time.Minute, srv.WriteTimeout)
	require.Equal(t, 2*time.Minute, srv.IdleTimeout)
}

// lockedBuffer is a bytes.Buffer the test may read while the command goroutine
// is still writing to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func TestServeIPv6WildcardBind(t *testing.T) {
	probe, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skipf("IPv6 wildcard is unavailable on this machine: %v", err)
	}
	require.NoError(t, probe.Close())
	t.Setenv("BROWSER", "")
	initRepo(t)
	stdout, stderr, code, opened := runServeCapturingStderr(t, true, "--host", "::")
	require.Equal(t, 0, code, stdout+stderr)
	require.Len(t, opened, 1)
	u, err := url.Parse(opened[0])
	require.NoError(t, err)
	require.Equal(t, "::1", u.Hostname())
	require.NotEqual(t, "0", u.Port())
	require.Contains(t, stdout, "Listening on "+net.JoinHostPort("::", u.Port()))
	require.Contains(t, stdout, opened[0])
	require.NotContains(t, stdout, "http://[::]:")
	require.NotContains(t, stdout, "http://127.")
	require.Contains(t, stderr, "every interface")
}

func TestServeNetworkModeRefusesMalformedLocalSettings(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	writeLocalSettingsFile(t, "console: [\n")
	out, _, code, opened := runServeCapturingStderr(t, false, "--host", "0.0.0.0", "--json")
	require.Equal(t, 3, code)
	require.Empty(t, opened)
	require.NotContains(t, out, "serving")
	require.Contains(t, out, `"local_settings_unreadable"`)
}

func TestServeNetworkModeSurvivesOpenerFailure(t *testing.T) {
	t.Setenv("BROWSER", "")
	initRepo(t)
	previous := openBrowser
	t.Cleanup(func() { openBrowser = previous })
	attempted := make(chan string, 1)
	openBrowser = func(u string) error {
		attempted <- u
		return errors.New("opener unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb lockedBuffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"serve", "--host", "0.0.0.0", "--port", "0"})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	select {
	case u := <-attempted:
		resp, err := http.Get(u + "/api/tree")
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)
	case err := <-done:
		require.FailNow(t, "serve returned before opener", "%v\n%s", err, errb.String())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "serve never attempted opener")
	}
	cancel()
	require.NoError(t, <-done)
	require.Contains(t, out.String(), "could not open a browser")
}

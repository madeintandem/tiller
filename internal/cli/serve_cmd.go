package cli

// This file wires the read-only console over this checkout. The handler lives
// in internal/serve and the built UI is embedded in the binary. The default
// listener is loopback; --host explicitly opts into unauthenticated plain-HTTP
// network access. Any future write endpoints need a separate security design.
//
// Once the port is bound, serve opens the console in the developer's browser
// (issue #109). Opening is a convenience, never a gate: a machine with no
// browser keeps serving and says so. Three words can turn it off, most
// specific first: --open=false on the command line, `console: { open: false }`
// in the hand-edited per-checkout .tiller/local.yml, and BROWSER=none in the
// environment, the conventional "never open a browser for me" signal.

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/madeintandem/tiller/internal/serve"
	"github.com/madeintandem/tiller/internal/store"
	"github.com/madeintandem/tiller/ui"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var host string
	var port int
	var open bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local web console (read-only in this build).",
		Long: "serve starts the watching console: the project summary, the\n" +
			"per-owner streams, and (as later passes land) the full read surface of\n" +
			"the working tree. It is read-only in this build; every action stays a\n" +
			"CLI command, and the console shows you which one.\n" +
			"\n" +
			"By default the console listens on 127.0.0.1 and is reachable only from\n" +
			"this machine. Bare --host listens on every IPv4 interface (0.0.0.0),\n" +
			"using port 4400 unless --port overrides it. --host also takes a bare\n" +
			"IPv4 or IPv6 literal: :: listens on every IPv6 interface, and a\n" +
			"specific address such as 192.168.1.5 on one. A\n" +
			"network bind serves the planning ledger, the .tiller/ Markdown corpus,\n" +
			"Markdown documents the ledger cites, and gate evidence files the ledger\n" +
			"records over plain HTTP without authentication or TLS; use it only on\n" +
			"a trusted network or private VPN, never directly on the public internet.\n" +
			"Only the console's History view advances the recap's last-visit mark;\n" +
			"other requests, such as curl or a foreign page, read it without moving it.\n" +
			"\n" +
			"Once the port is bound, serve opens the console in your browser. Pass\n" +
			"--open=false to skip that, or set it once for this checkout with\n" +
			"`console: { open: false }` in the git-ignored .tiller/local.yml. The\n" +
			"flag beats the file, the file beats BROWSER=none in the environment,\n" +
			"and a browser that cannot be opened never stops the server.",
		// Normalize only bare --host before handing flags to Cobra. pflag's
		// NoOptDefVal would stop consuming existing `--host <ip>` values.
		DisableFlagParsing: true,
		Args: func(cmd *cobra.Command, args []string) error {
			var normalized []string
			for i, arg := range args {
				if arg == "--" {
					break
				}
				if arg == "--host" && (i+1 == len(args) || strings.HasPrefix(args[i+1], "-")) {
					if normalized == nil {
						normalized = append([]string(nil), args...)
					}
					normalized[i] = "--host=0.0.0.0"
				}
			}
			if normalized != nil {
				args = normalized
			}
			cmd.DisableFlagParsing = false
			err := cmd.ParseFlags(args)
			cmd.DisableFlagParsing = true
			if err != nil {
				return err
			}
			if help, _ := cmd.Flags().GetBool("help"); help {
				return nil
			}
			return cobra.NoArgs(cmd, cmd.Flags().Args())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if help, _ := cmd.Flags().GetBool("help"); help {
				return cmd.Help()
			}
			var explicit *bool
			if cmd.Flags().Changed("open") {
				explicit = &open
			}
			return runServe(cmd, host, port, explicit)
		},
	}
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "address to serve on: bare --host uses 0.0.0.0 (every IPv4 interface); an explicit value accepts a bare IPv4 or IPv6 literal; omitted uses 127.0.0.1; hostnames are not resolved")
	cmd.Flags().IntVar(&port, "port", 4400, "port to serve on")
	cmd.Flags().BoolVar(&open, "open", true, "open the console in your browser once serving (--open=false to skip; .tiller/local.yml console.open and BROWSER=none also apply)")
	return cmd
}

// openBrowser starts the platform's URL opener and returns without waiting on
// it. A variable so the serve tests can record the attempt instead of opening
// a real browser window on every run.
var openBrowser = func(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}

// shouldOpenBrowser resolves the open decision, most specific word first: an
// explicit --open value, then the per-checkout local.yml, then BROWSER=none.
// A malformed local.yml is a real error, not a silent default.
func shouldOpenBrowser(root string, explicit *bool) (bool, error) {
	if explicit != nil {
		return *explicit, nil
	}
	settings, err := store.ReadLocalSettings(root)
	if err != nil {
		return false, &store.EnvError{Reason: store.ReasonLocalSettingsUnreadable, Msg: err.Error()}
	}
	if settings.Console.Open != nil {
		return *settings.Console.Open, nil
	}
	return os.Getenv("BROWSER") != "none", nil
}

// consoleServer wraps the console handler in an http.Server that bounds slow
// clients. A network bind accepts anyone who can reach the port, so trickled
// headers, an unfinished request, an unread response, or a parked keep-alive
// connection each time out instead of holding a goroutine and socket open.
// Every console request is a small GET, so 10s for headers and 30s for the
// whole request are generous. No endpoint streams (no SSE, no long poll), so
// a write deadline is safe; 2 minutes leaves room for the drift sweep, the
// slowest read, on a large repository.
func consoleServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
}

func runServe(cmd *cobra.Command, host string, port int, explicitOpen *bool) error {
	root, err := repoRoot()
	if err != nil {
		return emitFailure(cmd, "serve", err)
	}
	// Preflight the load so serving outside a Tiller repo refuses upfront with
	// the canonical not_a_repo environment error, like every other command,
	// instead of binding a port that can only answer 500s. Requests still load
	// fresh per hit; this is the doorstep check only.
	if _, err := store.Load(root); err != nil {
		return emitFailure(cmd, "serve", err)
	}
	open, err := shouldOpenBrowser(root, explicitOpen)
	if err != nil {
		return emitFailure(cmd, "serve", err)
	}
	assets, err := fs.Sub(ui.Dist, "dist")
	if err != nil {
		return emitFailure(cmd, "serve", err)
	}

	// The policy is built before the port is touched: an invalid --host is
	// argument misuse, and a wildcard bind whose interfaces cannot be
	// enumerated is an environment refusal; neither may bind a port or open
	// a browser.
	auth, err := serve.AuthorityForBind(host)
	if err != nil {
		return emitFailure(cmd, "serve", err)
	}
	bind := auth.Bind()
	family := "tcp4"
	if bind.Is6() {
		family = "tcp6"
	}
	listen := net.JoinHostPort(bind.String(), strconv.Itoa(port))
	ln, err := net.Listen(family, listen)
	if err != nil {
		// A held port or an address the machine has not assigned is the
		// environment's answer, not misuse of the command.
		return emitFailure(cmd, "serve", &store.EnvError{
			Reason: "port_unavailable",
			Msg:    fmt.Sprintf("cannot bind %s: %v (choose a --host the machine has assigned to an interface, pick another --port, or stop the process holding the port)", listen, err),
		})
	}
	defer ln.Close()
	// The bound address, not the requested one: --port 0 asks the kernel to
	// pick, and the browser needs the port it picked.
	port = ln.Addr().(*net.TCPAddr).Port
	hosts := auth.Hosts()
	urls := make([]string, 0, len(hosts))
	for _, h := range hosts {
		urls = append(urls, "http://"+net.JoinHostPort(h, strconv.Itoa(port)))
	}
	// The first destination is the browser's: the bound literal for a
	// specific or loopback bind, the standard loopback for a wildcard, so
	// the opened URL always works on this machine.
	openURL := urls[0]
	if auth.Network() {
		fmt.Fprintln(cmd.ErrOrStderr(), "tiller: warning: network mode serves the planning ledger, the .tiller/ Markdown corpus, Markdown documents the ledger cites, and gate evidence files the ledger records to anyone who can reach this port, over plain HTTP, without authentication or TLS; use it only on a trusted network or private VPN, never directly on the public internet")
		if bind.IsUnspecified() {
			fmt.Fprintln(cmd.ErrOrStderr(), "tiller: warning: a wildcard bind exposes every interface of the chosen family, including other connected networks; bind a specific address to restrict exposure")
		}
	}
	if bind.IsUnspecified() {
		// A wildcard listen address is not an HTTP destination: print it as
		// the listening socket and list the real destinations instead.
		fmt.Fprintf(cmd.OutOrStdout(), "Tiller console serving %s (read-only; Ctrl-C to stop)\n", root)
		fmt.Fprintf(cmd.OutOrStdout(), "Listening on %s\n", net.JoinHostPort(bind.String(), strconv.Itoa(port)))
		for _, u := range urls {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", u)
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Tiller console serving %s at %s (read-only; Ctrl-C to stop)\n", root, openURL)
	}
	if open {
		if err := openBrowser(openURL); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "could not open a browser: %v; open the address above yourself\n", err)
		}
	}

	srv := consoleServer(serve.Handler(root, assets, version, auth))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	// Ctrl-C ends the process outright; the context is how a caller that
	// embeds the command (the tests) asks the server to stop.
	select {
	case err := <-served:
		if err != nil && err != http.ErrServerClosed {
			return emitFailure(cmd, "serve", err)
		}
	case <-cmd.Context().Done():
		if err := srv.Shutdown(context.Background()); err != nil {
			return emitFailure(cmd, "serve", err)
		}
		<-served
	}
	return nil
}

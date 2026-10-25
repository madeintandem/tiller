package serve

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func networkPolicySpecific(t *testing.T) *Authority {
	t.Helper()
	a, err := AuthorityForBind("192.168.1.5")
	require.NoError(t, err)
	return a
}

func networkPolicyWildcard4(t *testing.T, ips ...string) *Authority {
	t.Helper()
	return authorityForTarget(bindTarget{addr: mustAddr(t, "0.0.0.0"), mode: modeWildcard4}, snapshotAddrs(t, ips...))
}

func networkAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("console app")},
		"assets/app.js": {Data: []byte("console asset")},
	}
}

type networkPolicyCase struct {
	name  string
	auth  *Authority
	host  string
	alias string
}

func networkPolicies(t *testing.T) []networkPolicyCase {
	t.Helper()
	v6, err := AuthorityForBind("fd12::5")
	require.NoError(t, err)
	return []networkPolicyCase{
		{name: "specific IPv4", auth: networkPolicySpecific(t), host: "192.168.1.5:4400", alias: "127.0.0.1:4400"},
		{name: "wildcard IPv4", auth: networkPolicyWildcard4(t, "192.168.1.5", "10.1.2.3"), host: "192.168.1.5:4400", alias: "10.1.2.3:4400"},
		{name: "specific IPv6", auth: v6, host: "[fd12::5]:4400", alias: "[::1]:4400"},
		{name: "wildcard IPv6", auth: authorityForTarget(bindTarget{addr: mustAddr(t, "::"), mode: modeWildcard6}, snapshotAddrs(t, "fd12::5", "fd12::6", "192.168.1.5")), host: "[fd12::5]:4400", alias: "[fd12::6]:4400"},
	}
}

func TestNetworkBoundaryHostAdmission(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			h := Handler(t.TempDir(), nil, "network-version", policy.auth)
			allowed := []string{policy.host, strings.TrimSuffix(policy.host, ":4400") + ":8080"}
			denied := []string{"attacker.example:4400", "mitbox.local:4400", "0.0.0.0:4400", "[::]:4400", "10.0.0.9:4400", "[fd12::9]:4400", " " + policy.host, policy.host + "/", policy.host + "?x", strings.TrimSuffix(policy.host, ":4400") + ":0"}
			switch policy.name {
			case "specific IPv4", "specific IPv6":
				denied = append(denied, "localhost:4400", "127.0.0.1:4400", "[::1]:4400")
			case "wildcard IPv4":
				allowed = append(allowed, "10.1.2.3:4400", "127.0.0.1:4400", "LOCALHOST:4400")
				denied = append(denied, "[::1]:4400", "[fd12::5]:4400")
			case "wildcard IPv6":
				allowed = append(allowed, "[fd12::6]:4400", "[::1]:4400", "LOCALHOST:4400")
				denied = append(denied, "127.0.0.1:4400", "192.168.1.5:4400")
			}
			if strings.Contains(policy.name, "IPv6") {
				allowed = append(allowed, "[FD12:0:0:0:0:0:0:5]:04400")
				denied = append(denied, "fd12::5", "[fd12::5%lo0]:4400", "[::ffff:192.168.1.5]:4400")
			}
			for _, host := range allowed {
				t.Run("admitted "+host, func(t *testing.T) {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, boundaryRequest(host, nil, ""))
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "network-version")
				})
			}
			for _, host := range denied {
				t.Run("refused "+host, func(t *testing.T) {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, boundaryRequest(host, []string{"http://" + host}, ""))
					require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
					require.NotContains(t, w.Body.String(), "network-version")
					require.NotContains(t, w.Body.String(), "loopback")
					if strings.HasPrefix(policy.name, "wildcard") {
						require.Contains(t, w.Body.String(), "interface addresses")
					} else {
						require.Contains(t, w.Body.String(), "bound host")
					}
				})
			}
		})
	}
}

func TestExplicitLoopbackBoundary(t *testing.T) {
	for _, tc := range []struct {
		bind   string
		host   string
		origin string
	}{
		{bind: "127.0.0.2", host: "127.0.0.2:4400", origin: "http://127.0.0.2:4400"},
		{bind: "::1", host: "[0:0:0:0:0:0:0:1]:4400", origin: "http://[::1]:4400"},
	} {
		t.Run(tc.bind, func(t *testing.T) {
			a, err := AuthorityForBind(tc.bind)
			require.NoError(t, err)
			h := Handler(t.TempDir(), nil, "test", a)
			for _, origin := range []string{tc.origin, "http://localhost:4400"} {
				r := boundaryRequest(tc.host, []string{origin}, "same-origin")
				r.Header.Set("Referer", "http://attacker.example/#")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				want := http.StatusOK
				if origin == "http://localhost:4400" {
					want = http.StatusForbidden
				}
				require.Equal(t, want, w.Code, w.Body.String())
				require.Empty(t, w.Header().Get("Content-Security-Policy"))
				require.Empty(t, w.Header().Get("X-Frame-Options"))
			}
		})
	}
	w := httptest.NewRecorder()
	Handler(t.TempDir(), nil, "test", DefaultAuthority()).ServeHTTP(w, boundaryRequest("127.0.0.2:4400", nil, ""))
	require.Equal(t, http.StatusForbidden, w.Code)
}

// Every network policy retains the default Origin and fetch-metadata refusals.
func TestNetworkBoundaryOriginAndFetchMetadata(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			h := Handler(t.TempDir(), nil, "test", policy.auth)
			origin := "http://" + policy.host
			for _, tc := range []struct {
				name    string
				origins []string
				sites   []string
				allowed bool
			}{
				{name: "headerless navigation", allowed: true},
				{name: "same origin", origins: []string{origin}, sites: []string{"same-origin"}, allowed: true},
				{name: "navigation metadata", sites: []string{"none"}, allowed: true},
				{name: "same-origin missing Origin", sites: []string{"same-origin"}, allowed: true},
				{name: "different port", origins: []string{strings.ReplaceAll(origin, ":4400", ":8080")}},
				{name: "different scheme", origins: []string{strings.Replace(origin, "http:", "https:", 1)}},
				{name: "alias origin", origins: []string{"http://" + policy.alias}},
				{name: "foreign origin", origins: []string{"http://attacker.example:4400"}},
				{name: "null origin", origins: []string{"null"}},
				{name: "empty origin", origins: []string{""}},
				{name: "repeated origin", origins: []string{origin, origin}},
				{name: "origin list", origins: []string{origin + " " + origin}},
				{name: "origin path", origins: []string{origin + "/"}},
				{name: "origin empty query", origins: []string{origin + "?"}},
				{name: "origin empty fragment", origins: []string{origin + "#"}},
				{name: "origin userinfo", origins: []string{strings.Replace(origin, "://", "://user@", 1)}},
				{name: "origin whitespace", origins: []string{" " + origin}},
				{name: "origin backslash", origins: []string{origin + "\\@attacker.example"}},
				{name: "origin missing host", origins: []string{"http:///"}},
				{name: "origin empty port", origins: []string{strings.TrimSuffix(origin, ":4400") + ":"}},
				{name: "origin overflowing port", origins: []string{strings.ReplaceAll(origin, ":4400", ":65536")}},
				{name: "origin mapped IPv6", origins: []string{"http://[::ffff:192.168.1.5]:4400"}},
				{name: "origin zoned IPv6", origins: []string{"http://[fd12::5%25lo0]:4400"}},
				{name: "same-site", sites: []string{"same-site"}},
				{name: "cross-site", sites: []string{"cross-site"}},
				{name: "matching Origin cross-site", origins: []string{origin}, sites: []string{"cross-site"}},
				{name: "invalid metadata", sites: []string{"unknown"}},
				{name: "empty metadata", sites: []string{""}},
				{name: "metadata case", sites: []string{"Same-Origin"}},
				{name: "repeated metadata", sites: []string{"same-origin", "same-origin"}},
				{name: "metadata list", sites: []string{"same-origin, none"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := boundaryRequest(policy.host, tc.origins, "")
					if tc.sites != nil {
						r.Header["Sec-Fetch-Site"] = tc.sites
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					want := http.StatusForbidden
					if tc.allowed {
						want = http.StatusOK
					}
					require.Equal(t, want, w.Code, w.Body.String())
				})
			}
		})
	}
}

func TestNetworkBoundaryReferer(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			h := Handler(t.TempDir(), nil, "test", policy.auth)
			origin := "http://" + policy.host
			for _, tc := range []struct {
				name    string
				refs    []string
				allowed bool
			}{
				{name: "absent", allowed: true},
				{name: "absolute origin", refs: []string{origin}, allowed: true},
				{name: "path and query", refs: []string{origin + "/explore?q=1"}, allowed: true},
				{name: "escaped path and query", refs: []string{origin + "/a%20b?q=%23x"}, allowed: true},
				{name: "foreign", refs: []string{"http://attacker.example:4400/"}},
				{name: "alias", refs: []string{"http://" + policy.alias + "/"}},
				{name: "different port", refs: []string{strings.ReplaceAll(origin, ":4400", ":8080") + "/"}},
				{name: "omitted nondefault port", refs: []string{strings.TrimSuffix(origin, ":4400") + "/"}},
				{name: "different scheme", refs: []string{strings.Replace(origin, "http:", "https:", 1)}},
				{name: "credentials", refs: []string{strings.Replace(origin, "://", "://user:pass@", 1)}},
				{name: "fragment", refs: []string{origin + "/#x"}},
				{name: "empty fragment", refs: []string{origin + "/#"}},
				{name: "query fragment", refs: []string{origin + "/?q=1#"}},
				{name: "empty", refs: []string{""}},
				{name: "relative", refs: []string{"/explore"}},
				{name: "scheme relative", refs: []string{"//" + policy.host + "/"}},
				{name: "malformed", refs: []string{"://not-a-url"}},
				{name: "malformed path escape", refs: []string{origin + "/%zz"}},
				{name: "malformed query escape", refs: []string{origin + "/?q=%zz"}},
				{name: "path backslash", refs: []string{origin + "/\\attacker.example"}},
				{name: "query backslash", refs: []string{origin + "/?q=\\x"}},
				{name: "path space", refs: []string{origin + "/bad path"}},
				{name: "trailing whitespace", refs: []string{origin + "/ "}},
				{name: "query whitespace", refs: []string{origin + "/?q=a b"}},
				{name: "unicode whitespace", refs: []string{origin + "/bad\u00a0path"}},
				{name: "repeated", refs: []string{origin + "/", origin + "/"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := boundaryRequest(policy.host, nil, "")
					if tc.refs != nil {
						r.Header["Referer"] = tc.refs
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					want := http.StatusForbidden
					if tc.allowed {
						want = http.StatusOK
					}
					require.Equal(t, want, w.Code, w.Body.String())
				})
			}
		})
	}
}

func TestNetworkBoundaryCanonicalOrigins(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		for _, secure := range []bool{false, true} {
			scheme, port := "http", "80"
			if secure {
				scheme, port = "https", "443"
			}
			host := strings.TrimSuffix(policy.host, ":4400")
			originHost := host
			if strings.Contains(policy.name, "IPv6") {
				originHost = "[FD12:0:0:0:0:0:0:5]"
			}
			r := boundaryRequest(host+":"+port, []string{scheme + "://" + originHost}, "same-origin")
			r.Header.Set("Referer", scheme+"://"+originHost+":"+port+"/explore?q=1")
			if secure {
				r.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			Handler(t.TempDir(), nil, "test", policy.auth).ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code, policy.name, w.Body.String())
		}
	}
}

func TestNetworkBoundaryFramingHTTPResponses(t *testing.T) {
	server := httptest.NewServer(Handler(t.TempDir(), networkAssets(), "network-version", networkPolicySpecific(t)))
	defer server.Close()
	for _, tc := range []struct {
		name   string
		method string
		host   string
		path   string
		want   int
		body   string
	}{
		{name: "page", method: http.MethodGet, host: "192.168.1.5:4400", path: "/", want: http.StatusOK, body: "console app"},
		{name: "asset", method: http.MethodGet, host: "192.168.1.5:4400", path: "/assets/app.js", want: http.StatusOK, body: "console asset"},
		{name: "API", method: http.MethodGet, host: "192.168.1.5:4400", path: "/api/version", want: http.StatusOK, body: "network-version"},
		{name: "refusal", method: http.MethodGet, host: "localhost:4400", path: "/", want: http.StatusForbidden, body: "bound host 192.168.1.5"},
		{name: "not found", method: http.MethodGet, host: "192.168.1.5:4400", path: "/api/unknown", want: http.StatusNotFound, body: "unknown API route"},
		{name: "read-only", method: http.MethodPost, host: "192.168.1.5:4400", path: "/api/version", want: http.StatusMethodNotAllowed, body: "read-only"},
		{name: "project load error", method: http.MethodGet, host: "192.168.1.5:4400", path: "/api/status", want: http.StatusInternalServerError, body: "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(tc.method, server.URL+tc.path, nil)
			require.NoError(t, err)
			r.Host = tc.host
			resp, err := server.Client().Do(r)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.want, resp.StatusCode, string(body))
			require.Contains(t, string(body), tc.body)
			require.Equal(t, "SAMEORIGIN", resp.Header.Get("X-Frame-Options"))
			require.Contains(t, resp.Header.Values("Content-Security-Policy"), "frame-ancestors 'self'")
			if tc.want == http.StatusMethodNotAllowed {
				require.Equal(t, "GET, HEAD", resp.Header.Get("Allow"))
			}
		})
	}
	defaultServer := httptest.NewServer(Handler(t.TempDir(), networkAssets(), "test", DefaultAuthority()))
	defer defaultServer.Close()
	for _, path := range []string{"/", "/api/version", "/api/status"} {
		r, err := http.NewRequest(http.MethodGet, defaultServer.URL+path, nil)
		require.NoError(t, err)
		r.Host = "localhost:8080"
		r.Header.Set("Referer", "http://attacker.example/#")
		resp, err := defaultServer.Client().Do(r)
		require.NoError(t, err)
		resp.Body.Close()
		require.NotEqual(t, http.StatusForbidden, resp.StatusCode)
		require.Empty(t, resp.Header.Get("X-Frame-Options"))
		require.Empty(t, resp.Header.Get("Content-Security-Policy"))
	}
}

func TestNetworkBoundaryPreservesExistingCSP(t *testing.T) {
	for _, existing := range [][]string{
		{"default-src 'self'"},
		{"default-src 'self'; frame-ancestors *"},
		{"frame-ancestors https://attacker.example; script-src 'none'", "img-src 'self'"},
	} {
		t.Run(strings.Join(existing, ","), func(t *testing.T) {
			h := Handler(t.TempDir(), nil, "test", networkPolicySpecific(t))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, policy := range existing {
					w.Header().Add("Content-Security-Policy", policy)
				}
				h.ServeHTTP(w, r)
			}))
			defer server.Close()
			for _, host := range []string{"192.168.1.5:4400", "localhost:4400"} {
				r, err := http.NewRequest(http.MethodGet, server.URL+"/api/version", nil)
				require.NoError(t, err)
				r.Host = host
				resp, err := server.Client().Do(r)
				require.NoError(t, err)
				resp.Body.Close()
				status := http.StatusOK
				if host == "localhost:4400" {
					status = http.StatusForbidden
				}
				require.Equal(t, status, resp.StatusCode)
				want := append(append([]string(nil), existing...), "frame-ancestors 'self'")
				require.Equal(t, want, resp.Header.Values("Content-Security-Policy"), "a distinct policy cannot be bypassed by an earlier frame-ancestors directive")
				require.Equal(t, "SAMEORIGIN", resp.Header.Get("X-Frame-Options"))
			}
		})
	}
}

type networkRefusal struct {
	name   string
	mutate func(*http.Request)
}

func networkRefusals() []networkRefusal {
	return []networkRefusal{
		{name: "foreign Host", mutate: func(r *http.Request) { r.Host = "attacker.example:4400" }},
		{name: "foreign Origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "http://attacker.example:4400") }},
		{name: "malformed Origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "http://"+r.Host+"/") }},
		{name: "repeated Origin", mutate: func(r *http.Request) { r.Header["Origin"] = []string{"http://" + r.Host, "http://" + r.Host} }},
		{name: "fetch metadata", mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{name: "empty fetch metadata", mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "") }},
		{name: "repeated fetch metadata", mutate: func(r *http.Request) { r.Header["Sec-Fetch-Site"] = []string{"same-origin", "same-origin"} }},
		{name: "foreign Referer", mutate: func(r *http.Request) { r.Header.Set("Referer", "http://attacker.example:4400/") }},
		{name: "malformed Referer", mutate: func(r *http.Request) { r.Header.Set("Referer", "http://"+r.Host+"/bad path") }},
		{name: "empty fragment Referer", mutate: func(r *http.Request) { r.Header.Set("Referer", "http://"+r.Host+"/#") }},
		{name: "repeated Referer", mutate: func(r *http.Request) {
			r.Header["Referer"] = []string{"http://" + r.Host + "/", "http://" + r.Host + "/"}
		}},
	}
}

func TestNetworkBoundaryCoversAllRoutesBeforeReads(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			assets := &boundaryAssets{FS: networkAssets()}
			root := t.TempDir()
			h := Handler(root, assets, "private-version", policy.auth)
			for _, path := range []string{"/", "/assets/app.js", "/explore", "/api/version", "/api/status", "/api/tree", "/api/prose?ref=README.md", "/api/evidence?ref=README.md", "/api/recap", "/api/recap/peek", "/api/unknown"} {
				for _, refusal := range networkRefusals() {
					t.Run(path+"/"+refusal.name, func(t *testing.T) {
						r := boundaryRequest(policy.host, nil, "")
						r.URL.Path, r.URL.RawQuery, _ = strings.Cut(path, "?")
						refusal.mutate(r)
						w := httptest.NewRecorder()
						h.ServeHTTP(w, r)
						require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
						require.NotContains(t, w.Body.String(), root)
						require.NotContains(t, w.Body.String(), "private-version")
						require.Zero(t, assets.opens, "the boundary precedes asset reads")
					})
				}
			}
			r := boundaryRequest(policy.host, nil, "")
			r.URL.Path = "/api/status"
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, http.StatusInternalServerError, w.Code, "a legitimate request reaches the project load")
		})
	}
}

func TestNetworkBoundaryRefusalsLeaveVisitMemory(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			root := visitRepo(t)
			seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
			writeLastVisit(root, seeded)
			before, err := os.ReadFile(consoleCachePath(root))
			require.NoError(t, err)
			visits := &visitState{now: fixedNow(seeded.Add(time.Hour))}
			h := newMux(root, nil, visits, "test", policy.auth)
			for _, refusal := range networkRefusals() {
				for _, path := range []string{"/api/recap", "/api/recap/peek"} {
					t.Run(refusal.name+path, func(t *testing.T) {
						r := boundaryRequest(policy.host, nil, "")
						r.URL.Path = path
						refusal.mutate(r)
						w := httptest.NewRecorder()
						h.ServeHTTP(w, r)
						require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
						after, err := os.ReadFile(consoleCachePath(root))
						require.NoError(t, err)
						require.Equal(t, before, after, "refused requests cannot consume the recap")
						require.False(t, visits.loaded, "refused requests cannot open a session")
					})
				}
			}
			r := boundaryRequest(policy.host, []string{"http://" + policy.host}, "same-origin")
			r.URL.Path = "/api/recap"
			r.Header.Set(visitHeader, "1") // the console's own History visit
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.True(t, readCacheVisit(t, root).Equal(seeded.Add(time.Hour)))
		})
	}
}

func TestBoundaryMissingPolicyFailsClosed(t *testing.T) {
	for name, policy := range map[string]*Authority{"missing": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			assets := &boundaryAssets{FS: networkAssets()}
			visits := &visitState{}
			h := newMux(t.TempDir(), assets, visits, "private-version", policy)
			for _, path := range []string{"/", "/assets/app.js", "/api/version", "/api/status", "/api/recap", "/api/recap/peek"} {
				r := boundaryRequest("localhost:4400", nil, "")
				r.URL.Path = path
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), "private-version")
				require.Zero(t, assets.opens)
				require.False(t, visits.loaded)
			}
		})
	}
}

func TestNetworkBoundaryIgnoresProxyHeaders(t *testing.T) {
	for _, policy := range networkPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			h := Handler(t.TempDir(), nil, "test", policy.auth)
			for _, tc := range []struct {
				name   string
				host   string
				origin []string
				ref    string
				want   int
			}{
				{name: "cannot admit foreign Host", host: "attacker.example:4400", want: http.StatusForbidden},
				{name: "cannot change direct scheme", host: policy.host, origin: []string{"https://" + policy.host}, want: http.StatusForbidden},
				{name: "cannot change Referer scheme", host: policy.host, ref: "https://" + policy.host + "/", want: http.StatusForbidden},
				{name: "cannot break admitted Host", host: policy.host, want: http.StatusOK},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := boundaryRequest(tc.host, tc.origin, "")
					if tc.ref != "" {
						r.Header.Set("Referer", tc.ref)
					}
					r.Header.Set("Forwarded", "host="+policy.host+";proto=https")
					r.Header.Set("X-Forwarded-Host", policy.host)
					r.Header.Set("X-Forwarded-Proto", "https")
					r.Header.Set("X-Forwarded-Port", "4400")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					require.Equal(t, tc.want, w.Code, w.Body.String())
				})
			}
		})
	}
}

package serve

import (
	"crypto/tls"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func boundaryRequest(host string, origin []string, fetchSite string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/version", nil)
	r.Host = host
	if origin != nil {
		r.Header["Origin"] = origin
	}
	if fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	return r
}

func TestConsoleRequestBoundary(t *testing.T) {
	h := Handler(t.TempDir(), nil, "boundary-test", DefaultAuthority())
	cases := []struct {
		name      string
		host      string
		origin    []string
		fetchSite string
		tls       bool
		allowed   bool
	}{
		{name: "CLI IPv4", host: "127.0.0.1:8080", allowed: true},
		{name: "localhost navigation", host: "localhost:8080", fetchSite: "none", allowed: true},
		{name: "IPv6 navigation", host: "[::1]:8080", allowed: true},
		{name: "same-origin fetch without Origin", host: "localhost:8080", fetchSite: "same-origin", allowed: true},
		{name: "same-origin fetch", host: "localhost:8080", origin: []string{"http://localhost:8080"}, fetchSite: "same-origin", allowed: true},
		{name: "IPv4 origin", host: "127.0.0.1:8080", origin: []string{"http://127.0.0.1:8080"}, allowed: true},
		{name: "IPv6 origin", host: "[::1]:8080", origin: []string{"http://[::1]:8080"}, allowed: true},
		{name: "expanded IPv6 equivalent", host: "[0:0:0:0:0:0:0:1]:8080", origin: []string{"http://[::1]:8080"}, allowed: true},
		{name: "hostname case equivalent", host: "LOCALHOST:8080", origin: []string{"http://localhost:8080"}, allowed: true},
		{name: "default HTTP port", host: "localhost", origin: []string{"http://localhost:80"}, allowed: true},
		{name: "omitted HTTP origin port", host: "localhost:80", origin: []string{"http://localhost"}, allowed: true},
		{name: "default HTTPS port", host: "localhost:443", origin: []string{"https://localhost"}, tls: true, allowed: true},
		{name: "numeric port equivalence", host: "localhost:08080", origin: []string{"http://localhost:8080"}, allowed: true},
		{name: "foreign DNS host", host: "attacker.example:8080"},
		{name: "loopback-looking suffix", host: "localhost.attacker.example:8080"},
		{name: "loopback-looking prefix", host: "attacker.localhost:8080"},
		{name: "trailing DNS dot", host: "localhost.:8080"},
		{name: "Unicode case-fold lookalike", host: "localhoſt:8080"},
		{name: "other loopback IP", host: "127.0.0.2:8080"},
		{name: "IPv4-mapped IPv6", host: "[::ffff:127.0.0.1]:8080"},
		{name: "IPv6 zone", host: "[::1%lo0]:8080"},
		{name: "unbracketed IPv6", host: "::1"},
		{name: "unbracketed IPv6 port", host: "::1:8080"},
		{name: "bracketed hostname", host: "[localhost]:8080"},
		{name: "bracketed IPv4", host: "[127.0.0.1]:8080"},
		{name: "missing host", host: ""},
		{name: "host user info", host: "attacker@localhost:8080"},
		{name: "host whitespace", host: " localhost:8080"},
		{name: "host path", host: "localhost:8080/path"},
		{name: "host query", host: "localhost:8080?x"},
		{name: "host percent encoding", host: "%6cocalhost:8080"},
		{name: "empty host port", host: "localhost:"},
		{name: "zero host port", host: "localhost:0"},
		{name: "overflow host port", host: "localhost:65536"},
		{name: "signed host port", host: "localhost:+8080"},
		{name: "nonnumeric host port", host: "localhost:http"},
		{name: "multiple host ports", host: "localhost:8080:80"},
		{name: "foreign origin", host: "localhost:8080", origin: []string{"https://attacker.example"}},
		{name: "origin suffix", host: "localhost:8080", origin: []string{"http://localhost.attacker.example:8080"}},
		{name: "Unicode origin lookalike", host: "localhost:8080", origin: []string{"http://localhoſt:8080"}},
		{name: "different port", host: "localhost:8080", origin: []string{"http://localhost:8081"}},
		{name: "omitted nondefault port", host: "localhost:8080", origin: []string{"http://localhost"}},
		{name: "different scheme", host: "localhost:8080", origin: []string{"https://localhost:8080"}},
		{name: "IPv4 localhost aliases are different origins", host: "127.0.0.1:8080", origin: []string{"http://localhost:8080"}},
		{name: "IPv6 IPv4 aliases are different origins", host: "[::1]:8080", origin: []string{"http://127.0.0.1:8080"}},
		{name: "null origin", host: "localhost:8080", origin: []string{"null"}},
		{name: "empty present origin", host: "localhost:8080", origin: []string{""}},
		{name: "multiple origin headers", host: "localhost:8080", origin: []string{"http://localhost:8080", "http://localhost:8080"}},
		{name: "origin list", host: "localhost:8080", origin: []string{"http://localhost:8080 http://attacker.example"}},
		{name: "origin user info", host: "localhost:8080", origin: []string{"http://attacker@localhost:8080"}},
		{name: "origin path", host: "localhost:8080", origin: []string{"http://localhost:8080/"}},
		{name: "origin query", host: "localhost:8080", origin: []string{"http://localhost:8080?"}},
		{name: "origin fragment", host: "localhost:8080", origin: []string{"http://localhost:8080#"}},
		{name: "origin encoded hostname", host: "localhost:8080", origin: []string{"http://%6cocalhost:8080"}},
		{name: "origin empty port", host: "localhost:8080", origin: []string{"http://localhost:"}},
		{name: "origin port overflow", host: "localhost:8080", origin: []string{"http://localhost:65536"}},
		{name: "origin unbracketed IPv6", host: "[::1]:8080", origin: []string{"http://::1:8080"}},
		{name: "origin IPv6 zone", host: "[::1]:8080", origin: []string{"http://[::1%25lo0]:8080"}},
		{name: "origin whitespace", host: "localhost:8080", origin: []string{" http://localhost:8080"}},
		{name: "origin backslash", host: "localhost:8080", origin: []string{"http://localhost:8080\\@attacker.example"}},
		{name: "cross-site missing Origin", host: "localhost:8080", fetchSite: "cross-site"},
		{name: "cross-site matching Origin", host: "localhost:8080", origin: []string{"http://localhost:8080"}, fetchSite: "cross-site"},
		{name: "same-site foreign context", host: "localhost:8080", fetchSite: "same-site"},
		{name: "invalid fetch metadata", host: "localhost:8080", fetchSite: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := boundaryRequest(tc.host, tc.origin, tc.fetchSite)
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if tc.allowed {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), "boundary-test")
			} else {
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), "boundary-test")
			}
		})
	}
}

func TestConsoleBoundaryIgnoresProxyHeaders(t *testing.T) {
	h := Handler(t.TempDir(), nil, "test", DefaultAuthority())
	for _, tc := range []struct {
		host   string
		origin string
	}{
		{host: "attacker.example:8080", origin: "http://localhost:8080"},
		{host: "localhost:8080", origin: "https://localhost:8080"},
	} {
		r := boundaryRequest(tc.host, []string{tc.origin}, "")
		r.Header.Set("Forwarded", "host=localhost:8080;proto=https")
		r.Header.Set("X-Forwarded-Host", "localhost:8080")
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestConsoleBoundaryRejectsRepeatedFetchMetadata(t *testing.T) {
	r := boundaryRequest("localhost:8080", nil, "")
	r.Header["Sec-Fetch-Site"] = []string{"same-origin", "cross-site"}
	w := httptest.NewRecorder()
	Handler(t.TempDir(), nil, "test", DefaultAuthority()).ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestConsoleBoundaryAllowsLoopbackBrowsing(t *testing.T) {
	h := Handler(visitRepo(t), fstest.MapFS{
		"index.html":    {Data: []byte("console app")},
		"assets/app.js": {Data: []byte("console asset")},
	}, "test", DefaultAuthority())
	for _, host := range []string{"localhost:8080", "127.0.0.1:8080", "[::1]:8080"} {
		for _, path := range []string{"/", "/explore", "/assets/app.js", "/api/status", "/api/tree"} {
			for _, withOrigin := range []bool{false, true} {
				t.Run(host+path+strconv.FormatBool(withOrigin), func(t *testing.T) {
					var origins []string
					if withOrigin {
						origins = []string{"http://" + host}
					}
					r := boundaryRequest(host, origins, "same-origin")
					r.URL.Path = path
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				})
			}
		}
	}
}

type boundaryAssets struct {
	fs.FS
	opens int
}

func (a *boundaryAssets) Open(name string) (fs.File, error) {
	a.opens++
	return a.FS.Open(name)
}

func TestConsoleBoundaryCoversAllRoutesBeforeReads(t *testing.T) {
	assets := &boundaryAssets{FS: fstest.MapFS{
		"index.html":    {Data: []byte("private console app")},
		"assets/app.js": {Data: []byte("private console asset")},
	}}
	// A legitimate tree read fails here. Rejected requests must instead stop at
	// the boundary, without exposing the project load error or opening assets.
	root := t.TempDir()
	h := Handler(root, assets, "private-version", DefaultAuthority())
	for _, path := range []string{"/", "/assets/app.js", "/explore", "/api/version", "/api/status", "/api/tree", "/api/prose?ref=README.md", "/api/evidence?ref=README.md", "/api/recap", "/api/recap/peek", "/api/unknown"} {
		for _, foreign := range []string{"host", "origin", "fetch-site"} {
			t.Run(path+"/"+foreign, func(t *testing.T) {
				r := boundaryRequest("localhost:8080", nil, "")
				r.URL.Path, r.URL.RawQuery, _ = strings.Cut(path, "?")
				switch foreign {
				case "host":
					r.Host = "attacker.example:8080"
				case "origin":
					r.Header.Set("Origin", "http://attacker.example:8080")
				case "fetch-site":
					r.Header.Set("Sec-Fetch-Site", "cross-site")
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), root)
				require.NotContains(t, w.Body.String(), "private-")
				require.Zero(t, assets.opens, "the boundary precedes asset reads")
			})
		}
	}
	r := boundaryRequest("localhost:8080", nil, "")
	r.URL.Path = "/api/status"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError, w.Code, "a legitimate request reaches the tree load")
}

func TestConsoleBoundaryPreservesRecapCache(t *testing.T) {
	root := visitRepo(t)
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	writeLastVisit(root, seeded)
	before, err := os.ReadFile(consoleCachePath(root))
	require.NoError(t, err)
	visits := &visitState{now: fixedNow(seeded.Add(time.Hour))}
	h := newMux(root, nil, visits, "test", DefaultAuthority())
	for _, tc := range []struct {
		host      string
		origin    []string
		fetchSite string
	}{
		{host: "attacker.example:8080"},
		{host: "localhost:8080", origin: []string{"http://attacker.example:8080"}},
		{host: "localhost:8080", fetchSite: "cross-site"},
	} {
		for _, path := range []string{"/api/recap", "/api/recap/peek"} {
			r := boundaryRequest(tc.host, tc.origin, tc.fetchSite)
			r.URL.Path = path
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, http.StatusForbidden, w.Code)
			after, err := os.ReadFile(consoleCachePath(root))
			require.NoError(t, err)
			require.Equal(t, before, after, "foreign requests cannot consume the recap")
			require.False(t, visits.loaded, "foreign requests cannot open a session")
		}
	}
	r := boundaryRequest("localhost:8080", []string{"http://localhost:8080"}, "same-origin")
	r.URL.Path = "/api/recap"
	r.Header.Set(visitHeader, "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, readCacheVisit(t, root).Equal(seeded.Add(time.Hour)), "a legitimate request still records the visit")
}

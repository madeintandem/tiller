package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// visitPolicy is one bind the visit-marker rule must hold on.
type visitPolicy struct {
	name string
	host string
	auth *Authority
}

func visitPolicies(t *testing.T) []visitPolicy {
	policies := []visitPolicy{{name: "loopback", host: "127.0.0.1:4400", auth: DefaultAuthority()}}
	for _, p := range networkPolicies(t) {
		policies = append(policies, visitPolicy{name: p.name, host: p.host, auth: p.auth})
	}
	return policies
}

// recapWith serves one visit-window recap from a peer at the given address,
// with the given extra headers, and asserts it answers against the seeded
// horizon.
func recapWith(t *testing.T, h http.Handler, host, remote string, header http.Header) {
	t.Helper()
	r := boundaryRequest(host, nil, "")
	r.URL.Path = "/api/recap"
	r.RemoteAddr = remote
	for name, values := range header {
		r.Header[name] = values
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"since": "2026-07-01T09:00:00Z"`, "every reader sees the recap since the developer's horizon")
}

// Any page or peer that reaches the port can send a plain GET, and plain HTTP
// lets it drop every header that would name the initiating site; a peer
// address cannot tell the developer's console from a hostile page in the
// developer's own browser either. So the horizon advances only for a request
// carrying the console's visit marker, a custom header a cross-site page cannot
// attach without a CORS preflight the server never answers. Every other
// request reads the recap at the peeked horizon and writes nothing, whatever
// its peer and whatever the bind.
func TestRecapAdvancesOnlyForTheVisitMarker(t *testing.T) {
	seeded := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	later := seeded.Add(time.Hour)
	peers := map[string]string{"loopback": "127.0.0.1:51000", "LAN peer": "192.168.1.77:51000", "IPv6 peer": "[fd12::77]:51000"}
	for _, policy := range visitPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			for name, remote := range peers {
				t.Run(name+" without marker", func(t *testing.T) {
					root := visitRepo(t)
					writeLastVisit(root, seeded)
					before, err := os.ReadFile(consoleCachePath(root))
					require.NoError(t, err)
					visits := &visitState{now: fixedNow(later)}
					recapWith(t, newMux(root, nil, visits, "test", policy.auth), policy.host, remote, nil)
					after, err := os.ReadFile(consoleCachePath(root))
					require.NoError(t, err)
					require.Equal(t, before, after, "an unmarked request cannot consume the developer's recap")
					require.False(t, visits.loaded, "an unmarked request cannot open a visit")
				})
				t.Run(name+" with marker", func(t *testing.T) {
					root := visitRepo(t)
					writeLastVisit(root, seeded)
					h := newMux(root, nil, &visitState{now: fixedNow(later)}, "test", policy.auth)
					recapWith(t, h, policy.host, remote, http.Header{visitHeader: {"1"}})
					require.True(t, readCacheVisit(t, root).Equal(later), "the console's visit advances the horizon")
				})
			}
		})
	}
}

// The marker is only safe while the server answers no CORS preflight: a
// cross-site page needs one before it may attach a custom header. OPTIONS is
// refused like every other write method and no response grants CORS access.
func TestRecapPreflightIsRefused(t *testing.T) {
	for _, policy := range visitPolicies(t) {
		t.Run(policy.name, func(t *testing.T) {
			h := newMux(visitRepo(t), nil, &visitState{}, "test", policy.auth)
			r := boundaryRequest(policy.host, []string{"http://attacker.example"}, "")
			r.Method = http.MethodOptions
			r.URL.Path = "/api/recap"
			r.Header.Set("Access-Control-Request-Method", http.MethodGet)
			r.Header.Set("Access-Control-Request-Headers", visitHeader)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.GreaterOrEqual(t, w.Code, 400, w.Body.String())
			for name := range w.Header() {
				require.NotContains(t, name, "Access-Control-", "the console never grants CORS access")
			}
		})
	}
}

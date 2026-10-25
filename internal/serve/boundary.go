package serve

import (
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// requestBoundary guards every route before reads. It validates direct
// request origins, not proxy headers, and does not authenticate clients.
func requestBoundary(a *Authority) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Refusals need framing protection too.
			if a != nil && a.network {
				setFramingHeaders(w)
			}
			if a == nil || !a.acceptsRequest(r) {
				writeError(w, http.StatusForbidden, a.refusal())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Keep the default refusal wording; network modes name their admitted hosts.
func (a *Authority) refusal() string {
	if a == nil || !a.network {
		return "the console accepts only same-origin loopback requests"
	}
	if a.bind.IsUnspecified() {
		family := "IPv4"
		if !a.bind.Is4() {
			family = "IPv6"
		}
		return "the console accepts only requests from this machine's " + family + " interface addresses"
	}
	return "the console accepts only requests from the bound host " + a.bind.String()
}

// The direct connection determines the scheme; proxy headers are untrusted.
func (a *Authority) acceptsRequest(r *http.Request) bool {
	scheme, defaultPort := "http", 80
	if r.TLS != nil {
		scheme, defaultPort = "https", 443
	}
	name, ip, port, ok := requestAuthority(r.Host, defaultPort)
	if !ok || !a.accepts(name, ip) {
		return false
	}

	// Navigation may omit Origin; admitted aliases remain distinct origins.
	if origins := r.Header.Values("Origin"); len(origins) != 0 {
		if len(origins) != 1 || strings.ContainsAny(origins[0], "?#") {
			return false
		}
		origin, err := url.Parse(origins[0])
		if err != nil || origin.Scheme != scheme || origin.Opaque != "" || origin.User != nil || origin.Host == "" || origin.Path != "" {
			return false
		}
		oName, oIP, oPort, ok := requestAuthority(origin.Host, defaultPort)
		if !ok || oName != name || oIP != ip || oPort != port {
			return false
		}
	}

	// Same-site is not same-origin: another port is another application.
	if sites := r.Header.Values("Sec-Fetch-Site"); len(sites) != 0 {
		if len(sites) != 1 {
			return false
		}
		switch sites[0] {
		case "none", "same-origin":
		default:
			return false
		}
	}

	// Plain HTTP may omit browser metadata. A present Referer must match;
	// an absent one still permits navigation and CLI reads, not authentication.
	if a.network {
		if refs := r.Header.Values("Referer"); len(refs) != 0 {
			if len(refs) != 1 || !validRefererSyntax(refs[0]) {
				return false
			}
			ref, err := url.Parse(refs[0])
			if err != nil || ref.Scheme != scheme || ref.Opaque != "" || ref.User != nil || ref.Host == "" {
				return false
			}
			rName, rIP, rPort, ok := requestAuthority(ref.Host, defaultPort)
			if !ok || rName != name || rIP != ip || rPort != port {
				return false
			}
		}
	}
	return true
}

// Separate CSP policies intersect, unlike duplicate directives within one
// policy, where the first frame-ancestors directive wins.
func setFramingHeaders(w http.ResponseWriter) {
	w.Header().Add("Content-Security-Policy", "frame-ancestors 'self'")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
}

func validRefererSyntax(raw string) bool {
	if strings.IndexFunc(raw, func(r rune) bool {
		return r == '#' || r == '\\' || unicode.IsSpace(r)
	}) >= 0 {
		return false
	}
	isHex := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	// url.Parse validates path escapes, but leaves query escapes unchecked.
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' {
			if i+2 >= len(raw) || !isHex(raw[i+1]) || !isHex(raw[i+2]) {
				return false
			}
			i += 2
		}
	}
	return true
}

// requestAuthority accepts localhost, bare IPv4, or bracketed IPv6 with an
// optional decimal TCP port. Canonical host and effective-port equality is
// origin equality; no DNS aliases, mapped addresses, zones, or proxy trust.
func requestAuthority(authority string, defaultPort int) (name string, ip netip.Addr, port int, ok bool) {
	host, portText := authority, ""
	hasPort := false
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return
		}
		candidate, err := netip.ParseAddr(authority[1:end])
		// Neither bracketed IPv4 nor mapped IPv6 is a supported authority.
		if err != nil || !candidate.Is6() || candidate.Is4In6() || candidate.Zone() != "" {
			return
		}
		ip = candidate
		if suffix := authority[end+1:]; suffix != "" {
			if suffix[0] != ':' {
				return
			}
			hasPort, portText = true, suffix[1:]
		}
	} else {
		if colon := strings.IndexByte(authority, ':'); colon >= 0 {
			host, portText, hasPort = authority[:colon], authority[colon+1:], true
		}
		switch {
		case len(host) == len("localhost") && strings.EqualFold(host, "localhost"):
			name = "localhost"
		default:
			candidate, err := netip.ParseAddr(host)
			if err != nil || candidate.Is4In6() {
				return
			}
			ip = candidate
		}
	}
	port = defaultPort
	if hasPort {
		if portText == "" {
			return
		}
		for i := range len(portText) {
			if portText[i] < '0' || portText[i] > '9' {
				return
			}
		}
		var err error
		port, err = strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return
		}
	}
	return name, ip, port, true
}

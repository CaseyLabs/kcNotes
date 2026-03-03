// TEACHING NOTES:
// Middleware wraps handlers to apply cross-cutting behavior (auth, CSRF, etc.).
// In Go, middleware is usually implemented as higher-order functions:
// `func(next http.Handler) http.Handler`.
// Useful Go concepts to notice:
// 1. Wrapping order matters and affects security behavior.
// 2. Request context can carry values down the chain safely.
// 3. Keep middleware focused on one concern for composability.
// 4. Tests should validate both allow and deny paths.
package middleware

import (
	"net"
	"net/http"
	"strings"
)

type ClientIPResolver struct {
	trustedProxies []*net.IPNet
}

// NewClientIPResolver explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func NewClientIPResolver(trustedProxyCIDRs []string) (*ClientIPResolver, error) {
	resolver := &ClientIPResolver{}
	for _, raw := range trustedProxyCIDRs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			cidr := "/32"
			if ip.To4() == nil {
				cidr = "/128"
			}
			value += cidr
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, err
		}
		resolver.trustedProxies = append(resolver.trustedProxies, network)
	}
	return resolver, nil
}

// ClientIP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *ClientIPResolver) ClientIP(req *http.Request) string {
	remote := remoteAddrIP(req.RemoteAddr)
	if remote == nil {
		return req.RemoteAddr
	}
	if r != nil && r.isTrustedProxy(remote) {
		if forwarded := forwardedFor(req.Header.Get("X-Forwarded-For")); forwarded != nil {
			return forwarded.String()
		}
	}
	return remote.String()
}

// isTrustedProxy explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func (r *ClientIPResolver) isTrustedProxy(ip net.IP) bool {
	for _, network := range r.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteAddrIP explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func remoteAddrIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	return net.ParseIP(host)
}

// forwardedFor explains one unit of behavior in this package.
// In Go, functions often return early on errors to keep the success path simple.
func forwardedFor(value string) net.IP {
	for _, part := range strings.Split(value, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		if ip := net.ParseIP(candidate); ip != nil {
			return ip
		}
	}
	return nil
}

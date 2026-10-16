package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type clientIPKey struct{}

// ClientIPFromContext returns the client IP resolved by ClientIP.
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// ClientIP resolves the real client IP and stores it in the context.
//
// X-Forwarded-For is only trusted when the direct peer is one of the trusted
// proxies; it is then walked right-to-left and the first address that is not
// a trusted proxy is the client. Taking the left-most value instead would let
// any client spoof its IP (and bypass rate limiting).
func ClientIP(trustedProxies []string) (func(http.Handler) http.Handler, error) {
	nets := make([]*net.IPNet, 0, len(trustedProxies))
	for _, cidr := range trustedProxies {
		_, n, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	resolve := func(r *http.Request) string { return resolveClientIP(r, nets) }
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), clientIPKey{}, resolve(r))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

func resolveClientIP(r *http.Request, trusted []*net.IPNet) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	isTrusted := func(ip net.IP) bool {
		for _, n := range trusted {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	if len(trusted) == 0 || !isTrusted(net.ParseIP(peer)) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			continue
		}
		if !isTrusted(ip) {
			return ip.String()
		}
	}
	return peer
}

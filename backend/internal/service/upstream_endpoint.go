package service

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Upstream Center accepts explicitly configured HTTP as well as HTTPS origins,
// including public IP literals and non-default ports. Keep this separate from
// the channel monitor's HTTPS-only policy; both use the same public-IP checks
// and DNS-rebinding-safe transport.
func parseUpstreamEndpointURL(endpoint string) (*url.URL, error) {
	invalid := func() (*url.URL, error) {
		return nil, ErrUpstreamInvalid.WithMetadata(map[string]string{
			"field": "endpoint", "detail": "provide a valid HTTP or HTTPS URL with an optional port (1-65535) and path prefix",
		})
	}
	endpoint = strings.TrimSpace(endpoint)
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(endpoint, "#") || strings.Contains(endpoint, "\\") ||
		strings.IndexFunc(endpoint, unicode.IsSpace) >= 0 {
		return invalid()
	}
	// URL parsing accepts empty or out-of-range ports and unbracketed IPv6
	// authorities. Reject those before storing a configuration that cannot dial.
	if strings.HasSuffix(u.Host, ":") {
		return invalid()
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return invalid()
		}
	}
	if strings.Contains(u.Hostname(), ":") || strings.HasPrefix(u.Host, "[") {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !strings.HasPrefix(u.Host, "[") || !strings.Contains(u.Hostname(), ":") {
			return invalid()
		}
	}
	return u, nil
}

func validateUpstreamEndpoint(endpoint string) error {
	u, err := parseUpstreamEndpointURL(endpoint)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), monitorEndpointResolveTimeout)
	defer cancel()
	blocked, err := isPrivateOrLoopbackHost(ctx, u.Hostname())
	if err != nil {
		return ErrChannelMonitorEndpointUnreachable
	}
	if blocked {
		return ErrChannelMonitorEndpointPrivate
	}
	return nil
}

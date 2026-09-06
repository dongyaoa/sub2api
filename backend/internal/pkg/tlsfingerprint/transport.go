package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CacheKey identifies handshake configuration, independently of a display name.
func (p *Profile) CacheKey() string {
	if p == nil {
		return ""
	}
	copy := *p
	copy.Name = ""
	data, _ := json.Marshal(copy)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ConfigureTransport installs the same TLS handshake for HTTPS and WSS callers.
// The caller must route plaintext HTTP separately because it has no ClientHello.
func ConfigureTransport(transport *http.Transport, profile *Profile, proxyURL *url.URL) error {
	if transport == nil || profile == nil {
		return fmt.Errorf("TLS fingerprint transport and profile are required")
	}
	for _, protocol := range profile.ALPNProtocols {
		if protocol != "http/1.1" {
			return fmt.Errorf("TLS fingerprint transport supports only http/1.1 ALPN, got %q", protocol)
		}
	}
	transport.Proxy = nil
	transport.ForceAttemptHTTP2 = false
	if proxyURL == nil {
		transport.DialTLSContext = NewDialer(profile, nil).DialTLSContext
		return nil
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https":
		transport.DialTLSContext = NewHTTPProxyDialer(profile, proxyURL).DialTLSContext
	case "socks5", "socks5h":
		transport.DialTLSContext = NewSOCKS5ProxyDialer(profile, proxyURL).DialTLSContext
	default:
		return fmt.Errorf("unsupported TLS fingerprint proxy scheme %q", proxyURL.Scheme)
	}
	return nil
}

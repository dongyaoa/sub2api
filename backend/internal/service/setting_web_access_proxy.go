package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// This policy is private to the opt-in local webpage probe. It does not change
// http.DefaultTransport or any API/upstream client's proxy configuration.
type webAccessLocalProxyPolicy struct {
	fingerprint string
	proxy       func(*http.Request) (*url.URL, error)
}

type webAccessSystemProxySettings struct {
	Enabled       bool
	Server        string
	Override      string
	AutoConfigURL string
	AutoDetect    bool
}

type webAccessProxyEnvironment struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
	HasProxy   bool
	HasNoProxy bool
	CGI        bool
}

func loadWebAccessLocalProxyPolicy() (webAccessLocalProxyPolicy, error) {
	return buildWebAccessLocalProxyPolicy(readWebAccessProxyEnvironment(os.LookupEnv), readWebAccessSystemProxy)
}

func readWebAccessProxyEnvironment(lookup func(string) (string, bool)) webAccessProxyEnvironment {
	// Match the installed httpproxy version's lowercase-first precedence.
	value := func(names ...string) (string, bool) {
		present := false
		for _, name := range names {
			if entry, exists := lookup(name); exists {
				present = true
				if entry != "" {
					return entry, true
				}
			}
		}
		return "", present
	}
	httpProxy, hasHTTP := value("http_proxy", "HTTP_PROXY")
	httpsProxy, hasHTTPS := value("https_proxy", "HTTPS_PROXY")
	noProxy, hasNoProxy := value("no_proxy", "NO_PROXY")
	requestMethod, _ := lookup("REQUEST_METHOD")
	return webAccessProxyEnvironment{
		HTTPProxy: httpProxy, HTTPSProxy: httpsProxy, NoProxy: noProxy,
		HasProxy: hasHTTP || hasHTTPS, HasNoProxy: hasNoProxy, CGI: requestMethod != "",
	}
}

func buildWebAccessLocalProxyPolicy(environment webAccessProxyEnvironment, readSystem func() (webAccessSystemProxySettings, error)) (webAccessLocalProxyPolicy, error) {
	if environment.HasProxy {
		normalized := []string{environment.HTTPProxy, environment.HTTPSProxy}
		for index, raw := range normalized {
			if raw != "" {
				proxy, err := parseWebAccessProxyURL(raw)
				if err != nil {
					return webAccessLocalProxyPolicy{}, errors.New("invalid environment proxy configuration")
				}
				normalized[index] = proxy.String()
			}
		}
		proxy := (&httpproxy.Config{
			HTTPProxy: normalized[0], HTTPSProxy: normalized[1],
			NoProxy: environment.NoProxy, CGI: environment.CGI,
		}).ProxyFunc()
		return webAccessLocalProxyPolicy{
			fingerprint: webAccessProxyFingerprint("environment", environment, webAccessSystemProxySettings{}),
			proxy: func(req *http.Request) (*url.URL, error) {
				selected, err := proxy(req.URL)
				if err != nil {
					return nil, errors.New("environment proxy policy rejected the request")
				}
				return selected, nil
			},
		}, nil
	}
	// NO_PROXY alone supplies explicit bypasses without disabling an otherwise
	// active system proxy. '*' is an explicit request to use direct connections.
	if environment.HasNoProxy && strings.TrimSpace(environment.NoProxy) == "*" {
		return webAccessLocalProxyPolicy{
			fingerprint: webAccessProxyFingerprint("environment-direct", environment, webAccessSystemProxySettings{}),
			proxy:       func(*http.Request) (*url.URL, error) { return nil, nil },
		}, nil
	}
	system, err := readSystem()
	if err != nil {
		return webAccessLocalProxyPolicy{}, errors.New("cannot read system proxy configuration")
	}
	if system.AutoConfigURL != "" || system.AutoDetect {
		return webAccessLocalProxyPolicy{}, errors.New("automatic system proxy configuration is not supported for local network checks")
	}
	systemProxy, err := buildWebAccessSystemProxy(system)
	if err != nil {
		return webAccessLocalProxyPolicy{}, err
	}
	// The dummy URL is only used by the standard NO_PROXY matcher. It is never
	// assigned to a transport or dialled.
	environmentBypass := (&httpproxy.Config{
		HTTPProxy: "http://proxy.invalid:1", HTTPSProxy: "http://proxy.invalid:1", NoProxy: environment.NoProxy,
	}).ProxyFunc()
	return webAccessLocalProxyPolicy{
		fingerprint: webAccessProxyFingerprint("system", environment, system),
		proxy: func(req *http.Request) (*url.URL, error) {
			if environment.HasNoProxy {
				selected, err := environmentBypass(req.URL)
				if err != nil {
					return nil, errors.New("environment proxy bypass policy rejected the request")
				}
				if selected == nil {
					return nil, nil
				}
			}
			return systemProxy(req)
		},
	}, nil
}

func webAccessProxyFingerprint(source string, environment webAccessProxyEnvironment, system webAccessSystemProxySettings) string {
	// Only the digest is retained as the cache key. It must not be logged: the
	// source values may contain proxy credentials or a private PAC address.
	encoded, _ := json.Marshal(struct {
		Source      string
		Environment webAccessProxyEnvironment
		System      webAccessSystemProxySettings
	}{source, environment, system})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func parseWebAccessProxyURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid proxy address")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("unsupported proxy protocol")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("invalid proxy port")
		}
	}
	parsed.Path = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	return parsed, nil
}

func buildWebAccessSystemProxy(settings webAccessSystemProxySettings) (func(*http.Request) (*url.URL, error), error) {
	if !settings.Enabled {
		return func(*http.Request) (*url.URL, error) { return nil, nil }, nil
	}
	server := strings.TrimSpace(settings.Server)
	if server == "" {
		return nil, errors.New("enabled system proxy has no address")
	}
	proxies := make(map[string]*url.URL, 2)
	hasUnsupportedSOCKSFallback := false
	separator := strings.IndexByte(server, '=')
	isMapping := separator >= 0 && !strings.ContainsAny(server[:separator], ":/@")
	if isMapping {
		for _, item := range strings.Split(server, ";") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			protocol, address, found := strings.Cut(item, "=")
			protocol = strings.ToLower(strings.TrimSpace(protocol))
			if !found || strings.TrimSpace(address) == "" {
				return nil, errors.New("invalid system proxy mapping")
			}
			if protocol == "socks" {
				hasUnsupportedSOCKSFallback = true
				continue
			}
			if protocol == "ftp" {
				continue // Not a supported HTTP(S) mapping.
			}
			if protocol != "http" && protocol != "https" {
				return nil, errors.New("unsupported system proxy mapping")
			}
			if _, exists := proxies[protocol]; exists {
				return nil, errors.New("duplicate system proxy mapping")
			}
			proxy, err := parseWebAccessProxyURL(address)
			if err != nil {
				return nil, errors.New("invalid system proxy address")
			}
			proxies[protocol] = proxy
		}
		if len(proxies) == 0 {
			return nil, errors.New("system proxy has no supported HTTP or HTTPS mapping")
		}
	} else {
		proxy, err := parseWebAccessProxyURL(server)
		if err != nil {
			return nil, errors.New("invalid system proxy address")
		}
		proxies["http"], proxies["https"] = proxy, proxy
	}
	bypass, err := parseWebAccessProxyOverrides(settings.Override)
	if err != nil {
		return nil, err
	}
	return func(req *http.Request) (*url.URL, error) {
		if bypass(req.URL) {
			return nil, nil
		}
		selected := proxies[strings.ToLower(req.URL.Scheme)]
		if selected == nil && hasUnsupportedSOCKSFallback {
			return nil, errors.New("system proxy requires an unsupported SOCKS fallback")
		}
		// An omitted protocol in an explicit per-protocol map means direct,
		// unless it would require the unsupported SOCKS fallback above.
		return selected, nil
	}, nil
}

func parseWebAccessProxyOverrides(raw string) (func(*url.URL) bool, error) {
	patterns := make([]string, 0)
	for _, value := range strings.Split(raw, ";") {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if value != "<local>" {
			if _, err := path.Match(value, ""); err != nil {
				return nil, errors.New("invalid system proxy bypass pattern")
			}
		}
		patterns = append(patterns, value)
	}
	return func(target *url.URL) bool {
		host := strings.ToLower(target.Hostname())
		port := target.Port()
		if port == "" {
			port = "80"
			if target.Scheme == "https" {
				port = "443"
			}
		}
		for _, pattern := range patterns {
			if pattern == "<local>" {
				if _, err := netip.ParseAddr(host); err != nil && !strings.Contains(host, ".") {
					return true
				}
				continue
			}
			if prefix, err := netip.ParsePrefix(pattern); err == nil {
				if address, err := netip.ParseAddr(host); err == nil && prefix.Contains(address) {
					return true
				}
				continue
			}
			if address, err := netip.ParseAddr(strings.Trim(pattern, "[]")); err == nil {
				if targetAddress, err := netip.ParseAddr(host); err == nil && address == targetAddress {
					return true
				}
				continue
			}
			if patternHost, patternPort, err := net.SplitHostPort(pattern); err == nil {
				if patternPort != port && patternPort != "*" {
					continue
				}
				pattern = patternHost
			}
			if matched, _ := path.Match(pattern, host); matched {
				return true
			}
		}
		return false
	}, nil
}

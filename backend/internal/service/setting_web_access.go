package service

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	SettingKeyWebAccessCNBlockEnabled = "web_access_cn_block_enabled"
	SettingKeyWebAccessBlockMessage   = "web_access_block_message"
	SettingKeyWebAccessIPAllowlist    = "web_access_ip_allowlist"
	DefaultWebAccessBlockMessage      = "本網站暫不向中國大陸地區提供網頁訪問服務。"
	webAccessSettingsCacheTTL         = 30 * time.Second
	webAccessSettingsDBTimeout        = time.Second
	webAccessMessageMaxRunes          = 500
	webAccessAllowlistMaxEntries      = 256
)

// WebAccessDecision is a per-visitor result. It must never be cached in the
// shared public settings or HTML cache. API authentication does not use it.
type WebAccessDecision struct {
	Enabled        bool   `json:"enabled"`
	Blocked        bool   `json:"blocked"`
	CountryCode    string `json:"country_code,omitempty"`
	Message        string `json:"message"`
	LocationSource string `json:"location_source"`
}

type cachedWebAccessSettings struct {
	enabled   bool
	message   string
	allowlist []netip.Prefix
	expiresAt time.Time
}

func normalizedWebAccessMessage(value string) string {
	// Upgrade the previously saved default while preserving custom messages.
	if trimmed := strings.TrimSpace(value); trimmed != "" && trimmed != "本网站暂不向中国大陆地区提供网页访问服务。" {
		return trimmed
	}
	return DefaultWebAccessBlockMessage
}

func normalizeWebAccessSettings(settings *SystemSettings) error {
	message := normalizedWebAccessMessage(settings.WebAccessBlockMessage)
	if !utf8.ValidString(message) || utf8.RuneCountInString(message) > webAccessMessageMaxRunes {
		return fmt.Errorf("web access message must be valid text of at most %d characters", webAccessMessageMaxRunes)
	}
	for _, character := range message {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return fmt.Errorf("web access message cannot contain control characters")
		}
	}
	allowlist, err := NormalizeWebAccessIPAllowlist(settings.WebAccessIPAllowlist)
	if err != nil {
		return err
	}
	settings.WebAccessBlockMessage = message
	settings.WebAccessIPAllowlist = allowlist
	return nil
}

// NormalizeWebAccessIPAllowlist accepts literal IPv4/IPv6 addresses and CIDRs.
// Host bits are masked, duplicates removed, and mapped IPv4 is canonicalized.
func NormalizeWebAccessIPAllowlist(values []string) ([]string, error) {
	if len(values) > webAccessAllowlistMaxEntries {
		return nil, fmt.Errorf("web access IP allowlist supports at most %d entries", webAccessAllowlistMaxEntries)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		var canonical string
		if address, err := netip.ParseAddr(value); err == nil && address.Zone() == "" {
			canonical = address.Unmap().String()
		} else {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, fmt.Errorf("invalid web access allowlist IP or CIDR: %q", value)
			}
			if prefix.Addr().Is4In6() {
				if prefix.Bits() < 96 {
					return nil, fmt.Errorf("mapped IPv4 CIDR must have at least 96 prefix bits: %q", value)
				}
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			prefix = prefix.Masked()
			canonical = prefix.String()
			if prefix.Bits() == prefix.Addr().BitLen() {
				canonical = prefix.Addr().String()
			}
		}
		if _, exists := seen[canonical]; !exists {
			seen[canonical] = struct{}{}
			result = append(result, canonical)
		}
	}
	return result, nil
}

func parseWebAccessIPAllowlist(value string) ([]string, error) {
	values := []string{}
	if strings.TrimSpace(value) != "" {
		if err := json.Unmarshal([]byte(value), &values); err != nil {
			return nil, fmt.Errorf("parse web access IP allowlist: %w", err)
		}
	}
	return NormalizeWebAccessIPAllowlist(values)
}

func parseWebAccessIPAllowlistForAdmin(value string) []string {
	values, err := parseWebAccessIPAllowlist(value)
	if err != nil {
		slog.Warn("invalid stored web access IP allowlist", "error", err)
		return []string{}
	}
	return values
}

func (s *SettingService) invalidateWebAccessSettings() {
	// Coordinate with a lookup already reading the database so an older lookup
	// cannot overwrite the invalidation after a successful settings save.
	s.webAccessSettingsMu.Lock()
	s.webAccessSettingsCache.Store(nil)
	s.webAccessSettingsMu.Unlock()
}

func (s *SettingService) getWebAccessSettings(ctx context.Context) (*cachedWebAccessSettings, error) {
	if cached := s.webAccessSettingsCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached, nil
	}
	s.webAccessSettingsMu.Lock()
	defer s.webAccessSettingsMu.Unlock()
	if cached := s.webAccessSettingsCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(ctx, webAccessSettingsDBTimeout)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{
		SettingKeyWebAccessCNBlockEnabled, SettingKeyWebAccessBlockMessage, SettingKeyWebAccessIPAllowlist,
	})
	if err != nil {
		return nil, fmt.Errorf("get web access settings: %w", err)
	}
	settings := &cachedWebAccessSettings{
		enabled:   values[SettingKeyWebAccessCNBlockEnabled] == "true",
		message:   normalizedWebAccessMessage(values[SettingKeyWebAccessBlockMessage]),
		expiresAt: time.Now().Add(webAccessSettingsCacheTTL),
	}
	// Disabled means inactive: unrelated optional configuration must not make
	// normal webpage access depend on parsing an allowlist that is not in use.
	if !settings.enabled {
		s.webAccessSettingsCache.Store(settings)
		return settings, nil
	}
	allowlist, err := parseWebAccessIPAllowlist(values[SettingKeyWebAccessIPAllowlist])
	if err != nil {
		return nil, err
	}
	settings.allowlist = make([]netip.Prefix, 0, len(allowlist))
	for _, value := range allowlist {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, _ := netip.ParseAddr(value) // Normalized above.
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		settings.allowlist = append(settings.allowlist, prefix)
	}
	s.webAccessSettingsCache.Store(settings)
	return settings, nil
}

func webAccessForceDisabled() bool {
	forceDisabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("WEB_ACCESS_FORCE_DISABLED")))
	return forceDisabled
}

// GetWebAccessDecision evaluates only webpage access. The caller must supply
// an IP obtained from the trusted proxy chain, never a raw forwarded header.
// Unknown/private addresses are allowed unless explicit local-network mode
// resolves a loopback visitor to this local instance's public egress address.
// Configuration and explicit local-network lookup errors are returned to
// the webpage caller; existing API routes never call this method.
func (s *SettingService) GetWebAccessDecision(ctx context.Context, clientIP string) (WebAccessDecision, error) {
	decision := WebAccessDecision{Message: DefaultWebAccessBlockMessage, LocationSource: WebAccessLocationClientIP}
	if webAccessForceDisabled() || s == nil || s.settingRepo == nil {
		return decision, nil
	}
	settings, err := s.getWebAccessSettings(ctx)
	if err != nil {
		return decision, err
	}
	decision.Enabled = settings.enabled
	decision.Message = settings.message
	if !settings.enabled {
		return decision, nil
	}
	address, err := netip.ParseAddr(strings.TrimSpace(clientIP))
	if err != nil || address.Zone() != "" {
		return decision, nil
	}
	address = address.Unmap()
	if s.shouldCheckLocalWebAccessNetwork(address) {
		address, err = s.localWebAccessNetwork().resolve(ctx)
		if err != nil {
			decision.LocationSource = WebAccessLocationLocalNetworkUnavailable
			return decision, err
		}
		decision.LocationSource = WebAccessLocationLocalNetwork
	}
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return decision, nil
	}
	for _, prefix := range settings.allowlist {
		if prefix.Contains(address) {
			return decision, nil
		}
	}
	networks, err := embeddedWebAccessCNNetworks()
	if err != nil {
		return decision, err
	}
	if networks.contains(address) {
		decision.CountryCode = "CN"
		decision.Blocked = true
	}
	return decision, nil
}

// Lists are pinned, licensed source data; see geoipdata/README.md. They are
// validated completely before publishing the immutable lookup index.
//
//go:embed geoipdata/china.txt
var webAccessChinaIPv4 string

//go:embed geoipdata/china6.txt
var webAccessChinaIPv6 string

type webAccessPrefixIndex struct {
	byBits [129]map[netip.Addr]struct{}
}

var embeddedWebAccessCNNetworks = sync.OnceValues(func() (*webAccessPrefixIndex, error) {
	return parseWebAccessCNNetworks(webAccessChinaIPv4, webAccessChinaIPv6)
})

func parseWebAccessCNNetworks(ipv4, ipv6 string) (*webAccessPrefixIndex, error) {
	index := &webAccessPrefixIndex{}
	for _, source := range []struct {
		data string
		bits int
	}{{ipv4, 32}, {ipv6, 128}} {
		count := 0
		for lineNumber, line := range strings.Split(source.data, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			prefix, err := netip.ParsePrefix(line)
			if err != nil || prefix.Addr().BitLen() != source.bits || prefix.Addr().Is4In6() {
				return nil, fmt.Errorf("invalid IPv%d country network on line %d", source.bits, lineNumber+1)
			}
			prefix = prefix.Masked()
			if index.byBits[prefix.Bits()] == nil {
				index.byBits[prefix.Bits()] = make(map[netip.Addr]struct{})
			}
			index.byBits[prefix.Bits()][prefix.Addr()] = struct{}{}
			count++
		}
		if count == 0 {
			return nil, fmt.Errorf("empty IPv%d country network data", source.bits)
		}
	}
	return index, nil
}

func (index *webAccessPrefixIndex) contains(address netip.Addr) bool {
	// At most 129 hash lookups; independent of the number of listed networks.
	for bits := 0; bits <= address.BitLen(); bits++ {
		if entries := index.byBits[bits]; entries != nil {
			prefix, _ := address.Prefix(bits)
			if _, exists := entries[prefix.Masked().Addr()]; exists {
				return true
			}
		}
	}
	return false
}

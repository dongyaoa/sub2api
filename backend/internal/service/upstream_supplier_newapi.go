package service

import (
	"net/url"
	"strings"
)

// Keep this normalization in sync with upstream_newapi_base in migration 269.
// Matching includes the API path prefix; sharing a host alone is insufficient.
func upstreamNewAPIBase(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		strings.ContainsAny(endpoint, "%\\\r\n\t ") || strings.Contains(u.Path, "//") {
		return ""
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return ""
		}
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimSuffix(host, ":443")
	path := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/models", "/v1/usage", "/v1beta/models", "/v1beta", "/v1"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	return "https://" + host + path
}

func (s *UpstreamCenterService) applySupplierNewAPICredentials(v *UpstreamSupplier, in UpstreamSupplierNewAPIInput, oldWebsite, oldBase string) error {
	if in.NewAPIAPIBase != nil {
		v.NewAPIAPIBase = strings.TrimSpace(*in.NewAPIAPIBase)
	}
	if v.NewAPIAPIBase == "" && (v.NewAPIUserID > 0 || in.NewAPIUserID != nil && *in.NewAPIUserID > 0) {
		v.NewAPIAPIBase = v.Website
	}
	if v.NewAPIAPIBase != "" {
		v.NewAPIAPIBase = upstreamNewAPIBase(v.NewAPIAPIBase)
		if v.NewAPIAPIBase == "" || len(v.NewAPIAPIBase) > 500 {
			return ErrUpstreamInvalid.WithMetadata(map[string]string{"field": "newapi_api_base", "detail": "provide an HTTPS New API site URL, including its API path prefix if any"})
		}
	}
	changed := oldBase != v.NewAPIAPIBase || normalizeEndpoint(oldWebsite) != normalizeEndpoint(v.Website)
	credentials := &UpstreamTarget{NewAPIUserID: v.NewAPIUserID, NewAPIAccessTokenEncrypted: v.NewAPIAccessTokenEncrypted}
	if err := s.applyNewAPICredentials(credentials, UpstreamTargetInput{NewAPIUserID: in.NewAPIUserID, NewAPIAccessToken: in.NewAPIAccessToken}, v.NewAPIUserID, v.NewAPIAccessTokenEncrypted, changed); err != nil {
		return err
	}
	if credentials.NewAPIUserID > 0 && v.NewAPIAPIBase == "" {
		return ErrUpstreamInvalid.WithMetadata(map[string]string{"field": "newapi_api_base", "detail": "provide a New API site URL before enabling site authorization"})
	}
	v.NewAPIUserID, v.NewAPIAccessTokenEncrypted = credentials.NewAPIUserID, credentials.NewAPIAccessTokenEncrypted
	v.NewAPIAccessTokenConfigured = v.NewAPIUserID > 0 && v.NewAPIAccessTokenEncrypted != ""
	if in.NewAPIUserID != nil || in.NewAPIAccessToken != nil && strings.TrimSpace(*in.NewAPIAccessToken) != "" {
		v.NewAPICredentialsManaged = true
		v.NewAPILegacyConflict = false
	}
	return nil
}

func inheritSupplierNewAPICredentials(t *UpstreamTarget, supplier *UpstreamSupplier) {
	t.NewAPIUserID, t.NewAPIAccessTokenEncrypted = 0, ""
	t.NewAPICredentialsInherited = false
	if supplier.NewAPIUserID > 0 && supplier.NewAPIAccessTokenEncrypted != "" && supplier.NewAPIAPIBase != "" &&
		upstreamNewAPIBase(t.Endpoint) == supplier.NewAPIAPIBase {
		t.NewAPIUserID, t.NewAPIAccessTokenEncrypted = supplier.NewAPIUserID, supplier.NewAPIAccessTokenEncrypted
		t.NewAPICredentialsInherited = true
	}
}

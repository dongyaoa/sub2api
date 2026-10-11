package service

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// UpstreamSupplierAccount is the credential-free projection needed by the
// existing-account picker. It deliberately avoids account usage and quota data.
type UpstreamSupplierAccount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Endpoint string `json:"endpoint"`
}

type UpstreamSupplierAccountsQuery struct {
	Origins  []string
	Search   string
	Page     int
	PageSize int
}

type UpstreamSupplierAccountsPage struct {
	Items    []*UpstreamSupplierAccount `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
	Pages    int                        `json:"pages"`
}

// Optional so existing repository decorators need not implement a picker.
type UpstreamSupplierAccountsRepository interface {
	ListSupplierAccounts(context.Context, UpstreamSupplierAccountsQuery) ([]*UpstreamSupplierAccount, int64, error)
}

// Account candidates are scoped to the site's configured origins, rather than
// its existing groups: a supplier with no groups must work immediately.
func (s *UpstreamCenterService) SupplierAccounts(ctx context.Context, id int64, query UpstreamSupplierAccountsQuery) (*UpstreamSupplierAccountsPage, error) {
	if id <= 0 {
		return nil, ErrUpstreamInvalid
	}
	supplier, err := s.repo.GetSupplier(ctx, id)
	if err != nil {
		return nil, err
	}
	if supplier == nil {
		return nil, ErrUpstreamNotFound
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 100
	}
	query.Search = strings.TrimSpace(query.Search)
	if search := []rune(query.Search); len(search) > 100 {
		query.Search = string(search[:100])
	}
	query.Origins = nil // Never accept caller-selected sites.
	for _, address := range []string{supplier.Website, supplier.NewAPIAPIBase} {
		origin := upstreamSupplierAccountOrigin(address)
		if origin != "" && (len(query.Origins) == 0 || query.Origins[0] != origin) {
			query.Origins = append(query.Origins, origin)
		}
	}
	result := &UpstreamSupplierAccountsPage{Items: []*UpstreamSupplierAccount{}, Page: query.Page, PageSize: query.PageSize}
	// An unspecified site must never fall back to the global account inventory.
	if len(query.Origins) == 0 {
		return result, nil
	}
	repo, ok := s.repo.(UpstreamSupplierAccountsRepository)
	if !ok {
		return nil, fmt.Errorf("upstream supplier account repository is unavailable")
	}
	items, total, err := repo.ListSupplierAccounts(ctx, query)
	if err != nil {
		return nil, err
	}
	if items != nil {
		result.Items = items
	}
	result.Total = total
	result.Pages = int((total + int64(query.PageSize) - 1) / int64(query.PageSize))
	return result, nil
}

// Paths do not change the site: /, /v1 and protocol-specific prefixes share an
// account picker. Different schemes, subdomains and non-default ports do.
func upstreamSupplierAccountOrigin(address string) string {
	address = strings.TrimSpace(address)
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.ContainsAny(address, "\\\r\n\t ") {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if rawPort := u.Port(); rawPort != "" {
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return ""
		}
		defaultPort := scheme == "https" && port == 443 || scheme == "http" && port == 80
		if !defaultPort {
			host += ":" + strconv.Itoa(port)
		}
	}
	return scheme + "://" + host
}

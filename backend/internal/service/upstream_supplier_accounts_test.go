package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type upstreamSupplierAccountsTestRepo struct {
	UpstreamCenterRepository
	supplier *UpstreamSupplier
	query    UpstreamSupplierAccountsQuery
	reads    int
}

func (r *upstreamSupplierAccountsTestRepo) GetSupplier(context.Context, int64) (*UpstreamSupplier, error) {
	if r.supplier == nil {
		return nil, ErrUpstreamNotFound
	}
	return r.supplier, nil
}

func (r *upstreamSupplierAccountsTestRepo) ListSupplierAccounts(_ context.Context, query UpstreamSupplierAccountsQuery) ([]*UpstreamSupplierAccount, int64, error) {
	r.query, r.reads = query, r.reads+1
	return []*UpstreamSupplierAccount{{ID: 7, Name: "Account", Platform: "openai", Endpoint: "https://example.com/v1"}}, 101, nil
}

func TestUpstreamSupplierAccountsUsesSiteWithoutExistingGroups(t *testing.T) {
	repo := &upstreamSupplierAccountsTestRepo{supplier: &UpstreamSupplier{ID: 3, Website: " https://EXAMPLE.com:443/dashboard/ ", NewAPIAPIBase: "https://api.example.com/prefix", NewAPIAccessTokenEncrypted: "secret-site-token"}}
	svc := NewUpstreamCenterService(repo, nil, nil, nil)
	page, err := svc.SupplierAccounts(context.Background(), 3, UpstreamSupplierAccountsQuery{Origins: []string{"https://not-this-site.com"}, Search: "  " + strings.Repeat("账", 101) + "  ", Page: -1, PageSize: 999})
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com", "https://api.example.com"}, repo.query.Origins)
	require.Len(t, []rune(repo.query.Search), 100)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 100, page.PageSize)
	require.Equal(t, 2, page.Pages)
	require.Equal(t, int64(101), page.Total)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-site-token")
	require.NotContains(t, string(encoded), "credentials")
	repo.supplier.NewAPIAPIBase = "https://EXAMPLE.com:443/prefix/v1"
	_, err = svc.SupplierAccounts(context.Background(), 3, UpstreamSupplierAccountsQuery{Page: 2, PageSize: 50})
	require.NoError(t, err)
	require.Equal(t, []string{"https://example.com"}, repo.query.Origins)
	require.Equal(t, 2, repo.query.Page)
	require.Equal(t, 50, repo.query.PageSize)
}

func TestUpstreamSupplierAccountsNeverFallsBackToGlobalInventory(t *testing.T) {
	repo := &upstreamSupplierAccountsTestRepo{supplier: &UpstreamSupplier{ID: 3}}
	svc := NewUpstreamCenterService(repo, nil, nil, nil)
	for _, website := range []string{"", "invalid", "ftp://example.com", "https://user:password@example.com"} {
		repo.supplier.Website = website
		page, err := svc.SupplierAccounts(context.Background(), 3, UpstreamSupplierAccountsQuery{})
		require.NoError(t, err)
		require.NotNil(t, page.Items)
		require.Empty(t, page.Items)
		require.Zero(t, page.Total)
	}
	require.Zero(t, repo.reads)
	repo.supplier = nil
	_, err := svc.SupplierAccounts(context.Background(), 3, UpstreamSupplierAccountsQuery{})
	require.ErrorIs(t, err, ErrUpstreamNotFound)
}

func TestUpstreamSupplierAccountOrigin(t *testing.T) {
	for _, tc := range []struct{ address, origin string }{
		{"HTTPS://Example.com:443/v1/", "https://example.com"},
		{"http://Example.com:80/", "http://example.com"},
		{"https://example.com:8443/prefix", "https://example.com:8443"},
		{"https://[2001:DB8::1]:443/v1", "https://[2001:db8::1]"},
		{"https://example.com:00443/", "https://example.com"},
		{"https://example.com:65536/", ""},
		{"https://user@example.com/", ""},
		{"https://example.com\\@different.com/", ""},
		{"//example.com", ""},
	} {
		t.Run(tc.address, func(t *testing.T) {
			require.Equal(t, tc.origin, upstreamSupplierAccountOrigin(tc.address))
		})
	}
}

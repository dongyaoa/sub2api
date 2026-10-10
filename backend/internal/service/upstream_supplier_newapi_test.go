//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamSupplierNewAPICredentialsEncryptPreserveClear(t *testing.T) {
	repo := &upstreamRechargeSupplierRepo{supplier: &UpstreamSupplier{ID: 7, Name: "Site", Website: "https://example.com/prefix"}}
	svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, nil, nil)
	userID, token := int64(42), " site-secret "
	saved, err := svc.SaveSupplier(context.Background(), 7, nil, nil, nil, nil,
		UpstreamSupplierNewAPIInput{NewAPIUserID: &userID, NewAPIAccessToken: &token})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/prefix", saved.NewAPIAPIBase)
	require.True(t, saved.NewAPIAccessTokenConfigured)
	require.True(t, saved.NewAPICredentialsManaged)
	require.Equal(t, "encrypted:site-secret", repo.saved.NewAPIAccessTokenEncrypted)
	encoded, err := json.Marshal(saved)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "site-secret")
	require.NotContains(t, string(encoded), "newapi_access_token_encrypted")
	repo.supplier = repo.saved
	blank := ""
	saved, err = svc.SaveSupplier(context.Background(), 7, nil, nil, nil, nil, UpstreamSupplierNewAPIInput{NewAPIAccessToken: &blank})
	require.NoError(t, err)
	require.Equal(t, "encrypted:site-secret", saved.NewAPIAccessTokenEncrypted)
	zero := int64(0)
	saved, err = svc.SaveSupplier(context.Background(), 7, nil, nil, nil, nil, UpstreamSupplierNewAPIInput{NewAPIUserID: &zero})
	require.NoError(t, err)
	require.Empty(t, saved.NewAPIAccessTokenEncrypted)
	require.False(t, saved.NewAPIAccessTokenConfigured)
	require.True(t, saved.NewAPICredentialsManaged, "explicit clearing must also disable legacy group credentials")
}

func TestUpstreamSupplierNewAPICredentialsRequireConfirmationOnRecipientChange(t *testing.T) {
	otherWebsite, otherBase, otherUser := "https://other.example", "https://example.com/another", int64(43)
	for _, tc := range []struct {
		name    string
		website *string
		input   UpstreamSupplierNewAPIInput
	}{
		{"website", &otherWebsite, UpstreamSupplierNewAPIInput{}},
		{"API prefix", nil, UpstreamSupplierNewAPIInput{NewAPIAPIBase: &otherBase}},
		{"user", nil, UpstreamSupplierNewAPIInput{NewAPIUserID: &otherUser}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamRechargeSupplierRepo{supplier: &UpstreamSupplier{ID: 7, Name: "Site", Website: "https://example.com", NewAPIAPIBase: "https://example.com", NewAPIUserID: 42, NewAPIAccessTokenEncrypted: "encrypted:old", NewAPICredentialsManaged: true}}
			svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, nil, nil)
			_, err := svc.SaveSupplier(context.Background(), 7, nil, tc.website, nil, nil, tc.input)
			require.ErrorIs(t, err, ErrUpstreamInvalid)
			require.Nil(t, repo.saved)
			replacement := "new-secret"
			tc.input.NewAPIAccessToken = &replacement
			_, err = svc.SaveSupplier(context.Background(), 7, nil, tc.website, nil, nil, tc.input)
			require.NoError(t, err)
			require.Equal(t, "encrypted:new-secret", repo.saved.NewAPIAccessTokenEncrypted)
		})
	}
}

type upstreamSiteTargetTestRepo struct {
	*upstreamTestRepo
	site *UpstreamSupplier
}

func (r *upstreamSiteTargetTestRepo) GetSupplier(context.Context, int64) (*UpstreamSupplier, error) {
	copy := *r.site
	return &copy, nil
}

func TestUpstreamSupplierNewAPITargetInheritanceIsScoped(t *testing.T) {
	siteID := int64(9)
	for _, tc := range []struct {
		endpoint   string
		authorized bool
	}{
		{"https://8.8.8.8/prefix/v1", true},
		{"https://8.8.8.8:443/prefix/v1/messages", true},
		{"https://8.8.8.8/other/v1", false},
		{"https://1.1.1.1/prefix/v1", false},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			target := validUpstreamTestTarget()
			target.SupplierID, target.Endpoint = &siteID, tc.endpoint
			repo := &upstreamSiteTargetTestRepo{upstreamTestRepo: &upstreamTestRepo{target: target}, site: &UpstreamSupplier{ID: siteID, NewAPIUserID: 42, NewAPIAccessTokenEncrypted: "encrypted:site-secret", NewAPIAPIBase: "https://8.8.8.8/prefix", NewAPICredentialsManaged: true}}
			svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, nil, nil)
			saved, err := svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{})
			require.NoError(t, err)
			require.Equal(t, tc.authorized, saved.NewAPIAccessTokenConfigured)
			require.Equal(t, tc.authorized, saved.NewAPICredentialsInherited)
			if !tc.authorized {
				require.Empty(t, repo.saved.NewAPIAccessTokenEncrypted)
			}
		})
	}
	target := validUpstreamTestTarget()
	target.SupplierID, target.NewAPIUserID, target.NewAPIAccessTokenEncrypted, target.NewAPICredentialsInherited = &siteID, 42, "encrypted:site-secret", true
	repo := &upstreamTestRepo{target: target}
	svc := NewUpstreamCenterService(repo, upstreamTestEncryptor{}, nil, nil)
	saved, err := svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{SupplierID: json.RawMessage("null")})
	require.NoError(t, err)
	require.False(t, saved.NewAPIAccessTokenConfigured)
	require.Empty(t, saved.NewAPIAccessTokenEncrypted)
}

func TestUpstreamSupplierNewAPIWalletDeduplicatesGroups(t *testing.T) {
	now := time.Now()
	targets := []*UpstreamTarget{
		{ID: 1, Endpoint: "https://EXAMPLE.com:443/site/v1", NewAPIUserID: 42, WalletRef: "group A", Balance: &UpstreamBalanceSnapshot{TargetID: 1, Kind: "wallet", Balance: financeFloat(30), Currency: "USD", Status: "ok", SyncedAt: &now}},
		{ID: 2, Endpoint: "https://example.com/site/v1/messages", NewAPIUserID: 42, WalletRef: "group B", Balance: &UpstreamBalanceSnapshot{TargetID: 2, Kind: "wallet", Balance: financeFloat(30), Currency: "USD", Status: "ok"}},
		{ID: 3, Endpoint: "https://example.com/site", NewAPIUserID: 42, WalletRef: "group C", Balance: &UpstreamBalanceSnapshot{TargetID: 3, Kind: "key_quota", QuotaRemaining: financeFloat(10), Status: "ok"}},
	}
	wallets := upstreamSupplierWallets(targets)
	require.Len(t, wallets, 1)
	require.Equal(t, 30.0, *wallets[0].Balance)
	require.Equal(t, int64(1), wallets[0].TargetID)
	targets[1].NewAPIUserID = 43
	require.Len(t, upstreamSupplierWallets(targets), 2, "different New API accounts stay separate")
	targets[1].NewAPIUserID, targets[1].Endpoint = 42, "https://example.com/other/v1"
	require.Len(t, upstreamSupplierWallets(targets), 2, "different deployments on one host stay separate")
}

func TestUpstreamSupplierNewAPIBaseRejectsAmbiguousRecipient(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "https://example.com/%2fprivate", "https://example.com/a/../b", "https://example.com//b", "https://user@example.com", "https://example.com?q=x", "https://example.com/#a"} {
		require.Empty(t, upstreamNewAPIBase(endpoint), endpoint)
	}
}

func TestUpstreamSupplierNewAPIWalletPrefersConfirmedUnitOverNewerQuota(t *testing.T) {
	now := time.Now()
	before := now.Add(-time.Minute)
	confirmed := &UpstreamBalanceSnapshot{TargetID: 1, Kind: "wallet", Balance: financeFloat(30), Currency: "USD", CurrencySource: "newapi_status", Status: "ok", SyncedAt: &before, LastAttemptAt: &before}
	raw := &UpstreamBalanceSnapshot{TargetID: 2, Kind: "wallet", Balance: financeFloat(16000000), Currency: "QUOTA", CurrencySource: "reported", Status: "ok", Error: "newapi_quota_unit_unknown", SyncedAt: &now, LastAttemptAt: &now}
	for _, order := range [][]*UpstreamBalanceSnapshot{{confirmed, raw}, {raw, confirmed}} {
		targets := make([]*UpstreamTarget, 0, len(order))
		for _, snapshot := range order {
			targets = append(targets, &UpstreamTarget{ID: snapshot.TargetID, Endpoint: "https://example.com/v1", NewAPIUserID: 42, Balance: snapshot})
		}
		wallets := upstreamSupplierWallets(targets)
		require.Len(t, wallets, 1, "unit lookup failure must not create a second site wallet")
		want := *confirmed
		want.WalletRef = "default"
		require.Equal(t, &want, wallets[0], "amount, timestamps, source group and status must come from one snapshot")
		require.Equal(t, "newapi_quota_unit_unknown", raw.Error, "group observation must retain its own failure")
	}
	newerConfirmed := *confirmed
	newerConfirmed.TargetID, newerConfirmed.SyncedAt, newerConfirmed.Balance = 3, &now, financeFloat(29)
	require.Equal(t, int64(3), mergeUpstreamNewAPIWalletBalances(confirmed, &newerConfirmed).TargetID, "same verified units still choose the newest observation")
}

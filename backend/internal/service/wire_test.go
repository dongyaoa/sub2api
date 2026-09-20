package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/collection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pluginWiringAccountRepository struct {
	AccountRepository
	accounts []Account
}

func (r *pluginWiringAccountRepository) ListByPlatform(context.Context, string) ([]Account, error) {
	return r.accounts, nil
}

func TestProvidePluginManager_ExposesConfiguredHostServices(t *testing.T) {
	store := newFakePluginKVStore()
	gateway := &OpenAIGatewayService{accountRepo: &pluginWiringAccountRepository{
		accounts: []Account{{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}},
	}}
	manager := ProvidePluginManager(nil, nil, &config.Config{}, PluginHostInfo{}, store, gateway)
	installation := &PluginInstallation{PluginKey: "test.wiring", Manifest: PluginManifest{
		Capabilities: []PluginCapability{{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth}},
	}}
	ctx := context.Background()

	server := manager.buildHostServices(installation)
	require.NotNil(t, server)
	_, err := server.KVSet(ctx, &pluginv1.KVSetRequest{Namespace: "state", Key: "cursor", Value: []byte("saved")})
	require.NoError(t, err)
	value, found, err := store.Get(ctx, installation.PluginKey, "state", "cursor")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []byte("saved"), value)

	accounts, err := server.ListAccounts(ctx, &pluginv1.ListAccountsRequest{Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})
	require.NoError(t, err)
	require.Equal(t, []int64{41}, accounts.AccountIds)

	// A different plugin still has KV storage but only a matching capability
	// allows its runtime to use the wired account directory.
	other := manager.buildHostServices(&PluginInstallation{PluginKey: "test.other"})
	require.NotNil(t, other)
	_, err = other.ListAccounts(ctx, &pluginv1.ListAccountsRequest{})
	require.Equal(t, codes.Unavailable, status.Code(err))
	_, err = other.KVGet(ctx, &pluginv1.KVGetRequest{Namespace: "state", Key: "cursor"})
	require.NoError(t, err)
}

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService()
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}

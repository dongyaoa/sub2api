//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelMonitorGroupRateEncryptor struct{}

func (channelMonitorGroupRateEncryptor) Encrypt(value string) (string, error) {
	return "encrypted:" + value, nil
}

func (channelMonitorGroupRateEncryptor) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "encrypted:"), nil
}

type channelMonitorGroupRateResolverStub struct {
	groups map[string]ChannelMonitorAPIKeyGroup
	keys   []string
}

func (r *channelMonitorGroupRateResolverStub) ResolveByKeys(
	_ context.Context,
	keys []string,
) (map[string]ChannelMonitorAPIKeyGroup, error) {
	r.keys = append([]string(nil), keys...)
	return r.groups, nil
}

func TestHydrateMonitorGroupsFromAPIKeysResolvesExistingMonitorWithoutGroupName(t *testing.T) {
	fallbackRate := 0.8
	monitors := []*ChannelMonitor{
		{ID: 1, APIKey: "encrypted:local-key"},
		{ID: 2, APIKey: "encrypted:external-key", GroupName: "fallback", GroupRateMultiplier: &fallbackRate},
		{ID: 3, APIKey: "encrypted:local-key"},
	}
	resolver := &channelMonitorGroupRateResolverStub{groups: map[string]ChannelMonitorAPIKeyGroup{
		"local-key": {Name: "standard", RateMultiplier: 0.35},
	}}
	svc := NewChannelMonitorService(nil, channelMonitorGroupRateEncryptor{})
	svc.SetAPIKeyGroupResolver(resolver)

	svc.hydrateMonitorGroupsFromAPIKeys(context.Background(), monitors)

	require.ElementsMatch(t, []string{"local-key", "external-key"}, resolver.keys)
	for _, index := range []int{0, 2} {
		require.Equal(t, "standard", monitors[index].GroupName)
		require.NotNil(t, monitors[index].GroupRateMultiplier)
		require.InDelta(t, 0.35, *monitors[index].GroupRateMultiplier, 1e-12)
	}
	require.Equal(t, "fallback", monitors[1].GroupName)
	require.Equal(t, &fallbackRate, monitors[1].GroupRateMultiplier)
}

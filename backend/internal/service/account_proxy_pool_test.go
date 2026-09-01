package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAccountProxyPool(t *testing.T) {
	entries, err := ParseAccountProxyPool([]any{
		map[string]any{"proxy_id": float64(11), "concurrency": float64(10)},
		map[string]any{"proxy_id": float64(12), "concurrency": float64(5)},
	})
	require.NoError(t, err)
	require.Equal(t, []AccountProxyPoolEntry{
		{ProxyID: 11, Concurrency: 10},
		{ProxyID: 12, Concurrency: 5},
	}, entries)

	for _, input := range []any{
		[]AccountProxyPoolEntry{{ProxyID: 1, Concurrency: 1}, {ProxyID: 1, Concurrency: 2}},
		[]AccountProxyPoolEntry{{ProxyID: 1, Concurrency: 0}},
		[]AccountProxyPoolEntry{{ProxyID: 0, Concurrency: 1}},
	} {
		_, err := ParseAccountProxyPool(input)
		require.Error(t, err)
	}
}

func TestSelectAccountProxyUsesConfiguredWeights(t *testing.T) {
	account := &Account{ProxyPool: []AccountProxyPoolEntry{
		{ProxyID: 21, Concurrency: 1, Proxy: &Proxy{ID: 21, Status: StatusActive}},
		{ProxyID: 22, Concurrency: 2, Proxy: &Proxy{ID: 22, Status: StatusActive}},
	}}
	counts := map[int64]int{}
	for i := 0; i < 30; i++ {
		SelectAccountProxy(account)
		require.NotNil(t, account.ProxyID)
		counts[*account.ProxyID]++
	}
	require.Equal(t, 10, counts[21])
	require.Equal(t, 20, counts[22])
}

func TestCarryAccountProxySelection(t *testing.T) {
	source := &Account{ProxyPool: []AccountProxyPoolEntry{
		{ProxyID: 31, Concurrency: 1, Proxy: &Proxy{ID: 31, Status: StatusActive}},
		{ProxyID: 32, Concurrency: 1, Proxy: &Proxy{ID: 32, Status: StatusActive}},
	}}
	SelectAccountProxy(source)
	target := &Account{ProxyPool: []AccountProxyPoolEntry{
		{ProxyID: 31, Concurrency: 1, Proxy: &Proxy{ID: 31, Status: StatusActive}},
		{ProxyID: 32, Concurrency: 1, Proxy: &Proxy{ID: 32, Status: StatusActive}},
	}}
	CarryAccountProxySelection(source, target)
	require.True(t, target.ProxyPoolSelected)
	require.Equal(t, *source.ProxyID, *target.ProxyID)
}

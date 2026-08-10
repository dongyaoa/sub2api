//go:build unit

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAssignMonitorGroupRatesMatchesCurrentPlatformAndName(t *testing.T) {
	monitors := []*service.ChannelMonitor{
		{Provider: "openai", GroupName: "Standard"},
		{Provider: "anthropic", GroupName: "Standard"},
		{Provider: "openai", GroupName: "Missing"},
		{Provider: "openai", GroupName: ""},
	}
	groups := []*dbent.Group{
		{Name: "Standard", Platform: "openai", RateMultiplier: 0.35},
		{Name: "Standard", Platform: "anthropic", RateMultiplier: 1.2},
	}

	assignMonitorGroupRates(monitors, groups)

	require.NotNil(t, monitors[0].GroupRateMultiplier)
	require.InDelta(t, 0.35, *monitors[0].GroupRateMultiplier, 1e-12)
	require.NotNil(t, monitors[1].GroupRateMultiplier)
	require.InDelta(t, 1.2, *monitors[1].GroupRateMultiplier, 1e-12)
	require.Nil(t, monitors[2].GroupRateMultiplier)
	require.Nil(t, monitors[3].GroupRateMultiplier)
}

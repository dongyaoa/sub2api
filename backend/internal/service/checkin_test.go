package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultCheckinConfigIsValid(t *testing.T) {
	require.NoError(t, ValidateCheckinConfig(DefaultCheckinConfig()))
}

func TestValidateCheckinConfigRejectsExcessivePeriodReward(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardMax = 0.07

	require.Error(t, ValidateCheckinConfig(config))
}

func TestCheckinCodeIsStablePerUserAndBusinessDate(t *testing.T) {
	require.Equal(t, "CI-20260816-z", checkinCode(35, "2026-08-16"))
	require.NotEqual(t, checkinCode(35, "2026-08-16"), checkinCode(35, "2026-08-17"))
}

func TestSecureCheckinRewardUsesWholeCentsWithinRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		reward, err := secureCheckinReward(0.01, 0.05)
		require.NoError(t, err)
		require.GreaterOrEqual(t, reward, 0.01)
		require.LessOrEqual(t, reward, 0.05)
		require.InDelta(t, reward*100, float64(int64(reward*100)), 0.0000001)
	}
}

func TestCheckinBusinessDateUsesChinaTimezone(t *testing.T) {
	now := time.Date(2026, 8, 16, 16, 30, 0, 0, time.UTC)
	require.Equal(t, "2026-08-17", checkinBusinessDate(now))
}

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type checkinSettingRepoStub struct {
	value string
}

func (r *checkinSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *checkinSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return r.value, nil
}

func (r *checkinSettingRepoStub) Set(_ context.Context, _ string, value string) error {
	r.value = value
	return nil
}

func (r *checkinSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}

func (r *checkinSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (r *checkinSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, nil
}

func (r *checkinSettingRepoStub) Delete(context.Context, string) error {
	return nil
}

func TestDefaultCheckinConfigIsValid(t *testing.T) {
	require.NoError(t, ValidateCheckinConfig(DefaultCheckinConfig()))
}

func TestValidateCheckinConfigRejectsExcessivePeriodReward(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardMax = 0.07

	require.Error(t, ValidateCheckinConfig(config))
}

func TestValidateCheckinConfigAllowsConfiguredPromotionLimit(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardMin = 1
	config.RewardMax = 2
	config.MaxPeriodReward = 60

	require.NoError(t, ValidateCheckinConfig(config))
}

func TestGetCheckinConfigMigratesLegacySafetyLimit(t *testing.T) {
	repo := &checkinSettingRepoStub{
		value: `{"enabled":true,"min_recharge_amount":25,"qualification_days":30,"reward_min":0.01,"reward_max":0.05,"min_account_age_hours":24,"timezone":"Asia/Shanghai","version":2}`,
	}
	svc := NewCheckinService(nil, repo, nil, nil)

	config, err := svc.GetConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, 5.0, config.MaxPeriodReward)
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

package service

import (
	"context"
	"encoding/json"
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

func TestValidateCheckinConfigAcceptsRewardTiers(t *testing.T) {
	config := DefaultCheckinConfig()
	config.MaxPeriodReward = 30
	config.RewardTiers = []CheckinRewardTier{
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.10, RewardMax: 0.20, Enabled: true, Visible: true},
		{ID: "gold", Name: "黄金档", MinRechargeAmount: 100, RewardMin: 0.30, RewardMax: 0.50, Enabled: true, Visible: false},
	}

	require.NoError(t, ValidateCheckinConfig(config))
}

func TestValidateCheckinConfigRejectsDuplicateTierThreshold(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.01, RewardMax: 0.05, Visible: true},
		{ID: "gold", Name: "黄金档", MinRechargeAmount: 50, RewardMin: 0.01, RewardMax: 0.05, Visible: true},
	}

	require.Error(t, ValidateCheckinConfig(config))
}

func TestValidateCheckinConfigRejectsTierAtOrBelowDefault(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "invalid", Name: "无效档", MinRechargeAmount: config.MinRechargeAmount, RewardMin: 0.01, RewardMax: 0.05, Visible: true},
	}

	require.Error(t, ValidateCheckinConfig(config))
}

func TestValidateCheckinConfigAppliesSafetyLimitToEveryTier(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "promotion", Name: "活动档", MinRechargeAmount: 100, RewardMin: 0.01, RewardMax: 1, Enabled: true, Visible: true},
	}

	require.Error(t, ValidateCheckinConfig(config))
}

func TestSelectCheckinRewardTierUsesHighestReachedThreshold(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "gold", Name: "黄金档", MinRechargeAmount: 100, RewardMin: 0.30, RewardMax: 0.50, Enabled: true, Visible: false},
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.10, RewardMax: 0.20, Enabled: true, Visible: true},
	}
	config = normalizeCheckinConfig(config)

	_, reached := selectCheckinRewardTier(config, 9.99)
	require.False(t, reached)

	defaultTier, reached := selectCheckinRewardTier(config, 10)
	require.True(t, reached)
	require.True(t, defaultTier.IsDefault)

	silver, reached := selectCheckinRewardTier(config, 99.99)
	require.True(t, reached)
	require.Equal(t, "silver", silver.ID)

	gold, reached := selectCheckinRewardTier(config, 100)
	require.True(t, reached)
	require.Equal(t, "gold", gold.ID)
	require.False(t, gold.Visible, "visibility only controls user-facing display")
}

func TestVisibleCheckinRewardTiersCanRequireQualification(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.10, RewardMax: 0.20, Enabled: true, Visible: true},
		{ID: "gold", Name: "黄金档", MinRechargeAmount: 100, RewardMin: 0.30, RewardMax: 0.50, Enabled: true, Visible: true, QualifiedOnlyVisible: true},
	}
	config = normalizeCheckinConfig(config)

	belowGold := visibleCheckinRewardTiers(config, 80)
	require.Equal(t, []string{"default", "silver"}, []string{belowGold[0].ID, belowGold[1].ID})

	atGold := visibleCheckinRewardTiers(config, 100)
	require.Equal(t, []string{"default", "silver", "gold"}, []string{atGold[0].ID, atGold[1].ID, atGold[2].ID})
}

func TestNextCheckinRewardTierUsesImmediateEnabledStage(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.10, RewardMax: 0.20, Enabled: true, ShowNextProgress: true},
		{ID: "disabled", Name: "停用档", MinRechargeAmount: 75, RewardMin: 0.20, RewardMax: 0.30, Enabled: false, ShowNextProgress: true},
		{ID: "gold", Name: "黄金档", MinRechargeAmount: 100, RewardMin: 0.30, RewardMax: 0.50, Enabled: true, ShowNextProgress: true},
	}
	config = normalizeCheckinConfig(config)

	next, ok := nextCheckinRewardTier(config, 10)
	require.True(t, ok)
	require.Equal(t, "silver", next.ID)

	next, ok = nextCheckinRewardTier(config, 50)
	require.True(t, ok)
	require.Equal(t, "gold", next.ID)

	_, ok = nextCheckinRewardTier(config, 100)
	require.False(t, ok)
}

func TestNextCheckinRewardTierHonorsTargetProgressSwitch(t *testing.T) {
	config := DefaultCheckinConfig()
	config.RewardTiers = []CheckinRewardTier{
		{ID: "silver", Name: "白银档", MinRechargeAmount: 50, RewardMin: 0.10, RewardMax: 0.20, Enabled: true, ShowNextProgress: false},
	}
	config = normalizeCheckinConfig(config)

	_, ok := nextCheckinRewardTier(config, 10)
	require.False(t, ok)
}

func TestParseCheckinRecordMetadataKeepsTierSnapshot(t *testing.T) {
	notesBytes, err := json.Marshal(checkinRecordMetadata{
		Kind:          "daily_checkin",
		BusinessDate:  "2026-08-25",
		ConfigVersion: 3,
		TierID:        "gold",
		TierName:      "黄金档",
		TierIsDefault: false,
	})
	require.NoError(t, err)
	notes := string(notesBytes)

	metadata := parseCheckinRecordMetadata(&notes)

	require.Equal(t, "gold", metadata.TierID)
	require.Equal(t, "黄金档", metadata.TierName)
	require.False(t, metadata.TierIsDefault)
}

func TestParseCheckinRecordMetadataTreatsLegacyRecordAsDefaultTier(t *testing.T) {
	notes := `{"kind":"daily_checkin","business_date":"2026-08-25","config_version":1}`

	metadata := parseCheckinRecordMetadata(&notes)

	require.Equal(t, "default", metadata.TierID)
	require.True(t, metadata.TierIsDefault)
}

func TestGetCheckinConfigMigratesLegacySafetyLimit(t *testing.T) {
	repo := &checkinSettingRepoStub{
		value: `{"enabled":true,"min_recharge_amount":25,"qualification_days":30,"reward_min":0.01,"reward_max":0.05,"min_account_age_hours":24,"timezone":"Asia/Shanghai","version":2}`,
	}
	svc := NewCheckinService(nil, repo, nil, nil)

	config, err := svc.GetConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, 5.0, config.MaxPeriodReward)
	require.Equal(t, "默认档位", config.DefaultTierName)
	require.True(t, config.DefaultTierVisible)
	require.Empty(t, config.RewardTiers)
}

func TestGetCheckinConfigMigratesLegacyTierExperienceFields(t *testing.T) {
	repo := &checkinSettingRepoStub{
		value: `{"enabled":true,"min_recharge_amount":10,"qualification_days":30,"reward_min":0.01,"reward_max":0.05,"max_period_reward":30,"min_account_age_hours":24,"timezone":"Asia/Shanghai","version":2,"reward_tiers":[{"id":"gold","name":"黄金档","min_recharge_amount":100,"reward_min":0.3,"reward_max":0.5,"visible":true,"exclusive_ui_enabled":true}]}`,
	}
	svc := NewCheckinService(nil, repo, nil, nil)

	config, err := svc.GetConfig(context.Background())

	require.NoError(t, err)
	require.Len(t, config.RewardTiers, 1)
	require.True(t, config.RewardTiers[0].Enabled)
	require.True(t, config.RewardTiers[0].CustomButtonEnabled)
	require.Equal(t, "emerald", config.RewardTiers[0].ButtonColor)
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

//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type rechargePromotionFeatureRepoStub struct {
	SettingRepository
	value string
	err   error
}

func (s *rechargePromotionFeatureRepoStub) GetValue(context.Context, string) (string, error) {
	return s.value, s.err
}

func TestSettingService_RechargePromotionEnabledFailsClosed(t *testing.T) {
	ctx := context.Background()
	var missing *SettingService
	require.False(t, missing.IsRechargePromotionEnabled(ctx))
	require.False(t, (&SettingService{}).IsRechargePromotionEnabled(ctx))
	for _, tc := range []struct {
		name  string
		value string
		err   error
		want  bool
	}{
		{name: "missing", err: ErrSettingNotFound},
		{name: "read error", value: "true", err: errors.New("database unavailable")},
		{name: "empty"},
		{name: "disabled", value: "false"},
		{name: "invalid", value: "1"},
		{name: "enabled", value: "true", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingService(&rechargePromotionFeatureRepoStub{value: tc.value, err: tc.err}, &config.Config{})
			require.Equal(t, tc.want, svc.IsRechargePromotionEnabled(ctx))
		})
	}
}

func TestSettingService_RechargePromotionPublicSettingsAndInjection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		values := map[string]string{}
		if enabled {
			values[SettingKeyRechargePromotionEnabled] = "true"
		}
		svc := NewSettingService(&settingPublicRepoStub{values: values}, &config.Config{})
		settings, err := svc.GetPublicSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, enabled, settings.RechargePromotionEnabled)
		injection, err := svc.GetPublicSettingsForInjection(context.Background())
		require.NoError(t, err)
		require.Equal(t, enabled, injection.(*PublicSettingsInjectionPayload).RechargePromotionEnabled)
	}
}

func TestSettingService_RechargePromotionSettingDefaultsAndPersistence(t *testing.T) {
	repo := &forwardedIPMigrationRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))
	require.Equal(t, "false", repo.values[SettingKeyRechargePromotionEnabled])
	require.False(t, svc.parseSettings(map[string]string{}).RechargePromotionEnabled)
	require.True(t, svc.parseSettings(map[string]string{SettingKeyRechargePromotionEnabled: "true"}).RechargePromotionEnabled)

	updateRepo := &settingUpdateRepoStub{}
	updateSvc := NewSettingService(updateRepo, &config.Config{})
	require.NoError(t, updateSvc.UpdateSettings(context.Background(), &SystemSettings{RechargePromotionEnabled: true}))
	require.Equal(t, "true", updateRepo.updates[SettingKeyRechargePromotionEnabled])
	require.NoError(t, updateSvc.UpdateSettings(context.Background(), &SystemSettings{RechargePromotionEnabled: false}))
	require.Equal(t, "false", updateRepo.updates[SettingKeyRechargePromotionEnabled])
}

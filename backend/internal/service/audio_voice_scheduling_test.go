package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAudioVoiceSelectsOnlyBoundAccountAndAllowsSameModelAlias(t *testing.T) {
	svc, key, account, _, binding := audioVoiceTestFixture(t)
	ctx := WithOpenAIProfitControlSuppressed(context.Background())
	for _, model := range []string{audioVoiceTestModel, "speech-alias"} {
		selection, err := svc.SelectAudioVoiceAccount(ctx, key, binding, model)
		require.NoError(t, err)
		require.Equal(t, account.ID, selection.Account.ID)
		require.True(t, selection.Acquired)
		selection.ReleaseFunc()
	}
	_, err := svc.SelectAudioVoiceAccount(ctx, key, binding, "another-real-model")
	require.ErrorIs(t, err, ErrAudioVoiceInvalid)
	_, err = svc.SelectAudioVoiceAccount(ctx, key, &AudioVoiceBinding{AccountID: account.ID}, audioVoiceTestModel)
	require.ErrorIs(t, err, ErrAudioVoiceInvalid, "client-constructed bindings cannot skip token validation")
}

func TestAudioVoiceBindingRejectsChangedCredentialsOrAccountEligibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Account)
		want   error
	}{
		{"key-rotation", func(a *Account) { a.Credentials["api_key"] = "rotated-key" }, ErrAudioVoiceInvalid},
		{"base-url-change", func(a *Account) { a.Credentials["base_url"] = "https://different.example" }, ErrAudioVoiceInvalid},
		{"deleted-account", func(a *Account) { a.ID++ }, ErrAudioVoiceUnavailable},
		{"group-removed", func(a *Account) { a.GroupIDs = nil }, ErrAudioVoiceUnavailable},
		{"disabled", func(a *Account) { a.Status = "disabled" }, ErrAudioVoiceUnavailable},
		{"unschedulable", func(a *Account) { a.Schedulable = false }, ErrAudioVoiceUnavailable},
		{"cooldown", func(a *Account) { until := time.Now().Add(time.Hour); a.TempUnschedulableUntil = &until }, ErrAudioVoiceUnavailable},
		{"rate-limited", func(a *Account) { until := time.Now().Add(time.Hour); a.RateLimitResetAt = &until }, ErrAudioVoiceUnavailable},
		{"capability-removed", func(a *Account) {
			a.Credentials[openAIEndpointCapabilitiesCredentialKey] = map[string]any{"audio_speech": false}
		}, ErrAudioVoiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, key, account, _, binding := audioVoiceTestFixture(t)
			tc.change(account)
			svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}
			selection, err := svc.SelectAudioVoiceAccount(WithOpenAIProfitControlSuppressed(context.Background()), key, binding, audioVoiceTestModel)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, selection)
		})
	}
}

func TestAudioVoiceAccountWaitPlanAndPostWaitCredentialRecheck(t *testing.T) {
	svc, key, account, _, binding := audioVoiceTestFixture(t)
	var acquiredIDs []int64
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{
		acquireResults: map[int64]bool{account.ID: false}, acquiredIDs: &acquiredIDs,
	})
	ctx := WithOpenAIProfitControlSuppressed(context.Background())
	selection, err := svc.SelectAudioVoiceAccount(ctx, key, binding, audioVoiceTestModel)
	require.NoError(t, err)
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, account.ID, selection.WaitPlan.AccountID)
	require.Equal(t, account.Concurrency, selection.WaitPlan.MaxConcurrency)
	require.Greater(t, selection.WaitPlan.Timeout, time.Duration(0))
	require.Greater(t, selection.WaitPlan.MaxWaiting, 0)
	require.Equal(t, []int64{account.ID}, acquiredIDs, "never try a different account for a bound voice")
	account.Credentials["api_key"] = "rotated-during-wait"
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}
	_, err = svc.RecheckAudioVoiceAccount(ctx, key, binding, audioVoiceTestModel)
	require.ErrorIs(t, err, ErrAudioVoiceInvalid)
}

func TestAudioVoiceRespectsPublicAllowlistAndCompositeChannelRestrictions(t *testing.T) {
	const public, routed, forwarded = "public-voice-model", "routed-voice-model", "speech-alias"
	for _, source := range []string{BillingModelSourceRequested, BillingModelSourceChannelMapped, BillingModelSourceUpstream} {
		t.Run(source, func(t *testing.T) {
			svc, key, _, _, binding := audioVoiceTestFixture(t)
			key.Group.Platform = PlatformComposite
			key.Group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{public}}
			pricedModel := public
			if source == BillingModelSourceChannelMapped {
				pricedModel = forwarded
			}
			if source == BillingModelSourceUpstream {
				pricedModel = audioVoiceTestModel
			}
			resolver := newOpenAIImageChannelPricingResolverForTest(t, *key.GroupID, pricedModel, 0.2)
			cache, ok := resolver.channelService.cache.Load().(*channelCache)
			require.True(t, ok)
			cache.channelByGroupID[*key.GroupID].BillingModelSource = source
			cache.channelByGroupID[*key.GroupID].RestrictModels = true
			cache.mappingByGroupModel[channelModelKey{groupID: *key.GroupID, model: routed}] = forwarded
			svc.channelService = resolver.channelService
			ctx := WithCompositeRouteDecision(WithOpenAIProfitControlSuppressed(context.Background()), CompositeRouteDecision{
				Matched: true, PublicModel: public, UpstreamModel: routed, TargetPlatform: PlatformGemini,
			})
			ctx = WithAudioVoiceBinding(ctx, binding, public, routed)
			selection, err := svc.SelectAudioVoiceAccount(ctx, key, binding, forwarded)
			require.NoError(t, err)
			selection.ReleaseFunc()
			delete(cache.pricingByGroupModel, channelModelKey{groupID: *key.GroupID, model: pricedModel})
			_, err = svc.SelectAudioVoiceAccount(ctx, key, binding, forwarded)
			require.ErrorIs(t, err, ErrAudioVoiceUnavailable)
		})
	}
}

func TestAudioVoiceSelectFailsClosedWithoutRepository(t *testing.T) {
	svc, key, _, _, binding := audioVoiceTestFixture(t)
	svc.accountRepo = nil
	_, err := svc.SelectAudioVoiceAccount(context.Background(), key, binding, audioVoiceTestModel)
	require.ErrorIs(t, err, ErrAudioVoiceServiceUnavailable)
}

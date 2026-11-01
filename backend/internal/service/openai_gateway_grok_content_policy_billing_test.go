package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayRecordUsage_GrokContentPolicyFixedFee(t *testing.T) {
	for _, tc := range []struct {
		name         string
		model        string
		groupRate    float64
		userRate     float64
		subscription bool
	}{
		{name: "text with zero group multiplier", model: "unpriced-grok-alias", groupRate: 0, userRate: 0},
		{name: "image with custom multiplier", model: "grok-imagine-image", groupRate: 5, userRate: 7},
		{name: "video subscription with peak pricing", model: "grok-imagine-video", groupRate: 5, userRate: 7, subscription: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			rateRepo := &openAIUserGroupRateRepoStub{rate: &tc.userRate}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
				&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, rateRepo)
			// A violation fee must not depend on model pricing being available.
			svc.billingService = nil
			group := &Group{
				ID: 4, Platform: PlatformGrok, RateMultiplier: tc.groupRate,
				ImageRateIndependent: true, ImageRateMultiplier: 8,
				VideoRateIndependent: true, VideoRateMultiplier: 9,
				PeakRateEnabled: true, PeakStart: "00:00", PeakEnd: "23:59", PeakRateMultiplier: 10,
			}
			var subscription *UserSubscription
			if tc.subscription {
				group.SubscriptionType = SubscriptionTypeSubscription
				subscription = &UserSubscription{ID: 5}
			}
			result := &OpenAIForwardResult{
				RequestID: "grok-policy-request", Model: tc.model,
				GrokContentPolicyRejection: true,
				Usage: OpenAIUsage{
					InputTokens: 900, OutputTokens: 500,
					CacheReadInputTokens: 100, CacheCreationInputTokens: 100,
					ImageInputTokens: 200, ImageCacheReadTokens: 50, ImageOutputTokens: 300,
				},
				ImageCount: 2, ImageEndpoint: true, ImageOutputSizes: []string{"1024x1024"},
				ImageSizeBreakdown: map[string]int{"1024x1024": 2},
				VideoCount:         1, VideoDurationSeconds: 15, VideoResolution: "1080p",
				SearchCount: 5, WebSearchCalls: 2, AudioUsage: &AudioUsage{},
				UpstreamResponseModel: "gpt-5.1",
			}
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: result,
				APIKey: &APIKey{ID: 2, GroupID: &group.ID, Group: group, Quota: 100, RateLimit5h: 100},
				User:   &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformGrok},
				Subscription: subscription, APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{},
				PricingAt:          time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
				ChannelUsageFields: ChannelUsageFields{BillingModelSource: BillingModelSourceResponse},
			})
			require.NoError(t, err)
			require.Equal(t, 1, billingRepo.calls)
			log := usageRepo.lastLog
			require.NotNil(t, log)
			require.Equal(t, 0.1, log.TotalCost)
			require.Equal(t, 0.1, log.ActualCost)
			require.Equal(t, 1.0, log.RateMultiplier)
			require.Equal(t, string(BillingModePerRequest), *log.BillingMode)
			require.Zero(t, log.TotalTokens())
			require.Zero(t, log.ImageInputTokens)
			require.Zero(t, log.ImageOutputTokens)
			require.Zero(t, log.ImageCount)
			require.Zero(t, log.VideoCount)
			require.Nil(t, log.ImageSizeBreakdown)
			require.Nil(t, log.VideoDurationSeconds)
			require.Zero(t, log.InputCost+log.OutputCost+log.CacheReadCost+log.CacheCreationCost)
			require.False(t, log.LongContextBillingApplied)
			require.Equal(t, 0.1, billingRepo.lastCmd.APIKeyQuotaCost)
			require.Equal(t, 0.1, billingRepo.lastCmd.APIKeyRateLimitCost)
			if tc.subscription {
				require.Zero(t, billingRepo.lastCmd.BalanceCost)
				require.Equal(t, 0.1, billingRepo.lastCmd.SubscriptionCost)
			} else {
				require.Equal(t, 0.1, billingRepo.lastCmd.BalanceCost)
				require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
			}
			require.Equal(t, 900, result.Usage.InputTokens, "billing must preserve the upstream result")
			require.Equal(t, 2, result.ImageCount)
			require.False(t, result.SucceededForScheduling())
		})
	}
}

func TestOpenAIGatewayRecordUsage_GrokContentPolicyDoesNotChangeOtherUsage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		rejected bool
	}{
		{name: "normal Grok completion", platform: PlatformGrok},
		{name: "non Grok refusal", platform: PlatformOpenAI, rejected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
				&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 200}
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: tc.name, Model: "gpt-5.1", Usage: usage,
					GrokContentPolicyRejection: tc.rejected,
				},
				APIKey: &APIKey{ID: 2}, User: &User{ID: 1},
				Account: &Account{ID: 3, Platform: tc.platform},
			})
			require.NoError(t, err)
			expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, 1.1)
			require.InDelta(t, expected.ActualCost, usageRepo.lastLog.ActualCost, 1e-12)
			require.Equal(t, 1000, usageRepo.lastLog.InputTokens)
			require.Equal(t, 200, usageRepo.lastLog.OutputTokens)
			require.Equal(t, string(BillingModeToken), *usageRepo.lastLog.BillingMode)
		})
	}
}

func TestOpenAIGatewayRecordUsage_GrokContentPolicyPreservesBillingDeduplication(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
		userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "same-policy-request")
	input := &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "upstream-first", Model: "grok-unknown", GrokContentPolicyRejection: true,
		},
		APIKey: &APIKey{ID: 2}, User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformGrok},
		RequestPayloadHash: "same-payload",
	}
	require.NoError(t, svc.RecordUsage(ctx, input))
	firstCommand := *billingRepo.lastCmd
	// A repeated submission uses the same persistent key and fingerprint even
	// if an upstream retry returned a different request ID or token count.
	billingRepo.result = &UsageBillingApplyResult{Applied: false}
	input.Result.RequestID = "upstream-second"
	input.Result.Usage = OpenAIUsage{InputTokens: 300, OutputTokens: 100}
	require.NoError(t, svc.RecordUsage(ctx, input))
	require.Equal(t, "client:same-policy-request", billingRepo.lastCmd.RequestID)
	require.Equal(t, firstCommand.RequestID, billingRepo.lastCmd.RequestID)
	require.Equal(t, firstCommand.RequestFingerprint, billingRepo.lastCmd.RequestFingerprint)
	require.Equal(t, 0.1, billingRepo.lastCmd.BalanceCost)
	require.Zero(t, userRepo.deductCalls, "persistent billing must not fall back to a second direct charge")
}

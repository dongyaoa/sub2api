package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGrokVideoStudioPerSecondPricesSurviveTokenCards(t *testing.T) {
	ctx, groupID := context.Background(), int64(920)
	for _, model := range []string{"grok-imagine-video", "grok-imagine-video-1.5", "svggrok-imagine-video-1.5"} {
		for _, source := range []string{PricingSourceGroup, PricingSourceChannel} {
			t.Run(model+"/"+source, func(t *testing.T) {
				resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
				cfg := &config.Config{}
				cfg.Billing.InflightReservation.Enabled = true
				gateway := &OpenAIGatewayService{cfg: cfg, resolver: resolver, billingService: resolver.billingService}
				prices := map[string]float64{"480p": 0.06, "720p": 0.08, "1080p": 0.1}
				key := &APIKey{User: &User{ID: 12}, GroupID: &groupID, Group: &Group{
					ID: groupID, Platform: PlatformGrok, RateMultiplier: 0.9,
					VideoRateIndependent: true, VideoRateMultiplier: 0.5,
					VideoModelPrices: map[string]map[string]float64{CanonicalGrokImagineVideoPriceFamily(model): prices},
				}}
				if source == PricingSourceGroup {
					tokenPrice := 1.0
					key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"*"}, BillingMode: BillingModeToken, InputPrice: &tokenPrice, OutputPrice: &tokenPrice}}
				}
				quote, err := gateway.VideoStudioPricing(ctx, key, model)
				require.NoError(t, err)
				require.Equal(t, BillingModeVideo, quote.BillingMode)
				require.Equal(t, PricingSourceGroup, quote.PricingSource)
				require.Len(t, quote.Tiers, 3)
				for resolution, price := range prices {
					tier := quote.Tiers[resolution]
					require.True(t, tier.Configured)
					require.InDelta(t, price, tier.BasePrice, 1e-12)
					require.InDelta(t, price*0.5, tier.UnitPrice, 1e-12)
					for _, duration := range []int{4, 15} {
						cost, err := gateway.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{
							Model: model, VideoCount: 1, VideoResolution: resolution, VideoDurationSeconds: duration,
						}, key, []string{model}, 0.9, 0.9, 0.5, 0.9,
							UsageTokens{InputTokens: 1000, OutputTokens: 2000}, "", nil, false, time.Time{})
						require.NoError(t, err)
						require.Equal(t, string(BillingModeVideo), cost.BillingMode)
						require.InDelta(t, tier.UnitPrice*float64(duration), cost.ActualCost, 1e-12)
						estimate, priced := gateway.EstimateInflightReservation(ctx, key, InflightEstimateRequest{
							Model: model, Kind: InflightEstimateVideo, Units: 1, VideoResolution: resolution, VideoDurationSeconds: duration,
						})
						require.True(t, priced)
						require.InDelta(t, cost.ActualCost, estimate, 1e-12)
					}
				}
			})
		}
	}
}

func TestGrokVideoStudioDefaultsRemainAvailableWithTokenCards(t *testing.T) {
	ctx, groupID := context.Background(), int64(921)
	for _, tc := range []struct {
		model  string
		prices map[string]float64
	}{
		{"grok-imagine-video", map[string]float64{"480p": 0.05, "720p": 0.07, "1080p": 0.07}},
		{"grok-imagine-video-1.5", map[string]float64{"480p": 0.08, "720p": 0.14, "1080p": 0.25}},
		{"svggrok-imagine-video-1.5", map[string]float64{"480p": 0.08, "720p": 0.14, "1080p": 0.25}},
	} {
		resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, tc.model)
		gateway := &OpenAIGatewayService{resolver: resolver, billingService: resolver.billingService}
		key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1}}
		quote, err := gateway.VideoStudioPricing(ctx, key, tc.model)
		require.NoError(t, err)
		require.Equal(t, BillingModeVideo, quote.BillingMode)
		require.Equal(t, "default", quote.PricingSource)
		for resolution, price := range tc.prices {
			require.InDelta(t, price, quote.Tiers[resolution].UnitPrice, 1e-12, tc.model+"/"+resolution)
			require.False(t, quote.Tiers[resolution].Configured)
		}
	}
}

func TestGrokVideoStudioFlatLegacyPriceMatchesActualCharge(t *testing.T) {
	ctx, groupID, standard, video15 := context.Background(), int64(922), 0.061, 0.181
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video")
	gateway := &OpenAIGatewayService{resolver: resolver, billingService: resolver.billingService}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGrok, RateMultiplier: 1,
		VideoPrice480P: &standard, VideoPrice720P: &video15}}
	for model, price := range map[string]float64{"grok-imagine-video": standard, "grok-imagine-video-1.5": video15, "svggrok-imagine-video-1.5": video15} {
		quote, err := gateway.VideoStudioPricing(ctx, key, model)
		require.NoError(t, err)
		for resolution, tier := range quote.Tiers {
			require.InDelta(t, price, tier.UnitPrice, 1e-12)
			cost, err := gateway.calculateOpenAIVideoCost(ctx, model, key, &OpenAIForwardResult{
				Model: model, VideoCount: 1, VideoResolution: resolution, VideoDurationSeconds: 4,
			}, 1, 1)
			require.NoError(t, err)
			require.InDelta(t, tier.UnitPrice*4, cost.ActualCost, 1e-12)
		}
	}
}

func TestVideoStudioUnrelatedTokenModelRemainsTokenPriced(t *testing.T) {
	groupID := int64(923)
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, "custom-token-model")
	gateway := &OpenAIGatewayService{resolver: resolver, billingService: resolver.billingService}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1}}
	quote, err := gateway.VideoStudioPricing(context.Background(), key, "custom-token-model")
	require.NoError(t, err)
	require.Equal(t, BillingModeToken, quote.BillingMode)
	require.Empty(t, quote.Tiers)
}

func TestVideoStudioCustomProviderDoesNotInheritMappedGrokDefault(t *testing.T) {
	ctx, groupID := context.Background(), int64(924)
	billing := NewBillingService(&config.Config{}, nil)
	gateway := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1}}
	for _, model := range []string{"gemini-omni-1.2-flash", "muse-video"} {
		result := &OpenAIForwardResult{Model: model, UpstreamModel: "grok-imagine-video-1.5", VideoCount: 1,
			VideoResolution: "4k", VideoDurationSeconds: 15}
		require.Equal(t, model, videoUsageModel(result, "grok-imagine-video-1.5"))
		_, err := gateway.calculateOpenAIVideoCost(ctx, "grok-imagine-video-1.5", key, result, 1, 1)
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured, "custom provider must not inherit a Grok fallback rate")
	}
}

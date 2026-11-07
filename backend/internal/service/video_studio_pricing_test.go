package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestVideoStudioPricingMuseMatchesCompletedVideoCharge(t *testing.T) {
	for _, source := range []string{PricingSourceGroup, PricingSourceChannel} {
		for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
			t.Run(source+"/"+string(mode), func(t *testing.T) {
				ctx, groupID, price, legacyPrice := context.Background(), int64(81), 0.2, 9.0
				resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "muse-video", price)
				cache := resolver.channelService.cache.Load().(*channelCache)
				cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "muse-video"}].BillingMode = mode
				key := &APIKey{UserID: 8, GroupID: &groupID, Group: &Group{
					ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 0.5,
					VideoPrice480P: &legacyPrice, VideoRateIndependent: true, VideoRateMultiplier: 0.25,
				}}
				if source == PricingSourceGroup {
					key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: mode, PerRequestPrice: &price}}
				}
				svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
				quote, err := svc.VideoStudioPricing(ctx, key, "muse-video")
				require.NoError(t, err)
				require.Equal(t, source, quote.PricingSource)
				require.Equal(t, mode, quote.BillingMode)
				require.Len(t, quote.Tiers, 1)
				require.InDelta(t, price, quote.Tiers["auto"].BasePrice, 1e-12)
				wantMultiplier := 0.25
				if mode == BillingModePerRequest {
					wantMultiplier = 0.5 // Matches the model square's user/group multiplier.
				}
				require.InDelta(t, wantMultiplier, quote.RateMultiplier, 1e-12)
				require.InDelta(t, price*wantMultiplier, quote.Tiers["auto"].UnitPrice, 1e-12)
				for _, duration := range []int{1, 15, 60} {
					cost, err := svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{
						Model: "muse-video", VideoCount: 1, VideoDurationSeconds: duration, VideoResolution: "auto",
					}, key, []string{"muse-video"}, 0.5, 0.5, 0.25, 0.5, UsageTokens{}, "", nil, false, time.Time{})
					require.NoError(t, err)
					units := 1
					if mode == BillingModeVideo {
						units = duration
					}
					require.InDelta(t, quote.Tiers["auto"].UnitPrice*float64(units), cost.ActualCost, 1e-12)
					require.Equal(t, string(mode), cost.BillingMode)
				}
			})
		}
	}
}

func TestVideoStudioPricingMuseRequiresExplicitModelPrice(t *testing.T) {
	groupID, legacy := int64(82), 0.1
	billing := NewBillingService(&config.Config{}, nil)
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1, VideoPrice480P: &legacy}}
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo, BillingModeToken, BillingModeImage} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: mode}}
		quote, err := svc.VideoStudioPricing(context.Background(), key, "muse-video")
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
		require.Nil(t, quote)
		cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
			Model: "muse-video", VideoCount: 1, VideoDurationSeconds: 60,
		}, key, []string{"muse-video"}, 1, 1, 1, 1, UsageTokens{}, "", nil, false, time.Time{})
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
		require.Nil(t, cost)
	}
}

func TestVideoStudioPricingMuseExplicitZeroAndAutomaticTier(t *testing.T) {
	ctx, groupID, price, free := context.Background(), int64(83), 0.3, 0.0
	billing := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1}}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: mode, PerRequestPrice: &price,
			Intervals: []PricingInterval{{TierLabel: "480p", PerRequestPrice: &price}, {TierLabel: "AUTO", PerRequestPrice: &free}},
		}}
		quote, err := svc.VideoStudioPricing(ctx, key, "muse-video")
		require.NoError(t, err)
		require.Zero(t, quote.Tiers["auto"].UnitPrice)
		key.Group.ModelPricing[0].PerRequestPrice = nil
		quote, err = svc.VideoStudioPricing(ctx, key, "muse-video")
		require.NoError(t, err)
		require.Zero(t, quote.Tiers["auto"].UnitPrice)
		key.Group.ModelPricing[0].Intervals = key.Group.ModelPricing[0].Intervals[:1]
		_, err = svc.VideoStudioPricing(ctx, key, "muse-video")
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured, "unrelated resolution tier cannot price Muse automatic output")
	}
}

func TestVideoStudioPricingChannelPerRequestOverridesLegacySecondPrice(t *testing.T) {
	ctx, groupID, perRequest, legacy := context.Background(), int64(84), 0.3, 0.5
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video", perRequest)
	cache := resolver.channelService.cache.Load().(*channelCache)
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "grok-imagine-video"}].BillingMode = BillingModePerRequest
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1, VideoPrice480P: &legacy}}
	quote, err := svc.VideoStudioPricing(ctx, key, "grok-imagine-video")
	require.NoError(t, err)
	require.Equal(t, BillingModePerRequest, quote.BillingMode)
	require.InDelta(t, perRequest, quote.Tiers["480p"].UnitPrice, 1e-12)
	cost, err := svc.calculateOpenAIVideoCost(ctx, "grok-imagine-video", key, &OpenAIForwardResult{VideoCount: 1, VideoDurationSeconds: 15}, 1, 1)
	require.NoError(t, err)
	require.InDelta(t, perRequest, cost.ActualCost, 1e-12)
}

func TestVideoStudioPricingLegacyChannelImageUsesPerGenerationUnits(t *testing.T) {
	ctx, groupID, price := context.Background(), int64(87), 0.3
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video", price)
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	key := &APIKey{GroupID: &groupID, Group: &Group{
		ID: groupID, RateMultiplier: 0.5, VideoRateIndependent: true, VideoRateMultiplier: 0.25,
	}}
	quote, err := svc.VideoStudioPricing(ctx, key, "grok-imagine-video")
	require.NoError(t, err)
	require.Equal(t, BillingModeImage, quote.BillingMode)
	require.InDelta(t, 0.25, quote.RateMultiplier, 1e-12)
	cost, err := svc.calculateOpenAIVideoCost(ctx, "grok-imagine-video", key, &OpenAIForwardResult{
		VideoCount: 1, VideoDurationSeconds: 15, VideoResolution: "720p",
	}, 0.25, 0.5)
	require.NoError(t, err)
	require.InDelta(t, price*0.25, cost.ActualCost, 1e-12)
	require.InDelta(t, quote.Tiers["720p"].UnitPrice, cost.ActualCost, 1e-12)
}

func TestVideoStudioPricingVideoTiersPreserveZeroAndDefault(t *testing.T) {
	ctx, groupID, price, price480, free := context.Background(), int64(88), 0.3, 0.1, 0.0
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "grok-imagine-video", price)
	cache := resolver.channelService.cache.Load().(*channelCache)
	pricing := cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: "grok-imagine-video"}]
	pricing.BillingMode = BillingModeVideo
	pricing.Intervals = []PricingInterval{{TierLabel: "480p", PerRequestPrice: &price480}, {TierLabel: "720P", PerRequestPrice: &free}}
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, RateMultiplier: 1}}
	quote, err := svc.VideoStudioPricing(ctx, key, "grok-imagine-video")
	require.NoError(t, err)
	require.InDelta(t, price480, quote.Tiers["480p"].UnitPrice, 1e-12)
	require.Zero(t, quote.Tiers["720p"].UnitPrice)
	require.InDelta(t, price, quote.Tiers["1080p"].UnitPrice, 1e-12)
}

func TestMuseVideoRecordUsagePreservesDurationAndPerRequestMode(t *testing.T) {
	groupID, price := int64(85), 0.4
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	key := &APIKey{ID: 5, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 0.5,
		ModelPricing: []ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}},
	}}
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "muse-task", ResponseID: "muse-task", Model: "muse-video", VideoCount: 1,
			VideoDurationSeconds: 60, VideoResolution: "auto"},
		APIKey: key, User: &User{ID: 6}, Account: &Account{ID: 7, Platform: PlatformOpenAI},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "grok-video:muse-task", usageRepo.lastLog.RequestID, "status and content polls share a durable dedup key")
	require.Equal(t, 60, *usageRepo.lastLog.VideoDurationSeconds)
	require.Equal(t, "auto", *usageRepo.lastLog.VideoResolution)
	require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	require.InDelta(t, price*0.5, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestVideoBillingDurationUsesProviderLimits(t *testing.T) {
	require.Equal(t, 60, NormalizeVideoBillingDurationForModelOrDefault("muse-video", 60))
	require.Equal(t, 60, NormalizeVideoBillingDurationForModelOrDefault("muse-video", 90))
	require.Equal(t, 5, NormalizeVideoBillingDurationForModelOrDefault("muse-video", 0))
	require.Equal(t, 15, NormalizeVideoBillingDurationForModelOrDefault("grok-imagine-video", 60))
	require.Equal(t, 8, NormalizeVideoBillingDurationForModelOrDefault("grok-imagine-video", 0))
}

func TestInflightEstimateMuseUsesCompletedVideoPricing(t *testing.T) {
	groupID, price, legacy := int64(86), 0.4, 99.0
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	billing := NewBillingService(cfg, nil)
	svc := &GatewayService{cfg: cfg, billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{User: &User{ID: 9}, GroupID: &groupID, Group: &Group{
		ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 0.5,
		VideoRateIndependent: true, VideoRateMultiplier: 0.25, VideoPrice480P: &legacy,
	}}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: mode, PerRequestPrice: &price}}
		for _, duration := range []int{5, 60} {
			estimate, priced := svc.EstimateInflightReservation(context.Background(), key, InflightEstimateRequest{
				Model: "muse-video", Kind: InflightEstimateVideo, Units: 1, VideoDurationSeconds: duration, VideoResolution: "auto",
			})
			require.True(t, priced)
			want := price * 0.5
			if mode == BillingModeVideo {
				want = price * 0.25 * float64(duration)
			}
			require.InDelta(t, want, estimate, 1e-12, "reservation follows model billing mode and multiplier")
		}
	}
}

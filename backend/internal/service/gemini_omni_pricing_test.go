package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const omniPricingTestModel = "gemini-omni-1.2-flash"

func TestGeminiOmniVideoPricingMatchesCompletedCharge(t *testing.T) {
	for _, source := range []string{PricingSourceGroup, PricingSourceChannel} {
		for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
			t.Run(source+"/"+string(mode), func(t *testing.T) {
				ctx, groupID, price, legacy := context.Background(), int64(91), 0.2, 99.0
				resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, omniPricingTestModel, price)
				cache := resolver.channelService.cache.Load().(*channelCache)
				cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: omniPricingTestModel}].BillingMode = mode
				key := &APIKey{UserID: 8, GroupID: &groupID, Group: &Group{
					ID: groupID, Platform: PlatformGemini, RateMultiplier: 0.5,
					VideoPrice480P: &legacy, VideoRateIndependent: true, VideoRateMultiplier: 0.25,
				}}
				if source == PricingSourceGroup {
					key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: mode, PerRequestPrice: &price}}
				}
				svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
				quote, err := svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
				require.NoError(t, err)
				require.Equal(t, source, quote.PricingSource)
				require.Equal(t, mode, quote.BillingMode)
				require.Len(t, quote.Tiers, 4)
				multiplier := 0.25
				if mode == BillingModePerRequest {
					multiplier = 0.5
				}
				require.InDelta(t, multiplier, quote.RateMultiplier, 1e-12)
				for _, resolution := range []string{"360p", "720p", "1080p", "4k"} {
					require.True(t, quote.Tiers[resolution].Configured)
					require.InDelta(t, price, quote.Tiers[resolution].BasePrice, 1e-12)
					require.InDelta(t, price*multiplier, quote.Tiers[resolution].UnitPrice, 1e-12)
					for _, duration := range []int{3, 5, 15} {
						cost, err := svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{
							Model: omniPricingTestModel, VideoCount: 1, VideoDurationSeconds: duration, VideoResolution: resolution,
						}, key, []string{omniPricingTestModel}, 0.5, 0.5, 0.25, 0.5, UsageTokens{}, "", nil, false, time.Time{})
						require.NoError(t, err)
						units := 1
						if mode == BillingModeVideo {
							units = duration
						}
						require.InDelta(t, quote.Tiers[resolution].UnitPrice*float64(units), cost.ActualCost, 1e-12)
						require.Equal(t, string(mode), cost.BillingMode)
					}
				}
			})
		}
	}
}

func TestGeminiOmniVideoPricingRequiresExplicitPrice(t *testing.T) {
	groupID, legacy := int64(92), 0.1
	billing := NewBillingService(&config.Config{}, nil)
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1, VideoPrice480P: &legacy}}
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo, BillingModeToken, BillingModeImage} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: mode}}
		_, err := svc.VideoStudioPricing(context.Background(), key, omniPricingTestModel)
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
		_, err = svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
			Model: omniPricingTestModel, VideoCount: 1, VideoDurationSeconds: 15, VideoResolution: "4k",
		}, key, []string{omniPricingTestModel}, 1, 1, 1, 1, UsageTokens{}, "", nil, false, time.Time{})
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
	}
}

func TestGeminiOmniVideoPricingPreservesTiersAndFreePrice(t *testing.T) {
	ctx, groupID, normalPrice, highPrice, free := context.Background(), int64(93), 0.3, 0.9, 0.0
	billing := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1}}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: mode, PerRequestPrice: &normalPrice,
			Intervals: []PricingInterval{{TierLabel: "360P", PerRequestPrice: &free}, {TierLabel: "4K", PerRequestPrice: &highPrice}},
		}}
		quote, err := svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
		require.NoError(t, err)
		require.Zero(t, quote.Tiers["360p"].UnitPrice)
		require.InDelta(t, highPrice, quote.Tiers["4k"].UnitPrice, 1e-12)
		require.InDelta(t, normalPrice, quote.Tiers["720p"].UnitPrice, 1e-12)
		cost, err := svc.calculateOpenAIVideoCost(ctx, omniPricingTestModel, key, &OpenAIForwardResult{
			Model: omniPricingTestModel, VideoCount: 1, VideoDurationSeconds: 15, VideoResolution: "4K",
		}, 1, 1)
		require.NoError(t, err)
		units := 1
		if mode == BillingModeVideo {
			units = 15
		}
		require.InDelta(t, highPrice*float64(units), cost.ActualCost, 1e-12)
		key.Group.ModelPricing[0].PerRequestPrice = &free
		key.Group.ModelPricing[0].Intervals = nil
		quote, err = svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
		require.NoError(t, err)
		require.Zero(t, quote.Tiers["4k"].UnitPrice)
	}
}

func TestGeminiOmniVideoPricingMappedBillingNameKeepsProvider(t *testing.T) {
	ctx, groupID, price := context.Background(), int64(94), 0.1
	const billingModel = "private-video-priced-name"
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, billingModel, price)
	cache := resolver.channelService.cache.Load().(*channelCache)
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: billingModel}].BillingMode = BillingModePerRequest
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: omniPricingTestModel}] = billingModel
	cache.channelByGroupID[groupID].BillingModelSource = BillingModelSourceChannelMapped
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1}}
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	quote, err := svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
	require.NoError(t, err)
	require.Equal(t, billingModel, quote.BillingModel)
	require.Len(t, quote.Tiers, 4)
	require.InDelta(t, price, quote.Tiers["4k"].UnitPrice, 1e-12)
	require.Equal(t, omniPricingTestModel, videoUsageModel(&OpenAIForwardResult{Model: omniPricingTestModel, UpstreamModel: billingModel}, billingModel))
	key.Group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"other-model"}}
	_, err = svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
	require.ErrorIs(t, err, ErrVideoStudioModelNotAllowed)
}

func TestGeminiOmniInflightEstimateUsesCompletedPricing(t *testing.T) {
	groupID, price, legacy := int64(95), 0.4, 99.0
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	billing := NewBillingService(cfg, nil)
	svc := &GatewayService{cfg: cfg, billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{User: &User{ID: 9}, GroupID: &groupID, Group: &Group{
		ID: groupID, Platform: PlatformGemini, RateMultiplier: 0.5,
		VideoRateIndependent: true, VideoRateMultiplier: 0.25, VideoPrice480P: &legacy,
	}}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: mode, PerRequestPrice: &price}}
		for _, duration := range []int{3, 15} {
			estimate, priced := svc.EstimateInflightReservation(context.Background(), key, InflightEstimateRequest{
				Model: omniPricingTestModel, Kind: InflightEstimateVideo, Units: 1, VideoDurationSeconds: duration, VideoResolution: "4k",
			})
			require.True(t, priced)
			want := price * 0.5
			if mode == BillingModeVideo {
				want = price * 0.25 * float64(duration)
			}
			require.InDelta(t, want, estimate, 1e-12)
		}
	}
}

func TestGeminiOmniVideoBillingProviderLimits(t *testing.T) {
	for _, model := range []string{omniPricingTestModel, "gemini-omni-1.2", "omni-1.2", "omni-featured"} {
		require.Equal(t, 5, NormalizeVideoBillingDurationForModelOrDefault(model, 0))
		require.Equal(t, 3, NormalizeVideoBillingDurationForModelOrDefault(model, 1))
		require.Equal(t, 15, NormalizeVideoBillingDurationForModelOrDefault(model, 60))
		require.Equal(t, "720p", NormalizeVideoBillingResolutionForModelOrDefault(model, ""))
		require.Equal(t, "4k", NormalizeVideoBillingResolutionForModelOrDefault(model, "4K"))
		require.Equal(t, "360p", NormalizeVideoBillingResolutionForModelOrDefault(model, "360P"))
	}
	require.Equal(t, "480p", NormalizeVideoBillingResolutionForModelOrDefault("grok-imagine-video", "4K"))
}

func TestGeminiOmniVideoUsageKeepsResolutionAndPerRequestMode(t *testing.T) {
	groupID, price := int64(96), 0.4
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	key := &APIKey{ID: 5, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 0.5,
		ModelPricing: []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}},
	}}
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "omni-task", ResponseID: "omni-task", Model: omniPricingTestModel,
			UpstreamModel: "arbitrary-upstream-alias", VideoCount: 1, VideoDurationSeconds: 15, VideoResolution: "4K"},
		APIKey: key, User: &User{ID: 6}, Account: &Account{ID: 7, Platform: PlatformGemini},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "grok-video:omni-task", usageRepo.lastLog.RequestID)
	require.Equal(t, 15, *usageRepo.lastLog.VideoDurationSeconds)
	require.Equal(t, "4k", *usageRepo.lastLog.VideoResolution)
	require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	require.InDelta(t, price*0.5, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestGeminiOmniVideoPricingOmitsUnconfiguredResolutions(t *testing.T) {
	ctx, groupID, price := context.Background(), int64(97), 0.3
	billing := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
	key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1}}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
		key.Group.ModelPricing = []ChannelModelPricing{{Models: []string{omniPricingTestModel}, BillingMode: mode,
			Intervals: []PricingInterval{{TierLabel: "720P", PerRequestPrice: &price}},
		}}
		quote, err := svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
		require.NoError(t, err)
		require.Len(t, quote.Tiers, 1)
		require.InDelta(t, price, quote.Tiers["720p"].UnitPrice, 1e-12)
		require.NotContains(t, quote.Tiers, "4k")
		_, err = svc.calculateOpenAIVideoCost(ctx, omniPricingTestModel, key, &OpenAIForwardResult{
			Model: omniPricingTestModel, VideoCount: 1, VideoDurationSeconds: 15, VideoResolution: "4k",
		}, 1, 1)
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
		key.Group.ModelPricing[0].Intervals = nil
		_, err = svc.VideoStudioPricing(ctx, key, omniPricingTestModel)
		require.ErrorIs(t, err, ErrVideoPricingNotConfigured)
	}
}

func TestGeminiOmniModelPlazaUsesVideoPricingPriority(t *testing.T) {
	ctx, groupID, channelPrice, groupPrice := context.Background(), int64(100), 0.4, 0.2
	for _, model := range []string{omniPricingTestModel, "gemini-omni-1.2", "omni-1.2", "omni-featured"} {
		for _, mode := range []BillingMode{BillingModePerRequest, BillingModeVideo} {
			resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, channelPrice)
			cache := resolver.channelService.cache.Load().(*channelCache)
			cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}].BillingMode = mode
			plaza := &ModelPlazaService{resolver: resolver, billingService: resolver.billingService}
			gateway := &OpenAIGatewayService{resolver: resolver, billingService: resolver.billingService}
			group := &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1,
				ModelPricing: []ChannelModelPricing{{Models: []string{model}, BillingMode: mode, PerRequestPrice: &groupPrice}},
			}
			entry := &PlazaModel{Name: model, Platform: PlatformGemini}
			plaza.fillDisplayPricing(ctx, entry, group)
			require.NotNil(t, entry.Pricing)
			require.Equal(t, mode, entry.Pricing.BillingMode)
			require.InDelta(t, groupPrice, *entry.Pricing.PerRequestPrice, 1e-12)
			quote, err := gateway.VideoStudioPricing(ctx, &APIKey{GroupID: &groupID, Group: group}, model)
			require.NoError(t, err)
			require.InDelta(t, *entry.Pricing.PerRequestPrice, quote.Tiers["720p"].BasePrice, 1e-12)
			group.ModelPricing = nil
			plaza.fillDisplayPricing(ctx, entry, group)
			require.InDelta(t, channelPrice, *entry.Pricing.PerRequestPrice, 1e-12)
		}
	}
}

func TestGeminiOmniModelPlazaResolvesMappedBillingModel(t *testing.T) {
	ctx, groupID, price := context.Background(), int64(101), 0.3
	const mapped = "private-omni-video-price"
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, mapped, price)
	cache := resolver.channelService.cache.Load().(*channelCache)
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: mapped}].BillingMode = BillingModePerRequest
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: omniPricingTestModel}] = mapped
	cache.channelByGroupID[groupID].BillingModelSource = BillingModelSourceChannelMapped
	plaza := &ModelPlazaService{resolver: resolver, billingService: resolver.billingService}
	entry := &PlazaModel{Name: omniPricingTestModel, Platform: PlatformGemini}
	plaza.fillDisplayPricing(ctx, entry, &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1})
	require.NotNil(t, entry.Pricing)
	require.Equal(t, BillingModePerRequest, entry.Pricing.BillingMode)
	require.InDelta(t, price, *entry.Pricing.PerRequestPrice, 1e-12)
}

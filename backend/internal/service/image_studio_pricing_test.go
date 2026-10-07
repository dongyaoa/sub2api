package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestImageStudioPricing_ChannelModelPriceMatchesActualCharge(t *testing.T) {
	for _, platform := range []string{PlatformGemini, PlatformOpenAI, PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			ctx := context.Background()
			groupID := int64(55)
			model, channelPrice, flatPrice, cardPrice := "gemini-nano-banana-2.1", 0.1, 0.06, 0.07
			billing := NewBillingService(&config.Config{}, nil)
			resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, channelPrice)
			apiKey := &APIKey{UserID: 5, GroupID: &groupID, Group: &Group{
				ID: groupID, Platform: platform, RateMultiplier: 0.5, ImagePrice1K: &flatPrice,
				ModelPricing: []ChannelModelPricing{{Models: []string{model}, BillingMode: BillingModeImage, PerRequestPrice: &cardPrice}},
			}}
			quote, err := buildImageStudioPricing(ctx, billing, resolver, apiKey, model, 0.5)
			require.NoError(t, err)
			require.Equal(t, PricingSourceChannel, quote.PricingSource)
			require.InDelta(t, 0.1, quote.Tiers["1K"].BasePrice, 1e-12)
			require.InDelta(t, 0.05, quote.Tiers["1K"].UnitPrice, 1e-12)
			var actual *CostBreakdown
			if platform == PlatformGemini {
				svc := &GatewayService{billingService: billing, resolver: resolver}
				actual = svc.calculateRecordUsageCost(ctx, &ForwardResult{ImageCount: 2, ImageSize: "1K"}, apiKey, model, 0.5, 0.5, time.Time{})
			} else {
				svc := &OpenAIGatewayService{billingService: billing, resolver: resolver}
				actual, err = svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{ImageCount: 2, ImageSize: "1K"}, apiKey, []string{model}, 0.5, 0.5, 1, 1, UsageTokens{}, "", nil, time.Time{})
				require.NoError(t, err)
			}
			require.InDelta(t, quote.Tiers["1K"].BasePrice*2, actual.TotalCost, 1e-12)
			require.InDelta(t, quote.Tiers["1K"].UnitPrice*2, actual.ActualCost, 1e-12)
			// Regular token/video callers retain the group-first resolver contract.
			require.Equal(t, PricingSourceGroup, resolver.Resolve(ctx, PricingInput{Model: model, GroupID: &groupID, Group: apiKey.Group}).Source)
		})
	}
}

func TestImageStudioPricing_ImageTiersRespectZeroAndDefault(t *testing.T) {
	ctx, groupID, model := context.Background(), int64(56), "image-custom"
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, model, 0.1)
	price1K, free := 0.06, 0.0
	cache, ok := resolver.channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	pricing := cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}]
	pricing.Intervals = []PricingInterval{{TierLabel: "1k", PerRequestPrice: &price1K}, {TierLabel: "4K", PerRequestPrice: &free}}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID}}
	quote, err := buildImageStudioPricing(ctx, resolver.billingService, resolver, apiKey, model, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.06, quote.Tiers["1K"].UnitPrice, 1e-12)
	require.InDelta(t, 0.1, quote.Tiers["2K"].UnitPrice, 1e-12, "missing 2K uses default, never the 1K interval")
	require.Zero(t, quote.Tiers["4K"].UnitPrice, "explicit zero tier stays free")
	require.True(t, quote.OpenAI4KAllowed)
	pricing.PerRequestPrice = nil
	quote, err = buildImageStudioPricing(ctx, resolver.billingService, resolver, apiKey, model, 1)
	require.NoError(t, err)
	require.InDelta(t, resolver.billingService.getDefaultImagePrice(model, "2K"), quote.Tiers["2K"].UnitPrice, 1e-12)
}

func TestImageStudioPricing_NoModelPriceIgnoresFlatGroupPrice(t *testing.T) {
	ctx, groupID, flat := context.Background(), int64(57), 0.06
	billing := NewBillingService(&config.Config{}, nil)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, ImagePrice1K: &flat, ImagePrice4K: &flat}}
	quote, err := buildImageStudioPricing(ctx, billing, nil, apiKey, "gpt-image-2", 1)
	require.NoError(t, err)
	require.Equal(t, "default", quote.PricingSource)
	require.InDelta(t, billing.getDefaultImagePrice("gpt-image-2", "1K"), quote.Tiers["1K"].UnitPrice, 1e-12)
	svc := &OpenAIGatewayService{billingService: billing}
	actual := svc.calculateOpenAIImageCost(ctx, "gpt-image-2", apiKey, &OpenAIForwardResult{ImageCount: 1, ImageSize: "1K"}, 1)
	require.InDelta(t, quote.Tiers["1K"].UnitPrice, actual.ActualCost, 1e-12)
	require.True(t, quote.OpenAI4KAllowed, "legacy 4K capability switch remains separate from price")
}

func TestImageStudioPricing_TokenModelHasNoPerImageEstimate(t *testing.T) {
	ctx, groupID, model := context.Background(), int64(58), "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID}}
	quote, err := buildImageStudioPricing(ctx, resolver.billingService, resolver, apiKey, model, 0.5)
	require.NoError(t, err)
	require.Equal(t, BillingModeToken, quote.BillingMode)
	require.Nil(t, quote.Tiers)
	require.InDelta(t, 0.5, quote.RateMultiplier, 1e-12)
	// Image token prices also beat a model-level group image card, while the
	// existing additive search fee still applies to a combined tool response.
	groupPrice, searchPrice := 0.06, 1000.0
	apiKey.Group.ModelPricing = []ChannelModelPricing{{Models: []string{model}, BillingMode: BillingModeImage, PerRequestPrice: &groupPrice}}
	apiKey.Group.SearchPricePer1k = &searchPrice
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	cost, err := svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{ImageCount: 1, SearchCount: 1}, apiKey, []string{model},
		0.5, 0.5, 1, 0.25, UsageTokens{InputTokens: 100, OutputTokens: 200}, "", nil, time.Time{})
	require.NoError(t, err)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
	require.InDelta(t, 1.0033, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.25165, cost.ActualCost, 1e-12, "search uses its base multiplier, separately from token/peak rates")
}

func TestImageStudioPricing_ChannelMappingMatchesBillingModel(t *testing.T) {
	ctx, groupID := context.Background(), int64(59)
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "gemini-nano-banana-2.1", 0.1)
	cache, ok := resolver.channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: "banana-alias"}] = "gemini-nano-banana-2.1"
	cache.channelByGroupID[groupID].BillingModelSource = BillingModelSourceChannelMapped
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID}}
	quote, err := buildImageStudioPricing(ctx, resolver.billingService, resolver, apiKey, "banana-alias", 1)
	require.NoError(t, err)
	require.Equal(t, "banana-alias", quote.Model)
	require.Equal(t, "gemini-nano-banana-2.1", quote.BillingModel)
	require.InDelta(t, 0.1, quote.Tiers["1K"].UnitPrice, 1e-12)
}

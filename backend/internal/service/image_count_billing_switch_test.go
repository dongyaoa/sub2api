//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func boolPtrForImageBillingTest(v bool) *bool { return &v }

func TestImageCountBillingApplies(t *testing.T) {
	enabled := &APIKey{Group: &Group{ImageCountBilling: true}}
	disabled := &APIKey{Group: &Group{ImageCountBilling: false}}

	require.True(t, imageCountBillingApplies(enabled, true))
	require.False(t, imageCountBillingApplies(enabled, false), "混合请求不能受开关影响")
	require.False(t, imageCountBillingApplies(disabled, true))
	require.False(t, imageCountBillingApplies(nil, true))
}

// F1：composite 分组配置了 token 定价又开启按张计费时，图片不能按 $0 结算。
func TestCalculateRecordUsageCost_ImageCountBillingGroupTokenPricing(t *testing.T) {
	groupID := int64(901)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformComposite,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.5),
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{model},
			BillingMode: BillingModeToken,
			InputPrice:  f64p(3e-6),
			OutputPrice: f64p(15e-6),
		}},
	}
	apiKey := &APIKey{ID: 1, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
		Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 1000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.InDelta(t, 1.0, cost.TotalCost, 1e-12, "应回落分组图片价，而不是 token 定价的 $0")
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

// F3：渠道 token 定价 + 开关开启时必须强制按张计费，与 OpenAI 流水线一致。
func TestCalculateRecordUsageCost_ImageCountBillingChannelTokenPricing(t *testing.T) {
	groupID := int64(902)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 2, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
		Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 10_000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.InDelta(t, 0.8, cost.TotalCost, 1e-12)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

// F4：开关只作用于图片端点，混合请求仍按 token 计费。
func TestCalculateRecordUsageCost_ImageCountBillingIgnoresMixedRequests(t *testing.T) {
	groupID := int64(903)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformComposite,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 3, GroupID: &groupID, Group: group}
	svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

	cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
		Model: model, ImageCount: 1, ImageSize: "1K", ImageEndpoint: false,
		Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 10_000},
	}, apiKey, model, 1, 1, time.Time{})

	require.NotNil(t, cost)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
	require.Greater(t, cost.TotalCost, 0.0)
}

func TestCalculateOpenAIRecordUsageCost_ImageCountBillingIgnoresMixedRequests(t *testing.T) {
	groupID := int64(904)
	model := "gpt-image-2"
	resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		AllowImageGeneration: true,
		ImageCountBilling:    true,
		ImagePrice1K:         f64p(0.4),
	}
	apiKey := &APIKey{ID: 4, GroupID: &groupID, Group: group}
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}

	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
		Model: model, ImageCount: 1, ImageSize: "1K", ImageEndpoint: false,
	}, apiKey, []string{model}, 1, 1, 1, 1,
		UsageTokens{InputTokens: 1_000_000, OutputTokens: 10_000},
		"", nil, false, time.Time{})

	require.NoError(t, err)
	require.NotNil(t, cost)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
	require.Greater(t, cost.TotalCost, 0.0)
}

func TestGroupService_ImageCountBillingValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("创建入口固定 anthropic 平台，拒绝开关", func(t *testing.T) {
		svc := NewGroupService(nil, nil)
		_, err := svc.Create(ctx, CreateGroupRequest{Name: "g", ImageCountBilling: true})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("更新时未开启图片生成则拒绝", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 1, Platform: PlatformComposite, AllowImageGeneration: false,
		}}
		svc := NewGroupService(repo, nil)
		_, err := svc.Update(ctx, 1, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("更新时不支持的平台拒绝", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 2, Platform: PlatformAnthropic, AllowImageGeneration: true,
		}}
		svc := NewGroupService(repo, nil)
		_, err := svc.Update(ctx, 2, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.ErrorContains(t, err, "image_count_billing")
	})

	t.Run("合法开启", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 3, Platform: PlatformComposite, AllowImageGeneration: true,
		}}
		svc := NewGroupService(repo, nil)
		updated, err := svc.Update(ctx, 3, UpdateGroupRequest{ImageCountBilling: boolPtrForImageBillingTest(true)})
		require.NoError(t, err)
		require.True(t, updated.ImageCountBilling)
	})

	t.Run("关闭图片生成时自动清理开关", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{getByID: &Group{
			ID: 4, Platform: PlatformComposite, AllowImageGeneration: false, ImageCountBilling: true,
		}}
		svc := NewGroupService(repo, nil)
		updated, err := svc.Update(ctx, 4, UpdateGroupRequest{})
		require.NoError(t, err)
		require.False(t, updated.ImageCountBilling)
	})
}

// 渠道按次定价只配置按上下文分档（无尺寸档）时，尺寸档未命中，必须用 token
// 上下文匹配档位；漏传 Tokens 会让档位永不命中而按 $0 结算。与开关无关。
func TestCalculateRecordUsageCost_ChannelPerRequestContextTierUsesTokens(t *testing.T) {
	for _, switchOn := range []bool{false, true} {
		groupID := int64(905)
		model := "gemini-3-pro-image"
		resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
		cached, ok := resolver.channelService.cache.Load().(*channelCache)
		require.True(t, ok)
		maxTokens := 1000
		cached.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &ChannelModelPricing{
			BillingMode: BillingModePerRequest,
			Intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: &maxTokens, PerRequestPrice: f64p(0.1)},
				{MinTokens: 1000, PerRequestPrice: f64p(0.3)},
			},
		}
		group := &Group{
			ID:                   groupID,
			Platform:             PlatformComposite,
			Status:               StatusActive,
			Hydrated:             true,
			AllowImageGeneration: true,
			ImageCountBilling:    switchOn,
		}
		apiKey := &APIKey{ID: 5, GroupID: &groupID, Group: group}
		svc := &GatewayService{billingService: resolver.billingService, resolver: resolver}

		cost := svc.calculateRecordUsageCost(context.Background(), &ForwardResult{
			Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
			Usage: ClaudeUsage{InputTokens: 5000, OutputTokens: 100},
		}, apiKey, model, 1, 1, time.Time{})

		require.NotNil(t, cost)
		require.InDelta(t, 0.6, cost.TotalCost, 1e-12, "switch=%v: 应按上下文档 0.3 × 2 张结算", switchOn)

		openAI := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
		openAICost, err := openAI.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
			Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
			Usage: OpenAIUsage{InputTokens: 5000, OutputTokens: 100},
		}, apiKey, []string{model}, 1, 1, 1, 1,
			UsageTokens{InputTokens: 5000, OutputTokens: 100}, "", nil, false, time.Time{})
		require.NoError(t, err)
		require.InDelta(t, 0.6, openAICost.TotalCost, 1e-12, "OpenAI switch=%v: 应使用相同上下文档", switchOn)
	}
}

func TestImageCountBilling_ModelPricingAndStudioQuote(t *testing.T) {
	for _, source := range []string{PricingSourceChannel, PricingSourceGroup} {
		for _, tc := range []struct {
			name       string
			mode       BillingMode
			groupPrice *float64
		}{
			{name: "image_model_overrides_flat", mode: BillingModeImage, groupPrice: f64p(0.4)},
			{name: "per_request_model_overrides_flat", mode: BillingModePerRequest, groupPrice: f64p(0.4)},
			{name: "token_uses_flat", mode: BillingModeToken, groupPrice: f64p(0.4)},
			{name: "token_respects_free_flat", mode: BillingModeToken, groupPrice: f64p(0)},
			{name: "token_uses_default", mode: BillingModeToken},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				ctx, groupID, model := context.Background(), int64(906), "gpt-image-2"
				resolver := newOpenAITokenImageChannelPricingResolverForTest(t, groupID, model)
				cached := resolver.channelService.cache.Load().(*channelCache)
				key := channelModelKey{groupID: groupID, model: model}
				pricing := cached.pricingByGroupModel[key]
				pricing.Models = []string{model}
				pricing.BillingMode = tc.mode
				pricing.PerRequestPrice = f64p(0.1)
				group := &Group{
					ID: groupID, ImageCountBilling: true, ImagePrice1K: tc.groupPrice,
					ImageRateIndependent: true, ImageRateMultiplier: 0.25,
				}
				if source == PricingSourceGroup {
					group.ModelPricing = []ChannelModelPricing{*pricing}
					delete(cached.pricingByGroupModel, key)
				} else {
					// A conflicting group model card must not displace channel pricing.
					group.ModelPricing = []ChannelModelPricing{{Models: []string{model}, BillingMode: BillingModeImage, PerRequestPrice: f64p(0.9)}}
				}
				apiKey := &APIKey{GroupID: &groupID, Group: group}
				quote, err := buildImageStudioPricing(ctx, resolver.billingService, resolver, apiKey, model, 0.5)
				require.NoError(t, err)
				require.NotEmpty(t, quote.Tiers)
				wantUnit := 0.1
				wantMode := tc.mode
				if tc.mode == BillingModeToken {
					wantMode = BillingModeImage
					wantUnit = resolver.billingService.getDefaultImagePrice(model, "1K")
					if tc.groupPrice != nil {
						wantUnit = *tc.groupPrice
					}
				}
				require.Equal(t, wantMode, quote.BillingMode)
				require.InDelta(t, wantUnit, quote.Tiers["1K"].BasePrice, 1e-12)
				require.InDelta(t, wantUnit*0.25, quote.Tiers["1K"].UnitPrice, 1e-12)
				gateway := &GatewayService{billingService: resolver.billingService, resolver: resolver}
				gatewayCost := gateway.calculateRecordUsageCost(ctx, &ForwardResult{
					Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
					Usage: ClaudeUsage{InputTokens: 1_000_000, OutputTokens: 10_000},
				}, apiKey, model, 0.5, 0.25, time.Time{})
				openAI := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
				openAICost, err := openAI.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{
					Model: model, ImageCount: 2, ImageSize: "1K", ImageEndpoint: true,
				}, apiKey, []string{model}, 0.5, 0.25, 1, 1,
					UsageTokens{InputTokens: 1_000_000, OutputTokens: 10_000}, "", nil, false, time.Time{})
				require.NoError(t, err)
				for _, cost := range []*CostBreakdown{gatewayCost, openAICost} {
					require.Equal(t, string(wantMode), cost.BillingMode)
					require.InDelta(t, wantUnit*2, cost.TotalCost, 1e-12)
					require.InDelta(t, quote.Tiers["1K"].UnitPrice*2, cost.ActualCost, 1e-12)
				}
			})
		}
	}
}

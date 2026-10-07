package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var ErrImageStudioModelNotAllowed = errors.New("image model is not available for this group")

type ImageStudioTierPrice struct {
	BasePrice float64 `json:"base_price"`
	UnitPrice float64 `json:"unit_price"`
}

// ImageStudioPricing is an authenticated quote using the exact resolver and
// multiplier used by image usage billing. Token-priced models have no flat quote.
type ImageStudioPricing struct {
	Model           string                          `json:"model"`
	BillingModel    string                          `json:"billing_model"`
	BillingMode     BillingMode                     `json:"billing_mode"`
	PricingSource   string                          `json:"pricing_source"`
	RateMultiplier  float64                         `json:"rate_multiplier"`
	Tiers           map[string]ImageStudioTierPrice `json:"tiers"`
	OpenAI4KAllowed bool                            `json:"openai4k_allowed"`
}

func resolveImagePricingForAPIKey(ctx context.Context, resolver *ModelPricingResolver, model string, apiKey *APIKey) *ResolvedPricing {
	if resolver == nil || apiKey == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	return resolver.ResolveImagePricing(ctx, PricingInput{Model: model, GroupID: &gid, Group: apiKey.Group})
}

func buildImageStudioPricing(ctx context.Context, billing *BillingService, resolver *ModelPricingResolver, apiKey *APIKey, model string, baseMultiplier float64) (*ImageStudioPricing, error) {
	if billing == nil || apiKey == nil || apiKey.Group == nil {
		return nil, errors.New("image pricing is unavailable")
	}
	model = strings.TrimSpace(model)
	if !apiKey.Group.ModelAllowlist.Allows(model) {
		return nil, ErrImageStudioModelNotAllowed
	}
	billingModel := model
	if resolver != nil && resolver.channelService != nil {
		gid := apiKey.Group.ID
		mapping, _ := resolver.channelService.ResolveChannelMappingAndRestrict(ctx, &gid, model)
		restrictedModel := billingModelForRestriction(mapping.BillingModelSource, model, mapping.MappedModel)
		if restrictedModel != "" && resolver.channelService.IsModelRestricted(ctx, gid, restrictedModel) {
			return nil, ErrImageStudioModelNotAllowed
		}
		if mapping.Mapped && mapping.BillingModelSource != BillingModelSourceRequested {
			billingModel = mapping.MappedModel
		}
	}
	resolved := resolveImagePricingForAPIKey(ctx, resolver, billingModel, apiKey)
	quote := &ImageStudioPricing{
		Model: model, BillingModel: billingModel, BillingMode: BillingModeImage, PricingSource: "default",
		RateMultiplier:  resolveImageRateMultiplier(apiKey, baseMultiplier),
		OpenAI4KAllowed: apiKey.Group.ImagePrice4K != nil,
	}
	if resolved != nil {
		quote.BillingMode = resolved.Mode
		quote.PricingSource = resolved.Source
		for _, tier := range resolved.RequestTiers {
			if strings.EqualFold(strings.TrimSpace(tier.TierLabel), ImageBillingSize4K) && tier.PerRequestPrice != nil {
				quote.OpenAI4KAllowed = true
			}
		}
		if resolved.Mode == BillingModeToken {
			quote.RateMultiplier = baseMultiplier * apiKey.Group.PeakMultiplierAt(timezone.Now())
			return quote, nil
		}
	}
	quote.Tiers = make(map[string]ImageStudioTierPrice, 3)
	for _, size := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		cost := billing.CalculateImageCost(billingModel, size, 1, nil, quote.RateMultiplier)
		if resolved != nil {
			gid := apiKey.Group.ID
			var err error
			cost, err = billing.CalculateCostUnified(CostInput{
				Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
				RequestCount: 1, SizeTier: size, RateMultiplier: quote.RateMultiplier,
				Resolver: resolver, Resolved: resolved,
			})
			if err != nil {
				return nil, err
			}
		}
		quote.Tiers[size] = ImageStudioTierPrice{BasePrice: cost.TotalCost, UnitPrice: cost.ActualCost}
	}
	return quote, nil
}

func (s *GatewayService) ImageStudioPricing(ctx context.Context, apiKey *APIKey, model string) (*ImageStudioPricing, error) {
	if s == nil || apiKey == nil || apiKey.Group == nil {
		return nil, errors.New("image pricing is unavailable")
	}
	rate := s.ResolveUserGroupRateMultiplier(ctx, apiKey.UserID, apiKey.Group.ID, apiKey.Group.RateMultiplier)
	return buildImageStudioPricing(ctx, s.billingService, s.resolver, apiKey, model, rate)
}

func (s *OpenAIGatewayService) ImageStudioPricing(ctx context.Context, apiKey *APIKey, model string) (*ImageStudioPricing, error) {
	if s == nil || apiKey == nil || apiKey.Group == nil {
		return nil, errors.New("image pricing is unavailable")
	}
	rate := s.ResolveUserGroupRateMultiplier(ctx, apiKey.UserID, apiKey.Group.ID, apiKey.Group.RateMultiplier)
	return buildImageStudioPricing(ctx, s.billingService, s.resolver, apiKey, model, rate)
}

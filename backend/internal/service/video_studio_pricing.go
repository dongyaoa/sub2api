package service

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrVideoStudioModelNotAllowed = errors.New("video model is not available for this group")
	ErrVideoPricingNotConfigured  = errors.New("video model requires a configured per-request or per-second price")
)

type VideoStudioTierPrice struct {
	BasePrice  float64 `json:"base_price"`
	UnitPrice  float64 `json:"unit_price"`
	Configured bool    `json:"configured"`
}

// VideoStudioPricing uses the same resolver as completed video billing. The
// video mode quotes one second; per_request/image modes quote one generation.
type VideoStudioPricing struct {
	Model          string                          `json:"model"`
	BillingModel   string                          `json:"billing_model"`
	BillingMode    BillingMode                     `json:"billing_mode"`
	PricingSource  string                          `json:"pricing_source"`
	RateMultiplier float64                         `json:"rate_multiplier"`
	Tiers          map[string]VideoStudioTierPrice `json:"tiers"`
}

func videoUsageModel(result *OpenAIForwardResult, billingModel string) string {
	if result != nil {
		for _, model := range []string{result.UpstreamModel, result.Model, result.BillingModel} {
			if IsGeminiOmniVideoModel(model) {
				return model
			}
			if IsMuseVideoModel(model) {
				return "muse-video"
			}
		}
		for _, model := range []string{result.UpstreamModel, result.Model, result.BillingModel} {
			if family := CanonicalGrokImagineVideoPriceFamily(model); family != "" {
				return family
			}
		}
	}
	return billingModel
}

func (s *OpenAIGatewayService) resolveVideoUsagePricing(ctx context.Context, apiKey *APIKey, billingModel, videoModel, resolution string) (*ResolvedPricing, string, error) {
	resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey)
	if IsMuseVideoModel(videoModel) || IsGeminiOmniVideoModel(videoModel) {
		// Custom providers require a model card, including an intentionally
		// configured zero. Legacy Grok rates must never price these models.
		if resolved == nil || (resolved.Mode != BillingModePerRequest && resolved.Mode != BillingModeVideo) ||
			!hasVideoRequestTierPrice(resolved, resolution) {
			return nil, "", ErrVideoPricingNotConfigured
		}
		return resolved, resolved.Source, nil
	}
	if resolved != nil && resolved.Mode == BillingModeToken && CanonicalGrokImagineVideoPriceFamily(videoModel) != "" {
		// Grok video generations keep their existing per-second group/default
		// prices. A text token card (including a wildcard card) must not hide
		// those prices or make a completed video with no token usage free.
		resolved = nil
	}
	// Per-request cards must not be shadowed by legacy per-second group prices.
	if resolved != nil && (resolved.Mode == BillingModeToken || resolved.Mode == BillingModePerRequest || resolved.Source == PricingSourceGroup) {
		return resolved, resolved.Source, nil
	}
	if apiKeyHasConfiguredVideoPrice(apiKey, billingModel, resolution) {
		return nil, PricingSourceGroup, nil
	}
	if resolved != nil && (resolved.Mode == BillingModeImage || resolved.Mode == BillingModeVideo) {
		return resolved, resolved.Source, nil
	}
	return nil, "default", nil
}

func hasVideoRequestTierPrice(resolved *ResolvedPricing, resolution string) bool {
	if resolved.channelPricing != nil && resolved.channelPricing.PerRequestPrice != nil {
		return true
	}
	for _, tier := range resolved.RequestTiers {
		if tier.PerRequestPrice != nil && strings.EqualFold(strings.TrimSpace(tier.TierLabel), resolution) {
			return true
		}
	}
	return false
}

// ResolveVideoStudioModels is shared by quotes and legacy delivered tasks that
// predate the create-time billing-model snapshot.
func (s *OpenAIGatewayService) ResolveVideoStudioModels(ctx context.Context, apiKey *APIKey, model string) (billingModel, videoModel string, err error) {
	if s == nil || apiKey == nil || apiKey.Group == nil {
		return "", "", errors.New("video pricing is unavailable")
	}
	model = strings.TrimSpace(model)
	if !apiKey.Group.ModelAllowlist.Allows(model) {
		return "", "", ErrVideoStudioModelNotAllowed
	}
	billingModel = model
	videoModel = model
	if s.resolver != nil && s.resolver.channelService != nil {
		gid := apiKey.Group.ID
		mapping, _ := s.resolver.channelService.ResolveChannelMappingAndRestrict(ctx, &gid, model)
		restrictedModel := billingModelForRestriction(mapping.BillingModelSource, model, mapping.MappedModel)
		if restrictedModel != "" && s.resolver.channelService.IsModelRestricted(ctx, gid, restrictedModel) {
			return "", "", ErrVideoStudioModelNotAllowed
		}
		if mapping.Mapped {
			// Preserve the requested provider's limits when a channel maps it
			// to an arbitrary upstream alias used only for routing or pricing.
			videoModel = videoUsageModel(&OpenAIForwardResult{Model: model, UpstreamModel: mapping.MappedModel}, mapping.MappedModel)
			if mapping.BillingModelSource != BillingModelSourceRequested {
				billingModel = mapping.MappedModel
			}
		}
	}
	return billingModel, videoModel, nil
}

func (s *OpenAIGatewayService) VideoStudioPricing(ctx context.Context, apiKey *APIKey, model string) (*VideoStudioPricing, error) {
	if s == nil || s.billingService == nil || apiKey == nil || apiKey.Group == nil {
		return nil, errors.New("video pricing is unavailable")
	}
	model = strings.TrimSpace(model)
	billingModel, videoModel, err := s.ResolveVideoStudioModels(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}
	apiKey = s.apiKeyWithFreshGroupMediaPricing(ctx, apiKey)
	baseMultiplier := s.ResolveUserGroupRateMultiplier(ctx, apiKey.UserID, apiKey.Group.ID, apiKey.Group.RateMultiplier)
	multiplier := resolveVideoRateMultiplier(apiKey, baseMultiplier)
	quote := &VideoStudioPricing{
		Model: model, BillingModel: billingModel, BillingMode: BillingModeVideo,
		PricingSource: "default", RateMultiplier: multiplier,
		Tiers: make(map[string]VideoStudioTierPrice),
	}
	resolutions := []string{VideoBillingResolution480P, VideoBillingResolution720P, VideoBillingResolution1080P}
	if IsMuseVideoModel(videoModel) {
		resolutions = []string{"auto"}
	} else if IsGeminiOmniVideoModel(videoModel) {
		resolutions = []string{VideoBillingResolution360P, VideoBillingResolution720P, VideoBillingResolution1080P, VideoBillingResolution4K}
	}
	for _, resolution := range resolutions {
		resolved, source, err := s.resolveVideoUsagePricing(ctx, apiKey, billingModel, videoModel, resolution)
		if err != nil {
			if IsGeminiOmniVideoModel(videoModel) && errors.Is(err, ErrVideoPricingNotConfigured) {
				// An operator may price only selected output resolutions. Omit
				// unavailable tiers so clients cannot mistake them for free ones.
				continue
			}
			return nil, err
		}
		quote.PricingSource = source
		if resolved != nil && resolved.Mode == BillingModePerRequest {
			quote.RateMultiplier = baseMultiplier
		}
		if resolved != nil && resolved.Mode == BillingModeToken {
			quote.BillingMode = BillingModeToken
			quote.Tiers = nil
			return quote, nil
		}
		var cost *CostBreakdown
		if resolved != nil {
			// A video price quote is for exactly one second, independent of
			// the provider's minimum generation duration (three for Omni).
			cost, err = s.calculateResolvedVideoCost(ctx, billingModel, apiKey, resolved, 1, 1, resolution, "", multiplier, baseMultiplier)
		} else {
			cost, err = s.calculateOpenAIVideoCost(ctx, billingModel, apiKey, &OpenAIForwardResult{
				Model: videoModel, VideoCount: 1, VideoDurationSeconds: 1, VideoResolution: resolution,
			}, multiplier, baseMultiplier)
		}
		if err != nil {
			return nil, err
		}
		quote.BillingMode = BillingMode(cost.BillingMode)
		quote.Tiers[resolution] = VideoStudioTierPrice{
			BasePrice: cost.TotalCost, UnitPrice: cost.ActualCost, Configured: source != "default",
		}
	}
	if len(quote.Tiers) == 0 {
		return nil, ErrVideoPricingNotConfigured
	}
	return quote, nil
}

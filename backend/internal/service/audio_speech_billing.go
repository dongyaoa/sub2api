package service

import (
	"context"
	"errors"
	"math"
	"strings"
)

var (
	ErrAudioSpeechPricingNotConfigured = errors.New("audio speech requires an explicitly configured flat per-request model price without pricing tiers")
	ErrAudioSpeechPricingUnavailable   = errors.New("audio speech pricing is unavailable")
)

// AudioSpeechBillingSnapshot freezes one successful synthesis's configured
// price before contacting the provider. Audio responses have no token usage;
// pricing them as text or as Grok's million-character units would undercharge.
// The snapshot is request-local and never accepted from a client payload.
type AudioSpeechBillingSnapshot struct {
	BillingModel   string
	PricingSource  string
	TotalCost      float64
	ActualCost     float64
	RateMultiplier float64
}

func (p *AudioSpeechBillingSnapshot) cost() (*CostBreakdown, error) {
	if p == nil || strings.TrimSpace(p.BillingModel) == "" ||
		!validAudioSpeechPrice(p.TotalCost) || !validAudioSpeechPrice(p.ActualCost) ||
		!validAudioSpeechPrice(p.RateMultiplier) {
		return nil, ErrAudioSpeechPricingUnavailable
	}
	return &CostBreakdown{
		TotalCost: p.TotalCost, ActualCost: p.ActualCost,
		BillingMode: string(BillingModePerRequest),
	}, nil
}

// InflightEstimate uses the same frozen amount as settlement, including an
// intentionally free model. It never falls back to a character/token price.
func (p *AudioSpeechBillingSnapshot) InflightEstimate() InflightEstimateRequest {
	model := ""
	if p != nil {
		model = p.BillingModel
	}
	return InflightEstimateRequest{
		Model: model, Kind: InflightEstimateAudioSpeech, Units: 1, AudioSpeechBilling: p,
	}
}

// PrepareAudioSpeechBilling must run after selecting an account and before
// forwarding. Model selection mirrors RecordUsage's requested/channel/upstream
// mapping rules, but requires a configured per-request card. ResolveImagePricing
// is also the model square's resolver for per-request cards (channel, then group).
func (s *OpenAIGatewayService) PrepareAudioSpeechBilling(ctx context.Context, apiKey *APIKey, account *Account, requestedModel string, mapping ChannelMappingResult) (*AudioSpeechBillingSnapshot, error) {
	if s == nil || s.billingService == nil || s.resolver == nil || apiKey == nil || apiKey.Group == nil || apiKey.GroupID == nil {
		return nil, ErrAudioSpeechPricingUnavailable
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return nil, ErrAudioSpeechPricingNotConfigured
	}
	// Composite routing may already have rewritten the request body's model.
	// Keep that name for account routing; requested-price cards and usage fields
	// must still refer to the public name the client actually sent.
	publicModel := requestedModel
	if original, ok := RequestedPublicModelFromContext(ctx); ok {
		publicModel = original
	}
	apiKey = s.apiKeyWithFreshGroupMediaPricing(ctx, apiKey)
	forwardModel := requestedModel
	if mapping.Mapped && strings.TrimSpace(mapping.MappedModel) != "" {
		forwardModel = strings.TrimSpace(mapping.MappedModel)
	}
	upstreamModel := resolveOpenAIForwardModel(account, forwardModel, "")
	usageFields := mapping.ToUsageFields(publicModel, upstreamModel)
	billingModel := upstreamModel
	switch usageFields.BillingModelSource {
	case BillingModelSourceRequested:
		billingModel = publicModel
	case BillingModelSourceChannelMapped:
		if usageFields.ChannelMappedModel != "" && usageFields.ChannelMappedModel != publicModel {
			billingModel = usageFields.ChannelMappedModel
		}
	}
	models := usageBillingModelCandidates(billingModel, upstreamModel, usageFields.ChannelMappedModel, publicModel, requestedModel)
	userID := apiKey.UserID
	if apiKey.User != nil {
		userID = apiKey.User.ID
	}
	rate := s.ResolveUserGroupRateMultiplier(ctx, userID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	if !validAudioSpeechPrice(rate) {
		return nil, ErrAudioSpeechPricingUnavailable
	}
	for _, model := range models {
		resolved := s.resolver.ResolveImagePricing(ctx, PricingInput{Model: model, GroupID: apiKey.GroupID, Group: apiKey.Group})
		if resolved == nil {
			continue
		}
		// An explicit but incompatible card is a configuration error, not a
		// reason to silently substitute another model's price or a default.
		if !hasAudioSpeechRequestPrice(resolved) {
			return nil, ErrAudioSpeechPricingNotConfigured
		}
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: model, GroupID: apiKey.GroupID, Group: apiKey.Group,
			RequestCount: 1, RateMultiplier: rate, Resolver: s.resolver, Resolved: resolved,
		})
		if err != nil {
			return nil, err
		}
		if cost == nil {
			return nil, ErrAudioSpeechPricingUnavailable
		}
		snapshot := &AudioSpeechBillingSnapshot{
			BillingModel: model, PricingSource: resolved.Source,
			TotalCost: cost.TotalCost, ActualCost: cost.ActualCost, RateMultiplier: rate,
		}
		if _, err := snapshot.cost(); err != nil {
			return nil, err
		}
		return snapshot, nil
	}
	return nil, ErrAudioSpeechPricingNotConfigured
}

func validAudioSpeechPrice(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hasAudioSpeechRequestPrice(resolved *ResolvedPricing) bool {
	if resolved == nil || resolved.Mode != BillingModePerRequest || resolved.channelPricing == nil {
		return false
	}
	// Binary TTS has no trustworthy token count or tier discriminator. The
	// model square displays priced tiers ahead of the default, so accepting
	// such a card while charging only its default would misrepresent the bill.
	for _, tier := range resolved.RequestTiers {
		if tier.PerRequestPrice != nil {
			return false
		}
	}
	// Explicit zero is a free flat price; nil is an unconfigured card.
	return resolved.channelPricing.PerRequestPrice != nil && validAudioSpeechPrice(*resolved.channelPricing.PerRequestPrice)
}

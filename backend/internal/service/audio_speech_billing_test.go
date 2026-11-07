package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

const audioSpeechPricingTestModel = "gemini-3.8-flash-tts"

func audioSpeechPricingTestKey(price float64) *APIKey {
	gid := int64(301)
	return &APIKey{ID: 9, UserID: 7, User: &User{ID: 7}, GroupID: &gid, Group: &Group{
		ID: gid, Platform: PlatformGemini, RateMultiplier: 0.5,
		ModelPricing: []ChannelModelPricing{{Models: []string{audioSpeechPricingTestModel}, BillingMode: BillingModePerRequest, PerRequestPrice: &price}},
	}}
}

func audioSpeechPricingTestService() *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Billing.InflightReservation.Enabled = true
	billing := NewBillingService(cfg, nil)
	return &OpenAIGatewayService{cfg: cfg, billingService: billing, resolver: NewModelPricingResolver(nil, billing)}
}

func TestAudioSpeechBillingUsesModelSquarePricesAndOneCall(t *testing.T) {
	ctx := context.Background()
	for _, price := range []float64{0, 0.02, 0.15} {
		for _, multiplier := range []float64{0, 0.5, 2} {
			key := audioSpeechPricingTestKey(price)
			key.Group.RateMultiplier = multiplier
			svc := audioSpeechPricingTestService()
			snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
			require.NoError(t, err)
			require.Equal(t, PricingSourceGroup, snapshot.PricingSource)
			require.InDelta(t, price, snapshot.TotalCost, 1e-12)
			require.InDelta(t, price*multiplier, snapshot.ActualCost, 1e-12)

			plazaModel := PlazaModel{Name: audioSpeechPricingTestModel, Platform: PlatformGemini}
			(&ModelPlazaService{resolver: svc.resolver}).fillDisplayPricing(ctx, &plazaModel, key.Group)
			require.NotNil(t, plazaModel.Pricing)
			require.Equal(t, *plazaModel.Pricing.PerRequestPrice, snapshot.TotalCost)
			for _, bodyBytes := range []int{1, 10_000, 1_000_000} {
				estimateRequest := snapshot.InflightEstimate()
				estimateRequest.BodyBytes = bodyBytes
				estimateRequest.MaxTokens = 500_000
				estimate, priced := svc.EstimateInflightReservation(ctx, key, estimateRequest)
				require.True(t, priced)
				require.InDelta(t, snapshot.ActualCost, estimate, 1e-12)
				cost, err := svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{
					AudioSpeech: true, AudioSpeechBilling: snapshot,
				}, key, []string{audioSpeechPricingTestModel}, 99, 99, 99, 99, UsageTokens{}, "", nil, false, time.Time{})
				require.NoError(t, err)
				require.InDelta(t, estimate, cost.ActualCost, 1e-12)
				require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
			}
		}
	}
}

func TestAudioSpeechBillingChannelPrecedenceMatchesModelSquare(t *testing.T) {
	ctx := context.Background()
	key := audioSpeechPricingTestKey(8)
	resolver := newOpenAIImageChannelPricingResolverForTest(t, *key.GroupID, audioSpeechPricingTestModel, 0.3)
	cache, ok := resolver.channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	cache.pricingByGroupModel[channelModelKey{groupID: *key.GroupID, model: audioSpeechPricingTestModel}].BillingMode = BillingModePerRequest
	svc := &OpenAIGatewayService{billingService: resolver.billingService, resolver: resolver}
	snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	require.Equal(t, PricingSourceChannel, snapshot.PricingSource)
	require.InDelta(t, 0.3, snapshot.TotalCost, 1e-12)
	plazaModel := PlazaModel{Name: audioSpeechPricingTestModel, Platform: PlatformGemini}
	(&ModelPlazaService{resolver: resolver}).fillDisplayPricing(ctx, &plazaModel, key.Group)
	require.Equal(t, *plazaModel.Pricing.PerRequestPrice, snapshot.TotalCost)
}

func TestAudioSpeechBillingRejectsMissingOrIncompatiblePricing(t *testing.T) {
	price := 0.2
	negative, notNumber := -0.1, math.NaN()
	for _, tc := range []struct {
		name string
		card *ChannelModelPricing
	}{
		{name: "missing"},
		{name: "empty", card: &ChannelModelPricing{BillingMode: BillingModePerRequest}},
		{name: "token", card: &ChannelModelPricing{BillingMode: BillingModeToken, PerRequestPrice: &price}},
		{name: "image", card: &ChannelModelPricing{BillingMode: BillingModeImage, PerRequestPrice: &price}},
		{name: "negative", card: &ChannelModelPricing{BillingMode: BillingModePerRequest, PerRequestPrice: &negative}},
		{name: "not-number", card: &ChannelModelPricing{BillingMode: BillingModePerRequest, PerRequestPrice: &notNumber}},
		{name: "unrelated-tier", card: &ChannelModelPricing{BillingMode: BillingModePerRequest,
			Intervals: []PricingInterval{{TierLabel: "4k", PerRequestPrice: &price}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := audioSpeechPricingTestKey(price)
			key.Group.ModelPricing = nil
			key.Group.AudioTTSPricePerMillionChars = &price
			if tc.card != nil {
				card := tc.card.Clone()
				card.Models = []string{audioSpeechPricingTestModel}
				key.Group.ModelPricing = []ChannelModelPricing{card}
			}
			snapshot, err := audioSpeechPricingTestService().PrepareAudioSpeechBilling(context.Background(), key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
			require.ErrorIs(t, err, ErrAudioSpeechPricingNotConfigured)
			require.Nil(t, snapshot, "legacy Grok rates must not make binary TTS free or character-billed")
		})
	}
}

func TestAudioSpeechBillingRejectsTieredCardsBeforeSynthesis(t *testing.T) {
	ctx := context.Background()
	svc := audioSpeechPricingTestService()
	for _, tierPrice := range []float64{0, 0.01, 0.2} {
		for _, label := range []string{"", "Kore", "4k"} {
			key := audioSpeechPricingTestKey(0.2)
			key.Group.ModelPricing[0].Intervals = []PricingInterval{{
				MinTokens: 0, TierLabel: label, PerRequestPrice: &tierPrice,
			}}
			snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
			require.ErrorIs(t, err, ErrAudioSpeechPricingNotConfigured)
			require.Nil(t, snapshot, "a priced tier must not be silently ignored or selected using a synthetic zero token count")
		}
	}
	key := audioSpeechPricingTestKey(0.2)
	key.Group.ModelPricing[0].Intervals = []PricingInterval{{MinTokens: 0}}
	snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err, "an empty editor row is not an effective pricing tier")
	require.InDelta(t, 0.2, snapshot.TotalCost, 1e-12)
	key.Group.ModelPricing[0].Intervals = nil
	snapshot, err = svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	require.InDelta(t, 0.2, snapshot.TotalCost, 1e-12)
}

func TestAudioSpeechBillingHonorsChannelAndAccountMappings(t *testing.T) {
	const requested, channelMapped, upstream = "speech-public", "speech-channel", "speech-provider"
	priceRequested, priceChannel, priceUpstream := 0.1, 0.2, 0.3
	key := audioSpeechPricingTestKey(1)
	key.Group.ModelPricing = []ChannelModelPricing{
		{Models: []string{requested}, BillingMode: BillingModePerRequest, PerRequestPrice: &priceRequested},
		{Models: []string{channelMapped}, BillingMode: BillingModePerRequest, PerRequestPrice: &priceChannel},
		{Models: []string{upstream}, BillingMode: BillingModePerRequest, PerRequestPrice: &priceUpstream},
	}
	account := &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{channelMapped: upstream},
	}}
	svc := audioSpeechPricingTestService()
	for _, tc := range []struct {
		source, model string
		price         float64
	}{
		{BillingModelSourceRequested, requested, priceRequested},
		{BillingModelSourceChannelMapped, channelMapped, priceChannel},
		{BillingModelSourceUpstream, upstream, priceUpstream},
		{BillingModelSourceResponse, upstream, priceUpstream},
	} {
		t.Run(tc.source, func(t *testing.T) {
			snapshot, err := svc.PrepareAudioSpeechBilling(context.Background(), key, account, requested,
				ChannelMappingResult{Mapped: true, MappedModel: channelMapped, BillingModelSource: tc.source})
			require.NoError(t, err)
			require.Equal(t, tc.model, snapshot.BillingModel)
			require.InDelta(t, tc.price, snapshot.TotalCost, 1e-12)
		})
	}
}

func TestAudioSpeechBillingCompositePublicModelAndLogSnapshot(t *testing.T) {
	const publicModel, routedModel, channelModel, upstreamModel = "speech-public", "speech-route", "speech-channel", "speech-provider"
	for _, tc := range []struct {
		source, billedModel string
		price               float64
	}{
		{BillingModelSourceRequested, publicModel, 0.1},
		{BillingModelSourceChannelMapped, channelModel, 0.3},
		{BillingModelSourceUpstream, upstreamModel, 0.4},
		{BillingModelSourceResponse, upstreamModel, 0.4},
	} {
		t.Run(tc.source, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)
			key := audioSpeechPricingTestKey(1)
			key.Group.Platform = PlatformComposite
			key.Group.ModelPricing = nil
			for i, model := range []string{publicModel, routedModel, channelModel, upstreamModel} {
				price := float64(i+1) / 10
				key.Group.ModelPricing = append(key.Group.ModelPricing, ChannelModelPricing{
					Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: &price,
				})
			}
			account := &Account{ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"model_mapping": map[string]any{channelModel: upstreamModel},
			}}
			ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
				Matched: true, GroupID: *key.GroupID, PublicModel: publicModel,
				UpstreamModel: routedModel, TargetPlatform: PlatformGemini,
			})
			mapping := ChannelMappingResult{Mapped: true, MappedModel: channelModel, BillingModelSource: tc.source}
			// This is the body model after the composite middleware has rewritten it.
			snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, account, routedModel, mapping)
			require.NoError(t, err)
			require.Equal(t, tc.billedModel, snapshot.BillingModel)
			require.InDelta(t, tc.price, snapshot.TotalCost, 1e-12)
			key.Group.RateMultiplier = 9 // Configuration changes while upstream generates audio.
			newPrice := 100.0
			for i := range key.Group.ModelPricing {
				key.Group.ModelPricing[i].PerRequestPrice = &newPrice
			}
			err = svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{RequestID: "audio_speech:composite-" + tc.source,
					Model: channelModel, UpstreamModel: upstreamModel,
					AudioSpeech: true, AudioSpeechBilling: snapshot},
				APIKey: key, User: key.User, Account: account,
				ChannelUsageFields: mapping.ToUsageFields(publicModel, upstreamModel),
			})
			require.NoError(t, err)
			require.Equal(t, snapshot.BillingModel, usageRepo.lastLog.Model)
			require.Equal(t, publicModel, usageRepo.lastLog.RequestedModel)
			require.Equal(t, upstreamModel, *usageRepo.lastLog.UpstreamModel)
			require.InDelta(t, 0.5, usageRepo.lastLog.RateMultiplier, 1e-12)
			require.InDelta(t, tc.price*0.5, usageRepo.lastLog.ActualCost, 1e-12)
			require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
		})
	}
}

func TestAudioSpeechBillingCompositeFallbackLogUsesActualPricedModel(t *testing.T) {
	const publicModel, routedModel = "unpriced-public-speech", "priced-provider-speech"
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	key := audioSpeechPricingTestKey(0.6)
	key.Group.Platform = PlatformComposite
	key.Group.ModelPricing[0].Models = []string{routedModel}
	ctx := WithCompositeRouteDecision(context.Background(), CompositeRouteDecision{
		Matched: true, PublicModel: publicModel, UpstreamModel: routedModel, TargetPlatform: PlatformGemini,
	})
	mapping := ChannelMappingResult{MappedModel: routedModel, BillingModelSource: BillingModelSourceRequested}
	account := &Account{ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey}
	snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, account, routedModel, mapping)
	require.NoError(t, err)
	require.Equal(t, routedModel, snapshot.BillingModel)
	err = svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "audio_speech:fallback", Model: routedModel, UpstreamModel: routedModel,
			AudioSpeech: true, AudioSpeechBilling: snapshot},
		APIKey: key, User: key.User, Account: account,
		ChannelUsageFields: mapping.ToUsageFields(publicModel, routedModel),
	})
	require.NoError(t, err)
	require.Equal(t, routedModel, usageRepo.lastLog.Model, "requested-source metadata must not replace the actual priced model label")
	require.Equal(t, publicModel, usageRepo.lastLog.RequestedModel)
	require.InDelta(t, 0.3, usageRepo.lastLog.ActualCost, 1e-12)
}

func TestAudioSpeechBillingSnapshotKeepsReservationAndSettlementStable(t *testing.T) {
	ctx := context.Background()
	svc := audioSpeechPricingTestService()
	key := audioSpeechPricingTestKey(0.4)
	snapshot, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	newPrice := 5.0
	key.Group.ModelPricing[0].PerRequestPrice = &newPrice
	key.Group.RateMultiplier = 9
	estimate, priced := svc.EstimateInflightReservation(ctx, key, snapshot.InflightEstimate())
	require.True(t, priced)
	cost, err := snapshot.cost()
	require.NoError(t, err)
	require.InDelta(t, 0.2, estimate, 1e-12)
	require.Equal(t, estimate, cost.ActualCost)
	next, err := svc.PrepareAudioSpeechBilling(ctx, key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	require.InDelta(t, 45, next.ActualCost, 1e-12, "the next request follows the updated model card")
	_, err = svc.calculateOpenAIRecordUsageCost(ctx, &OpenAIForwardResult{AudioSpeech: true}, key,
		[]string{audioSpeechPricingTestModel}, 1, 1, 1, 1, UsageTokens{}, "", nil, false, time.Time{})
	require.ErrorIs(t, err, ErrAudioSpeechPricingUnavailable, "a missing snapshot must not silently bill zero")
}

func TestAudioSpeechRecordUsageBillsZeroTokenAudioAndPreservesDurableID(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	key := audioSpeechPricingTestKey(0.4)
	snapshot, err := svc.PrepareAudioSpeechBilling(context.Background(), key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "reused-client-request")
	for _, id := range []string{"audio_speech:first", "audio_speech:second"} {
		err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{RequestID: id, Model: audioSpeechPricingTestModel, UpstreamModel: audioSpeechPricingTestModel,
				AudioSpeech: true, AudioSpeechBilling: snapshot},
			APIKey: key, User: key.User, Account: &Account{ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey},
			InboundEndpoint: "/v1/audio/speech", UpstreamEndpoint: "/v1/audio/speech",
		})
		require.NoError(t, err)
		require.Equal(t, id, usageRepo.lastLog.RequestID)
		require.Equal(t, 0, usageRepo.lastLog.TotalTokens())
		require.InDelta(t, 0.2, usageRepo.lastLog.ActualCost, 1e-12)
		require.InDelta(t, 0.5, usageRepo.lastLog.RateMultiplier, 1e-12)
		require.Equal(t, string(BillingModePerRequest), *usageRepo.lastLog.BillingMode)
	}
	require.Equal(t, 2, userRepo.deductCalls)
	require.InDelta(t, 0.2, userRepo.lastAmount, 1e-12)
	err = svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "audio_speech:missing-price", Model: audioSpeechPricingTestModel, AudioSpeech: true},
		APIKey: key, User: key.User, Account: &Account{ID: 8, Platform: PlatformGemini, Type: AccountTypeAPIKey},
	})
	require.ErrorIs(t, err, ErrAudioSpeechPricingUnavailable)
	require.Equal(t, 2, userRepo.deductCalls)
	require.Equal(t, 2, usageRepo.calls, "missing pricing cannot produce a misleading successful zero-cost log")
}

func TestAudioSpeechBillingHonorsUserGroupMultiplier(t *testing.T) {
	userRate := 0.25
	rateRepo := &openAIUserGroupRateRepoStub{rate: &userRate}
	svc := newOpenAIRecordUsageServiceForTest(&openAIRecordUsageLogRepoStub{}, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, rateRepo)
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	key := audioSpeechPricingTestKey(0.8)
	snapshot, err := svc.PrepareAudioSpeechBilling(context.Background(), key, nil, audioSpeechPricingTestModel, ChannelMappingResult{})
	require.NoError(t, err)
	require.InDelta(t, userRate, snapshot.RateMultiplier, 1e-12)
	require.InDelta(t, 0.2, snapshot.ActualCost, 1e-12)
}

func TestAudioSpeechBillingDoesNotChangeGrokCharacterPricing(t *testing.T) {
	svc := audioSpeechPricingTestService()
	key := audioSpeechPricingTestKey(9)
	charPrice := 20.0
	key.Group.AudioTTSPricePerMillionChars = &charPrice
	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), &OpenAIForwardResult{
		AudioUsage: &AudioUsage{Mode: "tts", DurationOrUnits: 0.001},
	}, key, []string{"tts"}, 0.5, 0.5, 0.5, 0.5, UsageTokens{}, "", nil, false, time.Time{})
	require.NoError(t, err)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
}

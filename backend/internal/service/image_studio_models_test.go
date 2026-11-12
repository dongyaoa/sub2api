package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func imageStudioModelIDs(models []ImageStudioModel) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func TestImageStudioModelsUsesAllowlistAndConfiguredAliases(t *testing.T) {
	group := &Group{ID: 13, Platform: PlatformGemini, ModelAllowlist: GroupModelAllowlist{
		Enabled: true,
		Models:  []string{"gemini-nano-banana-2.1", "custom-banana", "wildcard-banana", "gemini-text", "gemini-other-*"},
	}}
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {
		{ID: 1, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
			"custom-banana":       "gemini-3-pro-image",
			"wildcard-*":          "gemini-nano-banana-2.1",
			"gemini-text":         "gemini-2.5-pro",
			"gemini-other-image":  "gemini-other-image",
			"gemini-hidden-image": "gemini-hidden-image",
		}}},
		{ID: 2, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-other-openai-image": "gpt-image-2"}}},
	}}}}

	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, []string{"gemini-default-image"})
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-nano-banana-2.1", "custom-banana", "wildcard-banana", "gemini-other-image"}, imageStudioModelIDs(models))
	for _, model := range models {
		require.True(t, model.ImageGeneration)
		require.NotContains(t, model.ID, "*")
	}
}

func TestImageStudioModelsIncludesSyncedUpstreamCatalog(t *testing.T) {
	group := &Group{ID: 14, Platform: PlatformGemini}
	account := Account{ID: 1, Platform: PlatformGemini}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gemini-nano-banana-2.1": {ID: "gemini-nano-banana-2.1"},
		"gemini-2.5-pro":         {ID: "gemini-2.5-pro"},
	}})
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {account}}}}
	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, []string{"gemini-default-image", "gemini-default-text"})
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-default-image", "gemini-nano-banana-2.1"}, imageStudioModelIDs(models))
}

func TestImageStudioModelsRecognizesModelPricingForArbitraryAliases(t *testing.T) {
	const groupID = 100
	perRequestPrice := 0.2
	group := &Group{ID: groupID, Platform: PlatformGemini, ModelAllowlist: GroupModelAllowlist{
		Enabled: true, Models: []string{"CustomBanana2", "per-request-banana", "chat-model"},
	}, ModelPricing: []ChannelModelPricing{
		{Platform: PlatformGemini, Models: []string{"per-request-banana"}, BillingMode: BillingModePerRequest, PerRequestPrice: &perRequestPrice},
		{Platform: PlatformGemini, Models: []string{"chat-model"}, BillingMode: BillingModeToken},
	}}
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "custombanana2", 0.1)
	svc := &GatewayService{
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {{
			ID: 1, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
				"per-request-banana": "gemini-3-pro-image",
			}},
		}}}},
		resolver: resolver,
	}
	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"CustomBanana2", "per-request-banana"}, imageStudioModelIDs(models))
}

func TestImageStudioModelsRecognizesChannelMappedAliasesAndRestrictions(t *testing.T) {
	const groupID = 101
	group := &Group{ID: groupID, Platform: PlatformGemini, ModelAllowlist: GroupModelAllowlist{
		Enabled: true, Models: []string{"custom-art", "blocked-art"},
	}}
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "gemini-nano-banana-2.1", 0.1)
	cache, ok := resolver.channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: "custom-art"}] = "gemini-nano-banana-2.1"
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: "blocked-art"}] = "gemini-unpriced-image"
	cache.channelByGroupID[groupID].BillingModelSource = BillingModelSourceChannelMapped
	cache.channelByGroupID[groupID].RestrictModels = true
	svc := &GatewayService{
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {{ID: 1, Platform: PlatformGemini}}}},
		resolver:    resolver,
	}
	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"custom-art"}, imageStudioModelIDs(models))
}

func TestImageStudioModelsRejectsVideoNamesRegardlessOfPriceCard(t *testing.T) {
	videoModels := []string{
		"gemini-omni-1.2-flash", "gemini-omni-1.2", "omni-1.2", "omni-featured",
		"models/GEMINI-OMNI-1.2-FLASH", "muse-video", "grok-imagine-video",
		"grok-imagine-video-1.5", "grok-imagine-video-1.5-preview", "svggrok-imagine-video-1.5",
		"veo-3.1-generate-preview", "sora2", "seedance-1.0", "provider/image-to-video",
		"custom-t2v-model", "custom-i2v-model", "models/videos/custom",
	}
	for _, mode := range []BillingMode{BillingModePerRequest, BillingModeImage} {
		t.Run(string(mode), func(t *testing.T) {
			group := &Group{ID: 102, Platform: PlatformGemini, ModelAllowlist: GroupModelAllowlist{
				Enabled: true, Models: append(append([]string{}, videoModels...), "gemini-3-pro-image", "custom-art", "chat-per-call"),
			}}
			price := 0.05
			group.ModelPricing = []ChannelModelPricing{
				{Platform: PlatformGemini, Models: videoModels, BillingMode: mode, PerRequestPrice: &price},
				{Platform: PlatformGemini, Models: []string{"gemini-3-pro-image", "chat-per-call"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price},
			}
			svc := &GatewayService{
				accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {{ID: 1, Platform: PlatformGemini}}}},
				resolver:    newOpenAIImageChannelPricingResolverForTest(t, group.ID, "custom-art", 0.1),
			}
			models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, nil)
			require.NoError(t, err)
			require.Equal(t, []string{"gemini-3-pro-image", "custom-art"}, imageStudioModelIDs(models))
		})
	}
}

func TestImageStudioModelsRejectsVideoRoutingTargetsBeforeImageEvidence(t *testing.T) {
	price := 0.05
	group := &Group{ID: 103, Platform: PlatformGemini, ModelAllowlist: GroupModelAllowlist{
		Enabled: true, Models: []string{
			"image-channel-alias", "image-account-alias", "image-wildcard-alias", "priced-alias",
			"image-mixed-alias", "image-video-card", "motion-channel-alias", "motion-account-alias", "pro-alias",
		},
	}, ModelPricing: []ChannelModelPricing{
		{Platform: PlatformGemini, Models: []string{"priced-alias"}, BillingMode: BillingModeImage, PerRequestPrice: &price},
		{Platform: PlatformGemini, Models: []string{"image-video-card", "custom-motion"}, BillingMode: BillingModeVideo, PerRequestPrice: &price},
		{Platform: PlatformGemini, Models: []string{"pro-alias"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price},
	}}
	resolver := newOpenAIImageChannelPricingResolverForTest(t, group.ID, "image-channel-alias", 0.1)
	cache, ok := resolver.channelService.cache.Load().(*channelCache)
	require.True(t, ok)
	cache.mappingByGroupModel[channelModelKey{groupID: group.ID, model: "image-channel-alias"}] = "gemini-omni-1.2-flash"
	cache.mappingByGroupModel[channelModelKey{groupID: group.ID, model: "motion-channel-alias"}] = "custom-motion"
	svc := &GatewayService{
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {
			{ID: 1, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
				"image-account-alias": "omni-featured", "image-wildcard-*": "gemini-omni-1.2",
				"priced-alias": "muse-video", "image-mixed-alias": "gemini-3-pro-image",
				"motion-account-alias": "custom-motion", "pro-alias": "gemini-3-pro-image",
			}}},
			{ID: 2, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{
				"image-mixed-alias": "grok-imagine-video-1.5",
			}}},
		}}},
		resolver: resolver,
	}
	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"pro-alias"}, imageStudioModelIDs(models))
}

func TestImageStudioModelsFiltersVideosFromSyncedCatalogAndDefaults(t *testing.T) {
	group := &Group{ID: 104, Platform: PlatformGemini}
	account := Account{ID: 1, Platform: PlatformGemini}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gemini-omni-1.2-flash": {ID: "gemini-omni-1.2-flash"},
		"gemini-3-pro-image":    {ID: "gemini-3-pro-image"},
	}})
	price := 0.05
	group.ModelPricing = []ChannelModelPricing{{
		Platform: PlatformGemini, Models: []string{"*"}, BillingMode: BillingModePerRequest, PerRequestPrice: &price,
	}}
	svc := &GatewayService{
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {account}}},
		resolver:    newOpenAIImageChannelPricingResolverForTest(t, group.ID, "unused-art", 0.1),
	}
	models, err := svc.ImageStudioModels(context.Background(), group, PlatformGemini, []string{"omni-featured", "chat-per-call"})
	require.NoError(t, err)
	require.Equal(t, []string{"gemini-3-pro-image"}, imageStudioModelIDs(models))
}

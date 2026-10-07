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
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {{ID: 1, Platform: PlatformGemini}}}},
		resolver:    resolver,
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

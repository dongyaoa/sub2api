package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiOmniVideoStudioCatalogUsesGeminiAccountConfiguration(t *testing.T) {
	group := &Group{ID: 98, Platform: PlatformGemini}
	account := Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://video.example/v1"}}
	for _, tt := range []struct {
		name      string
		configure func(*Account)
		want      []string
	}{
		{"unconfigured models stay hidden", func(a *Account) {}, []string{}},
		{"synced catalog", func(a *Account) {
			a.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
				omniPricingTestModel: {ID: omniPricingTestModel}, "gemini-2.5-flash": {ID: "gemini-2.5-flash"},
			}})
		}, []string{omniPricingTestModel}},
		{"explicit mapping", func(a *Account) {
			a.Credentials["model_mapping"] = map[string]any{"omni-featured": "provider-target", "gemini-2.5-flash": "gemini-2.5-flash"}
		}, []string{"omni-featured"}},
		{"OAuth is not supported", func(a *Account) {
			a.Type = AccountTypeOAuth
			a.Credentials["model_mapping"] = map[string]any{omniPricingTestModel: omniPricingTestModel}
		}, []string{}},
		{"default Google endpoint is not supported", func(a *Account) {
			a.Credentials["base_url"] = ""
			a.Credentials["model_mapping"] = map[string]any{omniPricingTestModel: omniPricingTestModel}
		}, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidate := account
			candidate.Credentials = map[string]any{"base_url": "https://video.example/v1"}
			tt.configure(&candidate)
			svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {candidate}}}}
			models, err := svc.VideoStudioModels(context.Background(), group)
			require.NoError(t, err)
			require.Equal(t, tt.want, models)
		})
	}
}

func TestGeminiOmniVideoStudioCatalogCompositeAndAllowlist(t *testing.T) {
	group := &Group{ID: 99, Platform: PlatformComposite}
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"base_url": "https://video.example", "model_mapping": map[string]any{omniPricingTestModel: omniPricingTestModel},
		}},
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeAPIKey},
	}}}}
	models, err := svc.VideoStudioModels(context.Background(), group)
	require.NoError(t, err)
	require.Equal(t, []string{omniPricingTestModel, "grok-imagine-video", "grok-imagine-video-1.5"}, models)
	group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{omniPricingTestModel}}
	models, err = svc.VideoStudioModels(context.Background(), group)
	require.NoError(t, err)
	require.Equal(t, []string{omniPricingTestModel}, models)
}

func TestGeminiOmniVideoCapabilityRequiresCustomGeminiAPIKey(t *testing.T) {
	for _, tt := range []struct {
		platform string
		kind     string
		base     string
		want     bool
	}{
		{PlatformGemini, AccountTypeAPIKey, "https://video.example", true},
		{PlatformGemini, AccountTypeAPIKey, "", false},
		{PlatformGemini, AccountTypeOAuth, "https://video.example", false},
		{PlatformOpenAI, AccountTypeAPIKey, "https://video.example", false},
		{PlatformGrok, AccountTypeAPIKey, "https://video.example", false},
	} {
		a := &Account{Platform: tt.platform, Type: tt.kind, Credentials: map[string]any{"base_url": tt.base}}
		require.Equal(t, tt.want, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityGeminiOmniVideo))
	}
}

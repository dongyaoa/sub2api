package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoStudioModelsUsesSyncedMuseCatalogAndFiltersAccountTypes(t *testing.T) {
	group := &Group{ID: 15, Platform: PlatformOpenAI}
	account := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://muse.example/v1"}}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"muse-video": {ID: "muse-video"}, "muse-chat": {ID: "muse-chat"},
	}})
	for _, tt := range []struct {
		name   string
		typeID string
		want   []string
	}{
		{"custom API key", AccountTypeAPIKey, []string{"muse-video"}},
		{"OAuth is not a video upstream", AccountTypeOAuth, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account.Type = tt.typeID
			svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {account}}}}
			models, err := svc.VideoStudioModels(context.Background(), group)
			require.NoError(t, err)
			require.Equal(t, tt.want, models)
		})
	}
}

func TestVideoStudioModelsCompositeAndAllowlist(t *testing.T) {
	group := &Group{ID: 16, Platform: PlatformComposite}
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"base_url": "https://muse.example", "model_mapping": map[string]any{"muse-video": "muse-video", "muse-image": "muse-image"},
		}},
		{ID: 2, Platform: PlatformGrok, Type: AccountTypeAPIKey},
	}}}}
	models, err := svc.VideoStudioModels(context.Background(), group)
	require.NoError(t, err)
	require.Equal(t, []string{"grok-imagine-video", "grok-imagine-video-1.5", "muse-video"}, models)
	group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"muse-video"}}
	models, err = svc.VideoStudioModels(context.Background(), group)
	require.NoError(t, err)
	require.Equal(t, []string{"muse-video"}, models)
}

func TestVideoStudioModelsDoesNotInventMuseForGenericOpenAIAccount(t *testing.T) {
	group := &Group{ID: 17, Platform: PlatformOpenAI}
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://other.example"}},
	}}}}
	models, err := svc.VideoStudioModels(context.Background(), group)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestVideoStudioModelsUsesActualGrokCatalog(t *testing.T) {
	group := &Group{ID: 18, Platform: PlatformGrok}
	for _, tc := range []struct {
		name    string
		mapping map[string]any
		synced  []string
		want    []string
	}{
		{name: "explicit text models", mapping: map[string]any{"grok-4.5": "grok-4.5"}, want: []string{}},
		{name: "synchronized text models", synced: []string{"grok-4.5"}, want: []string{}},
		{name: "synchronized video", synced: []string{"grok-4.5", "grok-imagine-video"}, want: []string{"grok-imagine-video"}},
		{name: "explicit supported alias", mapping: map[string]any{"svggrok-imagine-video-1.5": "grok-imagine-video-1.5"}, want: []string{"svggrok-imagine-video-1.5"}},
		{name: "explicit model overrides catalog", mapping: map[string]any{"grok-imagine-video-1.5": "grok-imagine-video-1.5"}, synced: []string{"grok-4.5"}, want: []string{"grok-imagine-video-1.5"}},
		{name: "wildcard video mapping", mapping: map[string]any{"grok-imagine-video*": "grok-imagine-video"}, want: []string{"grok-imagine-video", "grok-imagine-video-1.5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"model_mapping": tc.mapping}}
			if tc.synced != nil {
				models := make(map[string]UpstreamModelMetadata)
				for _, model := range tc.synced {
					models[model] = UpstreamModelMetadata{ID: model}
				}
				account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: models})
			}
			svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{group.ID: {account}}}}
			models, err := svc.VideoStudioModels(context.Background(), group)
			require.NoError(t, err)
			require.Equal(t, tc.want, models)
		})
	}
}

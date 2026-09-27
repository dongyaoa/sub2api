//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserAvailableChannel_Unauthenticated401(t *testing.T) {
	// 没有 AuthSubject 注入时，handler 应返回 401 且不触达 service 依赖。
	gin.SetMode(gin.TestMode)
	h := &AvailableChannelHandler{} // nil services — 401 路径不会调用它们
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/channels/available", nil)

	h.List(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestModelSquareCatalog_Unauthenticated401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AvailableChannelHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-square/catalog", nil)
	h.ListModelSquare(c)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

type squareCatalogRepo struct {
	service.ChannelRepository
	channels []service.Channel
}

func (r *squareCatalogRepo) ListAll(context.Context) ([]service.Channel, error) {
	return r.channels, nil
}

type squareGroupRepo struct {
	service.GroupRepository
	groups []service.Group
}

func (r *squareGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return r.groups, nil
}

type squareUserRepo struct{ service.UserRepository }

func (*squareUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id}, nil
}

type squareSubscriptionRepo struct {
	service.UserSubscriptionRepository
}

func (*squareSubscriptionRepo) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

type squareSettingRepo struct {
	service.SettingRepository
	enabled string
}

func (r *squareSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{service.SettingKeyAvailableChannelsEnabled: r.enabled}, nil
}

func TestModelSquareCatalog_IndependentSwitchPreservesVisibilityAndPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groups := &squareGroupRepo{groups: []service.Group{
		{ID: 1, Name: "public", Platform: "openai", Status: service.StatusActive},
		{ID: 2, Name: "private", Platform: "anthropic", Status: service.StatusActive, IsExclusive: true},
	}}
	price := 0.04
	pricing := []service.ChannelModelPricing{
		{Platform: "openai", Models: []string{"test-image"}, BillingMode: service.BillingModeImage,
			Intervals: []service.PricingInterval{{TierLabel: "1K", PerRequestPrice: &price}}},
		{Platform: "anthropic", Models: []string{"private-model"}},
	}
	channels := &squareCatalogRepo{channels: []service.Channel{
		{Name: "visible", Status: service.StatusActive, GroupIDs: []int64{1, 2}, ModelPricing: pricing},
		{Name: "private", Status: service.StatusActive, GroupIDs: []int64{2}, ModelPricing: pricing},
		{Name: "disabled", Status: "inactive", GroupIDs: []int64{1}, ModelPricing: pricing},
	}}
	settings := &squareSettingRepo{enabled: "false"}
	h := NewAvailableChannelHandler(
		service.NewChannelService(channels, groups, nil, nil, nil),
		service.NewAPIKeyService(nil, &squareUserRepo{}, groups, &squareSubscriptionRepo{}, nil, nil, nil),
		service.NewSettingService(settings, nil),
	)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	})
	router.GET("/channels/available", h.List)
	router.GET("/model-square/catalog", h.ListModelSquare)
	get := func(path string) []userAvailableChannel {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Data []userAvailableChannel `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body.Data
	}

	// Disabling the old page must not empty the independent catalog.
	require.Empty(t, get("/channels/available"))
	catalog := get("/model-square/catalog")
	require.Len(t, catalog, 1)
	require.Equal(t, "visible", catalog[0].Name)
	require.Len(t, catalog[0].Platforms, 1)
	section := catalog[0].Platforms[0]
	require.Equal(t, "openai", section.Platform)
	require.Len(t, section.Groups, 1)
	require.Equal(t, int64(1), section.Groups[0].ID)
	require.Len(t, section.SupportedModels, 1)
	model := section.SupportedModels[0]
	require.Equal(t, "test-image", model.Name)
	require.NotNil(t, model.Pricing)
	require.Equal(t, "image", model.Pricing.BillingMode)
	require.Len(t, model.Pricing.Intervals, 1)
	require.Equal(t, "1K", model.Pricing.Intervals[0].TierLabel)
	require.Equal(t, &price, model.Pricing.Intervals[0].PerRequestPrice)

	settings.enabled = "true"
	require.Equal(t, catalog, get("/channels/available"))
	require.Equal(t, catalog, get("/model-square/catalog"))
}

func TestFilterUserVisibleGroups_IntersectionOnly(t *testing.T) {
	// 渠道挂在 {g1, g2, g3}，用户只允许 {g1, g3} —— 响应必须仅含 g1/g3。
	groups := []service.AvailableGroupRef{
		{ID: 1, Name: "g1", Platform: "anthropic"},
		{ID: 2, Name: "g2", Platform: "anthropic"},
		{ID: 3, Name: "g3", Platform: "openai"},
	}
	allowed := map[int64]struct{}{1: {}, 3: {}}

	visible := filterUserVisibleGroups(groups, allowed)
	require.Len(t, visible, 2)
	ids := []int64{visible[0].ID, visible[1].ID}
	require.ElementsMatch(t, []int64{1, 3}, ids)
}

func TestToUserSupportedModels_FiltersByAllowedPlatforms(t *testing.T) {
	// 用户可访问分组只覆盖 anthropic；anthropic 平台的模型保留，openai 模型被剔除。
	src := []service.SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: nil},
		{Name: "gpt-4o", Platform: "openai", Pricing: nil},
	}
	allowed := map[string]struct{}{"anthropic": {}}
	out := toUserSupportedModels(src, allowed)
	require.Len(t, out, 1)
	require.Equal(t, "claude-sonnet-4-6", out[0].Name)
}

func TestToUserSupportedModels_NilAllowedPlatformsKeepsAll(t *testing.T) {
	// 显式传 nil allowedPlatforms 表示不做过滤。
	src := []service.SupportedModel{
		{Name: "a", Platform: "anthropic"},
		{Name: "b", Platform: "openai"},
	}
	require.Len(t, toUserSupportedModels(src, nil), 2)
}

func TestUserAvailableChannel_FieldWhitelist(t *testing.T) {
	// 通过序列化 userAvailableChannel 结构体验证响应形状：
	// 只有 name / description / platforms；不含管理端字段。
	row := userAvailableChannel{
		Name:        "ch",
		Description: "d",
		Platforms: []userChannelPlatformSection{
			{
				Platform:        "anthropic",
				Groups:          []userAvailableGroup{{ID: 1, Name: "g1", Platform: "anthropic"}},
				SupportedModels: []userSupportedModel{},
			},
		},
	}
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"id", "status", "billing_model_source", "restrict_models"} {
		_, exists := decoded[key]
		require.Falsef(t, exists, "user DTO must not expose %q", key)
	}
	for _, key := range []string{"name", "description", "platforms"} {
		_, exists := decoded[key]
		require.Truef(t, exists, "user DTO must expose %q", key)
	}

	// 验证 section 的字段（platform / groups / supported_models）。
	rawSection, err := json.Marshal(row.Platforms[0])
	require.NoError(t, err)
	var sectionDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawSection, &sectionDecoded))
	for _, key := range []string{"platform", "groups", "supported_models"} {
		_, exists := sectionDecoded[key]
		require.Truef(t, exists, "platform section must expose %q", key)
	}

	// Group DTO 暴露区分专属/公开、订阅类型、默认倍率和高峰倍率规则所需的字段，
	// 前端据此渲染 GroupBadge 并与 API 密钥页保持一致的视觉。
	rawGroup, err := json.Marshal(row.Platforms[0].Groups[0])
	require.NoError(t, err)
	var groupDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawGroup, &groupDecoded))
	for _, key := range []string{"id", "name", "platform", "subscription_type", "rate_multiplier", "peak_rate_enabled", "peak_start", "peak_end", "peak_rate_multiplier", "is_exclusive"} {
		_, exists := groupDecoded[key]
		require.Truef(t, exists, "group DTO must expose %q", key)
	}

	// pricing interval 白名单：不应暴露 id / sort_order。
	inputMultiplier := 2.0
	outputMultiplier := 1.5
	cacheWriteMultiplier := 2.0
	cacheReadMultiplier := 2.0
	pricing := toUserPricing(&service.ChannelModelPricing{
		BillingMode: service.BillingModeToken,
		Intervals: []service.PricingInterval{
			{
				ID: 7, MinTokens: 0, MaxTokens: nil, SortOrder: 3,
				InputMultiplier: &inputMultiplier, OutputMultiplier: &outputMultiplier,
				CacheWriteMultiplier: &cacheWriteMultiplier, CacheReadMultiplier: &cacheReadMultiplier,
			},
		},
	})
	require.NotNil(t, pricing)
	require.Len(t, pricing.Intervals, 1)
	rawIv, err := json.Marshal(pricing.Intervals[0])
	require.NoError(t, err)
	var ivDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawIv, &ivDecoded))
	for _, key := range []string{"id", "pricing_id", "sort_order"} {
		_, exists := ivDecoded[key]
		require.Falsef(t, exists, "user pricing interval must not expose %q", key)
	}
	for key, want := range map[string]float64{
		"input_multiplier": inputMultiplier, "output_multiplier": outputMultiplier,
		"cache_write_multiplier": cacheWriteMultiplier, "cache_read_multiplier": cacheReadMultiplier,
	} {
		got, exists := ivDecoded[key]
		require.Truef(t, exists, "user pricing interval must expose %q", key)
		require.InDelta(t, want, got.(float64), 1e-12)
	}
}

func TestBuildPlatformSections_GroupsByPlatform(t *testing.T) {
	// 一个渠道横跨 anthropic / openai / 空平台：应该生成 2 个 section，
	// 按 platform 字母序排序，各自 groups 和 supported_models 只含同平台条目。
	ch := service.AvailableChannel{
		Name: "ch",
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic"},
			{Name: "gpt-4o", Platform: "openai"},
		},
	}
	visible := []userAvailableGroup{
		{ID: 1, Name: "g-openai", Platform: "openai"},
		{ID: 2, Name: "g-ant", Platform: "anthropic"},
		{ID: 3, Name: "g-empty", Platform: ""},
	}
	sections := buildPlatformSections(ch, visible)
	require.Len(t, sections, 2)
	require.Equal(t, "anthropic", sections[0].Platform)
	require.Equal(t, "openai", sections[1].Platform)
	require.Len(t, sections[0].Groups, 1)
	require.Equal(t, int64(2), sections[0].Groups[0].ID)
	require.Len(t, sections[0].SupportedModels, 1)
	require.Equal(t, "claude-sonnet-4-6", sections[0].SupportedModels[0].Name)
}

func TestBuildPlatformSections_CompositeGroupExpandsAcrossConfiguredModelPlatforms(t *testing.T) {
	anthropicPrice := 3e-6
	openAIPrice := 2.5e-6
	ch := service.AvailableChannel{
		Name: "composite-channel",
		SupportedModels: []service.SupportedModel{
			{
				Name:     "claude-sonnet-4-6",
				Platform: service.PlatformAnthropic,
				Pricing:  &service.ChannelModelPricing{InputPrice: &anthropicPrice},
			},
			{
				Name:     "gpt-5",
				Platform: service.PlatformOpenAI,
				Pricing:  &service.ChannelModelPricing{InputPrice: &openAIPrice},
			},
		},
	}
	visible := []userAvailableGroup{
		{ID: 9, Name: "composite", Platform: service.PlatformComposite},
	}

	sections := buildPlatformSections(ch, visible)

	require.Len(t, sections, 2)
	require.Equal(t, service.PlatformAnthropic, sections[0].Platform)
	require.Equal(t, service.PlatformOpenAI, sections[1].Platform)
	for _, section := range sections {
		require.Len(t, section.Groups, 1)
		require.Equal(t, int64(9), section.Groups[0].ID)
		require.Equal(t, service.PlatformComposite, section.Groups[0].Platform)
		require.Len(t, section.SupportedModels, 1)
		require.Equal(t, section.Platform, section.SupportedModels[0].Platform)
		require.NotNil(t, section.SupportedModels[0].Pricing)
	}
	require.Equal(t, "claude-sonnet-4-6", sections[0].SupportedModels[0].Name)
	require.Equal(t, "gpt-5", sections[1].SupportedModels[0].Name)
}

func TestBuildPlatformSections_OrdinaryGroupRemainsPlatformIsolated(t *testing.T) {
	ch := service.AvailableChannel{
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: service.PlatformAnthropic},
			{Name: "gpt-5", Platform: service.PlatformOpenAI},
		},
	}
	visible := []userAvailableGroup{
		{ID: 1, Name: "anthropic-only", Platform: service.PlatformAnthropic},
	}

	sections := buildPlatformSections(ch, visible)

	require.Len(t, sections, 1)
	require.Equal(t, service.PlatformAnthropic, sections[0].Platform)
	require.Len(t, sections[0].SupportedModels, 1)
	require.Equal(t, "claude-sonnet-4-6", sections[0].SupportedModels[0].Name)
}

func TestBuildPlatformSections_CompositeAndOrdinaryGroupsShareConcreteSection(t *testing.T) {
	ch := service.AvailableChannel{
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: service.PlatformAnthropic},
			{Name: "gpt-5", Platform: service.PlatformOpenAI},
		},
	}
	visible := []userAvailableGroup{
		{ID: 1, Name: "anthropic-only", Platform: service.PlatformAnthropic},
		{ID: 9, Name: "composite", Platform: service.PlatformComposite},
	}

	sections := buildPlatformSections(ch, visible)

	require.Len(t, sections, 2)
	require.Equal(t, service.PlatformAnthropic, sections[0].Platform)
	require.Equal(t, []int64{1, 9}, []int64{
		sections[0].Groups[0].ID,
		sections[0].Groups[1].ID,
	})
	require.Equal(t, service.PlatformOpenAI, sections[1].Platform)
	require.Len(t, sections[1].Groups, 1)
	require.Equal(t, int64(9), sections[1].Groups[0].ID)
}

func TestBuildPlatformSections_CompositeWithoutModelsKeepsEmptyCompositeSection(t *testing.T) {
	visible := []userAvailableGroup{
		{ID: 9, Name: "composite", Platform: service.PlatformComposite},
	}

	sections := buildPlatformSections(service.AvailableChannel{
		SupportedModels: []service.SupportedModel{{Name: "invalid-without-platform"}},
	}, visible)

	require.Len(t, sections, 1)
	require.Equal(t, service.PlatformComposite, sections[0].Platform)
	require.Len(t, sections[0].Groups, 1)
	require.Empty(t, sections[0].SupportedModels)
}

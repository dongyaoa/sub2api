package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageStudioPricingChannelRepo struct {
	service.ChannelRepository
	channels []service.Channel
}

func (r *imageStudioPricingChannelRepo) ListAll(context.Context) ([]service.Channel, error) {
	return r.channels, nil
}

func (r *imageStudioPricingChannelRepo) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{7: service.PlatformOpenAI}, nil
}

func newImageStudioPricingHandler(t *testing.T, restrict bool) *GatewayHandler {
	t.Helper()
	price := 0.1
	repo := &imageStudioPricingChannelRepo{channels: []service.Channel{{
		ID: 1, GroupIDs: []int64{7}, Status: service.StatusActive, RestrictModels: restrict,
		ModelPricing: []service.ChannelModelPricing{{
			Platform: service.PlatformOpenAI, Models: []string{"image-custom"}, BillingMode: service.BillingModeImage, PerRequestPrice: &price,
		}},
	}}}
	channel := service.NewChannelService(repo, nil, nil, nil, nil)
	billing := service.NewBillingService(&config.Config{}, nil)
	resolver := service.NewModelPricingResolver(channel, billing)
	openAI := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, billing,
		nil, nil, nil, nil, nil, nil, resolver, channel, nil, nil, nil,
	)
	return &GatewayHandler{openAIGatewayService: openAI}
}

func newImageStudioPricingContext(apiKey *service.APIKey, model string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/images/pricing?model="+model, nil)
	if apiKey != nil {
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	}
	return c, w
}

func TestGatewayHandlerImageStudioPricingContract(t *testing.T) {
	groupID, flat := int64(7), 0.06
	apiKey := &service.APIKey{UserID: 1, GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true, RateMultiplier: 1, ImagePrice1K: &flat,
	}}
	c, w := newImageStudioPricingContext(apiKey, "image-custom")
	newImageStudioPricingHandler(t, false).ImageStudioPricing(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var quote service.ImageStudioPricing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
	require.Equal(t, "image-custom", quote.Model)
	require.Equal(t, "image-custom", quote.BillingModel)
	require.Equal(t, service.BillingModeImage, quote.BillingMode)
	require.Equal(t, service.PricingSourceChannel, quote.PricingSource)
	require.InDelta(t, 1, quote.RateMultiplier, 1e-12)
	require.Len(t, quote.Tiers, 3)
	for _, tier := range quote.Tiers {
		require.InDelta(t, 0.1, tier.BasePrice, 1e-12)
		require.InDelta(t, 0.1, tier.UnitPrice, 1e-12)
	}
	require.False(t, quote.OpenAI4KAllowed)
}

func TestGatewayHandlerImageStudioPricingPermissions(t *testing.T) {
	groupID := int64(7)
	newKey := func() *service.APIKey {
		return &service.APIKey{UserID: 1, GroupID: &groupID, Group: &service.Group{
			ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true, RateMultiplier: 1,
		}}
	}
	for _, tc := range []struct {
		name  string
		key   *service.APIKey
		model string
		want  int
	}{
		{name: "unauthenticated", model: "image-custom", want: http.StatusUnauthorized},
		{name: "missing model", key: newKey(), want: http.StatusBadRequest},
		{name: "ungrouped key", key: &service.APIKey{}, model: "image-custom", want: http.StatusForbidden},
		{name: "image disabled", key: &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID}}, model: "image-custom", want: http.StatusForbidden},
		{name: "group whitelist", key: &service.APIKey{GroupID: &groupID, Group: &service.Group{
			ID: groupID, Platform: service.PlatformOpenAI, AllowImageGeneration: true,
			ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"other-model"}},
		}}, model: "image-custom", want: http.StatusNotFound},
		{name: "channel restriction", key: newKey(), model: "not-priced-model", want: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newImageStudioPricingContext(tc.key, tc.model)
			newImageStudioPricingHandler(t, true).ImageStudioPricing(c)
			require.Equal(t, tc.want, w.Code)
			require.NotContains(t, w.Body.String(), "tiers")
		})
	}
}

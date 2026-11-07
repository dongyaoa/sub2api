package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayHandlerVideoStudioPricingContract(t *testing.T) {
	groupID, price := int64(7), 0.4
	key := &service.APIKey{UserID: 1, GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformOpenAI, RateMultiplier: 0.5,
		VideoRateIndependent: true, VideoRateMultiplier: 99,
		ModelPricing: []service.ChannelModelPricing{{Models: []string{"muse-video"}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &price}},
	}}
	c, w := newImageStudioPricingContext(key, "muse-video")
	newImageStudioPricingHandler(t, false).VideoStudioPricing(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var quote service.VideoStudioPricing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
	require.Equal(t, "muse-video", quote.BillingModel)
	require.Equal(t, service.BillingModePerRequest, quote.BillingMode)
	require.Equal(t, service.PricingSourceGroup, quote.PricingSource)
	require.InDelta(t, 0.5, quote.RateMultiplier, 1e-12)
	require.Len(t, quote.Tiers, 1)
	require.InDelta(t, 0.2, quote.Tiers["auto"].UnitPrice, 1e-12)
}

func TestGatewayHandlerVideoStudioPricingPermissionsAndMissingPrice(t *testing.T) {
	groupID := int64(7)
	key := func(platform string) *service.APIKey {
		return &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: platform, RateMultiplier: 1}}
	}
	for _, tc := range []struct {
		name  string
		key   *service.APIKey
		model string
		want  int
	}{
		{name: "unauthenticated", model: "muse-video", want: http.StatusUnauthorized},
		{name: "missing model", key: key(service.PlatformOpenAI), want: http.StatusBadRequest},
		{name: "ungrouped", key: &service.APIKey{}, model: "muse-video", want: http.StatusForbidden},
		{name: "unsupported platform", key: key(service.PlatformAnthropic), model: "muse-video", want: http.StatusNotFound},
		{name: "missing model price", key: key(service.PlatformOpenAI), model: "muse-video", want: http.StatusServiceUnavailable},
		{name: "group allowlist", key: &service.APIKey{GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI,
			ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"other"}},
		}}, model: "muse-video", want: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newImageStudioPricingContext(tc.key, tc.model)
			newImageStudioPricingHandler(t, false).VideoStudioPricing(c)
			require.Equal(t, tc.want, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), "tiers")
		})
	}
}

func TestGatewayHandlerGeminiOmniPricingUsesPerRequestModelCard(t *testing.T) {
	groupID, price := int64(8), 0.75
	key := &service.APIKey{GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformGemini, RateMultiplier: 0.8,
		ModelPricing: []service.ChannelModelPricing{{Models: []string{"gemini-omni-1.2-flash"}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &price}},
	}}
	c, w := newImageStudioPricingContext(key, "gemini-omni-1.2-flash")
	newImageStudioPricingHandler(t, false).VideoStudioPricing(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var quote service.VideoStudioPricing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
	require.Equal(t, service.BillingModePerRequest, quote.BillingMode)
	for _, tier := range []string{"360p", "720p", "1080p", "4k"} {
		require.True(t, quote.Tiers[tier].Configured)
		require.InDelta(t, 0.6, quote.Tiers[tier].UnitPrice, 1e-12)
	}
}

func TestGatewayHandlerGrokVideoStudioKeepsPerSecondGroupPrices(t *testing.T) {
	groupID, tokenPrice := int64(9), 1.0
	key := &service.APIKey{GroupID: &groupID, Group: &service.Group{
		ID: groupID, Platform: service.PlatformGrok, RateMultiplier: 1,
		VideoModelPrices: map[string]map[string]float64{
			"grok-imagine-video": {"480p": 0.06, "720p": 0.08, "1080p": 0.1},
		},
		ModelPricing: []service.ChannelModelPricing{{Models: []string{"*"}, BillingMode: service.BillingModeToken, InputPrice: &tokenPrice}},
	}}
	c, w := newImageStudioPricingContext(key, "grok-imagine-video")
	newImageStudioPricingHandler(t, false).VideoStudioPricing(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var quote service.VideoStudioPricing
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
	require.Equal(t, service.BillingModeVideo, quote.BillingMode)
	require.InDelta(t, 0.06, quote.Tiers["480p"].UnitPrice, 1e-12)
	require.InDelta(t, 0.08, quote.Tiers["720p"].UnitPrice, 1e-12)
	require.InDelta(t, 0.1, quote.Tiers["1080p"].UnitPrice, 1e-12)
}

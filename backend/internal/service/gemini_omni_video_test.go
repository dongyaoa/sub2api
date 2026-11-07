//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func geminiOmniVideoTestAccount() *Account {
	return &Account{ID: 902, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "omni-test-key", "base_url": "https://gemini.example/proxy/v1beta",
		"model_mapping": map[string]any{"gemini-omni-1.2-flash": "gemini-omni-1.2-flash"},
	}}
}

func TestGeminiOmniVideoCreateParameters(t *testing.T) {
	for _, body := range []string{
		`{"model":"gemini-omni-1.2-flash","prompt":"waves","duration":15,"resolution":"4k","aspect_ratio":"9:16","first_frame":"data:image/png;base64,aW1hZ2U=","last_frame":"https://example.com/last.png","seed":0}`,
		`{"model":"gemini-omni-1.2-flash","prompt":"waves","seconds":"15","size":"2160x3840","input_reference":{"url":"data:image/png;base64,aW1hZ2U="},"last_frame":"https://example.com/last.png","seed":0}`,
	} {
		metadata, parseErr := ParseGeminiOmniVideoRequest([]byte(body))
		require.NoError(t, parseErr)
		require.NotNil(t, metadata.Seed, "zero is an explicit seed, not an absent value")
		require.Zero(t, *metadata.Seed)
		upstream := &httpUpstreamRecorder{resp: grokMediaContentStatusResponse(`{"id":"task_omni","status":"queued","progress":0}`)}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, w := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)
		result, err := svc.ForwardGeminiOmniVideo(context.Background(), c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(body))
		require.NoError(t, err)
		require.Equal(t, "https://gemini.example/proxy/v1/videos", upstream.lastReq.URL.String())
		require.Equal(t, "Bearer omni-test-key", upstream.lastReq.Header.Get("Authorization"))
		require.JSONEq(t, `{"model":"gemini-omni-1.2-flash","prompt":"waves","seconds":"15","duration":15,"resolution":"4K","aspect_ratio":"9:16","size":"2160x3840","image_size":"2160x3840","first_frame":"data:image/png;base64,aW1hZ2U=","last_frame":"https://example.com/last.png","seed":0}`, string(upstream.lastBody))
		require.Equal(t, "task_omni", result.ResponseID)
		require.Zero(t, result.VideoCount, "create acceptance must never bill")
		require.Equal(t, 15, result.VideoDurationSeconds)
		require.Equal(t, "4k", result.VideoResolution)
		require.Equal(t, "pending", gjson.Get(w.Body.String(), "status").String())
		require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
	}
}

func TestGeminiOmniVideoSizesAndDefaults(t *testing.T) {
	for _, model := range []string{"gemini-omni-1.2-flash", "gemini-omni-1.2", "omni-1.2", "omni-featured"} {
		info, payload, err := parseGeminiOmniVideoRequest([]byte(fmt.Sprintf(`{"model":%q,"prompt":"waves"}`, model)))
		require.NoError(t, err)
		require.Equal(t, 5, info.DurationSeconds)
		require.Equal(t, "720p", info.Resolution)
		require.Equal(t, "16:9", info.AspectRatio)
		require.Nil(t, info.Seed)
		require.Equal(t, "1280x720", payload["image_size"])
	}
	for _, tc := range []struct {
		fields, resolution, aspect, size string
	}{
		{`"image_size":"360P"`, "360p", "16:9", "640x360"},
		{`"image_size":[1080,1920]`, "1080p", "9:16", "1080x1920"},
		{`"image_size":{"width":3840,"height":2160}`, "4k", "16:9", "3840x2160"},
		{`"size":"360x640","resolution":"360p","aspect_ratio":"auto"`, "360p", "9:16", "360x640"},
		{`"resolution":"1080p","aspect_ratio":"auto"`, "1080p", "auto", ""},
		{`"resolution":"4K","aspect_ratio":"9:16"`, "4k", "9:16", "2160x3840"},
	} {
		info, payload, err := parseGeminiOmniVideoRequest([]byte(`{"model":"gemini-omni-1.2-flash","prompt":"waves",` + tc.fields + `}`))
		require.NoError(t, err, tc.fields)
		require.Equal(t, tc.resolution, info.Resolution)
		require.Equal(t, tc.aspect, info.AspectRatio)
		require.Equal(t, tc.size, info.Size)
		if tc.size == "" {
			require.NotContains(t, payload, "image_size")
			require.NotContains(t, payload, "size")
		}
	}
	for _, seconds := range []int{3, 15} {
		info, err := ParseGeminiOmniVideoRequest([]byte(fmt.Sprintf(`{"model":"gemini-omni-1.2-flash","prompt":"waves","duration":%d}`, seconds)))
		require.NoError(t, err)
		require.Equal(t, seconds, info.DurationSeconds)
	}
}

func TestGeminiOmniVideoChannelMappingPreservesProviderParametersAndQuote(t *testing.T) {
	for _, source := range []string{BillingModelSourceRequested, BillingModelSourceChannelMapped} {
		t.Run(source, func(t *testing.T) {
			groupID := int64(166)
			requestedModel, channelModel, accountModel := "gemini-omni-1.2-flash", "omni-channel-route", "vendor-video-route"
			billingModel := requestedModel
			if source == BillingModelSourceChannelMapped {
				billingModel = channelModel
			}
			resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, billingModel, 0.25)
			cache := resolver.channelService.cache.Load().(*channelCache)
			cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: requestedModel}] = channelModel
			cache.channelByGroupID[groupID].BillingModelSource = source
			cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: billingModel}].BillingMode = BillingModePerRequest
			account := geminiOmniVideoTestAccount()
			account.Credentials["model_mapping"] = map[string]any{requestedModel: "wrong-route", channelModel: accountModel}
			upstream := &httpUpstreamRecorder{resp: grokMediaContentStatusResponse(`{"id":"task_omni","status":"queued"}`)}
			svc := &OpenAIGatewayService{httpUpstream: upstream, resolver: resolver, billingService: resolver.billingService}
			key := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1}}
			quote, err := svc.VideoStudioPricing(context.Background(), key, requestedModel)
			require.NoError(t, err)
			c, _ := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)
			c.Set("api_key", key)
			result, err := svc.ForwardGeminiOmniVideo(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"gemini-omni-1.2-flash","prompt":"waves","duration":15,"resolution":"4k","aspect_ratio":"9:16"}`))
			require.NoError(t, err)
			require.Equal(t, accountModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, accountModel, result.UpstreamModel)
			require.Equal(t, requestedModel, result.Model)
			require.Equal(t, billingModel, result.BillingModel)
			require.Equal(t, quote.BillingModel, result.BillingModel)
			require.Equal(t, 15, result.VideoDurationSeconds)
			require.Equal(t, "4k", result.VideoResolution)
			require.Equal(t, "2160x3840", gjson.GetBytes(upstream.lastBody, "image_size").String())
			result.VideoCount = 1
			cost, err := svc.calculateOpenAIVideoCost(context.Background(), result.BillingModel, key, result, 1, 1)
			require.NoError(t, err)
			require.InDelta(t, quote.Tiers["4k"].UnitPrice, cost.ActualCost, 1e-12, "per-request price must not be multiplied by 15 seconds")
		})
	}
}

func TestGeminiOmniVideoReferenceModes(t *testing.T) {
	info, payload, err := parseGeminiOmniVideoRequest([]byte(`{"model":"gemini-omni-1.2-flash","prompt":"motion","image_references":["https://example.com/a.png"],"image_urls":["https://example.com/a.png",{"url":"https://example.com/b.png"}],"video_references":["https://example.com/motion.mp4"],"video_url":"https://example.com/motion.mp4","video_urls":["data:video/mp4;base64,dmlkZW8="]}`))
	require.NoError(t, err)
	require.Len(t, info.InputImageURLs, 2)
	require.Len(t, info.InputVideoURLs, 2)
	require.Equal(t, []string{"https://example.com/a.png", "https://example.com/b.png"}, payload["image_references"])
	require.Equal(t, []string{"https://example.com/motion.mp4", "data:video/mp4;base64,dmlkZW8="}, payload["video_references"])
	require.NotContains(t, payload, "image_urls")
	require.NotContains(t, payload, "video_urls")
	require.Contains(t, string(info.ModerationBody()), "https://example.com/b.png")
	for _, tc := range []struct {
		field string
		limit int
	}{
		{"image_references", 10}, {"image_urls", 10}, {"video_references", 3}, {"video_urls", 3},
	} {
		refs := make([]string, tc.limit)
		for i := range refs {
			refs[i] = fmt.Sprintf("https://example.com/ref-%d", i)
		}
		body := map[string]any{"model": "gemini-omni-1.2-flash", "prompt": "motion", tc.field: refs}
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		_, err = ParseGeminiOmniVideoRequest(encoded)
		require.NoError(t, err, tc.field)
		body[tc.field] = append(refs, "https://example.com/extra")
		encoded, err = json.Marshal(body)
		require.NoError(t, err)
		_, err = ParseGeminiOmniVideoRequest(encoded)
		require.Error(t, err, tc.field)
	}
}

func TestGeminiOmniVideoRejectsInvalidAndConflictingParameters(t *testing.T) {
	for _, fields := range []string{
		`"duration":2`, `"duration":16`, `"duration":3.5`, `"duration":"5"`, `"seconds":"5.5"`,
		`"duration":15,"seconds":"5"`, `"seconds":false`, `"n":2`, `"seed":"42"`, `"seed":2.5`, `"seed":9223372036854775808`,
		`"aspect_ratio":"1:1"`, `"aspect_ratio":16`, `"resolution":"480p"`,
		`"size":"1920x1080","resolution":"720p"`, `"size":"1080x1920","aspect_ratio":"16:9"`,
		`"size":"1080x1920","image_size":"1920x1080"`, `"image_size":"600x600"`, `"image_size":[1920]`,
		`"image_size":{"width":1920.5,"height":1080}`, `"image_size":["1920",1080]`,
		`"last_frame":"https://example.com/end.png"`,
		`"first_frame":"https://example.com/start.png","image_references":["https://example.com/ref.png"]`,
		`"first_frame":"https://example.com/start.png","video_url":"https://example.com/ref.mp4"`,
		`"first_frame":"https://example.com/start.png","image":"https://example.com/other.png"`,
		`"image_references":"https://example.com/ref.png"`, `"video_references":[null]`,
		`"image_references":["file:///private.png"]`, `"first_frame":"https://user:pass@example.com/a.png"`,
		`"first_frame":"data:text/plain;base64,aQ=="`, `"first_frame":"data:image/png;base64,?"`,
		`"video_url":"data:image/png;base64,aQ=="`, `"first_frame":{}`, `"first_frame":[]`,
		`"video_urls":["https://example.com/1","https://example.com/2","https://example.com/3"],"video_url":"https://example.com/4"`,
	} {
		_, err := ParseGeminiOmniVideoRequest([]byte(`{"model":"gemini-omni-1.2-flash","prompt":"waves",` + fields + `}`))
		require.Error(t, err, fields)
	}
	for _, body := range []string{`{`, `[]`, `{}`, `{"model":"gemini-other","prompt":"waves"}`, `{"model":"gemini-omni-1.2-flash","prompt":" "}`} {
		_, err := ParseGeminiOmniVideoRequest([]byte(body))
		require.Error(t, err, body)
	}
	oversized := gjson.Parse(`"data:image/png;base64,` + strings.Repeat("A", (geminiOmniMaxReferenceBytes/3+2)*4) + `"`)
	_, err := geminiOmniReference(oversized, "image")
	require.ErrorContains(t, err, "20 MB")
}

func TestGeminiOmniVideoTaskStates(t *testing.T) {
	for _, tc := range []struct {
		state, expected string
		count           int
	}{
		{"queued", "pending", 0}, {"in_progress", "pending", 0}, {"completed", "done", 1},
		{"failed", "failed", 0}, {"cancelled", "failed", 0},
	} {
		t.Run(tc.state, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"request_id":"task_omni","status":"` + tc.state + `","video":{"url":"https://untrusted.example/output.mp4"},"error":{"message":"failed"}}`)}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_omni", nil)
			result, err := svc.ForwardGeminiOmniVideo(context.Background(), c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideoStatus, "task_omni", nil)
			require.NoError(t, err)
			require.Equal(t, tc.count, result.VideoCount)
			require.Equal(t, tc.expected, gjson.Get(w.Body.String(), "status").String())
			require.NotContains(t, w.Body.String(), "untrusted.example")
			if tc.count > 0 {
				require.Equal(t, "/v1/videos/task_omni/content", gjson.Get(w.Body.String(), "video.url").String())
			}
		})
	}
	for _, body := range []string{
		`{"id":"other","status":"completed"}`, `{"id":"task_omni","status":"unknown"}`,
		`{"id":"../oops","status":"completed"}`, `{"status":"completed"}`, `[]`,
	} {
		_, _, err := normalizeGeminiOmniVideoResponse([]byte(body), "task_omni", "/content")
		require.Error(t, err, body)
	}
}

func TestGeminiOmniVideoContentPersistsBeforeDelivery(t *testing.T) {
	video := grokMediaContentMP4("omni")
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task_omni","status":"completed","url":"https://untrusted.example/secret"}`),
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader(video))},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_omni/content", map[string]string{"Range": "bytes=0-"})
	stored := false
	ctx := WithGrokVideoContentPersister(grokMediaContentTestTranscodeContext(context.Background()), func(_ context.Context, id, mime string, data []byte) error {
		stored = true
		require.Equal(t, "task_omni", id)
		require.Equal(t, "video/mp4", mime)
		require.Equal(t, video, string(data))
		require.Empty(t, w.Body.String())
		return nil
	})
	result, err := svc.ForwardGeminiOmniVideo(ctx, c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideoContent, "task_omni", nil)
	require.NoError(t, err)
	require.True(t, stored)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, video, w.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://gemini.example/proxy/v1/videos/task_omni/content", upstream.requests[1].URL.String())
	for _, req := range upstream.requests {
		require.Equal(t, "Bearer omni-test-key", req.Header.Get("Authorization"))
		require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
		require.Empty(t, req.Header.Get("Range"))
	}
}

func TestGeminiOmniVideoContentFailureDoesNotDeliver(t *testing.T) {
	for _, state := range []string{"queued", "in_progress", "failed"} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task_omni","status":"` + state + `"}`)}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_omni/content", nil)
		result, err := svc.ForwardGeminiOmniVideo(context.Background(), c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideoContent, "task_omni", nil)
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusConflict, w.Code)
		require.Len(t, upstream.requests, 1)
	}
	for _, status := range []int{http.StatusOK, http.StatusFound, http.StatusPartialContent} {
		upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
			grokMediaContentStatusResponse(`{"id":"task_omni","status":"completed"}`),
			{StatusCode: status, Header: http.Header{"Content-Type": []string{"video/mp4"}, "Location": []string{"https://untrusted.example/secret"}}, Body: io.NopCloser(strings.NewReader(grokMediaContentMP4("omni")))},
		}}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_omni/content", nil)
		ctx := WithGrokVideoContentPersister(grokMediaContentTestTranscodeContext(context.Background()), func(context.Context, string, string, []byte) error {
			return errors.New("storage unavailable")
		})
		result, err := svc.ForwardGeminiOmniVideo(ctx, c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideoContent, "task_omni", nil)
		require.Error(t, err)
		require.Nil(t, result)
		require.Empty(t, w.Body.String())
		require.Len(t, upstream.requests, 2)
	}
}

func TestGeminiOmniVideoURLAndAccountGuard(t *testing.T) {
	account := geminiOmniVideoTestAccount()
	require.True(t, SupportsGeminiOmniVideoAccount(account))
	account.Type = AccountTypeOAuth
	require.False(t, SupportsGeminiOmniVideoAccount(account))
	account.Type, account.Platform = AccountTypeAPIKey, PlatformOpenAI
	require.False(t, SupportsGeminiOmniVideoAccount(account))
	account.Platform, account.Credentials["base_url"] = PlatformGemini, ""
	require.False(t, SupportsGeminiOmniVideoAccount(account))
	for _, base := range []string{"https://gemini.example", "https://gemini.example/v1", "https://gemini.example/v1beta/"} {
		target, err := buildGeminiOmniVideoURL(base, GrokMediaEndpointVideosGenerations, "")
		require.NoError(t, err)
		require.Equal(t, "https://gemini.example/v1/videos", target)
	}
	for _, id := range []string{"", "..", "a/b", "a?b", "a#b", "%2e%2e"} {
		_, err := buildGeminiOmniVideoURL("https://gemini.example", GrokMediaEndpointVideoStatus, id)
		require.Error(t, err, id)
	}
	_, err := buildGeminiOmniVideoURL("https://gemini.example", GrokMediaEndpointVideosEdits, "")
	require.Error(t, err)
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusFound} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"error":{"message":"overloaded"}}`)}
		upstream.response.StatusCode = status
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "/v1/videos", nil)
		result, err := svc.ForwardGeminiOmniVideo(context.Background(), c, geminiOmniVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"gemini-omni-1.2-flash","prompt":"waves"}`))
		require.Error(t, err)
		require.Nil(t, result)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover), "paid asynchronous create must not be retried")
		require.Len(t, upstream.requests, 1)
	}
}

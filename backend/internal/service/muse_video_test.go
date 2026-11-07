//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func museVideoTestAccount() *Account {
	return &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "muse-key", "base_url": "https://muse.internal/proxy/v1",
		"model_mapping": map[string]any{"muse-video": "muse-video"},
	}}
}

func TestMuseVideoCreateAdaptsWorkbenchAndNativeParameters(t *testing.T) {
	for _, body := range []string{
		`{"model":"muse-video","prompt":"waves","resolution":"auto","aspect_ratio":"9:16","duration":60,"image":{"url":"data:image/png;base64,aW1hZ2U="}}`,
		`{"model":"muse-video","prompt":"waves","size":"9:16","seconds":60,"input_reference":"data:image/png;base64,aW1hZ2U="}`,
	} {
		upstream := &httpUpstreamRecorder{resp: grokMediaContentStatusResponse(`{"id":"task_muse","status":"queued","progress":0,"created_at":123,"model":"muse-video","result":null,"error":null}`)}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, w := grokMediaContentTestContext(http.MethodPost, "/v1/videos/generations", nil)
		result, err := svc.ForwardGrokMedia(context.Background(), c, museVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(body), "application/json")
		require.NoError(t, err)
		require.Equal(t, "https://muse.internal/proxy/v1/videos", upstream.lastReq.URL.String())
		require.Equal(t, "Bearer muse-key", upstream.lastReq.Header.Get("Authorization"))
		require.JSONEq(t, `{"model":"muse-video","prompt":"waves","size":"9:16","duration":60,"image":"data:image/png;base64,aW1hZ2U="}`, string(upstream.lastBody))
		require.Equal(t, "task_muse", result.ResponseID)
		require.Zero(t, result.VideoCount, "submitting a task must not charge")
		require.Equal(t, 60, result.VideoDurationSeconds)
		require.Equal(t, "auto", result.VideoResolution)
		require.Equal(t, "pending", gjson.Get(w.Body.String(), "status").String())
		require.Equal(t, "task_muse", gjson.Get(w.Body.String(), "request_id").String())
	}
}

func TestMuseVideoParameterValidation(t *testing.T) {
	for _, body := range []string{
		`{`, `[]`, `{}`, `{"model":"other","prompt":"waves"}`, `{"model":"muse-video","prompt":1}`,
		`{"model":"muse-video","prompt":" "}`, `{"model":"muse-video","prompt":"waves","duration":61}`,
		`{"model":"muse-video","prompt":"waves","duration":0}`, `{"model":"muse-video","prompt":"waves","seconds":-1}`,
		`{"model":"muse-video","prompt":"waves","duration":5.5}`, `{"model":"muse-video","prompt":"waves","duration":"5"}`,
		`{"model":"muse-video","prompt":"waves","resolution":"4k"}`, `{"model":"muse-video","prompt":"waves","resolution":"720p"}`,
		`{"model":"muse-video","prompt":"waves","size":5}`, `{"model":"muse-video","prompt":"waves","image":[]}`,
		`{"model":"muse-video","prompt":"waves","size":"12345678901234567"}`,
		`{"model":"muse-video","prompt":"waves","aspect_ratio":"12345678901234567"}`,
		`{"model":"muse-video","prompt":"waves","image":{}}`, `{"model":"muse-video","prompt":"waves","n":2}`,
		`{"model":"muse-video","prompt":"waves","images":["x","y"]}`,
	} {
		_, err := ParseMuseVideoRequest([]byte(body))
		require.Error(t, err, body)
	}
	info, err := ParseMuseVideoRequest([]byte(`{"model":"muse-video","prompt":"waves","duration":60,"seconds":1,"size":"1080x1920","image":"https://example.com/first.png"}`))
	require.NoError(t, err)
	require.Equal(t, 60, info.DurationSeconds, "native duration takes precedence")
	require.Equal(t, "1080x1920", info.AspectRatio)
	require.True(t, info.HasInputImage())
	require.Contains(t, string(info.ModerationBody()), "https://example.com/first.png")
	info = ParseGrokMediaRequest("application/json", []byte(`{"model":"muse-video","prompt":"waves","seconds":60,"input_reference":"data:image/png;base64,aQ=="}`))
	require.Equal(t, 60, info.DurationSeconds)
	require.Equal(t, "auto", info.Resolution)
	require.True(t, info.HasInputImage())
	info, err = ParseMuseVideoRequest([]byte(`{"model":"muse-video","prompt":"waves"}`))
	require.NoError(t, err)
	require.Equal(t, 5, info.DurationSeconds)
}

func TestMuseVideoCreateAppliesChannelMappingBeforeAccountMapping(t *testing.T) {
	groupID := int64(65)
	resolver := newOpenAIImageChannelPricingResolverForTest(t, groupID, "channel-video", 0.25)
	cache := resolver.channelService.cache.Load().(*channelCache)
	cache.mappingByGroupModel[channelModelKey{groupID: groupID, model: "muse-video"}] = "channel-video"
	account := museVideoTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"muse-video": "wrong-account-route", "channel-video": "sora"}
	upstream := &httpUpstreamRecorder{resp: grokMediaContentStatusResponse(`{"id":"task_muse","status":"queued"}`)}
	svc := &OpenAIGatewayService{httpUpstream: upstream, resolver: resolver}
	require.Equal(t, "channel-video", svc.ResolveVideoStudioRoutingModel(context.Background(), &groupID, "muse-video"))
	c, _ := grokMediaContentTestContext(http.MethodPost, "/v1/videos/generations", nil)
	c.Set("api_key", &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI}})
	result, err := svc.ForwardMuseVideo(context.Background(), c, account, GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"muse-video","prompt":"waves","duration":60,"aspect_ratio":"16:9"}`))
	require.NoError(t, err)
	require.Equal(t, "sora", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "sora", result.UpstreamModel)
	require.Equal(t, "muse-video", result.Model)
	require.Equal(t, "muse-video", result.BillingModel)
	require.Equal(t, 60, result.VideoDurationSeconds)
	require.Equal(t, "auto", result.VideoResolution)
}

func TestMuseVideoTaskStatesAndOwnership(t *testing.T) {
	for _, state := range []string{"queued", "running", "succeeded", "failed", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task_muse","status":"` + state + `","progress":100,"model":"muse-video","result":{"url":"https://public.example/v1/media/vid_generated.mp4"},"error":{"code":"failure","message":"failed"}}`)}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse", nil)
			result, err := svc.ForwardGrokMedia(context.Background(), c, museVideoTestAccount(), GrokMediaEndpointVideoStatus, "task_muse", nil, "")
			require.NoError(t, err)
			switch state {
			case "succeeded":
				require.Equal(t, 1, result.VideoCount)
				require.Equal(t, "done", gjson.Get(w.Body.String(), "status").String())
				require.Equal(t, "/v1/videos/task_muse/content", gjson.Get(w.Body.String(), "video.url").String())
				require.NotContains(t, w.Body.String(), "public.example")
				bill := ExtractGrokVideoBillingFromStatusBody(result.GrokMediaResponseBody, &GrokVideoPendingBilling{Model: "muse-video", BillingModel: "video-priced-alias", VideoResolution: "auto", VideoDurationSeconds: 60}, "task_muse")
				require.NotNil(t, bill)
				require.Equal(t, 60, bill.VideoDurationSeconds)
				require.Equal(t, "auto", bill.VideoResolution)
			case "failed", "cancelled":
				require.Zero(t, result.VideoCount)
				require.Equal(t, "failed", gjson.Get(w.Body.String(), "status").String())
				require.Equal(t, "failure", gjson.Get(w.Body.String(), "error.code").String())
			default:
				require.Zero(t, result.VideoCount)
				require.Equal(t, "pending", gjson.Get(w.Body.String(), "status").String())
			}
		})
	}
	for _, body := range []string{
		`{"id":"wrong","status":"queued"}`, `{"id":"task_muse","status":"succeeded","result":null}`,
		`{"id":"task_muse","status":"unknown"}`, `{"id":"../oops","status":"queued"}`,
	} {
		_, _, err := normalizeMuseVideoResponse([]byte(body), "task_muse", "/content")
		require.Error(t, err)
	}
}

func TestMuseVideoContentUsesConfiguredMediaHostAndPersistsBeforeDelivery(t *testing.T) {
	video := grokMediaContentMP4("muse")
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task_muse","status":"succeeded","result":{"url":"https://public.example/v1/media/vid_generated.mp4"}}`),
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader(video))},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse/content", map[string]string{"Range": "bytes=0-"})
	stored := false
	ctx := WithGrokVideoContentPersister(grokMediaContentTestTranscodeContext(context.Background()), func(_ context.Context, id, mime string, data []byte) error {
		stored = true
		require.Equal(t, "task_muse", id)
		require.Equal(t, "video/mp4", mime)
		require.Equal(t, video, string(data))
		require.Empty(t, w.Body.String(), "persistence and billing must precede delivery")
		return nil
	})
	result, err := svc.ForwardMuseVideo(ctx, c, museVideoTestAccount(), GrokMediaEndpointVideoContent, "task_muse", nil)
	require.NoError(t, err)
	require.True(t, stored)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, video, w.Body.String())
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://muse.internal/proxy/v1/media/vid_generated.mp4", upstream.requests[1].URL.String())
	require.Empty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[1].Header.Get("Range"), "first download must not bypass persistence and billing")
	require.Equal(t, http.StatusOK, w.Code)
	for _, req := range upstream.requests {
		require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
	}
}

func TestMuseVideoContentFailureDoesNotDeliverOrCharge(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task_muse","status":"succeeded","result":{"url":"/v1/media/vid_generated.mp4"}}`),
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader(grokMediaContentMP4("muse")))},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse/content", nil)
	ctx := WithGrokVideoContentPersister(grokMediaContentTestTranscodeContext(context.Background()), func(context.Context, string, string, []byte) error {
		return errors.New("storage unavailable")
	})
	result, err := svc.ForwardMuseVideo(ctx, c, museVideoTestAccount(), GrokMediaEndpointVideoContent, "task_muse", nil)
	require.ErrorContains(t, err, "storage unavailable")
	require.Nil(t, result)
	require.Empty(t, w.Body.String())
}

func TestMuseVideoURLValidationAndNoRetry(t *testing.T) {
	account := museVideoTestAccount()
	require.True(t, SupportsMuseVideoAccount(account))
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMuseVideo))
	account.Type = AccountTypeOAuth
	require.False(t, SupportsMuseVideoAccount(account))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMuseVideo))
	account.Type = AccountTypeAPIKey
	account.Credentials["base_url"] = ""
	require.False(t, SupportsMuseVideoAccount(account))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMuseVideo))
	account = museVideoTestAccount()
	account.Platform = PlatformGrok
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMuseVideo))
	_, err := buildMuseVideoURL("https://muse.internal", GrokMediaEndpointVideosEdits, "")
	require.Error(t, err)
	for _, raw := range []string{
		"https://evil.example/private", "/v1/media/../secret", "/v1/media/vid_a.mp4/extra",
		"file:///v1/media/vid_a.mp4", "https://user:pass@host/v1/media/vid_a.mp4", "/v1/media/vid_a.mp4?url=secret",
		"/v1/media/vid_a%2emp4", "/v1/media/config.json", "/v1/media/vid_a.mp4#secret",
	} {
		_, err := museVideoMediaURL("https://muse.internal/v1", raw)
		require.Error(t, err, raw)
	}
	for _, id := range []string{"", "..", "a/b", "a?b", "a#b", "%2e%2e"} {
		_, err := buildMuseVideoURL("https://muse.internal", GrokMediaEndpointVideoStatus, id)
		require.Error(t, err)
	}
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusFound} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"error":{"message":"overloaded"}}`)}
		upstream.response.StatusCode = status
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "/v1/videos/generations", nil)
		result, err := svc.ForwardMuseVideo(context.Background(), c, museVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", []byte(`{"model":"muse-video","prompt":"waves"}`))
		require.Error(t, err)
		require.Nil(t, result)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover), "an asynchronous create must not be retried")
		require.Len(t, upstream.requests, 1)
	}
}

func TestMuseVideoDoesNotDownloadIncompleteTasksOrFollowRedirects(t *testing.T) {
	for _, state := range []string{"queued", "running", "failed", "cancelled"} {
		upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"task_muse","status":"` + state + `","result":{"url":"/v1/media/vid_generated.mp4"}}`)}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse/content", nil)
		result, err := svc.ForwardMuseVideo(context.Background(), c, museVideoTestAccount(), GrokMediaEndpointVideoContent, "task_muse", nil)
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusConflict, w.Code)
		require.Len(t, upstream.requests, 1)
	}
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task_muse","status":"succeeded","result":{"url":"/v1/media/vid_generated.mp4"}}`),
		{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://evil.example/secret"}}, Body: io.NopCloser(strings.NewReader(""))},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c, w := grokMediaContentTestContext(http.MethodGet, "/v1/videos/task_muse/content", nil)
	result, err := svc.ForwardMuseVideo(context.Background(), c, museVideoTestAccount(), GrokMediaEndpointVideoContent, "task_muse", nil)
	require.Error(t, err)
	require.Nil(t, result)
	require.Empty(t, w.Body.String())
	require.Len(t, upstream.requests, 2)
}

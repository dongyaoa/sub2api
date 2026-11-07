package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Muse exposes a single video model. Other OpenAI video models must not be
// silently sent to this adapter, because Muse itself accepts unknown aliases.
func IsMuseVideoModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "muse-video")
}

func SupportsMuseVideoAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		strings.TrimSpace(account.GetCredential("base_url")) != ""
}

// ResolveVideoStudioRoutingModel supplies the same channel target to account
// scheduling and forwarding; account-level mapping is applied only afterward.
func (s *OpenAIGatewayService) ResolveVideoStudioRoutingModel(ctx context.Context, groupID *int64, model string) string {
	if s == nil || groupID == nil {
		return model
	}
	channelService := s.channelService
	if s.resolver != nil && s.resolver.channelService != nil {
		channelService = s.resolver.channelService
	}
	if channelService != nil {
		mapping, _ := channelService.ResolveChannelMappingAndRestrict(ctx, groupID, model)
		if strings.TrimSpace(mapping.MappedModel) != "" {
			return mapping.MappedModel
		}
	}
	return model
}

// ParseMuseVideoRequest accepts both the workbench's common video fields and
// Muse's native size/seconds/input_reference fields. Muse has no resolution
// control: size and duration are requests expressed through its media prompt.
func ParseMuseVideoRequest(body []byte) (GrokMediaRequestInfo, error) {
	info := GrokMediaRequestInfo{N: 1, Resolution: "auto", DurationSeconds: 5}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return info, fmt.Errorf("request body must be a JSON object")
	}
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.String || !IsMuseVideoModel(model.String()) {
		return info, fmt.Errorf("model must be muse-video")
	}
	info.Model = strings.TrimSpace(model.String())
	prompt := gjson.GetBytes(body, "prompt")
	if prompt.Type != gjson.String || strings.TrimSpace(prompt.String()) == "" {
		return info, fmt.Errorf("prompt is required")
	}
	info.Prompt = strings.TrimSpace(prompt.String())
	for _, field := range []string{"duration", "seconds"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if value.Type != gjson.Number || value.Float() != float64(value.Int()) || value.Int() < 1 || value.Int() > 60 {
			return info, fmt.Errorf("%s must be an integer from 1 to 60", field)
		}
		// Native Muse gives duration priority over seconds.
		if field == "duration" || !gjson.GetBytes(body, "duration").Exists() || gjson.GetBytes(body, "duration").Type == gjson.Null {
			info.DurationSeconds = int(value.Int())
		}
	}
	for _, field := range []string{"size", "aspect_ratio"} {
		value := gjson.GetBytes(body, field)
		if value.Exists() && value.Type != gjson.Null && value.Type != gjson.String {
			return info, fmt.Errorf("%s must be a string", field)
		}
	}
	info.Size = firstNonEmpty(strings.TrimSpace(gjson.GetBytes(body, "size").String()), strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String()))
	// Task history stores aspect_ratio in VARCHAR(16). Validate before creating
	// an upstream task so an oversized hint cannot make its history disappear.
	if utf8.RuneCountInString(info.Size) > 16 {
		return info, fmt.Errorf("size or aspect_ratio must not exceed 16 characters; use a ratio such as 16:9 or a size such as 1080x1920")
	}
	info.AspectRatio = info.Size
	if value := gjson.GetBytes(body, "resolution"); value.Exists() && value.Type != gjson.Null && value.String() != "" && value.String() != "auto" {
		return info, fmt.Errorf("muse-video does not support a fixed resolution; use auto")
	}
	if value := gjson.GetBytes(body, "n"); value.Exists() && (value.Type != gjson.Number || value.Float() != 1) {
		return info, fmt.Errorf("muse-video creates one video per request")
	}
	for _, field := range []string{"images", "reference_images"} {
		if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.Null {
			return info, fmt.Errorf("muse-video supports one first-frame image in image or input_reference")
		}
	}
	for _, field := range []string{"image", "input_reference"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null || value.String() == "" {
			continue
		}
		if value.Type != gjson.String && !value.IsObject() {
			return info, fmt.Errorf("%s must contain a single image URL or data URL", field)
		}
		ref := extractGrokMediaImageURL(value)
		if ref == "" {
			return info, fmt.Errorf("%s must contain a single image URL or data URL", field)
		}
		if len(info.InputImageURLs) == 0 {
			info.InputImageURLs = []string{ref}
		}
	}
	return info, nil
}

func museVideoAPIBase(base string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base
}

func buildMuseVideoURL(base string, endpoint GrokMediaEndpoint, id string) (string, error) {
	target := museVideoAPIBase(base) + "/videos"
	switch endpoint {
	case GrokMediaEndpointVideosGenerations:
		return target, nil
	case GrokMediaEndpointVideoStatus, GrokMediaEndpointVideoContent:
		if strings.TrimSpace(id) == "" || validateUpstreamPathSegment("Muse task ID", id) != nil {
			return "", fmt.Errorf("invalid Muse task ID")
		}
		return target + "/" + url.PathEscape(id), nil
	default:
		return "", fmt.Errorf("muse-video does not support this video operation")
	}
}

var museMediaNamePattern = regexp.MustCompile(`^vid_[A-Za-z0-9_-]+\.(mp4|webm|mov|bin)$`)

// Muse's public_base may differ from the internal account base. Only extract a
// safe media filename; always retrieve it from the administrator's validated
// API base, never from an arbitrary result.url host or redirect target.
func museVideoMediaURL(base, raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "" && parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawPath != "" {
		return "", fmt.Errorf("invalid Muse media URL")
	}
	pos := strings.LastIndex(parsed.Path, "/v1/media/")
	if pos < 0 {
		return "", fmt.Errorf("invalid Muse media path")
	}
	name := strings.TrimPrefix(parsed.Path[pos:], "/v1/media/")
	if !museMediaNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid Muse media filename")
	}
	return museVideoAPIBase(base) + "/media/" + name, nil
}

func normalizeMuseVideoResponse(body []byte, requestedID, proxyURL string) ([]byte, string, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, "", fmt.Errorf("invalid Muse video response")
	}
	id := strings.TrimSpace(gjson.GetBytes(body, "id").String())
	if id == "" || validateUpstreamPathSegment("Muse task ID", id) != nil || (requestedID != "" && id != requestedID) {
		return nil, "", fmt.Errorf("invalid Muse video response task ID")
	}
	status := strings.ToLower(gjson.GetBytes(body, "status").String())
	mediaURL := ""
	payload := map[string]any{"id": id, "request_id": id, "object": "video.generation.task", "model": "muse-video"}
	switch status {
	case "queued", "running":
		payload["status"] = "pending"
	case "succeeded":
		mediaURL = strings.TrimSpace(gjson.GetBytes(body, "result.url").String())
		if mediaURL == "" {
			return nil, "", fmt.Errorf("completed Muse video has no media URL")
		}
		payload["status"] = "done"
		payload["video"] = map[string]string{"url": proxyURL}
	case "failed", "cancelled":
		payload["status"] = "failed"
		if value := gjson.GetBytes(body, "error"); value.Exists() && value.Type != gjson.Null {
			payload["error"] = json.RawMessage(value.Raw)
		} else {
			payload["error"] = map[string]string{"message": "Muse video generation " + status}
		}
	default:
		return nil, "", fmt.Errorf("unknown Muse video task status")
	}
	for _, field := range []string{"progress", "created_at"} {
		if value := gjson.GetBytes(body, field); value.Type == gjson.Number {
			payload[field] = value.Value()
		}
	}
	result, err := json.Marshal(payload)
	return result, mediaURL, err
}

// ForwardMuseVideo adapts Muse to the existing owned asynchronous video tasks.
// A create is never retried on another account after an ambiguous upstream
// failure: the first request may already have submitted a paid generation.
func (s *OpenAIGatewayService) ForwardMuseVideo(ctx context.Context, c *gin.Context, account *Account, endpoint GrokMediaEndpoint, requestID string, body []byte) (*OpenAIForwardResult, error) {
	if !SupportsMuseVideoAccount(account) {
		return nil, fmt.Errorf("muse-video requires an OpenAI API key account with a custom base URL")
	}
	base, err := s.validateUpstreamBaseURL(account.GetCredential("base_url"))
	if err != nil {
		return nil, err
	}
	target, err := buildMuseVideoURL(base, endpoint, requestID)
	if err != nil {
		return nil, err
	}
	info := GrokMediaRequestInfo{Model: "muse-video"}
	upstreamModel := "muse-video"
	if endpoint == GrokMediaEndpointVideosGenerations {
		info, err = ParseMuseVideoRequest(body)
		if err != nil {
			return nil, err
		}
		mappedModel := info.Model
		if apiKey := getAPIKeyFromContext(c); apiKey != nil {
			mappedModel = s.ResolveVideoStudioRoutingModel(ctx, apiKey.GroupID, info.Model)
		}
		upstreamModel = account.GetMappedModel(mappedModel)
		payload := map[string]any{"model": upstreamModel, "prompt": info.Prompt, "duration": info.DurationSeconds}
		if info.Size != "" {
			payload["size"] = info.Size
		}
		if len(info.InputImageURLs) > 0 {
			payload["image"] = info.InputImageURLs[0]
		}
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	token := strings.TrimSpace(account.GetCredential("api_key"))
	if token == "" {
		return nil, fmt.Errorf("muse-video account missing api_key")
	}
	started := time.Now()
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	upstreamCtx = WithHTTPUpstreamRedirectsDisabled(upstreamCtx)
	method := http.MethodGet
	if endpoint == GrokMediaEndpointVideosGenerations {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(upstreamCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(req.Header)
	proxy := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxy = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("Muse video request failed: %w", err)
	}
	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		if resp.StatusCode >= 400 {
			writeGrokMediaResponse(c, resp, responseBody, s.responseHeaderFilter)
		}
		return nil, fmt.Errorf("Muse video upstream status %d", resp.StatusCode)
	}
	id := strings.TrimSpace(gjson.GetBytes(responseBody, "id").String())
	normalized, mediaURL, err := normalizeMuseVideoResponse(responseBody, requestID, grokMediaContentProxyURL(c, id))
	if err != nil {
		return nil, err
	}
	if mediaURL != "" {
		if _, err := museVideoMediaURL(base, mediaURL); err != nil {
			return nil, err
		}
	}
	result := &OpenAIForwardResult{
		RequestID: resp.Header.Get("x-request-id"), ResponseID: id, Model: info.Model, BillingModel: info.Model,
		UpstreamModel: upstreamModel, ResponseHeaders: resp.Header.Clone(), UpstreamHeaders: resp.Header,
		Duration: time.Since(started), GrokMediaResponseBody: normalized,
		VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds,
	}
	if endpoint == GrokMediaEndpointVideoContent {
		if mediaURL == "" {
			writeGrokMediaErrorResponse(c, http.StatusConflict, "video_not_ready", "Video content is not available")
			return nil, fmt.Errorf("Muse video content is not available")
		}
		if err := s.forwardMuseVideoContent(upstreamCtx, c, account, base, proxy, requestID, mediaURL); err != nil {
			return nil, err
		}
	} else {
		writeGrokMediaResponse(c, resp, normalized, s.responseHeaderFilter)
	}
	if endpoint != GrokMediaEndpointVideosGenerations && mediaURL != "" {
		result.VideoCount = 1
	}
	result.Duration = time.Since(started)
	return result, nil
}

func (s *OpenAIGatewayService) forwardMuseVideoContent(ctx context.Context, c *gin.Context, account *Account, base, proxy, requestID, mediaURL string) error {
	target, err := museVideoMediaURL(base, mediaURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	// Muse media is public. No bearer token or account header overrides are
	// needed for this binary download, and redirects are disabled. Retrieve the
	// complete first download even when the client sends Range: partial content
	// must not bypass persistence and its one-time delivery charge. Stored video
	// ranges are served by the handler without another upstream download.
	req.Header.Set("Accept", "video/*,application/octet-stream;q=0.8")
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Muse media download returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxStoredVideoBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxStoredVideoBytes {
		return fmt.Errorf("Muse video exceeds %d bytes", maxStoredVideoBytes)
	}
	data, contentType, err := ensureBrowserPlayableVideo(ctx, resp.Header.Get("Content-Type"), data)
	if err != nil {
		return err
	}
	if persist := grokVideoContentPersisterFromContext(ctx); persist != nil {
		if err := persist(ctx, requestID, contentType, data); err != nil {
			return err
		}
	}
	resp.Header.Set("Content-Type", contentType)
	resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	resp.ContentLength = int64(len(data))
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return writeGrokMediaContentResponse(c, resp)
}

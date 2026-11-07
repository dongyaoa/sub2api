package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const geminiOmniMaxReferenceBytes = 20 * 1024 * 1024

func IsGeminiOmniVideoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gemini-omni-1.2-flash", "gemini-omni-1.2", "omni-1.2", "omni-featured":
		return true
	default:
		return false
	}
}

func SupportsGeminiOmniVideoAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformGemini && account.Type == AccountTypeAPIKey &&
		strings.TrimSpace(account.GetCredential("base_url")) != ""
}

// ParseGeminiOmniVideoRequest also supplies moderation and billing metadata.
// The upstream's OpenAI videos aliases and native parameters are validated by
// the same parser, so the billed resolution and duration match what is sent.
func ParseGeminiOmniVideoRequest(body []byte) (GrokMediaRequestInfo, error) {
	info, _, err := parseGeminiOmniVideoRequest(body)
	return info, err
}

func parseGeminiOmniVideoRequest(body []byte) (GrokMediaRequestInfo, map[string]any, error) {
	info := GrokMediaRequestInfo{N: 1, Resolution: "720p", AspectRatio: "16:9", DurationSeconds: 5}
	fail := func(message string) (GrokMediaRequestInfo, map[string]any, error) {
		return info, nil, fmt.Errorf("%s", message)
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return fail("request body must be a JSON object")
	}
	model, prompt := gjson.GetBytes(body, "model"), gjson.GetBytes(body, "prompt")
	if model.Type != gjson.String || !IsGeminiOmniVideoModel(model.String()) {
		return fail("model must be gemini-omni-1.2-flash or a supported Omni alias")
	}
	if prompt.Type != gjson.String || strings.TrimSpace(prompt.String()) == "" {
		return fail("prompt is required")
	}
	info.Model, info.Prompt = strings.TrimSpace(model.String()), strings.TrimSpace(prompt.String())
	if n := gjson.GetBytes(body, "n"); n.Exists() && (n.Type != gjson.Number || n.Float() != 1) {
		return fail("Gemini Omni creates one video per request")
	}
	durationSet := false
	for _, field := range []string{"duration", "seconds"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if value.Type != gjson.Number && !(field == "seconds" && value.Type == gjson.String) {
			return fail(field + " must be an integer from 3 to 15")
		}
		n, err := strconv.Atoi(value.String())
		if err != nil || n < 3 || n > 15 {
			return fail(field + " must be an integer from 3 to 15")
		}
		if durationSet && info.DurationSeconds != n {
			return fail("duration and seconds must match")
		}
		info.DurationSeconds, durationSet = n, true
	}
	resolutionSet, aspectSet := false, false
	if value := gjson.GetBytes(body, "resolution"); value.Exists() && value.Type != gjson.Null {
		resolution, ok := geminiOmniResolution(value.String())
		if value.Type != gjson.String || !ok {
			return fail("resolution must be 360P, 720P, 1080P or 4K")
		}
		info.Resolution, resolutionSet = resolution, true
	}
	if value := gjson.GetBytes(body, "aspect_ratio"); value.Exists() && value.Type != gjson.Null {
		if value.Type != gjson.String || (value.String() != "16:9" && value.String() != "9:16" && value.String() != "auto") {
			return fail("aspect_ratio must be 16:9, 9:16 or auto")
		}
		info.AspectRatio, aspectSet = value.String(), true
	}
	// A size tier sets resolution; exact dimensions additionally set orientation.
	// Explicit contradictory fields are rejected rather than silently billed at
	// a different tier. auto with exact dimensions keeps the explicit dimensions.
	for _, field := range []string{"size", "image_size"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		resolution, aspect, size, err := geminiOmniImageSize(value)
		if err != nil {
			return fail(field + ": " + err.Error())
		}
		if resolutionSet && resolution != info.Resolution {
			return fail(field + " must match resolution")
		}
		if aspect != "" && aspectSet && info.AspectRatio != "auto" && aspect != info.AspectRatio {
			return fail(field + " must match aspect_ratio")
		}
		if size != "" && info.Size != "" && size != info.Size {
			return fail("size and image_size must match")
		}
		info.Resolution, resolutionSet = resolution, true
		if aspect != "" {
			info.AspectRatio, aspectSet = aspect, true
			info.Size = size
		}
	}
	if info.Size == "" && info.AspectRatio != "auto" {
		info.Size = geminiOmniDimensions(info.Resolution, info.AspectRatio)
	}
	payload := map[string]any{
		"model": info.Model, "prompt": info.Prompt, "seconds": strconv.Itoa(info.DurationSeconds),
		"duration": info.DurationSeconds, "resolution": strings.ToUpper(info.Resolution), "aspect_ratio": info.AspectRatio,
	}
	if info.Size != "" {
		payload["size"], payload["image_size"] = info.Size, info.Size
	}
	if value := gjson.GetBytes(body, "seed"); value.Exists() && value.Type != gjson.Null {
		seed, err := strconv.ParseInt(value.Raw, 10, 64)
		if value.Type != gjson.Number || err != nil {
			return fail("seed must be a 64-bit integer")
		}
		payload["seed"] = seed
		info.Seed = &seed
	}
	// image/input_reference are the workbench/OpenAI aliases for one first frame.
	for _, field := range []string{"first_frame", "image", "input_reference", "last_frame"} {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		ref, err := geminiOmniReference(value, "image")
		if err != nil {
			return fail(field + ": " + err.Error())
		}
		key := "first_frame"
		if field == "last_frame" {
			key = field
		}
		if previous, exists := payload[key]; exists && previous != ref {
			return fail("first_frame, image and input_reference must match when combined")
		}
		payload[key] = ref
	}
	imageRefs, err := geminiOmniReferences(body, []string{"image_references", "image_urls", "reference_images", "images"}, "image", 10)
	if err != nil {
		return fail(err.Error())
	}
	videoRefs, err := geminiOmniReferences(body, []string{"video_references", "video_urls"}, "video", 3)
	if err != nil {
		return fail(err.Error())
	}
	if value := gjson.GetBytes(body, "video_url"); value.Exists() && value.Type != gjson.Null {
		ref, err := geminiOmniReference(value, "video")
		if err != nil {
			return fail("video_url: " + err.Error())
		}
		videoRefs = appendUniqueGeminiOmniReference(videoRefs, ref)
		if len(videoRefs) > 3 {
			return fail("at most 3 video references are supported")
		}
	}
	_, hasFirst := payload["first_frame"]
	_, hasLast := payload["last_frame"]
	if hasLast && !hasFirst {
		return fail("last_frame requires first_frame")
	}
	if (hasFirst || hasLast) && (len(imageRefs) > 0 || len(videoRefs) > 0) {
		return fail("first_frame/last_frame cannot be combined with image or video references")
	}
	if hasFirst {
		info.InputImageURLs = append(info.InputImageURLs, payload["first_frame"].(string))
	}
	if hasLast {
		info.InputImageURLs = append(info.InputImageURLs, payload["last_frame"].(string))
	}
	if len(imageRefs) > 0 {
		payload["image_references"] = imageRefs
		info.InputImageURLs = append(info.InputImageURLs, imageRefs...)
	}
	if len(videoRefs) > 0 {
		payload["video_references"] = videoRefs
		info.InputVideoURLs = videoRefs
	}
	return info, payload, nil
}

func geminiOmniResolution(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "360p", "720p", "1080p", "4k":
		return strings.ToLower(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func geminiOmniDimensions(resolution, aspect string) string {
	width, height := 1280, 720
	switch resolution {
	case "360p":
		width, height = 640, 360
	case "1080p":
		width, height = 1920, 1080
	case "4k":
		width, height = 3840, 2160
	}
	if aspect == "9:16" {
		width, height = height, width
	}
	return fmt.Sprintf("%dx%d", width, height)
}

func geminiOmniImageSize(value gjson.Result) (resolution, aspect, size string, err error) {
	if value.Type == gjson.String {
		if tier, ok := geminiOmniResolution(value.String()); ok {
			return tier, "", "", nil
		}
		size = strings.ToLower(strings.TrimSpace(value.String()))
	} else if value.IsArray() {
		items := value.Array()
		if len(items) == 2 && items[0].Type == gjson.Number && items[1].Type == gjson.Number {
			size = items[0].Raw + "x" + items[1].Raw
		}
	} else if value.IsObject() {
		width, height := value.Get("width"), value.Get("height")
		if width.Type == gjson.Number && height.Type == gjson.Number {
			size = width.Raw + "x" + height.Raw
		}
	}
	for _, tier := range []string{"360p", "720p", "1080p", "4k"} {
		for _, ratio := range []string{"16:9", "9:16"} {
			if size == geminiOmniDimensions(tier, ratio) {
				return tier, ratio, size, nil
			}
		}
	}
	return "", "", "", fmt.Errorf("use a resolution tier or matching 16:9/9:16 dimensions (string, [width,height], or {width,height})")
}

func geminiOmniReference(value gjson.Result, kind string) (string, error) {
	if value.Type != gjson.String && !value.IsObject() {
		return "", fmt.Errorf("reference must contain a %s URL or data URI", kind)
	}
	ref := strings.TrimSpace(extractGrokMediaImageURL(value))
	if strings.HasPrefix(ref, "data:") {
		header, encoded, ok := strings.Cut(ref, ",")
		if !ok || !strings.HasPrefix(header, "data:"+kind+"/") || !strings.HasSuffix(header, ";base64") {
			return "", fmt.Errorf("reference must be a base64 %s data URI", kind)
		}
		if base64.StdEncoding.DecodedLen(len(encoded)) > geminiOmniMaxReferenceBytes+2 {
			return "", fmt.Errorf("reference exceeds the 20 MB limit")
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(decoded) == 0 {
			return "", fmt.Errorf("reference contains invalid base64 data")
		}
		if len(decoded) > geminiOmniMaxReferenceBytes {
			return "", fmt.Errorf("reference exceeds the 20 MB limit")
		}
		return ref, nil
	}
	parsed, err := url.Parse(ref)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("reference must contain an HTTP(S) URL or %s data URI", kind)
	}
	return ref, nil
}

func appendUniqueGeminiOmniReference(refs []string, ref string) []string {
	for _, existing := range refs {
		if existing == ref {
			return refs
		}
	}
	return append(refs, ref)
}

func geminiOmniReferences(body []byte, fields []string, kind string, limit int) ([]string, error) {
	var refs []string
	for _, field := range fields {
		value := gjson.GetBytes(body, field)
		if !value.Exists() || value.Type == gjson.Null {
			continue
		}
		if !value.IsArray() || len(value.Array()) > limit {
			return nil, fmt.Errorf("%s must be an array of at most %d references", field, limit)
		}
		for _, item := range value.Array() {
			ref, err := geminiOmniReference(item, kind)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", field, err)
			}
			refs = appendUniqueGeminiOmniReference(refs, ref)
			if len(refs) > limit {
				return nil, fmt.Errorf("at most %d %s references are supported across aliases", limit, kind)
			}
		}
	}
	return refs, nil
}

func buildGeminiOmniVideoURL(base string, endpoint GrokMediaEndpoint, id string) (string, error) {
	base = strings.TrimRight(base, "/")
	// Gemini accounts often include /v1beta in their base URL. Videos use the
	// OpenAI-compatible /v1 path on that same configured host and path prefix.
	base = strings.TrimSuffix(strings.TrimSuffix(base, "/v1beta"), "/v1")
	target := base + "/v1/videos"
	if endpoint == GrokMediaEndpointVideosGenerations {
		return target, nil
	}
	if endpoint != GrokMediaEndpointVideoStatus && endpoint != GrokMediaEndpointVideoContent {
		return "", fmt.Errorf("Gemini Omni does not support this video operation")
	}
	if strings.TrimSpace(id) == "" || validateUpstreamPathSegment("Gemini Omni task ID", id) != nil {
		return "", fmt.Errorf("invalid Gemini Omni task ID")
	}
	target += "/" + url.PathEscape(id)
	if endpoint == GrokMediaEndpointVideoContent {
		target += "/content"
	}
	return target, nil
}

func normalizeGeminiOmniVideoResponse(body []byte, requestedID, proxyURL string) ([]byte, bool, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, false, fmt.Errorf("invalid Gemini Omni video response")
	}
	id := strings.TrimSpace(firstNonEmpty(gjson.GetBytes(body, "id").String(), gjson.GetBytes(body, "request_id").String()))
	if id == "" || validateUpstreamPathSegment("Gemini Omni task ID", id) != nil || (requestedID != "" && requestedID != id) {
		return nil, false, fmt.Errorf("invalid Gemini Omni response task ID")
	}
	payload := map[string]any{"id": id, "request_id": id, "object": "video.generation.task"}
	complete := false
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String())) {
	case "queued", "pending", "in_queue", "in_progress", "running":
		payload["status"] = "pending"
	case "completed", "done", "succeeded":
		complete = true
		payload["status"], payload["video"] = "done", map[string]string{"url": proxyURL}
	case "failed", "cancelled", "canceled", "expired":
		payload["status"] = "failed"
		if value := gjson.GetBytes(body, "error"); value.Exists() && value.Type != gjson.Null {
			payload["error"] = json.RawMessage(value.Raw)
		} else {
			payload["error"] = map[string]string{"message": "Gemini Omni video generation failed"}
		}
	default:
		return nil, false, fmt.Errorf("unknown Gemini Omni video task status")
	}
	for _, field := range []string{"progress", "created_at", "model"} {
		if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.Null {
			payload[field] = value.Value()
		}
	}
	result, err := json.Marshal(payload)
	return result, complete, err
}

// ForwardGeminiOmniVideo intentionally returns ordinary errors, never failover
// errors: an ambiguous create failure may already have submitted a paid job.
// Account credentials remain Gemini credentials even though videos use the
// documented OpenAI-compatible Bearer /v1/videos API.
func (s *OpenAIGatewayService) ForwardGeminiOmniVideo(ctx context.Context, c *gin.Context, account *Account, endpoint GrokMediaEndpoint, requestID string, body []byte) (*OpenAIForwardResult, error) {
	if !SupportsGeminiOmniVideoAccount(account) {
		return nil, fmt.Errorf("Gemini Omni requires a Gemini API key account with a custom base URL")
	}
	base, err := s.validateUpstreamBaseURL(account.GetCredential("base_url"))
	if err != nil {
		return nil, err
	}
	lookupEndpoint := endpoint
	if endpoint == GrokMediaEndpointVideoContent {
		lookupEndpoint = GrokMediaEndpointVideoStatus
	}
	target, err := buildGeminiOmniVideoURL(base, lookupEndpoint, requestID)
	if err != nil {
		return nil, err
	}
	info := GrokMediaRequestInfo{Model: "gemini-omni-1.2-flash"}
	upstreamModel := info.Model
	billingModel := info.Model
	if endpoint == GrokMediaEndpointVideosGenerations {
		var payload map[string]any
		info, payload, err = parseGeminiOmniVideoRequest(body)
		if err != nil {
			return nil, err
		}
		// Validate the requested provider's parameters before replacing its name
		// with an arbitrary channel/account alias. Pricing uses the channel's
		// requested-versus-mapped setting, never the account's routing alias.
		mappedModel := info.Model
		billingModel = info.Model
		if apiKey := getAPIKeyFromContext(c); apiKey != nil {
			mappedModel = s.ResolveVideoStudioRoutingModel(ctx, apiKey.GroupID, info.Model)
			if apiKey.Group != nil {
				billingModel, _, err = s.ResolveVideoStudioModels(ctx, apiKey, info.Model)
				if err != nil {
					return nil, err
				}
			}
		}
		upstreamModel = account.GetMappedModel(mappedModel)
		payload["model"] = upstreamModel
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	token := strings.TrimSpace(account.GetCredential("api_key"))
	if token == "" {
		return nil, fmt.Errorf("Gemini Omni account missing api_key")
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
		return nil, fmt.Errorf("Gemini Omni video request failed: %w", err)
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
		return nil, fmt.Errorf("Gemini Omni upstream status %d", resp.StatusCode)
	}
	id := strings.TrimSpace(firstNonEmpty(gjson.GetBytes(responseBody, "id").String(), gjson.GetBytes(responseBody, "request_id").String()))
	normalized, complete, err := normalizeGeminiOmniVideoResponse(responseBody, requestID, grokMediaContentProxyURL(c, id))
	if err != nil {
		return nil, err
	}
	result := &OpenAIForwardResult{
		RequestID: resp.Header.Get("x-request-id"), ResponseID: id, Model: info.Model, BillingModel: billingModel,
		UpstreamModel: upstreamModel, ResponseHeaders: resp.Header.Clone(), UpstreamHeaders: resp.Header,
		GrokMediaResponseBody: normalized, VideoResolution: info.Resolution, VideoDurationSeconds: info.DurationSeconds,
	}
	if endpoint == GrokMediaEndpointVideoContent {
		if !complete {
			writeGrokMediaErrorResponse(c, http.StatusConflict, "video_not_ready", "Video content is not available")
			return nil, fmt.Errorf("Gemini Omni video content is not available")
		}
		if err := s.forwardGeminiOmniVideoContent(upstreamCtx, c, account, base, proxy, requestID, token); err != nil {
			return nil, err
		}
	} else {
		writeGrokMediaResponse(c, resp, normalized, s.responseHeaderFilter)
	}
	if complete && endpoint != GrokMediaEndpointVideosGenerations {
		result.VideoCount = 1
	}
	result.Duration = time.Since(started)
	return result, nil
}

func (s *OpenAIGatewayService) forwardGeminiOmniVideoContent(ctx context.Context, c *gin.Context, account *Account, base, proxy, requestID, token string) error {
	target, err := buildGeminiOmniVideoURL(base, GrokMediaEndpointVideoContent, requestID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(ctx), http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	// Download the complete file from the authenticated configured endpoint;
	// never follow provider-returned URLs or forward the client's Range header.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "video/*,application/octet-stream;q=0.8")
	account.ApplyHeaderOverrides(req.Header)
	resp, err := s.httpUpstream.Do(req, proxy, account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gemini Omni content download returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxStoredVideoBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxStoredVideoBytes {
		return fmt.Errorf("Gemini Omni video exceeds %d bytes", maxStoredVideoBytes)
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
	resp.Header.Del("Content-Range")
	resp.ContentLength = int64(len(data))
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return writeGrokMediaContentResponse(c, resp)
}

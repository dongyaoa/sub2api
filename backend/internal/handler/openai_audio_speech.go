package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// AudioSpeech serves the OpenAI-compatible binary speech endpoint. Gemini
// API-key providers can expose this protocol independently of generateContent.
func (h *OpenAIGatewayHandler) AudioSpeech(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group == nil {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "An API key group is required")
		return
	}
	if platform := effectiveAPIKeyPlatform(c, apiKey); platform != service.PlatformOpenAI && platform != service.PlatformGemini && platform != service.PlatformComposite {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Audio Speech API is only available for OpenAI and Gemini API key accounts")
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	body, reqModel, input, err := service.ParseAudioSpeechRequest(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	platform := effectiveAPIKeyPlatform(c, apiKey)
	if platform != service.PlatformOpenAI && platform != service.PlatformGemini {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "The speech model must route to an OpenAI or Gemini API key account")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(c, "handler.openai_gateway.audio_speech", zap.String("model", reqModel), zap.Int64("user_id", subject.UserID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}
	setOpsRequestContext(c, reqModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	// Audit the spoken text using the existing chat text extractor, while the
	// provider receives the original voice/style/extension fields intact.
	auditMessages := []map[string]any{{"role": "user", "content": input}}
	for _, field := range []string{"instructions", "style_prompt"} {
		if instruction := gjson.GetBytes(body, field); instruction.Type == gjson.String && instruction.String() != "" {
			auditMessages = append(auditMessages, map[string]any{"role": "system", "content": instruction.String()})
		}
	}
	auditBody, _ := json.Marshal(map[string]any{"messages": auditMessages})
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, reqModel, auditBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)
	forwardModel := openAIChannelForwardModel(channelMapping, reqModel)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	streamStarted := false
	userRelease, acquired := h.acquireOpenAIUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog, openAISlotReleaseOnReturn)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		h.audioSpeechBillingError(c, err)
		return
	}
	// Speech has a configured per-call price and no token usage. The text
	// profitability policy cannot estimate its cost.
	c.Request = c.Request.WithContext(service.WithOpenAIProfitControlSuppressed(c.Request.Context()))
	failed := make(map[int64]struct{})
	var last *service.UpstreamFailoverError
	maxSwitches := h.maxAccountSwitches
	if maxSwitches <= 0 {
		maxSwitches = 3
	}
	for attempt := 0; attempt <= maxSwitches; attempt++ {
		if failoverClientGone(c) {
			return
		}
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", "", forwardModel, failed,
			service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityAudioSpeech,
			false, false, false, platform,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if last != nil {
				h.handleFailoverExhausted(c, last, false)
			} else {
				markOpsRoutingCapacityLimited(c)
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available API key accounts for this speech model")
			}
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountRelease, slotResult := h.acquireOpenAIAccountSlotWithReleasePolicy(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog, nil, openAISlotReleaseOnReturn)
		if slotResult != openAISlotAcquireOK {
			return
		}
		// Reserve only after account mapping is known, using the same price
		// snapshot as settlement. Every attempt releases its own reservation.
		attemptedUpstream := false
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			price, priceErr := h.gatewayService.PrepareAudioSpeechBilling(c.Request.Context(), apiKey, account, reqModel, channelMapping)
			if priceErr != nil {
				if errors.Is(priceErr, service.ErrAudioSpeechPricingNotConfigured) {
					service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
					h.errorResponse(c, http.StatusBadRequest, "audio_speech_pricing_required", "Configure an explicit flat per-request price without pricing tiers for this speech model in its group or channel (model square pricing)")
				} else {
					h.errorResponse(c, http.StatusInternalServerError, "api_error", "Speech pricing is unavailable")
				}
				return nil, priceErr
			}
			inflightDone, reserveErr := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, price.InflightEstimate())
			if reserveErr != nil {
				h.audioSpeechBillingError(c, reserveErr)
				return nil, reserveErr
			}
			defer inflightDone()
			forwardBody := body
			if channelMapping.Mapped {
				forwardBody = h.gatewayService.ReplaceModelInBody(body, channelMapping.MappedModel)
			}
			started := time.Now()
			attemptedUpstream = true
			forwardResult, err := h.gatewayService.ForwardAudioSpeech(c.Request.Context(), c, account, forwardBody, "")
			service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(started).Milliseconds())
			// A complete upstream synthesis is billable even if delivery to the
			// client fails. Incomplete/non-audio upstream results return nil.
			if forwardResult != nil && forwardResult.AudioSpeech {
				forwardResult.AudioSpeechBilling = price
				forwardResult.BillingModel = price.BillingModel
				h.recordAudioSpeechUsage(c, apiKey, account, subscription, channelMapping, reqModel, body, forwardResult)
			}
			return forwardResult, err
		}()
		if result != nil && result.AudioSpeech {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, forwardModel, true, nil)
			if forwardErr != nil {
				reqLog.Info("audio_speech.client_delivery_failed", zap.Error(forwardErr))
			}
			return
		}
		if forwardErr == nil {
			return
		}
		if !attemptedUpstream {
			// Local price/balance errors are already reported and say nothing
			// about the selected provider's health.
			return
		}
		h.gatewayService.ReportOpenAIAccountScheduleResult(account, forwardModel, false, nil, forwardErr)
		var retry *service.UpstreamFailoverError
		if !c.Writer.Written() && errors.As(forwardErr, &retry) && retry.ShouldRetryNextAccount() && !failoverClientGone(c) {
			failed[account.ID] = struct{}{}
			last = retry
			h.gatewayService.RecordOpenAIAccountSwitch()
			continue
		}
		if !c.Writer.Written() && !failoverClientGone(c) {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Speech generation failed")
		}
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

func (h *OpenAIGatewayHandler) audioSpeechBillingError(c *gin.Context, err error) {
	status, code, message, retryAfter := billingErrorDetails(err)
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}
	h.errorResponse(c, status, code, message)
}

func (h *OpenAIGatewayHandler) recordAudioSpeechUsage(c *gin.Context, apiKey *service.APIKey, account *service.Account, subscription *service.UserSubscription, mapping service.ChannelMappingResult, requestedModel string, body []byte, result *service.OpenAIForwardResult) {
	upstream := result.UpstreamEndpoint
	if upstream == "" {
		upstream = EndpointAudioSpeech
	}
	input := &service.OpenAIRecordUsageInput{
		Result: result, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
		InboundEndpoint: EndpointAudioSpeech, UpstreamEndpoint: upstream,
		UserAgent: c.GetHeader("User-Agent"), IPAddress: ip.GetClientIP(c),
		RequestPayloadHash: service.HashUsageRequestPayload(body), APIKeyService: h.apiKeyService,
		QuotaPlatform: service.QuotaPlatform(c.Request.Context(), apiKey), SessionID: service.ExtractClientSessionID(c),
		ChannelUsageFields: clientRequestedUsageFields(c, mapping, requestedModel, result.UpstreamModel),
	}
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, input); err != nil {
			logger.L().Error("audio_speech.record_usage_failed", zap.Int64("account_id", account.ID), zap.Int64("api_key_id", apiKey.ID), zap.Error(err))
		}
	})
}

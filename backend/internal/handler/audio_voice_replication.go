package handler

import (
	"errors"
	"net/http"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AudioVoiceReplication accepts the site's API key, then creates a stateless
// voice with the provider and returns an encrypted, owner-bound gateway ID.
func (h *OpenAIGatewayHandler) AudioVoiceReplication(c *gin.Context) {
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
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Voice replication requires an OpenAI or Gemini API key account")
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
		} else {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read voice recordings")
		}
		return
	}
	request, err := service.ParseAudioVoiceReplicationRequest(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, request.Model)
	platform := effectiveAPIKeyPlatform(c, apiKey)
	if platform != service.PlatformOpenAI && platform != service.PlatformGemini {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "The voice model must route to an OpenAI or Gemini API key account")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(c, "handler.audio_voice_replication", zap.String("model", request.Model), zap.Int64("user_id", subject.UserID))
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}
	if err := h.gatewayService.CheckAudioVoiceService(); err != nil {
		h.audioVoiceBindingError(c, err)
		return
	}
	setOpsRequestContext(c, request.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	// Recordings are forwarded only to the chosen provider for its speaker and
	// consent verification. They are never put into text audit/prompt logs.
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, request.Model)
	forwardModel := openAIChannelForwardModel(channelMapping, request.Model)
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
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available accounts for this voice model")
			}
			return
		}
		accountRelease, slotResult := h.acquireOpenAIAccountSlotWithReleasePolicy(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog, nil, openAISlotReleaseOnReturn)
		if slotResult != openAISlotAcquireOK {
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		attemptedUpstream := false
		completed, forwardErr := func() (bool, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			price, priceErr := h.gatewayService.PrepareAudioSpeechBilling(c.Request.Context(), apiKey, account, request.Model, channelMapping)
			if priceErr != nil {
				if errors.Is(priceErr, service.ErrAudioSpeechPricingNotConfigured) {
					service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalModelConfiguration)
					h.errorResponse(c, http.StatusBadRequest, "audio_speech_pricing_required", "Configure a flat per-request model price without pricing tiers; creating a cloned voice is one request")
				} else {
					h.errorResponse(c, http.StatusInternalServerError, "api_error", "Voice pricing is unavailable")
				}
				return false, priceErr
			}
			inflightDone, reserveErr := reserveInflightBalance(c, h.billingCacheService, h.gatewayService, apiKey, subscription, price.InflightEstimate())
			if reserveErr != nil {
				h.audioSpeechBillingError(c, reserveErr)
				return false, reserveErr
			}
			defer inflightDone()
			forwardRequest := *request
			forwardRequest.Model = forwardModel
			attemptedUpstream = true
			created, err := h.gatewayService.CreateAudioVoiceUpstream(c.Request.Context(), c, account, &forwardRequest, "")
			if created == nil || created.ForwardResult == nil {
				return false, err
			}
			result := created.ForwardResult
			result.AudioSpeechBilling = price
			result.BillingModel = price.BillingModel
			result.ClientDisconnect = c.Request.Context().Err() != nil
			// A verified upstream voice is a completed paid operation, even
			// when the client disconnects or issuing its local envelope fails.
			h.recordAudioUsage(c, apiKey, account, subscription, channelMapping, request.Model, body, result, EndpointAudioVoices, EndpointGeminiVoices)
			id, sealErr := h.gatewayService.SealAudioVoice(apiKey, account, result.UpstreamModel, created.VoiceKey, created.ExpireTime)
			if sealErr != nil {
				h.errorResponse(c, http.StatusInternalServerError, "voice_service_unavailable", "Could not issue the cloned voice ID")
				return true, sealErr
			}
			binding, openErr := h.gatewayService.OpenAudioVoice(apiKey, id)
			if openErr != nil {
				h.errorResponse(c, http.StatusInternalServerError, "voice_service_unavailable", "Could not validate the cloned voice ID")
				return true, openErr
			}
			if !result.ClientDisconnect {
				c.JSON(http.StatusCreated, gin.H{
					"id": id, "object": "voice", "model": clientRequestedModel(c, request.Model),
					"display_name": request.DisplayName, "created_at": time.Now().Unix(), "expires_at": binding.ExpiresAt,
				})
			}
			return true, nil
		}()
		if completed {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account, forwardModel, true, nil)
			return
		}
		if !attemptedUpstream {
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
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Voice replication failed")
		}
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

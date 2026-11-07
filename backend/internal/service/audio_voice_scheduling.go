package service

import (
	"context"
	"crypto/hmac"
	"strings"
	"time"
)

// RecheckAudioVoiceAccount loads the bound account from the repository, never
// from a stale scheduler snapshot. Call again after waiting for a slot so a
// credential rotation or group removal during the wait fails closed.
// requestedModel is the channel-mapped model that will be sent to the forwarder.
func (s *OpenAIGatewayService) RecheckAudioVoiceAccount(ctx context.Context, apiKey *APIKey, binding *AudioVoiceBinding, requestedModel string) (*Account, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.accountRepo == nil {
		return nil, ErrAudioVoiceServiceUnavailable
	}
	if !binding.validFor(apiKey, time.Now()) || strings.TrimSpace(requestedModel) == "" {
		return nil, ErrAudioVoiceInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	account, err := s.accountRepo.GetByID(ctx, binding.AccountID)
	if err != nil || account == nil || account.ID != binding.AccountID {
		return nil, ErrAudioVoiceUnavailable
	}
	if !hmac.Equal([]byte(audioVoiceCredentialFingerprint(account)), []byte(binding.credentialFingerprint)) {
		return nil, ErrAudioVoiceInvalid
	}
	upstreamModel := normalizeOpenAIModelForUpstream(account, resolveOpenAIForwardModel(account, requestedModel, ""))
	if upstreamModel != binding.UpstreamModel {
		return nil, ErrAudioVoiceInvalid
	}
	publicModel, routingModel := audioVoiceModelsFromContext(ctx, requestedModel)
	if !apiKey.Group.ModelAllowlist.Allows(publicModel) {
		return nil, ErrAudioVoiceUnavailable
	}
	platform := apiKey.Group.Platform
	if resolved, ok := ResolvedTargetPlatformFromContext(ctx); ok {
		platform = resolved
	}
	if (platform != PlatformOpenAI && platform != PlatformGemini) || !openAIStickyAccountMatchesGroup(account, apiKey.GroupID) {
		return nil, ErrAudioVoiceUnavailable
	}
	ctx = s.withOpenAIQuotaAutoPauseContext(ctx)
	ctx = s.withOpenAIGroupPrivacyRequirement(ctx, apiKey.GroupID)
	ctx = s.withOpenAIProfitControlGate(ctx, apiKey.GroupID)
	if !s.audioVoiceChannelAllows(ctx, *apiKey.GroupID, publicModel, routingModel, upstreamModel) ||
		!isOpenAICompatibleAccountEligibleForRequest(ctx, account, platform, requestedModel, false, OpenAIEndpointCapabilityAudioSpeech) ||
		s.isOpenAIAccountBlockedBySchedulingThreshold(ctx, account) {
		return nil, ErrAudioVoiceUnavailable
	}
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	req := OpenAIAccountScheduleRequest{
		GroupID: apiKey.GroupID, Platform: platform, RequestedModel: requestedModel,
		RequiredCapability: OpenAIEndpointCapabilityAudioSpeech, RequiredTransport: OpenAIUpstreamTransportHTTPSSE,
		RequirePrivacySet: apiKey.Group.RequirePrivacySet || s.openAIGroupRequiresPrivacySet(ctx, apiKey.GroupID),
	}
	if !scheduler.isAccountRequestCompatible(ctx, account, req) || !scheduler.isAccountTransportCompatible(account, req.RequiredTransport) {
		return nil, ErrAudioVoiceUnavailable
	}
	SelectAccountProxy(account)
	return account, nil
}

func audioVoiceModelsFromContext(ctx context.Context, fallback string) (public, routing string) {
	if models, ok := ctx.Value(audioVoiceRequestModelsContextKey{}).(audioVoiceRequestModels); ok {
		public, routing = models.public, models.routing
	}
	if public == "" {
		public, _ = RequestedPublicModelFromContext(ctx)
	}
	if public == "" {
		public = fallback
	}
	if routing == "" {
		routing, _ = ResolvedUpstreamModelFromContext(ctx)
	}
	if routing == "" {
		routing = public
	}
	return public, routing
}

func (s *OpenAIGatewayService) audioVoiceChannelAllows(ctx context.Context, groupID int64, publicModel, routingModel, upstreamModel string) bool {
	if s.channelService == nil {
		return true
	}
	lookup, err := s.channelService.lookupGroupChannel(ctx, groupID)
	if err != nil {
		return false
	}
	if lookup == nil {
		return true
	}
	mapping := resolveMapping(lookup, groupID, routingModel)
	model := mapping.MappedModel
	switch mapping.BillingModelSource {
	case BillingModelSourceRequested:
		model = publicModel
	case BillingModelSourceUpstream:
		model = upstreamModel
	}
	return !checkRestricted(lookup, groupID, model)
}

// SelectAudioVoiceAccount acquires or waits on exactly the original voice's
// account. It never fails over a supplier-bound voice key to another account.
func (s *OpenAIGatewayService) SelectAudioVoiceAccount(ctx context.Context, apiKey *APIKey, binding *AudioVoiceBinding, requestedModel string) (*AccountSelectionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	account, err := s.RecheckAudioVoiceAccount(ctx, apiKey, binding, requestedModel)
	if err != nil {
		return nil, err
	}
	ctx = s.withOpenAIProfitControlGate(ctx, apiKey.GroupID)
	acquired, err := s.tryAcquireAccountSlot(ctx, account.ID, account.Concurrency, account)
	if err != nil {
		return nil, err
	}
	if acquired != nil && acquired.Acquired {
		// Narrow the race between selection and eager slot admission. The
		// handler also rechecks immediately before sending a bound voice.
		latest, err := s.RecheckAudioVoiceAccount(ctx, apiKey, binding, requestedModel)
		if err != nil {
			if acquired.ReleaseFunc != nil {
				acquired.ReleaseFunc()
			}
			return nil, err
		}
		return attachSelectionProfitGate(ctx, &AccountSelectionResult{
			Account: latest, Acquired: true, ReleaseFunc: acquired.ReleaseFunc,
		}), nil
	}
	if s.concurrencyService == nil {
		return nil, ErrAudioVoiceUnavailable
	}
	cfg := s.schedulingConfig()
	timeout, maxWaiting := cfg.StickySessionWaitTimeout, cfg.StickySessionMaxWaiting
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	if maxWaiting <= 0 {
		maxWaiting = 3
	}
	return attachSelectionProfitGate(ctx, &AccountSelectionResult{
		Account:  account,
		WaitPlan: &AccountWaitPlan{AccountID: account.ID, MaxConcurrency: EffectiveAccountConcurrency(account), Timeout: timeout, MaxWaiting: maxWaiting},
	}), nil
}

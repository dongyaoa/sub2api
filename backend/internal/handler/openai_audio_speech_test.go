package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const audioSpeechHandlerModel = "gemini-3.8-flash-tts"

var audioSpeechHandlerWAV = []byte{'R', 'I', 'F', 'F', 36, 0, 0, 0, 'W', 'A', 'V', 'E', 0, 255, 128, 1}

type audioSpeechHandlerCache struct {
	*handlerInflightCache
	charged float64
}

func (c *audioSpeechHandlerCache) DeductUserBalance(_ context.Context, _ int64, amount float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.balance -= amount
	c.charged += amount
	return nil
}

type audioSpeechHandlerBillingRepo struct {
	service.UsageBillingRepository
	mu       sync.Mutex
	commands []*service.UsageBillingCommand
	seen     map[string]bool
}

func (r *audioSpeechHandlerBillingRepo) Apply(_ context.Context, command *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[command.RequestID] {
		return &service.UsageBillingApplyResult{}, nil
	}
	r.seen[command.RequestID] = true
	r.commands = append(r.commands, command)
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

type audioSpeechHandlerUpstream struct {
	service.HTTPUpstream
	requests   []*http.Request
	bodies     [][]byte
	accountIDs []int64
	respond    func(*http.Request, int) (*http.Response, error)
}

func (u *audioSpeechHandlerUpstream) Do(request *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	u.requests = append(u.requests, request)
	u.bodies = append(u.bodies, body)
	u.accountIDs = append(u.accountIDs, accountID)
	return u.respond(request, len(u.requests))
}

func audioSpeechHandlerResponse(status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"supplier-reuses-id"}},
		ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body)),
	}
}

type audioSpeechHandlerFixture struct {
	handler         *OpenAIGatewayHandler
	key             *service.APIKey
	accounts        *openAIWSFailoverHandlerAccountRepoStub
	upstream        *audioSpeechHandlerUpstream
	usage           *openAIWSUsageHandlerUsageLogRepoStub
	billing         *audioSpeechHandlerBillingRepo
	cache           *audioSpeechHandlerCache
	slots           *concurrencyCacheMock
	accountAcquires atomic.Int32
	userAcquires    atomic.Int32
}

func newAudioSpeechHandlerFixture(t *testing.T, platform string) *audioSpeechHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(4301)
	price := 0.2
	f := &audioSpeechHandlerFixture{
		accounts: &openAIWSFailoverHandlerAccountRepoStub{},
		upstream: &audioSpeechHandlerUpstream{respond: func(*http.Request, int) (*http.Response, error) {
			return audioSpeechHandlerResponse(http.StatusOK, "audio/wav", audioSpeechHandlerWAV), nil
		}},
		usage:   &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 16)},
		billing: &audioSpeechHandlerBillingRepo{seen: map[string]bool{}},
		cache:   &audioSpeechHandlerCache{handlerInflightCache: newHandlerInflightCache(100)},
	}
	f.key = &service.APIKey{
		ID: 4302, UserID: 4303, GroupID: &groupID, User: &service.User{ID: 4303, Balance: 100, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Hydrated: true, Status: service.StatusActive, Platform: platform, RateMultiplier: 0.5,
			ModelPricing: []service.ChannelModelPricing{{Models: []string{audioSpeechHandlerModel}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &price}}},
	}
	f.accounts.accounts = []service.Account{{
		ID: 4304, Name: "speech-upstream", Platform: platform, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 2, GroupIDs: []int64{groupID},
		AccountGroups: []service.AccountGroup{{AccountID: 4304, GroupID: groupID}},
		Credentials: map[string]any{"api_key": "test-speech-key", "base_url": "https://speech.example/v1",
			"model_mapping": map[string]any{audioSpeechHandlerModel: audioSpeechHandlerModel}},
	}}
	f.slots = &concurrencyCacheMock{
		acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) {
			f.userAcquires.Add(1)
			return true, nil
		},
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
			f.accountAcquires.Add(1)
			return true, nil
		},
	}
	concurrency := service.NewConcurrencyService(f.slots)
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	cfg.Billing.InflightReservation = config.InflightReservationConfig{Enabled: true, TTLSeconds: 60}
	billingCache := service.NewBillingCacheService(f.cache, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	billingService := service.NewBillingService(cfg, nil)
	gateway := service.NewOpenAIGatewayService(
		f.accounts, f.usage, f.billing, nil, nil, nil, nil, cfg, nil, concurrency, billingService,
		nil, billingCache, f.upstream, &service.DeferredService{}, nil, nil,
		service.NewModelPricingResolver(nil, billingService), nil, nil, nil, nil,
	)
	f.handler = NewOpenAIGatewayHandler(gateway, concurrency, billingCache, &service.APIKeyService{}, nil, nil, nil, nil, cfg)
	return f
}

func (f *audioSpeechHandlerFixture) request(ctx context.Context, body string) *httptest.ResponseRecorder {
	ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-reuses-id")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), f.key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: f.key.UserID, Concurrency: 2})
		c.Next()
	})
	router.Use(middleware.GroupModelAllowlist())
	router.POST("/v1/audio/speech", f.handler.AudioSpeech)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func (f *audioSpeechHandlerFixture) assertReleased(t *testing.T, pendingReservations int) {
	t.Helper()
	require.Equal(t, f.accountAcquires.Load(), atomic.LoadInt32(&f.slots.releaseAccountCalled))
	require.Equal(t, f.userAcquires.Load(), atomic.LoadInt32(&f.slots.releaseUserCalled))
	require.Equal(t, pendingReservations, f.cache.count())
}

func TestAudioSpeechHandlerBillsOneCallAndPreservesVoiceParameters(t *testing.T) {
	for _, platform := range []string{service.PlatformOpenAI, service.PlatformGemini} {
		t.Run(platform, func(t *testing.T) {
			f := newAudioSpeechHandlerFixture(t, platform)
			body := `{"model":"gemini-3.8-flash-tts","text":"你好，语音测试","style_prompt":"warm tone","voice":{"id":"custom-voice-id"},"speakers":[{"speaker":"A","voice":"Kore"}],"provider_extra":{"strength":0.5},"response_format":"wav"}`
			for call := 0; call < 2; call++ {
				rec := f.request(context.Background(), body)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
				require.Equal(t, audioSpeechHandlerWAV, rec.Body.Bytes())
				f.assertReleased(t, 0)
			}
			require.Len(t, f.upstream.requests, 2)
			require.Len(t, f.billing.commands, 2, "reused client/upstream ids must not suppress independent successful calls")
			require.Len(t, f.usage.created, 2)
			require.NotEqual(t, f.billing.commands[0].RequestID, f.billing.commands[1].RequestID)
			for _, command := range f.billing.commands {
				require.InDelta(t, 0.1, command.BalanceCost, 1e-12)
				require.True(t, strings.HasPrefix(command.RequestID, "audio_speech:"))
				require.Zero(t, command.InputTokens)
				require.Zero(t, command.OutputTokens)
			}
			log := <-f.usage.created
			require.Equal(t, audioSpeechHandlerModel, log.RequestedModel)
			require.Equal(t, "/v1/audio/speech", *log.InboundEndpoint)
			require.Equal(t, "/v1/audio/speech", *log.UpstreamEndpoint)
			require.Equal(t, string(service.BillingModePerRequest), *log.BillingMode)
			require.InDelta(t, 0.1, log.ActualCost, 1e-12)
			require.InDelta(t, 0.2, f.cache.charged, 1e-12)
			require.Equal(t, "https://speech.example/v1/audio/speech", f.upstream.requests[0].URL.String())
			require.Equal(t, "Bearer test-speech-key", f.upstream.requests[0].Header.Get("Authorization"))
			require.Equal(t, "你好，语音测试", gjson.GetBytes(f.upstream.bodies[0], "input").String())
			require.Equal(t, "warm tone", gjson.GetBytes(f.upstream.bodies[0], "instructions").String())
			require.Equal(t, "custom-voice-id", gjson.GetBytes(f.upstream.bodies[0], "voice.id").String())
			require.Equal(t, "Kore", gjson.GetBytes(f.upstream.bodies[0], "speakers.0.voice").String())
			require.Equal(t, 0.5, gjson.GetBytes(f.upstream.bodies[0], "provider_extra.strength").Float())
		})
	}
}

func TestAudioSpeechHandlerMissingPriceReleasesSlotsBeforeUpstream(t *testing.T) {
	for _, scenario := range []string{"missing price", "pricing tiers"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
			if scenario == "missing price" {
				f.key.Group.ModelPricing = nil
			} else {
				tierPrice := 0.4
				f.key.Group.ModelPricing[0].Intervals = []service.PricingInterval{{TierLabel: "large", PerRequestPrice: &tierPrice}}
			}
			rec := f.request(context.Background(), `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "audio_speech_pricing_required")
			require.Contains(t, rec.Body.String(), "without pricing tiers")
			require.Empty(t, f.upstream.requests)
			require.Empty(t, f.billing.commands)
			require.Empty(t, f.usage.created)
			require.Greater(t, f.accountAcquires.Load(), int32(0), "exercise the post-acquisition pricing rejection")
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioSpeechHandlerReservationRejectionReleasesAccount(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
	f.cache.res["already-pending"] = 99.95
	rec := f.request(context.Background(), `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
	require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, strings.ToLower(rec.Body.String()), "balance")
	require.Empty(t, f.upstream.requests)
	require.Empty(t, f.billing.commands)
	f.assertReleased(t, 1)
}

func TestAudioSpeechHandlerUpstreamBadRequestDoesNotBill(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformOpenAI)
	f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
		return audioSpeechHandlerResponse(http.StatusBadRequest, "application/json", []byte(`{"error":{"message":"invalid voice"}}`)), nil
	}
	rec := f.request(context.Background(), `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "invalid voice")
	require.Len(t, f.upstream.requests, 1)
	require.Empty(t, f.billing.commands)
	require.Empty(t, f.usage.created)
	require.Zero(t, f.cache.charged)
	f.assertReleased(t, 0)
}

func TestAudioSpeechHandlerFailoverBillsOnlySuccessfulSynthesis(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
	second := f.accounts.accounts[0]
	second.ID = 4305
	second.Priority = 1
	second.AccountGroups = []service.AccountGroup{{AccountID: 4305, GroupID: *f.key.GroupID}}
	f.accounts.accounts = append(f.accounts.accounts, second)
	f.upstream.respond = func(_ *http.Request, attempt int) (*http.Response, error) {
		if attempt == 1 {
			return audioSpeechHandlerResponse(http.StatusTooManyRequests, "application/json", []byte(`{"error":{"message":"rate limited"}}`)), nil
		}
		return audioSpeechHandlerResponse(http.StatusOK, "audio/wav", audioSpeechHandlerWAV), nil
	}
	rec := f.request(context.Background(), `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, audioSpeechHandlerWAV, rec.Body.Bytes())
	require.Len(t, f.upstream.accountIDs, 2)
	require.NotEqual(t, f.upstream.accountIDs[0], f.upstream.accountIDs[1])
	require.Len(t, f.billing.commands, 1)
	require.Len(t, f.usage.created, 1)
	require.InDelta(t, 0.1, f.cache.charged, 1e-12)
	f.assertReleased(t, 0)
}

func TestAudioSpeechHandlerClientCancellationAfterSynthesisStillBills(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.upstream.respond = func(request *http.Request, _ int) (*http.Response, error) {
		cancel()
		require.NoError(t, request.Context().Err(), "upstream synthesis lifecycle must outlive client disconnect")
		return audioSpeechHandlerResponse(http.StatusOK, "audio/wav", audioSpeechHandlerWAV), nil
	}
	rec := f.request(ctx, `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
	require.Empty(t, rec.Body.Bytes(), "do not append a JSON error after a canceled delivery")
	require.Len(t, f.upstream.requests, 1)
	require.Len(t, f.billing.commands, 1)
	require.Len(t, f.usage.created, 1)
	require.InDelta(t, 0.1, f.cache.charged, 1e-12)
	f.assertReleased(t, 0)
}

func TestAudioSpeechHandlerAlreadyCanceledDoesNotGenerateOrBill(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformOpenAI)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.request(ctx, `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
	require.Empty(t, f.upstream.requests)
	require.Empty(t, f.billing.commands)
	f.assertReleased(t, 0)
}

func TestAudioSpeechHandlerRejectsUnsupportedAccountAndModel(t *testing.T) {
	for _, scenario := range []string{"oauth", "official gemini", "unmapped model"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
			model := audioSpeechHandlerModel
			switch scenario {
			case "oauth":
				f.accounts.accounts[0].Type = service.AccountTypeOAuth
			case "official gemini":
				f.accounts.accounts[0].Credentials["base_url"] = "https://generativelanguage.googleapis.com"
			case "unmapped model":
				model = "not-available"
			}
			rec := f.request(context.Background(), `{"model":"`+model+`","input":"hello"}`)
			require.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Empty(t, f.upstream.requests)
			require.Empty(t, f.billing.commands)
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioSpeechHandlerKeepsSlotsUntilDetachedSynthesisCompletes(t *testing.T) {
	f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, finish := make(chan struct{}), make(chan struct{})
	var finishOnce sync.Once
	releaseUpstream := func() { finishOnce.Do(func() { close(finish) }) }
	defer releaseUpstream()
	f.upstream.respond = func(*http.Request, int) (*http.Response, error) {
		close(started)
		<-finish
		return audioSpeechHandlerResponse(http.StatusOK, "audio/wav", audioSpeechHandlerWAV), nil
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- f.request(ctx, `{"model":"gemini-3.8-flash-tts","input":"hello"}`) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("speech request never reached the upstream")
	}
	require.Equal(t, int32(1), f.userAcquires.Load())
	require.Equal(t, int32(1), f.accountAcquires.Load())
	require.Equal(t, 1, f.cache.count())
	cancel()
	require.Never(t, func() bool {
		return atomic.LoadInt32(&f.slots.releaseUserCalled) != 0 || atomic.LoadInt32(&f.slots.releaseAccountCalled) != 0 || f.cache.count() != 1
	}, 100*time.Millisecond, time.Millisecond, "canceling delivery must not release capacity or reservation while synthesis runs")
	select {
	case <-done:
		t.Fatal("handler returned before upstream synthesis completed")
	default:
	}
	releaseUpstream()
	select {
	case rec := <-done:
		require.Empty(t, rec.Body.Bytes())
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after upstream synthesis completed")
	}
	require.Len(t, f.billing.commands, 1)
	require.InDelta(t, 0.1, f.cache.charged, 1e-12)
	f.assertReleased(t, 0)
}

func TestAudioSpeechHandlerQueueWaitRemainsCancelable(t *testing.T) {
	for _, slot := range []string{"user", "account"} {
		t.Run(slot, func(t *testing.T) {
			f := newAudioSpeechHandlerFixture(t, service.PlatformGemini)
			f.handler.cfg.Gateway.Scheduling.FallbackWaitTimeout = 10 * time.Second
			f.handler.cfg.Gateway.Scheduling.FallbackMaxWaiting = 10
			queued := make(chan struct{})
			var queuedOnce sync.Once
			deny := func(context.Context, int64, int, string) (bool, error) {
				queuedOnce.Do(func() { close(queued) })
				return false, nil
			}
			if slot == "user" {
				f.slots.acquireUserSlotFn = deny
			} else {
				f.slots.acquireAccountSlotFn = deny
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				f.request(ctx, `{"model":"gemini-3.8-flash-tts","input":"hello"}`)
				close(done)
			}()
			select {
			case <-queued:
			case <-time.After(2 * time.Second):
				t.Fatal("request did not try to acquire the occupied slot")
			}
			select {
			case <-done:
				t.Fatal("request should be waiting for the occupied slot")
			default:
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("canceling the client did not interrupt slot admission")
			}
			require.Empty(t, f.upstream.requests)
			require.Empty(t, f.billing.commands)
			f.assertReleased(t, 0)
		})
	}
}

func TestAudioSpeechSlotReleasePoliciesPreserveDefaultCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var normalCalls, heldCalls atomic.Int32
	normal := wrapOpenAISlotRelease(ctx, func() { normalCalls.Add(1) }, openAISlotReleaseOnClientDone)
	held := wrapOpenAISlotRelease(ctx, func() { heldCalls.Add(1) }, openAISlotReleaseOnReturn)
	cancel()
	require.Eventually(t, func() bool { return normalCalls.Load() == 1 }, time.Second, time.Millisecond)
	require.Zero(t, heldCalls.Load())
	normal()
	normal()
	held()
	held()
	require.Equal(t, int32(1), normalCalls.Load())
	require.Equal(t, int32(1), heldCalls.Load())
}

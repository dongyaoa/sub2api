package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSupportsAudioSpeechAccount(t *testing.T) {
	newAccount := func(platform, kind, base string, capabilities any) *Account {
		credentials := map[string]any{"api_key": "test-key", "base_url": base}
		if capabilities != nil {
			credentials[openAIEndpointCapabilitiesCredentialKey] = capabilities
		}
		return &Account{Platform: platform, Type: kind, Credentials: credentials}
	}
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"nil", nil, false},
		{"openai default base", newAccount(PlatformOpenAI, AccountTypeAPIKey, "", nil), true},
		{"openai custom base", newAccount(PlatformOpenAI, AccountTypeAPIKey, "https://speech.example/v1", nil), true},
		{"gemini custom base", newAccount(PlatformGemini, AccountTypeAPIKey, "https://speech.example/v1beta", nil), true},
		{"gemini missing base", newAccount(PlatformGemini, AccountTypeAPIKey, "", nil), false},
		{"gemini native google", newAccount(PlatformGemini, AccountTypeAPIKey, "https://generativelanguage.googleapis.com/v1beta", nil), false},
		{"gemini vertex", newAccount(PlatformGemini, AccountTypeAPIKey, "https://us-central1-aiplatform.googleapis.com/v1", nil), false},
		{"gemini studio page", newAccount(PlatformGemini, AccountTypeAPIKey, "https://aistudio.google.com", nil), false},
		{"gemini malformed base", newAccount(PlatformGemini, AccountTypeAPIKey, "speech.example", nil), false},
		{"gemini url credentials", newAccount(PlatformGemini, AccountTypeAPIKey, "https://user@proxy.example", nil), false},
		{"gemini oauth", newAccount(PlatformGemini, AccountTypeOAuth, "https://speech.example", nil), false},
		{"openai oauth", newAccount(PlatformOpenAI, AccountTypeOAuth, "https://speech.example", nil), false},
		{"openai setup token", newAccount(PlatformOpenAI, AccountTypeSetupToken, "https://speech.example", nil), false},
		{"unrelated platform", newAccount(PlatformGrok, AccountTypeAPIKey, "https://speech.example", nil), false},
		{"existing chat capability", newAccount(PlatformOpenAI, AccountTypeAPIKey, "", []string{"chat_completions"}), true},
		{"speech only", newAccount(PlatformOpenAI, AccountTypeAPIKey, "", []string{"audio_speech"}), true},
		{"embeddings only", newAccount(PlatformOpenAI, AccountTypeAPIKey, "", []string{"embeddings"}), false},
		{"explicit opt out", newAccount(PlatformOpenAI, AccountTypeAPIKey, "", map[string]any{"chat_completions": true, "audio_speech": false}), false},
		{"typed explicit opt out", newAccount(PlatformGemini, AccountTypeAPIKey, "https://speech.example", map[string]bool{"chat_completions": true, "audio_speech": false}), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, SupportsAudioSpeechAccount(test.account))
			require.Equal(t, test.want, test.account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityAudioSpeech))
		})
	}
	missingKey := newAccount(PlatformOpenAI, AccountTypeAPIKey, "", nil)
	delete(missingKey.Credentials, "api_key")
	require.False(t, SupportsAudioSpeechAccount(missingKey))
}

func TestAudioSpeechSchedulingPreservesPlatformAndModelRestrictions(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	const publicModel = "my-speech-model"
	const upstreamModel = "gemini-3.8-flash-tts"
	groupID := int64(173)
	newAccount := func(id int64, platform, kind, model string) Account {
		return Account{ID: id, Platform: platform, Type: kind,
			Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
			Credentials: map[string]any{
				"base_url": "https://speech.example/v1beta", "api_key": "test-key",
				"model_mapping":       map[string]any{model: upstreamModel},
				"openai_capabilities": []string{"chat_completions"},
			},
		}
	}
	for _, platform := range []string{PlatformGemini, PlatformOpenAI} {
		for _, loadBatch := range []bool{false, true} {
			for _, advanced := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/load_batch_%t/advanced_%t", platform, loadBatch, advanced), func(t *testing.T) {
					valid := newAccount(10, platform, AccountTypeAPIKey, publicModel)
					oauth := newAccount(1, platform, AccountTypeOAuth, publicModel)
					wrongModel := newAccount(2, platform, AccountTypeAPIKey, "another-model")
					otherPlatform := PlatformOpenAI
					if platform == PlatformOpenAI {
						otherPlatform = PlatformGemini
					}
					wrongPlatform := newAccount(3, otherPlatform, AccountTypeAPIKey, publicModel)
					disabled := newAccount(4, platform, AccountTypeAPIKey, publicModel)
					disabled.Credentials[openAIEndpointCapabilitiesCredentialKey] = []string{"embeddings"}
					cfg := &config.Config{}
					cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
					svc := &OpenAIGatewayService{cfg: cfg, cache: &schedulerTestGatewayCache{},
						accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{oauth, wrongModel, wrongPlatform, disabled, valid}},
						concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
					}
					ctx := WithOpenAIProfitControlSuppressed(context.Background())
					selectAccount := func(model, session string, capability OpenAIEndpointCapability) (*AccountSelectionResult, error) {
						if advanced {
							selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(ctx, OpenAIAccountScheduleRequest{
								GroupID: &groupID, Platform: platform, SessionHash: session, RequestedModel: model,
								RequiredTransport: OpenAIUpstreamTransportHTTPSSE, RequiredCapability: capability,
							})
							return selection, err
						}
						selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", session,
							model, nil, OpenAIUpstreamTransportHTTPSSE, capability, false, false, false, platform)
						return selection, err
					}
					selection, err := selectAccount(publicModel, "", OpenAIEndpointCapabilityAudioSpeech)
					require.NoError(t, err)
					require.Equal(t, valid.ID, selection.Account.ID)
					require.Equal(t, upstreamModel, selection.Account.GetMappedModel(publicModel))
					selection.ReleaseFunc()

					// A stale chat sticky binding must not admit an OAuth account.
					require.NoError(t, svc.BindStickySession(ctx, &groupID, "speech-session", oauth.ID))
					selection, err = selectAccount(publicModel, "speech-session", OpenAIEndpointCapabilityAudioSpeech)
					require.NoError(t, err)
					require.Equal(t, valid.ID, selection.Account.ID)
					selection.ReleaseFunc()

					selection, err = selectAccount("not-configured", "", OpenAIEndpointCapabilityAudioSpeech)
					require.ErrorIs(t, err, ErrNoAvailableAccounts)
					require.Nil(t, selection)
					if platform == PlatformGemini {
						selection, err = selectAccount(publicModel, "", OpenAIEndpointCapabilityChatCompletions)
						require.ErrorIs(t, err, ErrNoAvailableAccounts)
						require.Nil(t, selection)
					}
				})
			}
		}
	}
	// The public platform normalization must retain its previous semantics.
	require.Equal(t, PlatformOpenAI, NormalizeOpenAICompatiblePlatform(PlatformGemini))
}

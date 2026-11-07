package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiOmniSchedulingKeepsGeminiAccountsAndCapability(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	groupID := int64(172)
	valid := Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
		Credentials: map[string]any{"base_url": "https://omni.example/v1beta", "api_key": "test-key",
			"model_mapping": map[string]any{"gemini-omni-1.2-flash": "gemini-omni-1.2-flash"}},
	}
	openai := valid
	openai.ID, openai.Platform = 2, PlatformOpenAI
	oauth := valid
	oauth.ID, oauth.Type = 3, AccountTypeOAuth
	for _, loadBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("load_batch_%t", loadBatch), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
			svc := &OpenAIGatewayService{cfg: cfg, cache: &schedulerTestGatewayCache{},
				accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{openai, oauth, valid}},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			ctx := context.Background()
			selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "",
				"gemini-omni-1.2-flash", nil, OpenAIUpstreamTransportHTTPSSE,
				OpenAIEndpointCapabilityGeminiOmniVideo, false, false, false, PlatformGemini)
			require.NoError(t, err)
			require.Equal(t, valid.ID, selection.Account.ID)
			selection.ReleaseFunc()
			advanced := newDefaultOpenAIAccountScheduler(svc, nil)
			selection, _, err = advanced.Select(ctx, OpenAIAccountScheduleRequest{
				GroupID: &groupID, Platform: PlatformGemini, RequestedModel: "gemini-omni-1.2-flash",
				RequiredTransport: OpenAIUpstreamTransportHTTPSSE, RequiredCapability: OpenAIEndpointCapabilityGeminiOmniVideo,
			})
			require.NoError(t, err)
			require.Equal(t, valid.ID, selection.Account.ID)
			selection.ReleaseFunc()

			// Gemini text endpoints must not enter this narrowly scoped exception.
			selection, _, err = svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "",
				"gemini-omni-1.2-flash", nil, OpenAIUpstreamTransportHTTPSSE,
				OpenAIEndpointCapabilityChatCompletions, false, false, false, PlatformGemini)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Nil(t, selection)

			require.NoError(t, svc.BindGrokMediaVideoRequestAccount(ctx, &groupID, "omni-task", 10, 20, valid.ID))
			selection, _, err = svc.SelectMediaVideoRequestAccount(ctx, &groupID,
				GrokMediaVideoRequestSessionHash("omni-task", 10, 20), valid.ID, "", PlatformGemini)
			require.NoError(t, err)
			require.Equal(t, valid.ID, selection.Account.ID)
			selection.ReleaseFunc()
		})
	}
}

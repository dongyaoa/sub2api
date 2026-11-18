//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntelligenceClaudePlanBindsMessagesToAnthropicTargets(t *testing.T) {
	ctx := context.Background()
	target := validUpstreamTestTarget()
	upstreams := &upstreamTestRepo{target: target}
	repo := &intelligenceTestRepository{}
	svc := NewIntelligenceMonitorService(repo, upstreamTestEncryptor{}, upstreams, nil, nil, nil, nil)
	name, source := "Claude artwork", "upstream"
	for _, model := range []string{"", "claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-2", "claude-sonnet-5-5-20261112"} {
		input := IntelligenceMonitorInput{Name: &name, SourceType: &source, UpstreamTargetID: []byte(`7`)}
		if model != "" {
			input.Model = &model
		}
		plan, err := svc.SavePlan(ctx, 0, 1, input)
		require.NoError(t, err)
		want := model
		if want == "" {
			want = IntelligenceMonitorClaudeDefault
		}
		require.Equal(t, want, plan.Model)
		require.Equal(t, IntelligenceMonitorAPIModeMessages, plan.APIMode)
		plan.ID = 11
		repo.plan = plan
		run, err := svc.Enqueue(ctx, plan.ID)
		require.NoError(t, err)
		require.Equal(t, want, run.Model)
		require.Equal(t, "high", run.ReasoningEffort)
		require.Equal(t, IntelligenceMonitorAPIModeMessages, run.APIMode)
		require.Equal(t, target.APIKeyEncrypted, run.RequestKeyEncrypted)
	}
	mode, model := MonitorAPIModeResponses, "claude-sonnet-5-5"
	input := IntelligenceMonitorInput{Name: &name, SourceType: &source, UpstreamTargetID: []byte(`7`), Model: &model, APIMode: &mode}
	_, err := svc.SavePlan(ctx, 0, 1, input)
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
	mode = IntelligenceMonitorAPIModeMessages
	target.Provider = MonitorProviderOpenAI
	_, err = svc.SavePlan(ctx, 0, 1, input)
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
}

func TestIntelligenceClaudeExternalAndRestrictedSources(t *testing.T) {
	ctx := context.Background()
	repo := &intelligenceTestRepository{}
	svc := NewIntelligenceMonitorService(repo, upstreamTestEncryptor{}, nil, nil, nil, nil, nil)
	name, endpoint, key := "External Claude", "https://8.8.8.8", "test-key"
	mode := IntelligenceMonitorAPIModeMessages
	plan, err := svc.SavePlan(ctx, 0, 1, IntelligenceMonitorInput{Name: &name, Endpoint: &endpoint, APIKey: &key, APIMode: &mode})
	require.NoError(t, err)
	require.Equal(t, IntelligenceMonitorClaudeDefault, plan.Model)
	require.Equal(t, "messages", plan.APIMode)
	model := "claude-fable-5-2"
	localSvc, localRepo, keys := newLocalIntelligenceKeyTestService()
	in := localIntelligenceInput()
	in.Model, in.APIMode = &model, &mode
	_, err = localSvc.SavePlan(ctx, 0, 7, in)
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
	require.Nil(t, localRepo.saved)
	require.Empty(t, keys.created)
	oauth, _, _ := intelligenceOAuthFixture()
	src := "openai_oauth"
	_, err = oauth.SavePlan(ctx, 0, 1, IntelligenceMonitorInput{SourceType: &src, AccountID: []byte(`55`), Model: &model, APIMode: &mode})
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
	for _, bad := range []string{"claude-", "claude-opus/5", "claude-a..b", "claude-" + strings.Repeat("z", 94), "GPT-6.2 Astra"} {
		require.ErrorIs(t, validateIntelligenceMonitorModel(bad), ErrIntelligenceInvalid, bad)
	}
}

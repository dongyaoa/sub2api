package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestIntelligenceLocalChannelsQueryUsesOnlyCurrentGroupAndKeepsDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &intelligenceMonitorRepository{db: db}
	mock.ExpectQuery(`SELECT a.id,a.name,a.platform,a.type,a.status\s+FROM accounts a\s+JOIN account_groups ag ON ag.account_id=a.id\s+JOIN groups g ON g.id=ag.group_id\s+WHERE ag.group_id=\$1 AND a.deleted_at IS NULL AND g.deleted_at IS NULL`).WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "platform", "type", "status"}).AddRow(11, "Active", "openai", "apikey", "active").AddRow(12, "Paused", "openai", "apikey", "disabled"))
	channels, err := repo.ListIntelligenceLocalChannels(context.Background(), 8)
	require.NoError(t, err)
	require.Equal(t, []service.IntelligenceLocalChannel{{AccountID: 11, Name: "Active", Platform: "openai", Type: "apikey", Status: "active"}, {AccountID: 12, Name: "Paused", Platform: "openai", Type: "apikey", Status: "disabled"}}, channels)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIntelligencePromptsPostgresRoundTripAndCompletedPrompt(t *testing.T) {
	db, ctx := intelligenceMonitorTestDB(t)
	repo := &intelligenceMonitorRepository{db: db}
	groupID := int64(8)
	plan := &service.IntelligenceMonitorPlan{Name: "Prompt plan", SourceType: "local_group", GroupID: &groupID, APIMode: service.MonitorAPIModeResponses, IntervalSeconds: 300, TimeoutSeconds: 600, CreatedBy: 7, CustomPrompt: "default artwork prompt", ChannelPrompts: []service.IntelligenceChannelPrompt{{AccountID: 11, Prompt: "channel artwork prompt"}}}
	require.NoError(t, repo.SavePlan(ctx, plan))
	read, err := repo.GetPlan(ctx, plan.ID)
	require.NoError(t, err)
	require.Equal(t, plan.CustomPrompt, read.CustomPrompt)
	require.Equal(t, plan.ChannelPrompts, read.ChannelPrompts)
	read.CustomPrompt, read.ChannelPrompts = "edited artwork prompt", nil
	require.NoError(t, repo.SavePlan(ctx, read))
	read, err = repo.GetPlan(ctx, plan.ID)
	require.NoError(t, err)
	require.Equal(t, "edited artwork prompt", read.CustomPrompt)
	require.Empty(t, read.ChannelPrompts)
	run := &service.IntelligenceMonitorRun{PlanID: plan.ID, PlanName: plan.Name, TestKind: service.IntelligenceMonitorTestPelican, Trigger: "manual", Model: service.IntelligenceMonitorModel, ReasoningEffort: "high", Prompt: read.CustomPrompt, SourceType: "local_group", SourceSnapshot: map[string]any{"channel_prompts": plan.ChannelPrompts}, NotesSnapshot: map[string]string{}, APIMode: service.MonitorAPIModeResponses, TimeoutSeconds: 600, PlanUpdatedAt: read.UpdatedAt}
	require.NoError(t, repo.Enqueue(ctx, run, false))
	summaries, err := repo.ListRuns(ctx, service.IntelligenceMonitorRunQuery{PlanID: &plan.ID, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, summaries.Items, 1)
	require.NotContains(t, summaries.Items[0].SourceSnapshot, "channel_prompts")
	claimed, err := repo.ClaimNext(ctx, "prompt-test")
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Contains(t, claimed.SourceSnapshot, "channel_prompts", "worker must retain the complete queued mapping")
	claimed.Prompt, claimed.Status = "actual failover channel prompt", "succeeded"
	require.NoError(t, repo.CompleteRun(ctx, claimed))
	completed, err := repo.GetRun(ctx, claimed.ID)
	require.NoError(t, err)
	require.Equal(t, claimed.Prompt, completed.Prompt)
	require.NotContains(t, completed.SourceSnapshot, "channel_prompts")
}

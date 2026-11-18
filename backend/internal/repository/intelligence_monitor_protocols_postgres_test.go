package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestIntelligenceModelsPostgresNewProtocolAndPublicArtwork(t *testing.T) {
	db, ctx, repo := publicPelicanTestDB(t)
	// Real upgrades may already have public plans and saved artwork.
	legacy := publicPelicanInsertRun(t, ctx, db, 101, "pelican", "local_group", 1, "succeeded")
	migration, err := migrations.FS.ReadFile("277_intelligence_monitor_protocols.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-2", "claude-sonnet-5-5-20261112"} {
		p := &service.IntelligenceMonitorPlan{Name: model, Model: model, SourceType: "external", APIMode: "messages", Endpoint: "https://example.test", CreatedBy: 1, IntervalSeconds: 300, TimeoutSeconds: 600}
		require.NoError(t, repo.SavePlan(ctx, p))
		actual, err := repo.GetPlan(ctx, p.ID)
		require.NoError(t, err)
		require.Equal(t, model, actual.Model)
		require.Equal(t, "messages", actual.APIMode)
		_, err = db.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET api_mode='responses' WHERE id=$1`, p.ID)
		require.Error(t, err)
	}
	groupID := int64(1)
	cfg := service.DefaultIntelligencePublicDisplay()
	cfg.Enabled, cfg.PlanIDs = true, []int64{101}
	runs := map[string]int64{service.IntelligenceMonitorModel: legacy}
	for _, model := range []string{service.IntelligenceMonitorAstra62Model, service.IntelligenceMonitorSol62Model} {
		p := &service.IntelligenceMonitorPlan{Name: model, Model: model, SourceType: "local_group", GroupID: &groupID, APIMode: "responses", CreatedBy: 1, IntervalSeconds: 300, TimeoutSeconds: 600}
		require.NoError(t, repo.SavePlan(ctx, p))
		runID := publicPelicanInsertRun(t, ctx, db, p.ID, "pelican", "local_group", groupID, "succeeded")
		_, err = db.ExecContext(ctx, `UPDATE intelligence_monitor_runs SET model=$1 WHERE id=$2`, model, runID)
		require.NoError(t, err)
		cfg.PlanIDs = append(cfg.PlanIDs, p.ID)
		runs[model] = runID
	}
	require.NoError(t, repo.SavePublicDisplay(ctx, cfg))
	page, err := repo.ListPublicPelican(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, page.Items, len(runs))
	for _, plan := range page.Items {
		require.Equal(t, runs[plan.Model], plan.LatestRun.ID)
		detail, err := repo.GetPublicPelicanRun(ctx, plan.LatestRun.ID, []int64{groupID})
		require.NoError(t, err)
		require.Equal(t, plan.Model, detail.Model)
		require.Contains(t, detail.HTML, "<svg>")
		_, err = repo.GetPublicPelicanRun(ctx, plan.LatestRun.ID, []int64{2})
		require.ErrorIs(t, err, service.ErrIntelligenceNotFound)
	}
}

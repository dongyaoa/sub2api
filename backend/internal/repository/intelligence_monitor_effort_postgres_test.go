package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestIntelligencePlanEffortPostgresPersistsAndPreservesPublicHistory(t *testing.T) {
	db, ctx, repo := publicPelicanTestDB(t)
	const planID = int64(101)
	legacy, err := repo.GetPlan(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, "high", legacy.ReasoningEffort)
	historyID := publicPelicanInsertRun(t, ctx, db, planID, "pelican", "local_group", 1, "succeeded")
	legacy.ReasoningEffort = "max"
	require.NoError(t, repo.SavePlan(ctx, legacy))
	groupID := int64(1)
	newPlan := &service.IntelligenceMonitorPlan{Name: "Another model", Model: "gpt-6.2-sol", SourceType: "local_group", GroupID: &groupID, APIMode: "responses", ReasoningEffort: "xhigh", CreatedBy: 1, IntervalSeconds: 300, TimeoutSeconds: 600}
	require.NoError(t, repo.SavePlan(ctx, newPlan))
	cfg := service.DefaultIntelligencePublicDisplay()
	cfg.Enabled, cfg.PlanIDs = true, []int64{legacy.ID, newPlan.ID}
	require.NoError(t, repo.SavePublicDisplay(ctx, cfg))
	page, err := repo.ListPublicPelican(ctx, []int64{groupID})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	efforts := map[int64]string{}
	for _, p := range page.Items {
		efforts[p.ID] = p.ReasoningEffort
		if p.ID == legacy.ID {
			require.NotNil(t, p.LatestRun)
			require.Equal(t, "high", p.LatestRun.ReasoningEffort)
		}
	}
	require.Equal(t, map[int64]string{legacy.ID: "max", newPlan.ID: "xhigh"}, efforts)
	detail, err := repo.GetPublicPelicanRun(ctx, historyID, []int64{groupID})
	require.NoError(t, err)
	require.Equal(t, "high", detail.ReasoningEffort)
	_, err = db.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET reasoning_effort='invalid' WHERE id=$1`, planID)
	require.Error(t, err)
}

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestIntelligencePelicanRetentionUpgradePostgres(t *testing.T) {
	db, ctx, repo := publicPelicanTestDB(t)
	cfg := service.DefaultIntelligencePublicDisplay()
	cfg.Enabled, cfg.HideFailed, cfg.PlanIDs = true, true, []int64{101}
	require.NoError(t, repo.SavePublicDisplay(ctx, cfg))
	_, err := db.ExecContext(ctx, `UPDATE intelligence_monitor_plans SET enabled=FALSE WHERE id=102`)
	require.NoError(t, err)

	// Exercise active, paused and archived plans. Identical timestamps make the
	// descending ID order observable across both successful and failed runs.
	want := map[int64][]int64{}
	var discarded int64
	for _, planID := range []int64{101, 102, 107} {
		for i := 0; i < 20; i++ {
			status := "succeeded"
			if i%2 == 0 {
				status = "failed"
			}
			id := publicPelicanInsertRun(t, ctx, db, planID, "pelican", "local_group", 1, status)
			if i >= 10 {
				want[planID] = append(want[planID], id)
			} else if planID == 101 && i == 9 {
				discarded = id
			}
		}
	}
	activeID := publicPelicanInsertRun(t, ctx, db, 101, "pelican", "local_group", 1, "running")
	for i := 0; i < 12; i++ {
		publicPelicanInsertRun(t, ctx, db, 101, "candy", "local_group", 1, "succeeded")
	}
	_, err = db.ExecContext(ctx, `UPDATE intelligence_monitor_runs SET created_at=NOW()-INTERVAL '1 day' WHERE status IN ('succeeded','failed');
UPDATE intelligence_monitor_runs SET created_at=NOW()-INTERVAL '2 days' WHERE status='running'`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("280_intelligence_pelican_retention.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)

	for planID, newest := range want {
		var ids []int64
		err = db.QueryRowContext(ctx, `SELECT array_agg(id ORDER BY id) FROM intelligence_monitor_runs
WHERE plan_id=$1 AND test_kind='pelican' AND status IN ('succeeded','failed')`, planID).Scan(pq.Array(&ids))
		require.NoError(t, err)
		require.Equal(t, newest, ids)
	}
	planID := int64(101)
	candy, err := repo.ListRuns(ctx, service.IntelligenceMonitorRunQuery{PlanID: &planID, TestKind: "candy", Page: 1, PageSize: 60})
	require.NoError(t, err)
	require.Equal(t, int64(12), candy.Total)
	_, err = repo.GetRun(ctx, discarded)
	require.ErrorIs(t, err, service.ErrIntelligenceNotFound)
	_, err = repo.GetPublicPelicanRun(ctx, discarded, []int64{1})
	require.ErrorIs(t, err, service.ErrIntelligenceNotFound)
	page, err := repo.ListPublicPelican(ctx, []int64{1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, activeID, page.Items[0].LatestRun.ID)
	require.Len(t, page.Items[0].RecentRuns, 5, "hiding failures never brings deleted older successes back")
}

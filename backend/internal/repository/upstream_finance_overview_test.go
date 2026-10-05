package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamOverviewFinanceQueryCountIndependentOfCardCount(t *testing.T) {
	for _, count := range []int{1, 100, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			now := time.Now()
			q := service.UpstreamFinanceQuery{From: now.Add(-24 * time.Hour), To: now}
			supplierIDs := make([]int64, 0, count)
			scopes := make([]service.UpstreamFinanceOverviewTarget, 0, count)
			summaries := sqlmock.NewRows([]string{"kind", "id", "revenue", "business", "requests", "monitor", "unpriced", "reported", "estimated", "tokens", "unknown_tokens", "partial"}).AddRow("total", 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
			metadata := sqlmock.NewRows([]string{"id", "supplier_id", "provider", "endpoint", "key", "key_fingerprint", "wallet", "user_id", "pat", "profit_identity_since"})
			balances := sqlmock.NewRows([]string{"id", "wallet", "kind", "balance", "quota", "today", "day_used", "day_start", "day_end", "day_synced", "last_30_days_used", "period_start", "period_end", "period_synced", "total", "unlimited", "currency", "currency_source", "status", "synced", "error", "attempted"})
			billings := sqlmock.NewRows([]string{"id", "data", "status", "attempted", "error"})
			for n := 1; n <= count; n++ {
				id := int64(n)
				supplierIDs = append(supplierIDs, id)
				scopes = append(scopes, service.UpstreamFinanceOverviewTarget{ID: id, SupplierID: &id})
				summaries.AddRow("supplier", id, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
				summaries.AddRow("target", id, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
				metadata.AddRow(id, id, "openai", "https://example.test", "encrypted", "fingerprint", "default", 0, "", now)
				balances.AddRow(id, "default", "wallet", 10, nil, 1, 1, q.From, q.To, now.Add(-time.Minute), 30, now.AddDate(0, 0, -30), q.To, now.Add(-2*time.Minute), nil, false, "USD", "reported", "ok", now, "", now)
				billings.AddRow(id, []byte(`{"effective_rate_multiplier":0.5}`), "ok", now, "")
			}
			mock.ExpectQuery(`WITH requested_targets AS`).WithArgs(q.From, q.To, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(summaries)
			mock.ExpectQuery(`SELECT t.id,t.supplier_id,t.provider`).WithArgs(sqlmock.AnyArg()).WillReturnRows(metadata)
			mock.ExpectQuery(`WITH identities AS.*upstream_balance_snapshots`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(balances)
			mock.ExpectQuery(`WITH identities AS.*upstream_billing_snapshots`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(billings)
			data, err := (&upstreamFinanceRepository{db: db}).LoadOverviewFinance(context.Background(), q, supplierIDs, scopes)
			require.NoError(t, err)
			require.Len(t, data.Suppliers, count)
			require.Len(t, data.Targets, count)
			require.Len(t, data.Balances, count)
			require.Equal(t, 1.0, *data.Balances[int64(count)].DayUsed)
			require.Equal(t, now.Add(-time.Minute), *data.Balances[int64(count)].DaySyncedAt)
			require.Equal(t, 30.0, *data.Balances[int64(count)].Last30DaysUsed)
			require.Equal(t, now.Add(-2*time.Minute), *data.Balances[int64(count)].PeriodSyncedAt)
			require.Equal(t, 0.5, *data.Balances[int64(count)].Billing.EffectiveRateMultiplier)
			require.NoError(t, mock.ExpectationsWereMet(), "exactly four queries regardless of supplier/target count")
		})
	}
}

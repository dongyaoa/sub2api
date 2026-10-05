package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func expectStorageRestoreLocks(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(251,0\)`).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestUpstreamStorageRestoreTargetRequiresActiveSupplier(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	expectStorageRestoreLocks(mock)
	mock.ExpectQuery(`SELECT supplier_id FROM upstream_targets WHERE id=\$1 AND deleted_at IS NOT NULL`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"supplier_id"}).AddRow(3))
	mock.ExpectQuery(`SELECT deleted_at FROM upstream_suppliers WHERE id=\$1 FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"deleted_at"}).AddRow(time.Now()))
	mock.ExpectRollback()
	_, err = (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "target", ID: 7})
	require.ErrorIs(t, err, service.ErrUpstreamStorageParentArchived)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamStorageRestoreSupplierRejectsActiveWorker(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	archivedAt := time.Now()
	expectStorageRestoreLocks(mock)
	mock.ExpectQuery(`SELECT deleted_at FROM upstream_suppliers WHERE id=\$1 FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"deleted_at"}).AddRow(archivedAt))
	mock.ExpectQuery(`SELECT id,COALESCE.*FROM upstream_targets WHERE supplier_id=\$1 AND deleted_at=\$2 ORDER BY id FOR UPDATE`).WithArgs(int64(3), archivedAt).WillReturnRows(sqlmock.NewRows([]string{"id", "busy"}).AddRow(7, true))
	mock.ExpectRollback()
	_, err = (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "supplier", ID: 3})
	require.ErrorIs(t, err, service.ErrUpstreamStorageBusy)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamStorageRestoreDoesNotStealOrReopenBindings(t *testing.T) {
	for _, conflict := range []string{"occupied", "changed key", "none"} {
		t.Run(conflict, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			expectStorageRestoreLocks(mock)
			mock.ExpectQuery(`SELECT supplier_id FROM upstream_targets`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"supplier_id"}).AddRow(3))
			mock.ExpectQuery(`SELECT deleted_at FROM upstream_suppliers.*FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"deleted_at"}).AddRow(nil))
			mock.ExpectQuery(`SELECT id,COALESCE.*FROM upstream_targets WHERE id=\$1 AND deleted_at IS NOT NULL ORDER BY id FOR UPDATE`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "busy"}).AddRow(7, false))
			fingerprint := upstreamRestoreAccountFingerprint("openai", "https://example.com/v1", "key")
			mock.ExpectQuery(`SELECT b.target_id,b.account_id,t.provider,t.api_key_fingerprint.*b.valid_until=t.deleted_at`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"target_id", "account_id", "provider", "fingerprint"}).AddRow(7, 11, "openai", fingerprint))
			key := "key"
			if conflict == "changed key" {
				key = "replacement"
			}
			mock.ExpectQuery(`SELECT type,platform,COALESCE.*FROM accounts.*FOR SHARE`).WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"type", "platform", "key", "endpoint"}).AddRow("apikey", "openai", key, "https://example.com/v1/"))
			if conflict != "changed key" {
				mock.ExpectQuery(`SELECT EXISTS.*account_id=\$1 AND valid_until IS NULL`).WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"bound"}).AddRow(conflict == "occupied"))
			}
			if conflict == "none" {
				mock.ExpectQuery(`SELECT id FROM intelligence_monitor_plans.*FOR UPDATE`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(21))
				mock.ExpectQuery(`SELECT EXISTS.*status IN \('pending','running'\)`).WithArgs(pq.Array([]int64{21})).WillReturnRows(sqlmock.NewRows([]string{"busy"}).AddRow(false))
				mock.ExpectExec(`UPDATE intelligence_monitor_plans SET enabled=FALSE,next_run_at=NULL,candy_next_run_at=NULL`).WithArgs(pq.Array([]int64{21})).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE upstream_targets SET deleted_at=NULL,enabled=FALSE,next_check_at=NULL.*balance_next_sync_at=clock_timestamp\(\)`).WithArgs(pq.Array([]int64{7})).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO upstream_account_bindings.*valid_from\).*clock_timestamp\(\)`).WithArgs(int64(7), int64(11)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			result, err := (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "target", ID: 7})
			if conflict == "none" {
				require.NoError(t, err)
				require.Equal(t, &service.UpstreamStorageRestoreResult{TargetsRestored: 1, BindingsRestored: 1}, result)
			} else {
				require.ErrorIs(t, err, service.ErrUpstreamBindingConflict)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpstreamStorageRestoreDuplicateKeyRollsBackSupplierAndPlans(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	archivedAt := time.Now()
	expectStorageRestoreLocks(mock)
	mock.ExpectQuery(`SELECT deleted_at FROM upstream_suppliers.*FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"deleted_at"}).AddRow(archivedAt))
	mock.ExpectQuery(`SELECT id,COALESCE.*supplier_id=\$1 AND deleted_at=\$2.*FOR UPDATE`).WithArgs(int64(3), archivedAt).WillReturnRows(sqlmock.NewRows([]string{"id", "busy"}).AddRow(7, false))
	mock.ExpectQuery(`SELECT b.target_id,b.account_id.*b.valid_until=t.deleted_at`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"target_id", "account_id", "provider", "fingerprint"}))
	mock.ExpectQuery(`SELECT id FROM intelligence_monitor_plans.*FOR UPDATE`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`UPDATE upstream_suppliers SET deleted_at=NULL`).WithArgs(int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE upstream_targets SET deleted_at=NULL`).WithArgs(pq.Array([]int64{7})).WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_upstream_targets_unique_key"})
	mock.ExpectRollback()
	_, err = (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "supplier", ID: 3})
	require.ErrorIs(t, err, service.ErrUpstreamDuplicateKey)
	require.NoError(t, mock.ExpectationsWereMet())
}

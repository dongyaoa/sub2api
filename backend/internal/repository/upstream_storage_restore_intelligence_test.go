package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestUpstreamStorageRestoreIntelligenceRetainsWorksAndPausesMissingCredentials(t *testing.T) {
	for _, source := range []string{"external", "local_group", "upstream", "openai_oauth"} {
		t.Run(source, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			expectStorageRestoreLocks(mock)
			var targetID, groupID, accountID, keyID, ownerID any
			switch source {
			case "upstream":
				targetID = int64(9)
			case "local_group":
				groupID, keyID, ownerID = int64(11), int64(12), int64(13)
			case "openai_oauth":
				accountID = int64(14)
			}
			mock.ExpectQuery(`SELECT source_type,model,api_key_encrypted.*deleted_at IS NOT NULL FOR UPDATE`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"source", "model", "key", "target_id", "group_id", "account_id", "key_id", "owner_id", "borrowed"}).AddRow(source, "gpt-6-astra", "", targetID, groupID, accountID, keyID, ownerID, false))
			mock.ExpectQuery(`SELECT EXISTS.*status IN \('pending','running'\)`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"busy"}).AddRow(false))
			if source != "external" {
				sourceID := targetID
				if source == "local_group" {
					sourceID = groupID
				}
				if source == "openai_oauth" {
					sourceID = accountID
				}
				mock.ExpectQuery(`SELECT EXISTS.*source_type=\$1.*deleted_at IS NULL AND id<>\$3 AND model=\$4`).WithArgs(source, sourceID, int64(7), "gpt-6-astra").WillReturnRows(sqlmock.NewRows([]string{"duplicate"}).AddRow(false))
			}
			switch source {
			case "upstream":
				mock.ExpectQuery(`SELECT EXISTS.*FROM upstream_targets`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows([]string{"available"}).AddRow(false))
			case "local_group":
				mock.ExpectQuery(`SELECT EXISTS.*FROM groups`).WithArgs(groupID).WillReturnRows(sqlmock.NewRows([]string{"available"}).AddRow(true))
				mock.ExpectQuery(`SELECT EXISTS.*FROM api_keys.*status='active'`).WithArgs(keyID, ownerID, groupID).WillReturnRows(sqlmock.NewRows([]string{"available"}).AddRow(false))
			case "openai_oauth":
				mock.ExpectQuery(`SELECT EXISTS.*FROM accounts`).WithArgs(accountID).WillReturnRows(sqlmock.NewRows([]string{"available"}).AddRow(false))
			}
			mock.ExpectExec(`UPDATE intelligence_monitor_plans SET deleted_at=NULL,enabled=FALSE,next_run_at=NULL,candy_next_run_at=NULL,local_api_key_id=CASE WHEN \$2 THEN NULL`).WithArgs(int64(7), source == "local_group").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			result, err := (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "intelligence", ID: 7})
			require.NoError(t, err)
			require.Equal(t, &service.UpstreamStorageRestoreResult{PlansRestored: 1, RequiresConfiguration: true}, result)
			require.NoError(t, mock.ExpectationsWereMet(), "no API keys or completed runs are mutated by restoration")
		})
	}
}

func TestUpstreamStorageRestoreIntelligenceRejectsDuplicateOrActiveGeneration(t *testing.T) {
	for _, busy := range []bool{true, false} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectStorageRestoreLocks(mock)
		mock.ExpectQuery(`SELECT source_type,model,api_key_encrypted.*FOR UPDATE`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"source", "model", "key", "target_id", "group_id", "account_id", "key_id", "owner_id", "borrowed"}).AddRow("upstream", "gpt-6-astra", "", 9, nil, nil, nil, nil, false))
		mock.ExpectQuery(`SELECT EXISTS.*status IN \('pending','running'\)`).WithArgs(pq.Array([]int64{7})).WillReturnRows(sqlmock.NewRows([]string{"busy"}).AddRow(busy))
		want := service.ErrUpstreamStorageBusy
		if !busy {
			mock.ExpectQuery(`SELECT EXISTS.*source_type=\$1.*upstream_target_id=\$2.*model=\$4`).WithArgs("upstream", int64(9), int64(7), "gpt-6-astra").WillReturnRows(sqlmock.NewRows([]string{"duplicate"}).AddRow(true))
			want = service.ErrIntelligenceUpstreamPlanExists
		}
		mock.ExpectRollback()
		_, err = (&upstreamCenterRepository{db: db}).RestoreStorage(context.Background(), service.UpstreamStorageRestoreInput{Kind: "intelligence", ID: 7})
		require.ErrorIs(t, err, want)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

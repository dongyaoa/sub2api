package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceProxyAccountProjection(t *testing.T) {
	for _, tc := range []struct {
		name                string
		available, matching bool
		ids                 string
		want                []int64
		err                 error
	}{
		{"bound or explicit source group", true, true, "{5,6}", []int64{5, 6}, nil},
		{"manual key has no account route", true, true, "{}", []int64{}, nil},
		{"archived source", false, true, "{5}", nil, service.ErrUpstreamProxyUnavailable},
		{"replaced source key", true, false, "{5}", nil, service.ErrUpstreamProxyAccountMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery(`(?s)SELECT.*COALESCE\(t.source_target_id,t.id\).*t.deleted_at IS NULL`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"available", "matching", "ids"}).AddRow(tc.available, tc.matching, tc.ids))
			ids, err := (&upstreamFinanceRepository{db: db}).ProxyAccountIDs(context.Background(), 7)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, ids)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(`SELECT`).WithArgs(int64(404)).WillReturnError(sql.ErrNoRows)
	_, err = (&upstreamFinanceRepository{db: db}).ProxyAccountIDs(context.Background(), 404)
	require.ErrorIs(t, err, service.ErrUpstreamFinanceTargetNotFound)
}

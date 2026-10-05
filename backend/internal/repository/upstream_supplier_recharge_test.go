package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSupplierRechargePersistence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &upstreamCenterRepository{db: db}
	now, ratio := time.Now().UTC(), 10.0
	supplier := &service.UpstreamSupplier{Name: "Converted", Website: "https://example.com", Notes: "test", RechargeRatio: &ratio}
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO upstream_suppliers\(name,website,notes,recharge_ratio,sort_order\)`).WithArgs(supplier.Name, supplier.Website, supplier.Notes, ratio).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(7, now, now))
	mock.ExpectCommit()
	require.NoError(t, repo.SaveSupplier(context.Background(), supplier))

	columns := []string{"id", "name", "website", "notes", "created_at", "updated_at", "recharge_ratio"}
	mock.ExpectQuery(`SELECT id,name,website,notes,created_at,updated_at,recharge_ratio FROM upstream_suppliers WHERE id=\$1`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows(columns).AddRow(7, supplier.Name, supplier.Website, supplier.Notes, now, now, ratio))
	loaded, err := repo.GetSupplier(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, &ratio, loaded.RechargeRatio)
	mock.ExpectQuery(`SELECT id,name,website,notes,created_at,updated_at,recharge_ratio FROM upstream_suppliers WHERE deleted_at IS NULL`).WillReturnRows(sqlmock.NewRows(columns).AddRow(7, supplier.Name, supplier.Website, supplier.Notes, now, now, ratio).AddRow(8, "Unconverted", "", "", now, now, nil))
	list, err := repo.ListSuppliers(context.Background())
	require.NoError(t, err)
	require.Equal(t, &ratio, list[0].RechargeRatio)
	require.Nil(t, list[1].RechargeRatio)

	supplier.RechargeRatio = nil
	mock.ExpectQuery(`UPDATE upstream_suppliers SET name=\$2,website=\$3,notes=\$4,recharge_ratio=\$5`).WithArgs(int64(7), supplier.Name, supplier.Website, supplier.Notes, nil).WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))
	require.NoError(t, repo.SaveSupplier(context.Background(), supplier))
	require.NoError(t, mock.ExpectationsWereMet())
}

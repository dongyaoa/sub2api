package repository

import (
	"context"
	"database/sql"
)

type upstreamFinanceReadScopeKey struct{}

type upstreamFinanceReadScope struct {
	db *sql.DB
	tx *sql.Tx
}

type upstreamFinanceReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *upstreamFinanceRepository) financeReader(ctx context.Context) upstreamFinanceReader {
	if scope, ok := ctx.Value(upstreamFinanceReadScopeKey{}).(*upstreamFinanceReadScope); ok && scope.db == r.db {
		return scope.tx
	}
	return r.db
}

// This capability scopes only upstream finance reads. Background writes keep
// their normal bounded transactions and do not wait for this MVCC snapshot.
func (r *upstreamFinanceRepository) WithFinanceReadSnapshot(ctx context.Context, read func(context.Context) error) error {
	if r.financeReader(ctx) != r.db {
		return read(ctx)
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	readCtx := context.WithValue(ctx, upstreamFinanceReadScopeKey{}, &upstreamFinanceReadScope{db: r.db, tx: tx})
	if err = read(readCtx); err != nil {
		return err
	}
	return tx.Commit()
}

package service

import "context"

type UpstreamFinanceReadSnapshotRepository interface {
	WithFinanceReadSnapshot(context.Context, func(context.Context) error) error
}

// Pending imports, participation, balances and local revenue belong to one
// snapshot even when another process creates or completes an import mid-read.
func withUpstreamFinanceReadSnapshot[T any](ctx context.Context, repo UpstreamFinanceRepository, read func(context.Context) (T, error)) (value T, err error) {
	if snapshots, ok := repo.(UpstreamFinanceReadSnapshotRepository); ok {
		err = snapshots.WithFinanceReadSnapshot(ctx, func(readCtx context.Context) error {
			value, err = read(readCtx)
			return err
		})
		return value, err
	}
	return read(ctx)
}

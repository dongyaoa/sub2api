package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type storageRestoreRepoStub struct {
	UpstreamCenterRepository
	called bool
	input  UpstreamStorageRestoreInput
	ctx    context.Context
	err    error
}

func (r *storageRestoreRepoStub) RestoreStorage(ctx context.Context, in UpstreamStorageRestoreInput) (*UpstreamStorageRestoreResult, error) {
	r.called, r.input, r.ctx = true, in, ctx
	return &UpstreamStorageRestoreResult{TargetsRestored: 1}, r.err
}

func TestUpstreamStorageRestoreValidatesBeforeRepositoryMutation(t *testing.T) {
	for _, in := range []UpstreamStorageRestoreInput{
		{Kind: "supplier", ID: 0}, {Kind: "target", ID: -1}, {Kind: "intelligence", ID: 0}, {Kind: "all", ID: 1},
	} {
		repo := &storageRestoreRepoStub{}
		svc := &UpstreamCenterService{repo: repo}
		_, err := svc.RestoreStorage(context.Background(), in)
		require.ErrorIs(t, err, ErrUpstreamStorageRestoreInvalid)
		require.False(t, repo.called)
	}
}

func TestUpstreamStorageRestorePropagatesConflictAndBoundsDeadline(t *testing.T) {
	repo := &storageRestoreRepoStub{err: ErrUpstreamStorageParentArchived}
	svc := &UpstreamCenterService{repo: repo}
	in := UpstreamStorageRestoreInput{Kind: "target", ID: 7}
	started := time.Now()
	_, err := svc.RestoreStorage(context.Background(), in)
	require.ErrorIs(t, err, ErrUpstreamStorageParentArchived)
	require.Equal(t, in, repo.input)
	deadline, ok := repo.ctx.Deadline()
	require.True(t, ok)
	require.WithinRange(t, deadline, started.Add(30*time.Second), time.Now().Add(30*time.Second))
	require.ErrorIs(t, repo.ctx.Err(), context.Canceled)
}

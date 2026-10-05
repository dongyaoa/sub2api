package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrUpstreamStorageRestoreInvalid = infraerrors.BadRequest("UPSTREAM_STORAGE_RESTORE_INVALID", "invalid archive restoration request")
	ErrUpstreamStorageParentArchived = infraerrors.Conflict("UPSTREAM_STORAGE_PARENT_ARCHIVED", "restore the archived supplier before restoring its key group")
)

type UpstreamStorageRestoreInput struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

type UpstreamStorageRestoreResult struct {
	SuppliersRestored     int64 `json:"suppliers_restored"`
	TargetsRestored       int64 `json:"targets_restored"`
	BindingsRestored      int64 `json:"bindings_restored"`
	PlansRestored         int64 `json:"plans_restored"`
	RequiresConfiguration bool  `json:"requires_configuration"`
}

type UpstreamStorageRestoreRepository interface {
	RestoreStorage(context.Context, UpstreamStorageRestoreInput) (*UpstreamStorageRestoreResult, error)
}

func (s *UpstreamCenterService) RestoreStorage(ctx context.Context, in UpstreamStorageRestoreInput) (*UpstreamStorageRestoreResult, error) {
	if in.ID <= 0 || (in.Kind != "supplier" && in.Kind != "target" && in.Kind != "intelligence") {
		return nil, ErrUpstreamStorageRestoreInvalid
	}
	repo, ok := s.repo.(UpstreamStorageRestoreRepository)
	if !ok {
		return nil, ErrUpstreamStorageUnavailable
	}
	restoreCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return repo.RestoreStorage(restoreCtx, in)
}

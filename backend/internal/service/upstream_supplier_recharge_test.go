//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type upstreamRechargeSupplierRepo struct {
	UpstreamCenterRepository
	supplier *UpstreamSupplier
	saved    *UpstreamSupplier
}

func (r *upstreamRechargeSupplierRepo) GetSupplier(context.Context, int64) (*UpstreamSupplier, error) {
	copy := *r.supplier
	return &copy, nil
}

func (r *upstreamRechargeSupplierRepo) SaveSupplier(_ context.Context, supplier *UpstreamSupplier) error {
	copy := *supplier
	r.saved = &copy
	return nil
}

func TestUpstreamSupplierRechargeOptionalUpdates(t *testing.T) {
	ten, half := 10.0, 0.5
	for _, tc := range []struct {
		name    string
		initial *float64
		input   json.RawMessage
		want    *float64
	}{
		{name: "disabled by default"},
		{name: "enable one to ten", input: json.RawMessage("10"), want: &ten},
		{name: "omitted update preserves setting", initial: &ten, want: &ten},
		{name: "explicit null disables", initial: &ten, input: json.RawMessage("null")},
		{name: "accept fractional credits", input: json.RawMessage("0.5"), want: &half},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &upstreamRechargeSupplierRepo{supplier: &UpstreamSupplier{ID: 7, Name: "Example", RechargeRatio: tc.initial}}
			svc := NewUpstreamCenterService(repo, nil, nil, nil)
			result, err := svc.SaveSupplier(context.Background(), 7, nil, nil, nil, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, result.RechargeRatio)
			require.Equal(t, tc.want, repo.saved.RechargeRatio)
		})
	}
	name := "New supplier"
	repo := &upstreamRechargeSupplierRepo{}
	svc := NewUpstreamCenterService(repo, nil, nil, nil)
	result, err := svc.SaveSupplier(context.Background(), 0, &name, nil, nil, json.RawMessage("10"))
	require.NoError(t, err)
	require.Equal(t, &ten, result.RechargeRatio)
}

func TestUpstreamSupplierRechargeRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{"0", "-1", "0.0000001", "1000001", "1e309", "\"10\"", "true", "{}", "[]"} {
		t.Run(raw, func(t *testing.T) {
			repo := &upstreamRechargeSupplierRepo{supplier: &UpstreamSupplier{ID: 7, Name: "Example"}}
			svc := NewUpstreamCenterService(repo, nil, nil, nil)
			_, err := svc.SaveSupplier(context.Background(), 7, nil, nil, nil, json.RawMessage(raw))
			require.ErrorIs(t, err, ErrUpstreamInvalid)
			require.Nil(t, repo.saved)
		})
	}
}

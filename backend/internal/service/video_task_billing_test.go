package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoTaskBillingWaitsForDurableDelivery(t *testing.T) {
	store := newMemoryVideoTaskStore()
	svc := NewVideoTaskService(store, nil)
	owner := VideoTaskOwner{UserID: 7, APIKeyID: 9}
	metadata := VideoTaskMetadata{
		Model:              "grok-imagine-video",
		Resolution:         "720p",
		Duration:           8,
		SubscriptionID:     44,
		RequestPayloadHash: "payload-hash",
	}

	require.NoError(t, svc.RecordSubmission(
		context.Background(), owner, 3, 12, "task-billing", metadata,
		[]byte(`{"id":"task-billing","status":"queued"}`),
	))
	record, err := svc.GetRecord(context.Background(), owner, "task-billing")
	require.NoError(t, err)
	require.Equal(t, VideoTaskBillingPending, record.BillingStatus)
	require.Equal(t, int64(44), record.SubscriptionID)
	require.Equal(t, "payload-hash", record.RequestPayloadHash)

	require.NoError(t, svc.RecordLookupError(
		context.Background(), owner, "task-billing",
		errors.New("auth_not_found: no auth available"),
	))
	record, err = svc.GetRecord(context.Background(), owner, "task-billing")
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusProcessing, record.Status)
	require.Equal(t, VideoTaskBillingPending, record.BillingStatus)
	require.Contains(t, record.LastUpstreamError, "auth_not_found")

	require.NoError(t, svc.MarkBilling(context.Background(), owner, "task-billing", VideoTaskBillingFailed, errors.New("temporary billing error")))
	record, err = svc.GetRecord(context.Background(), owner, "task-billing")
	require.NoError(t, err)
	require.Equal(t, VideoTaskBillingFailed, record.BillingStatus)
	require.Nil(t, record.BilledAt)

	require.NoError(t, svc.MarkBilling(context.Background(), owner, "task-billing", VideoTaskBillingCharged, nil))
	record, err = svc.GetRecord(context.Background(), owner, "task-billing")
	require.NoError(t, err)
	require.Equal(t, VideoTaskBillingCharged, record.BillingStatus)
	require.NotNil(t, record.BilledAt)
}

func TestVideoTaskExplicitFailureIsNotCharged(t *testing.T) {
	store := newMemoryVideoTaskStore()
	svc := NewVideoTaskService(store, nil)
	owner := VideoTaskOwner{UserID: 7, APIKeyID: 9}
	require.NoError(t, svc.RecordSubmission(
		context.Background(), owner, 3, 12, "task-failed",
		VideoTaskMetadata{Model: "grok-imagine-video", Duration: 6}, nil,
	))

	require.NoError(t, svc.UpdateStatus(
		context.Background(), owner, "task-failed", 200,
		[]byte(`{"status":"failed","error":{"message":"generation failed"}}`),
	))
	record, err := svc.GetRecord(context.Background(), owner, "task-failed")
	require.NoError(t, err)
	require.Equal(t, VideoTaskStatusFailed, record.Status)
	require.Equal(t, VideoTaskBillingNotCharged, record.BillingStatus)
	require.Contains(t, string(record.Error), "generation failed")
}

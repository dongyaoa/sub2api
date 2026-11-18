//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntelligencePlanReasoningEffortSnapshots(t *testing.T) {
	ctx := context.Background()
	repo := &intelligenceTestRepository{}
	svc := NewIntelligenceMonitorService(repo, upstreamTestEncryptor{}, nil, nil, nil, nil, nil)
	name, endpoint, key := "Effort", "https://8.8.8.8", "test-key"
	plan, err := svc.SavePlan(ctx, 0, 1, IntelligenceMonitorInput{Name: &name, Endpoint: &endpoint, APIKey: &key})
	require.NoError(t, err)
	require.Equal(t, "high", plan.ReasoningEffort)
	plan.ID, plan.Enabled, plan.CandyEnabled = 1, true, true
	repo.plan = plan
	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		p, err := svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{ReasoningEffort: &level})
		require.NoError(t, err)
		require.Equal(t, level, p.ReasoningEffort)
		repo.plan = p
		for _, scheduled := range []bool{false, true} {
			for _, kind := range []string{IntelligenceMonitorTestPelican, IntelligenceMonitorTestCandy} {
				run, err := svc.enqueueTest(ctx, p.ID, scheduled, kind)
				require.NoError(t, err)
				require.Equal(t, level, run.ReasoningEffort)
			}
		}
	}
	paused, invalid := false, "very-high"
	p, err := svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{Enabled: &paused})
	require.NoError(t, err)
	require.Equal(t, "max", p.ReasoningEffort, "a schedule toggle keeps the selected effort")
	_, err = svc.SavePlan(ctx, plan.ID, 1, IntelligenceMonitorInput{Enabled: &paused, ReasoningEffort: &invalid})
	require.ErrorIs(t, err, ErrIntelligenceInvalid)
	require.Equal(t, "max", repo.saved.ReasoningEffort)
}

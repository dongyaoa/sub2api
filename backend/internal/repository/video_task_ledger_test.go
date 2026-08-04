package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildVideoTaskAdminWhereReusesOneSafeSearchParameter(t *testing.T) {
	where, args := buildVideoTaskAdminWhere(service.VideoTaskAdminQuery{Search: "user@example.com"})

	require.Len(t, args, 1)
	require.Equal(t, "%user@example.com%", args[0])
	require.Equal(t, 4, strings.Count(where, "$1"))
	require.NotContains(t, where, "%!d")
}

func TestBuildVideoTaskAdminWhereSupportsChargedWithoutOutput(t *testing.T) {
	where, args := buildVideoTaskAdminWhere(service.VideoTaskAdminQuery{
		DeliveryStatus: "charged_without_output",
		BillingStatus:  service.VideoTaskBillingCharged,
	})

	require.Len(t, args, 1)
	require.Contains(t, where, "v.billing_status = $1")
	require.Contains(t, where, "NOT v.browser_playable")
}

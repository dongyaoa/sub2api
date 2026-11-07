package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokVideoGenerationLedgerMigrationKeepsDeliveryAndBillingEvidence(t *testing.T) {
	raw, err := FS.ReadFile("194_grok_video_generation_ledger.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "create table if not exists grok_video_generation_tasks")
	for _, column := range []string{
		"request_payload_hash",
		"last_upstream_error",
		"browser_playable",
		"delivery_error",
		"billing_status",
		"billing_error",
		"hidden_at",
	} {
		require.Contains(t, sql, column)
	}
	require.Contains(t, sql, "primary key")
}

func TestVideoTaskBillingModelMigrationPreservesLegacyTasks(t *testing.T) {
	raw, err := FS.ReadFile("275_video_task_billing_model.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))
	require.Contains(t, sql, "alter table grok_video_generation_tasks")
	require.Contains(t, sql, "add column if not exists billing_model text")
	require.NotContains(t, sql, "not null")
	require.NotContains(t, sql, "update grok_video_generation_tasks")
}

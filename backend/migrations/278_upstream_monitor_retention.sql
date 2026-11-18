ALTER TABLE upstream_storage_policy
    DROP CONSTRAINT IF EXISTS upstream_storage_policy_history_retention_days_check,
    ALTER COLUMN history_retention_days SET DEFAULT 3;
ALTER TABLE upstream_storage_policy
    ADD CONSTRAINT upstream_storage_policy_history_retention_days_check
    CHECK (history_retention_days BETWEEN 3 AND 365);

-- Migration 255 inserted the default policy in the transaction that recorded
-- its applied_at. Only an explicit policy save changes updated_at afterward.
-- Keep previously saved choices, including explicit 30-day policies.
UPDATE upstream_storage_policy p
SET history_retention_days = 3
FROM schema_migrations m
WHERE m.filename = '255_upstream_storage_policy.sql'
    AND p.updated_at = m.applied_at
    AND p.history_retention_days = 30;

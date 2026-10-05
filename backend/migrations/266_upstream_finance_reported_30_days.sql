-- Keep the reported 30-day charge together with its requested accounting
-- window. Old snapshots remain unknown until a successful upstream refresh.
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS last_30_days_used NUMERIC(24,10);
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS period_start TIMESTAMPTZ;
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS period_end TIMESTAMPTZ;

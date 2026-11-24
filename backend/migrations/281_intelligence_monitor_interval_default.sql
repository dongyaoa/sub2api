-- A new artwork plan runs every 20 minutes unless the caller supplies its own interval.
ALTER TABLE intelligence_monitor_plans ALTER COLUMN interval_seconds SET DEFAULT 1200;

-- Completed and queued runs already carry their own reasoning_effort snapshot.
ALTER TABLE intelligence_monitor_plans
    ADD COLUMN reasoning_effort VARCHAR(20) NOT NULL DEFAULT 'high'
    CHECK (reasoning_effort IN ('low', 'medium', 'high', 'xhigh', 'max'));

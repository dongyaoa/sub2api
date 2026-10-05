-- Optional credits-per-unit-paid conversion. Stored usage snapshots remain raw.
-- NULL means conversion is disabled; e.g. 10 converts 100 credits to 10 paid.
ALTER TABLE upstream_suppliers
    ADD COLUMN IF NOT EXISTS recharge_ratio NUMERIC(24,10)
        CHECK (recharge_ratio IS NULL OR recharge_ratio BETWEEN 0.000001 AND 1000000);

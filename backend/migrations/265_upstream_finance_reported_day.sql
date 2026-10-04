-- A reported daily charge is useful only together with the exact requested
-- day window. Existing snapshots lack this provenance and remain unpriced.
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS day_used NUMERIC(24,10);
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS day_start TIMESTAMPTZ;
ALTER TABLE upstream_balance_snapshots ADD COLUMN IF NOT EXISTS day_end TIMESTAMPTZ;

-- Existing targets inherit the last application update as a conservative
-- lower bound for their current identity. New targets use their creation time.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'upstream_targets'
          AND column_name = 'profit_identity_since'
    ) THEN
        ALTER TABLE upstream_targets ADD COLUMN profit_identity_since TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp();
        UPDATE upstream_targets SET profit_identity_since = updated_at;
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION upstream_profit_identity_changed() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.supplier_id IS DISTINCT FROM NEW.supplier_id
       OR OLD.provider IS DISTINCT FROM NEW.provider
       OR OLD.endpoint IS DISTINCT FROM NEW.endpoint
       OR OLD.api_key_encrypted IS DISTINCT FROM NEW.api_key_encrypted
       OR OLD.api_key_fingerprint IS DISTINCT FROM NEW.api_key_fingerprint
       OR OLD.wallet_ref IS DISTINCT FROM NEW.wallet_ref
       OR OLD.newapi_user_id IS DISTINCT FROM NEW.newapi_user_id
       OR OLD.newapi_access_token_encrypted IS DISTINCT FROM NEW.newapi_access_token_encrypted THEN
        NEW.profit_identity_since := clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS upstream_profit_identity_changed ON upstream_targets;
CREATE TRIGGER upstream_profit_identity_changed BEFORE UPDATE ON upstream_targets
FOR EACH ROW EXECUTE FUNCTION upstream_profit_identity_changed();

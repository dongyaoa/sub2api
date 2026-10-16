-- An independent monitor may import a supplier group's inference credentials.
-- The source link only reuses console authorization; it does not create a
-- business-account binding or change revenue attribution.
ALTER TABLE upstream_targets
    ADD COLUMN IF NOT EXISTS source_target_id BIGINT REFERENCES upstream_targets(id) ON DELETE SET NULL;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='upstream_targets'::regclass AND conname='upstream_monitor_source_group') THEN
        ALTER TABLE upstream_targets ADD CONSTRAINT upstream_monitor_source_group
            CHECK (source_target_id IS NULL OR (supplier_id IS NULL AND source_target_id<>id));
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_upstream_targets_source_group ON upstream_targets(source_target_id)
    WHERE source_target_id IS NOT NULL;

CREATE OR REPLACE FUNCTION upstream_inherit_supplier_newapi() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE site upstream_suppliers%ROWTYPE; source upstream_targets%ROWTYPE;
BEGIN
    IF NEW.supplier_id IS NOT NULL THEN
        SELECT * INTO site FROM upstream_suppliers WHERE id=NEW.supplier_id;
    END IF;
    IF NEW.source_target_id IS NOT NULL THEN
        NEW.newapi_user_id := 0;
        NEW.newapi_access_token_encrypted := '';
        NEW.newapi_credentials_inherited := FALSE;
        SELECT t.* INTO source FROM upstream_targets t JOIN upstream_suppliers s ON s.id=t.supplier_id
        WHERE t.id=NEW.source_target_id AND t.deleted_at IS NULL AND s.deleted_at IS NULL
          AND t.provider=NEW.provider AND rtrim(btrim(t.endpoint),'/')=rtrim(btrim(NEW.endpoint),'/');
        IF source.newapi_user_id>0 AND source.newapi_access_token_encrypted<>'' THEN
            NEW.newapi_user_id := source.newapi_user_id;
            NEW.newapi_access_token_encrypted := source.newapi_access_token_encrypted;
            NEW.newapi_credentials_inherited := TRUE;
        END IF;
    ELSIF COALESCE(site.newapi_credentials_managed,FALSE) THEN
        NEW.newapi_user_id := 0;
        NEW.newapi_access_token_encrypted := '';
        NEW.newapi_credentials_inherited := FALSE;
        IF site.newapi_user_id>0 AND site.newapi_api_base=upstream_newapi_base(NEW.endpoint) THEN
            NEW.newapi_user_id := site.newapi_user_id;
            NEW.newapi_access_token_encrypted := site.newapi_access_token_encrypted;
            NEW.newapi_credentials_inherited := TRUE;
        END IF;
    ELSIF TG_OP='UPDATE' AND OLD.newapi_credentials_inherited AND NOT (
        OLD.source_target_id IS NOT NULL AND NEW.source_target_id IS NULL
        AND NEW.supplier_id IS NULL AND NEW.newapi_user_id>0
        AND NEW.newapi_access_token_encrypted<>''
        AND NEW.newapi_access_token_encrypted IS DISTINCT FROM OLD.newapi_access_token_encrypted
    ) THEN
        -- Detaching drops the inherited secret. A newly encrypted, explicitly
        -- supplied independent token may replace it in the same save.
        NEW.newapi_user_id := 0;
        NEW.newapi_access_token_encrypted := '';
        NEW.newapi_credentials_inherited := FALSE;
    ELSE
        NEW.newapi_credentials_inherited := FALSE;
    END IF;
    IF TG_OP='UPDATE' AND (OLD.newapi_user_id IS DISTINCT FROM NEW.newapi_user_id
       OR OLD.newapi_access_token_encrypted IS DISTINCT FROM NEW.newapi_access_token_encrypted) THEN
        NEW.balance_next_sync_at := clock_timestamp();
        NEW.balance_lease_until := NULL;
        NEW.balance_lease_token := NULL;
        NEW.updated_at := clock_timestamp();
    END IF;
    RETURN NEW;
END;
$$;

-- Site rotation is already propagated to supplier groups. Group updates then
-- refresh their monitor copies, including archived monitors, so restoring one
-- cannot revive revoked authorization. Endpoint/provider moves revoke reuse.
CREATE OR REPLACE FUNCTION upstream_propagate_monitor_newapi() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.newapi_user_id IS DISTINCT FROM NEW.newapi_user_id
       OR OLD.newapi_access_token_encrypted IS DISTINCT FROM NEW.newapi_access_token_encrypted
       OR OLD.supplier_id IS DISTINCT FROM NEW.supplier_id
       OR OLD.provider IS DISTINCT FROM NEW.provider
       OR OLD.endpoint IS DISTINCT FROM NEW.endpoint
       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
        UPDATE upstream_targets SET newapi_access_token_encrypted=newapi_access_token_encrypted
        WHERE source_target_id=NEW.id;
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS upstream_propagate_monitor_newapi ON upstream_targets;
CREATE TRIGGER upstream_propagate_monitor_newapi AFTER UPDATE ON upstream_targets
FOR EACH ROW EXECUTE FUNCTION upstream_propagate_monitor_newapi();

CREATE OR REPLACE FUNCTION upstream_propagate_supplier_newapi() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.newapi_user_id IS DISTINCT FROM NEW.newapi_user_id
       OR OLD.newapi_access_token_encrypted IS DISTINCT FROM NEW.newapi_access_token_encrypted
       OR OLD.newapi_api_base IS DISTINCT FROM NEW.newapi_api_base
       OR OLD.newapi_credentials_managed IS DISTINCT FROM NEW.newapi_credentials_managed THEN
        UPDATE upstream_targets SET newapi_access_token_encrypted=newapi_access_token_encrypted
        WHERE supplier_id=NEW.id;
    END IF;
    IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
        UPDATE upstream_targets SET newapi_access_token_encrypted=newapi_access_token_encrypted
        WHERE source_target_id IN (SELECT id FROM upstream_targets WHERE supplier_id=NEW.id);
    END IF;
    RETURN NEW;
END;
$$;

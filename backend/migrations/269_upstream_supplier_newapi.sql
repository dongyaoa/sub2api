-- Site authorization is stored once. Internal target credential copies are
-- maintained atomically so existing snapshot identities, storage retention and
-- in-flight sync compare-and-swap checks all use the effective authorization.
ALTER TABLE upstream_suppliers
    ADD COLUMN IF NOT EXISTS newapi_user_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS newapi_access_token_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS newapi_api_base VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS newapi_credentials_managed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS newapi_legacy_conflict BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE upstream_targets
    ADD COLUMN IF NOT EXISTS newapi_credentials_inherited BOOLEAN NOT NULL DEFAULT FALSE;

CREATE OR REPLACE FUNCTION upstream_newapi_base(endpoint TEXT) RETURNS TEXT
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE parts TEXT[]; host TEXT; path TEXT;
BEGIN
    endpoint := btrim(endpoint);
    IF endpoint IS NULL OR strpos(endpoint, '%') > 0 OR strpos(endpoint, chr(92)) > 0 THEN RETURN ''; END IF;
    parts := regexp_match(endpoint, '^https://([^/?#[:space:]@]+)(/[^?#[:space:]]*)?$');
    IF parts IS NULL THEN RETURN ''; END IF;
    host := regexp_replace(lower(parts[1]), ':443$', '');
    path := COALESCE(parts[2], '');
    IF strpos(path, '//') > 0 OR path ~ '(^|/)[.]{1,2}(/|$)' THEN RETURN ''; END IF;
    path := regexp_replace(path, '/+$', '');
    path := regexp_replace(path, '/(v1/(chat/completions|responses|messages|models|usage)|v1beta/models|v1beta|v1)$', '');
    RETURN 'https://' || host || path;
END;
$$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='upstream_suppliers'::regclass AND conname='upstream_suppliers_newapi_credentials_pair') THEN
        ALTER TABLE upstream_suppliers ADD CONSTRAINT upstream_suppliers_newapi_credentials_pair CHECK (
            (newapi_user_id=0 AND newapi_access_token_encrypted='') OR
            (newapi_user_id>0 AND newapi_access_token_encrypted<>'' AND newapi_api_base<>''
             AND upstream_newapi_base(newapi_api_base)=newapi_api_base AND newapi_credentials_managed)
        );
    END IF;
END $$;

-- Older groups may have different Access Tokens for the same user. Pick the
-- most recently edited credential only when every credential-bearing group
-- belongs to the SAME deployment and user; never choose between accounts.
WITH compatible AS (
    SELECT supplier_id, MIN(newapi_user_id) AS user_id, MIN(upstream_newapi_base(endpoint)) AS base
    FROM upstream_targets WHERE supplier_id IS NOT NULL AND newapi_user_id>0
    GROUP BY supplier_id
    HAVING COUNT(DISTINCT newapi_user_id)=1 AND COUNT(DISTINCT upstream_newapi_base(endpoint))=1
       AND MIN(upstream_newapi_base(endpoint))<>''
), selected AS (
    SELECT c.*, t.newapi_access_token_encrypted
    FROM compatible c CROSS JOIN LATERAL (
        SELECT newapi_access_token_encrypted FROM upstream_targets
        WHERE supplier_id=c.supplier_id AND newapi_user_id=c.user_id
        ORDER BY (deleted_at IS NULL) DESC, updated_at DESC, id DESC LIMIT 1
    ) t
)
UPDATE upstream_suppliers s SET newapi_user_id=c.user_id,newapi_access_token_encrypted=c.newapi_access_token_encrypted,
    newapi_api_base=c.base,newapi_credentials_managed=TRUE,newapi_legacy_conflict=FALSE
FROM selected c WHERE s.id=c.supplier_id AND NOT s.newapi_credentials_managed;

UPDATE upstream_suppliers s SET newapi_legacy_conflict=TRUE
WHERE NOT s.newapi_credentials_managed AND EXISTS (
    SELECT 1 FROM upstream_targets t WHERE t.supplier_id=s.id AND t.newapi_user_id>0
);

CREATE OR REPLACE FUNCTION upstream_inherit_supplier_newapi() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE site upstream_suppliers%ROWTYPE;
BEGIN
    IF NEW.supplier_id IS NOT NULL THEN
        SELECT * INTO site FROM upstream_suppliers WHERE id=NEW.supplier_id;
    END IF;
    IF COALESCE(site.newapi_credentials_managed,FALSE) THEN
        NEW.newapi_user_id := 0;
        NEW.newapi_access_token_encrypted := '';
        NEW.newapi_credentials_inherited := FALSE;
        IF site.newapi_user_id>0 AND site.newapi_api_base=upstream_newapi_base(NEW.endpoint) THEN
            NEW.newapi_user_id := site.newapi_user_id;
            NEW.newapi_access_token_encrypted := site.newapi_access_token_encrypted;
            NEW.newapi_credentials_inherited := TRUE;
        END IF;
    ELSIF TG_OP='UPDATE' AND OLD.newapi_credentials_inherited THEN
        -- Moving an inherited group to another supplier/independent monitor
        -- must not send the previous site's console token to the new recipient.
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

-- PostgreSQL orders same-kind triggers by name. Inherit before the existing
-- upstream_profit_identity_changed trigger checks the effective credential.
DROP TRIGGER IF EXISTS upstream_00_inherit_newapi ON upstream_targets;
CREATE TRIGGER upstream_00_inherit_newapi BEFORE INSERT OR UPDATE ON upstream_targets
FOR EACH ROW EXECUTE FUNCTION upstream_inherit_supplier_newapi();

CREATE OR REPLACE FUNCTION upstream_propagate_supplier_newapi() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.newapi_user_id IS DISTINCT FROM NEW.newapi_user_id
       OR OLD.newapi_access_token_encrypted IS DISTINCT FROM NEW.newapi_access_token_encrypted
       OR OLD.newapi_api_base IS DISTINCT FROM NEW.newapi_api_base
       OR OLD.newapi_credentials_managed IS DISTINCT FROM NEW.newapi_credentials_managed THEN
        -- Includes archived groups, so restoration cannot resurrect old auth.
        UPDATE upstream_targets SET newapi_access_token_encrypted=newapi_access_token_encrypted
        WHERE supplier_id=NEW.id;
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS upstream_propagate_supplier_newapi ON upstream_suppliers;
CREATE TRIGGER upstream_propagate_supplier_newapi AFTER UPDATE ON upstream_suppliers
FOR EACH ROW EXECUTE FUNCTION upstream_propagate_supplier_newapi();

-- Populate inherited copies for safely migrated sites; re-running is harmless.
UPDATE upstream_targets t SET newapi_access_token_encrypted=t.newapi_access_token_encrypted
FROM upstream_suppliers s WHERE s.id=t.supplier_id AND s.newapi_credentials_managed;

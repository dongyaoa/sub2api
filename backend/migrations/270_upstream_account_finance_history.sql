-- Account imports include retained account usage from before the import. This
-- is account history: old usage rows do not contain a per-request API-key hash.
-- Never rewrite the attribution of a request that is already in the ledger.
ALTER TABLE upstream_account_bindings ADD COLUMN IF NOT EXISTS history_from TIMESTAMPTZ;

CREATE OR REPLACE FUNCTION upstream_import_account_finance_history(binding_id BIGINT)
RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE
    binding upstream_account_bindings%ROWTYPE;
    previous_until TIMESTAMPTZ;
    lower_bound TIMESTAMPTZ;
    upper_bound TIMESTAMPTZ;
    imported BIGINT := 0;
BEGIN
    SELECT * INTO binding FROM upstream_account_bindings WHERE id = binding_id;
    IF NOT FOUND OR binding.valid_until IS NOT NULL OR binding.supplier_id IS NULL
       OR binding.history_from IS NOT NULL THEN
        RETURN 0;
    END IF;

    -- The usage trigger takes the shared form of this lock. A usage write that
    -- races an import is either visible to this backfill or sees the new binding
    -- after it commits; no billing request can fall between the two paths.
    -- One lock also avoids cross-account lock-order inversions in bulk usage
    -- inserts and multi-account imports. Normal usage writers share the lock.
    PERFORM pg_advisory_xact_lock(hashtextextended('upstream-finance-history-import', 0));

    -- Existing imports are upgraded only when the account still has the exact
    -- provider/endpoint/key of the active target. Independent monitors do not
    -- acquire business history merely because they copied an account's key.
    PERFORM 1
    FROM accounts a
    JOIN upstream_targets t ON t.id = binding.target_id
    JOIN upstream_suppliers s ON s.id = t.supplier_id
    WHERE a.id = binding.account_id AND a.deleted_at IS NULL AND a.type = 'apikey'
      AND a.parent_account_id IS NULL
      AND COALESCE(a.extra->'synthetic_ui_test', 'false'::jsonb) != 'true'::jsonb
      AND t.deleted_at IS NULL AND s.deleted_at IS NULL
      AND t.supplier_id = binding.supplier_id AND a.platform = t.provider
      AND BTRIM(COALESCE(a.credentials->>'api_key', ''), E' \t\r\n') != ''
      AND t.endpoint = COALESCE(NULLIF(RTRIM(BTRIM(COALESCE(a.credentials->>'base_url', ''), E' \t\r\n'), '/'), ''),
          CASE a.platform WHEN 'openai' THEN 'https://api.openai.com'
              WHEN 'anthropic' THEN 'https://api.anthropic.com'
              WHEN 'gemini' THEN 'https://generativelanguage.googleapis.com' END)
      AND t.api_key_fingerprint = encode(sha256(convert_to(t.endpoint, 'UTF8') || decode('00', 'hex') ||
          convert_to(BTRIM(a.credentials->>'api_key', E' \t\r\n'), 'UTF8')), 'hex')
    FOR SHARE OF a, t, s;
    IF NOT FOUND THEN RETURN 0; END IF;

    -- Include a one-day timezone/DST margin; the existing summary queries still
    -- choose today's/last-30-calendar-days' exact local boundaries. Work remains
    -- bounded to this account's indexed recent usage, never its entire lifetime.
    upper_bound := clock_timestamp();
    lower_bound := upper_bound - INTERVAL '31 days';
    SELECT MAX(b.valid_until) INTO previous_until
    FROM upstream_account_bindings b
    WHERE b.account_id = binding.account_id AND b.id != binding.id
      AND b.valid_until <= binding.valid_from;
    lower_bound := GREATEST(lower_bound, previous_until);
    IF EXISTS (SELECT 1 FROM upstream_account_bindings b
        WHERE b.account_id = binding.account_id AND b.target_id = binding.target_id
          AND b.id != binding.id AND b.valid_until IS NOT NULL) THEN
        -- Restoring/rebinding an existing target starts a new accounting
        -- interval. In particular, time spent archived must stay excluded.
        lower_bound := GREATEST(lower_bound, binding.valid_from);
    END IF;

    -- A prior closed identity is a hard boundary even if its source logs were
    -- retained. Do not move its revenue to a replacement key or supplier.
    UPDATE upstream_account_bindings SET history_from = LEAST(valid_from, lower_bound)
    WHERE id = binding.id AND history_from IS NULL;

    INSERT INTO upstream_finance_ledger
        (usage_id, created_at, target_id, target_name, supplier_id, supplier_name,
         account_id, group_id, user_id, api_key_id, model, request_id,
         revenue, business_cost, billing_type, total_tokens)
    SELECT u.id, u.created_at, binding.target_id, binding.target_name,
        binding.supplier_id, binding.supplier_name, u.account_id, u.group_id,
        u.user_id, u.api_key_id, COALESCE(NULLIF(u.requested_model, ''), u.model),
        COALESCE(u.request_id, ''), u.actual_cost,
        COALESCE(u.account_stats_cost, u.total_cost) * COALESCE(u.account_rate_multiplier, 1),
        u.billing_type, u.input_tokens::bigint + u.output_tokens::bigint +
            u.cache_creation_tokens::bigint + u.cache_read_tokens::bigint
    FROM usage_logs u
    WHERE u.account_id = binding.account_id AND u.created_at >= lower_bound AND u.created_at < upper_bound
      AND NOT EXISTS (SELECT 1 FROM upstream_account_bindings old
          WHERE old.account_id = u.account_id AND old.id != binding.id
            AND u.created_at >= old.valid_from
            AND (old.valid_until IS NULL OR u.created_at < old.valid_until))
      AND NOT EXISTS (SELECT 1 FROM upstream_finance_ledger l
          WHERE l.usage_id = u.id AND l.created_at = u.created_at)
    ON CONFLICT (usage_id, created_at) DO NOTHING;
    GET DIAGNOSTICS imported = ROW_COUNT;
    RETURN imported;
END;
$$;

CREATE OR REPLACE FUNCTION upstream_import_bound_account_finance()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM upstream_import_account_finance_history(NEW.id);
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS upstream_account_finance_history_import ON upstream_account_bindings;
CREATE TRIGGER upstream_account_finance_history_import
    AFTER INSERT ON upstream_account_bindings
    FOR EACH ROW EXECUTE FUNCTION upstream_import_bound_account_finance();

CREATE OR REPLACE FUNCTION upstream_record_usage_finance()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE changed INTEGER;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        -- Corrections retain the originally captured attribution, including
        -- imports. Retention can delete the source log without deleting history.
        UPDATE upstream_finance_ledger SET
            revenue = NEW.actual_cost,
            business_cost = COALESCE(NEW.account_stats_cost, NEW.total_cost) * COALESCE(NEW.account_rate_multiplier, 1),
            total_tokens = NEW.input_tokens::bigint + NEW.output_tokens::bigint + NEW.cache_creation_tokens::bigint + NEW.cache_read_tokens::bigint,
            model = COALESCE(NULLIF(NEW.requested_model, ''), NEW.model),
            request_id = COALESCE(NEW.request_id, ''),
            billing_type = NEW.billing_type,
            updated_at = clock_timestamp()
        WHERE usage_id = OLD.id AND created_at = OLD.created_at;
        GET DIAGNOSTICS changed = ROW_COUNT;
        IF changed > 0 THEN RETURN NEW; END IF;
    END IF;

    PERFORM pg_advisory_xact_lock_shared(hashtextextended('upstream-finance-history-import', 0));
    INSERT INTO upstream_finance_ledger
        (usage_id, created_at, target_id, target_name, supplier_id, supplier_name,
         account_id, group_id, user_id, api_key_id, model, request_id, revenue, business_cost, billing_type, total_tokens)
    SELECT NEW.id, NEW.created_at, b.target_id, b.target_name, b.supplier_id, b.supplier_name,
        NEW.account_id, NEW.group_id, NEW.user_id, NEW.api_key_id,
        COALESCE(NULLIF(NEW.requested_model, ''), NEW.model), COALESCE(NEW.request_id, ''),
        NEW.actual_cost,
        COALESCE(NEW.account_stats_cost, NEW.total_cost) * COALESCE(NEW.account_rate_multiplier, 1), NEW.billing_type,
        NEW.input_tokens::bigint + NEW.output_tokens::bigint + NEW.cache_creation_tokens::bigint + NEW.cache_read_tokens::bigint
    FROM upstream_account_bindings b
    WHERE b.account_id = NEW.account_id AND NEW.created_at >= COALESCE(b.history_from, b.valid_from)
      AND (b.valid_until IS NULL OR NEW.created_at < b.valid_until)
    -- Explicit historical intervals win over an imported lookback interval.
    ORDER BY (NEW.created_at >= b.valid_from) DESC, b.valid_from DESC, b.id DESC LIMIT 1
    ON CONFLICT (usage_id, created_at) DO UPDATE SET
        revenue = EXCLUDED.revenue, business_cost = EXCLUDED.business_cost,
        total_tokens = EXCLUDED.total_tokens, model = EXCLUDED.model,
        request_id = EXCLUDED.request_id, billing_type = EXCLUDED.billing_type,
        updated_at = clock_timestamp();
    RETURN NEW;
END;
$$;

-- Upgrade existing live imports once. The function marks completed bindings,
-- so a migration replay does not scan their usage again or reset attribution.
DO $$
DECLARE binding_id BIGINT;
BEGIN
    FOR binding_id IN SELECT id FROM upstream_account_bindings
        WHERE valid_until IS NULL AND supplier_id IS NOT NULL AND history_from IS NULL
        ORDER BY account_id, id
    LOOP
        PERFORM upstream_import_account_finance_history(binding_id);
    END LOOP;
END;
$$;

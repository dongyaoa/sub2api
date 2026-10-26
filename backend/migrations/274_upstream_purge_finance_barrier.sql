-- A permanent purge deletes bindings before their accounting copies. Lock the
-- selected binding while a usage transaction records attribution, so a write
-- that overlaps deletion cannot recreate an orphaned finance row afterwards.
CREATE OR REPLACE FUNCTION upstream_record_usage_finance()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE changed INTEGER;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        UPDATE upstream_finance_ledger SET
            revenue = NEW.actual_cost,
            business_cost = COALESCE(NEW.account_stats_cost, NEW.total_cost) * COALESCE(NEW.account_rate_multiplier, 1),
            total_tokens = NEW.input_tokens::bigint + NEW.output_tokens::bigint + NEW.cache_creation_tokens::bigint + NEW.cache_read_tokens::bigint,
            model = COALESCE(NULLIF(NEW.requested_model, ''), NEW.model),
            request_id = COALESCE(NEW.request_id, ''), billing_type = NEW.billing_type, updated_at = clock_timestamp()
        WHERE usage_id = OLD.id AND created_at = OLD.created_at;
        GET DIAGNOSTICS changed = ROW_COUNT;
        IF changed > 0 THEN RETURN NEW; END IF;
    END IF;
    PERFORM pg_advisory_xact_lock_shared(hashtextextended('upstream-finance-history-import:' || NEW.account_id::text, 0));
    INSERT INTO upstream_finance_ledger
        (usage_id, created_at, target_id, target_name, supplier_id, supplier_name,
         account_id, group_id, user_id, api_key_id, model, request_id, revenue, business_cost, billing_type, total_tokens)
    SELECT NEW.id, NEW.created_at, b.target_id, b.target_name, b.supplier_id, b.supplier_name,
        NEW.account_id, NEW.group_id, NEW.user_id, NEW.api_key_id,
        COALESCE(NULLIF(NEW.requested_model, ''), NEW.model), COALESCE(NEW.request_id, ''), NEW.actual_cost,
        COALESCE(NEW.account_stats_cost, NEW.total_cost) * COALESCE(NEW.account_rate_multiplier, 1), NEW.billing_type,
        NEW.input_tokens::bigint + NEW.output_tokens::bigint + NEW.cache_creation_tokens::bigint + NEW.cache_read_tokens::bigint
    FROM upstream_account_bindings b
    WHERE b.account_id = NEW.account_id AND NEW.created_at >= COALESCE(b.history_from, b.valid_from)
      AND (b.valid_until IS NULL OR NEW.created_at < b.valid_until)
    ORDER BY (NEW.created_at >= b.valid_from) DESC, b.valid_from DESC, b.id DESC LIMIT 1 FOR SHARE OF b
    ON CONFLICT (usage_id, created_at) DO UPDATE SET
        revenue = EXCLUDED.revenue, business_cost = EXCLUDED.business_cost,
        total_tokens = EXCLUDED.total_tokens, model = EXCLUDED.model,
        request_id = EXCLUDED.request_id, billing_type = EXCLUDED.billing_type, updated_at = clock_timestamp();
    RETURN NEW;
END;
$$;

-- These partial indexes support archive keyset batches without scanning the
-- much larger live inventory. Retained history remains indexed by its owner.
CREATE INDEX IF NOT EXISTS idx_upstream_suppliers_archived_id ON upstream_suppliers(id) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_upstream_targets_archived_id ON upstream_targets(id) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_intelligence_plans_archived_id ON intelligence_monitor_plans(id) WHERE deleted_at IS NOT NULL;

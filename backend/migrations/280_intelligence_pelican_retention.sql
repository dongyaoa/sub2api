-- Match the worker's plan-then-run lock order so an in-flight completion or
-- manual deletion cannot change the retained set while it is being trimmed.
DO $$
DECLARE
    current_plan BIGINT;
BEGIN
    FOR current_plan IN
        SELECT p.id FROM intelligence_monitor_plans AS p
        WHERE EXISTS (
            SELECT 1 FROM intelligence_monitor_runs AS r
            WHERE r.plan_id = p.id AND r.test_kind = 'pelican'
              AND r.status IN ('succeeded', 'failed')
            OFFSET 10
        )
        ORDER BY p.id FOR UPDATE
    LOOP
        DELETE FROM intelligence_monitor_runs
        WHERE id IN (
            SELECT r.id FROM intelligence_monitor_runs AS r
            WHERE r.plan_id = current_plan AND r.test_kind = 'pelican'
              AND r.status IN ('succeeded', 'failed')
            ORDER BY r.created_at DESC, r.id DESC
            OFFSET 10
        );
    END LOOP;
END;
$$;

-- A plan keeps the wire model ID; existing plans and run snapshots need no rewrite.
ALTER TABLE intelligence_monitor_plans
    DROP CONSTRAINT IF EXISTS intelligence_monitor_plans_model_check,
    DROP CONSTRAINT IF EXISTS intelligence_monitor_plans_api_mode_check;

ALTER TABLE intelligence_monitor_plans
    ADD CONSTRAINT intelligence_monitor_plans_model_check CHECK (
        (model IN ('gpt-6-astra', 'gpt-6.1-sol', 'gpt-6.2-astra', 'gpt-6.2-sol')
            AND api_mode IN ('responses', 'chat_completions'))
        OR (model COLLATE "C" ~ '^claude-[a-z0-9]+([.-][a-z0-9]+)*$'
            AND api_mode = 'messages' AND source_type IN ('upstream', 'external'))
    ),
    ADD CONSTRAINT intelligence_monitor_plans_api_mode_check CHECK (
        api_mode IN ('responses', 'chat_completions', 'messages')
    );

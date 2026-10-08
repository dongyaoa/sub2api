-- Empty values preserve the existing fixed pelican test for every plan.
-- Run.prompt stores the final prompt used by the actual forwarding account.
ALTER TABLE intelligence_monitor_plans
 ADD COLUMN IF NOT EXISTS custom_prompt TEXT NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS channel_prompts JSONB NOT NULL DEFAULT '[]'::jsonb;

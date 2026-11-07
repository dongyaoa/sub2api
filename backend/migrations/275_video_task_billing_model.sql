-- Preserve the billing model selected when a video task was submitted so a
-- channel mapping change or Redis expiry cannot change the model charged.
-- Existing tasks remain NULL and use the compatible runtime resolver.
ALTER TABLE grok_video_generation_tasks
    ADD COLUMN IF NOT EXISTS billing_model TEXT;

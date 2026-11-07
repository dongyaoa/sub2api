-- Preserve the optional generation seed for history and parameter reuse.
-- NULL represents an omitted seed; zero is a valid explicitly requested seed.
ALTER TABLE grok_video_generation_tasks
    ADD COLUMN IF NOT EXISTS seed BIGINT;

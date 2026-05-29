ALTER TABLE chores ADD COLUMN IF NOT EXISTS indicator_defaults jsonb NOT NULL DEFAULT '[]'::jsonb;

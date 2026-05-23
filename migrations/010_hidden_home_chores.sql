ALTER TABLE user_preferences ADD COLUMN IF NOT EXISTS hidden_home_chore_ids JSONB NOT NULL DEFAULT '[]';

ALTER TABLE user_preferences
  ADD COLUMN IF NOT EXISTS stats_section_order JSONB NOT NULL DEFAULT '[]';
ALTER TABLE user_preferences
  ADD COLUMN IF NOT EXISTS stats_section_hidden JSONB NOT NULL DEFAULT '[]';

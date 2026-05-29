ALTER TABLE reminder_preferences
    ADD COLUMN IF NOT EXISTS enabled_push_types JSONB NOT NULL DEFAULT '[]';

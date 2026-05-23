ALTER TABLE chores ADD COLUMN IF NOT EXISTS predefined_key TEXT;
UPDATE chores SET predefined_key = name WHERE is_predefined = TRUE;

-- The name an activity pack gave a predefined activity ("Feed Mongo"), which
-- Restore default returns to. NULL means the template's own name.
ALTER TABLE chores ADD COLUMN IF NOT EXISTS default_name TEXT;

ALTER TABLE chores ALTER COLUMN follow_up_enabled SET DEFAULT TRUE;
UPDATE chores SET follow_up_enabled = TRUE WHERE follow_up_enabled = FALSE;

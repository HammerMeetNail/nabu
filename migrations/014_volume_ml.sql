ALTER TABLE chores
  ADD COLUMN IF NOT EXISTS has_volume_ml BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE chore_logs
  ADD COLUMN IF NOT EXISTS volume_ml INT;

UPDATE chores
SET has_volume_ml = TRUE
WHERE name = 'Feed Baby'
  AND is_predefined = TRUE;

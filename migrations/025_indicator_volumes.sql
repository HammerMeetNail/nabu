ALTER TABLE chore_logs ADD COLUMN IF NOT EXISTS indicator_volumes JSONB;

UPDATE chore_logs
SET indicator_volumes = jsonb_build_object(indicators->>0, volume_ml::int)
WHERE volume_ml IS NOT NULL
  AND volume_ml > 0
  AND indicators IS NOT NULL
  AND indicators <> '[]'
  AND jsonb_array_length(indicators::jsonb) = 1;

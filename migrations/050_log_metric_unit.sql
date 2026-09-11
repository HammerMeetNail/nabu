SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
ALTER TABLE chore_logs ADD COLUMN IF NOT EXISTS metric_unit TEXT NOT NULL DEFAULT '';
UPDATE chore_logs l SET metric_unit = COALESCE(NULLIF(c.metric_unit, ''), 'mL')
FROM chores c WHERE c.id=l.chore_id AND c.household_id=l.household_id AND l.metric_unit=''
AND (c.has_volume_ml OR l.volume_ml IS NOT NULL OR COALESCE(l.indicator_volumes,'{}'::jsonb) <> '{}'::jsonb);
-- Old clients/binaries omit the new column. Snapshot their activity default
-- on insert and update too, so later activity edits cannot relabel retained history.
CREATE OR REPLACE FUNCTION snapshot_log_metric_unit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.metric_unit = '' THEN
  SELECT COALESCE(NULLIF(c.metric_unit,''),'mL') INTO NEW.metric_unit
  FROM chores c WHERE c.id=NEW.chore_id AND c.household_id=NEW.household_id
  AND (c.has_volume_ml OR NEW.volume_ml IS NOT NULL OR COALESCE(NEW.indicator_volumes,'{}'::jsonb) <> '{}'::jsonb);
  NEW.metric_unit := COALESCE(NEW.metric_unit, '');
 END IF;
 RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS chore_logs_snapshot_unit ON chore_logs;
CREATE TRIGGER chore_logs_snapshot_unit BEFORE INSERT OR UPDATE ON chore_logs
FOR EACH ROW EXECUTE FUNCTION snapshot_log_metric_unit();

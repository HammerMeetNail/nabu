CREATE TEMP TABLE _cat_chore_updates AS
SELECT
    source.id AS source_id,
    source.household_id,
    source.sort_order,
    CASE COALESCE(source.predefined_key, source.name)
        WHEN 'Feed Orange Cat' THEN 'Feed Mongo'
        WHEN 'Feed Black Cat' THEN 'Feed Roger'
    END AS new_name,
    duplicate.id AS duplicate_id
FROM chores source
LEFT JOIN chores duplicate
  ON duplicate.household_id = source.household_id
 AND duplicate.name = CASE COALESCE(source.predefined_key, source.name)
        WHEN 'Feed Orange Cat' THEN 'Feed Mongo'
        WHEN 'Feed Black Cat' THEN 'Feed Roger'
    END
 AND duplicate.id <> source.id
WHERE source.is_predefined = TRUE
  AND COALESCE(source.predefined_key, source.name) IN ('Feed Orange Cat', 'Feed Black Cat');

UPDATE chores target
SET sort_order = updates.sort_order
FROM _cat_chore_updates updates
WHERE target.id = updates.duplicate_id;

UPDATE chore_logs logs
SET chore_id = updates.duplicate_id
FROM _cat_chore_updates updates
WHERE updates.duplicate_id IS NOT NULL
  AND logs.chore_id = updates.source_id;

UPDATE chore_schedules schedules
SET chore_id = updates.duplicate_id
FROM _cat_chore_updates updates
WHERE updates.duplicate_id IS NOT NULL
  AND schedules.chore_id = updates.source_id;

UPDATE user_preferences preferences
SET chore_order = COALESCE(
    (
        SELECT jsonb_agg(to_jsonb(deduped.id) ORDER BY deduped.ordinality)
        FROM (
            SELECT DISTINCT ON (replaced.id)
                replaced.id,
                replaced.ordinality
            FROM (
                SELECT
                    COALESCE(updates.duplicate_id, elements.value::bigint) AS id,
                    elements.ordinality
                FROM jsonb_array_elements_text(preferences.chore_order) WITH ORDINALITY AS elements(value, ordinality)
                LEFT JOIN _cat_chore_updates updates
                  ON updates.household_id = users.household_id
                 AND updates.source_id = elements.value::bigint
                ORDER BY elements.ordinality
            ) replaced
            ORDER BY replaced.id, replaced.ordinality
        ) deduped
    ),
    '[]'::jsonb
)
FROM users
WHERE preferences.user_id = users.id
  AND EXISTS (
      SELECT 1
      FROM _cat_chore_updates updates
      WHERE updates.household_id = users.household_id
        AND updates.duplicate_id IS NOT NULL
  );

UPDATE user_preferences preferences
SET hidden_home_chore_ids = COALESCE(
    (
        SELECT jsonb_agg(to_jsonb(deduped.id) ORDER BY deduped.ordinality)
        FROM (
            SELECT DISTINCT ON (replaced.id)
                replaced.id,
                replaced.ordinality
            FROM (
                SELECT
                    COALESCE(updates.duplicate_id, elements.value::bigint) AS id,
                    elements.ordinality
                FROM jsonb_array_elements_text(preferences.hidden_home_chore_ids) WITH ORDINALITY AS elements(value, ordinality)
                LEFT JOIN _cat_chore_updates updates
                  ON updates.household_id = users.household_id
                 AND updates.source_id = elements.value::bigint
                ORDER BY elements.ordinality
            ) replaced
            ORDER BY replaced.id, replaced.ordinality
        ) deduped
    ),
    '[]'::jsonb
)
FROM users
WHERE preferences.user_id = users.id
  AND EXISTS (
      SELECT 1
      FROM _cat_chore_updates updates
      WHERE updates.household_id = users.household_id
        AND updates.duplicate_id IS NOT NULL
  );

DELETE FROM chores chores_to_delete
USING _cat_chore_updates updates
WHERE chores_to_delete.id = updates.source_id
  AND updates.duplicate_id IS NOT NULL;

UPDATE chores chores_to_update
SET name = updates.new_name,
    is_predefined = FALSE,
    predefined_key = NULL
FROM _cat_chore_updates updates
WHERE chores_to_update.id = updates.source_id
  AND updates.duplicate_id IS NULL;

DROP TABLE _cat_chore_updates;

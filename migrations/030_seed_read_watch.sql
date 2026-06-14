INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, indicator_labels, indicator_defaults, has_rating)
SELECT h.id, 'Read Book', '📖', '#8B5CF6', 13, 'personal', TRUE, 'Read Book', '[]', '[]', TRUE
FROM households h
WHERE NOT EXISTS (
  SELECT 1 FROM chores c WHERE c.household_id = h.id AND c.predefined_key = 'Read Book'
);

INSERT INTO chores (household_id, name, icon, color, sort_order, category, is_predefined, predefined_key, indicator_labels, indicator_defaults, has_rating)
SELECT h.id, 'Watch Movie', '🎬', '#EF4444', 14, 'personal', TRUE, 'Watch Movie', '[]', '[]', TRUE
FROM households h
WHERE NOT EXISTS (
  SELECT 1 FROM chores c WHERE c.household_id = h.id AND c.predefined_key = 'Watch Movie'
);

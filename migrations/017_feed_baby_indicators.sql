-- Set indicator_labels for existing "Feed Baby" predefined chores
UPDATE chores
SET indicator_labels = '["🍼 formula","🤱 breast"]'
WHERE name = 'Feed Baby'
  AND is_predefined = TRUE
  AND indicator_labels = '[]';

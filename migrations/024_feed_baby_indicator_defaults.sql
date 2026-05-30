UPDATE chores SET indicator_defaults = '["🍼 formula"]'::jsonb WHERE predefined_key = 'Feed Baby' AND indicator_defaults = '[]'::jsonb;

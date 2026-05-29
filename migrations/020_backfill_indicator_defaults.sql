UPDATE chores SET indicator_defaults = '["💛 pee"]'::jsonb WHERE predefined_key = 'Change Baby' AND indicator_defaults = '[]'::jsonb;

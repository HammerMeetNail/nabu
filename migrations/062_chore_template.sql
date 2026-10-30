-- What a pack's own activity started from ("Feed Baby" for "Feed Ada") and
-- whom the pack made it for ("Ada"), which a rename keeps: Stats gives each
-- baby a card. Shared and seeded activities have their template in
-- predefined_key; both stay NULL for them and for activities members make.
ALTER TABLE chores ADD COLUMN IF NOT EXISTS template_key TEXT;
ALTER TABLE chores ADD COLUMN IF NOT EXISTS named_for TEXT;

-- Packs made own activities before they recorded either: custom tasks no
-- member created, named in their pack's format and with their template's
-- icon. One since renamed or given another icon keeps neither, and Stats
-- shows it as any other activity, as before.
UPDATE chores c
SET template_key = f.template,
    named_for = substr(c.name, length(f.prefix) + 1, length(c.name) - length(f.prefix) - length(f.suffix))
FROM (VALUES
    ('Feed Baby', 'Feed ', '', '🍼'),
    ('Change Baby', 'Change ', '', '👶'),
    ('Baby Bath', '', '''s Bath', '🛀'),
    ('Feed Cats', 'Feed ', '', '🐱'),
    ('Walk Dog', 'Walk ', '', '🐕'),
    ('Feed Dog', 'Feed ', '', '🐶')
) AS f(template, prefix, suffix, icon)
WHERE c.template_key IS NULL AND NOT c.is_predefined AND c.created_by IS NULL
  AND c.icon = f.icon
  AND length(c.name) > length(f.prefix) + length(f.suffix)
  AND left(c.name, length(f.prefix)) = f.prefix
  AND right(c.name, length(f.suffix)) = f.suffix;

-- Every meds template has the same icon: "Ada's Meds" follows the pack whose
-- own activities are for Ada.
WITH owner AS (
    SELECT DISTINCT ON (household_id, named_for) household_id, named_for,
        CASE WHEN template_key IN ('Feed Baby', 'Change Baby', 'Baby Bath') THEN 'Baby Meds'
             WHEN template_key = 'Feed Cats' THEN 'Cat Meds'
             ELSE 'Dog Meds' END AS meds
    FROM chores
    WHERE named_for IS NOT NULL
      AND template_key IN ('Feed Baby', 'Change Baby', 'Baby Bath', 'Feed Cats', 'Walk Dog', 'Feed Dog')
    ORDER BY household_id, named_for, id
)
UPDATE chores c
SET template_key = owner.meds, named_for = owner.named_for
FROM owner
WHERE c.template_key IS NULL AND NOT c.is_predefined AND c.created_by IS NULL
  AND c.icon = '💊'
  AND c.household_id = owner.household_id
  AND c.name = owner.named_for || '''s Meds';

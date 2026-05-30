-- Multi-household support migration
-- 1. Add initials to households (for header indicator and short display)
ALTER TABLE households ADD COLUMN IF NOT EXISTS initials TEXT NOT NULL DEFAULT '';

-- 2. Add active_household_id to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS active_household_id BIGINT REFERENCES households(id);

-- 3. Create user_households join table
CREATE TABLE IF NOT EXISTS user_households (
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    household_id BIGINT NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'member',
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, household_id)
);

-- 4. Backfill user_households from existing users.household_id / users.role
INSERT INTO user_households (user_id, household_id, role, joined_at)
SELECT id, household_id, role, created_at
FROM users
WHERE household_id IS NOT NULL
ON CONFLICT (user_id, household_id) DO NOTHING;

-- 5. Set active_household_id from household_id
UPDATE users SET active_household_id = household_id WHERE household_id IS NOT NULL;

-- 6. Backfill initials for existing households from their name
-- Take first letter of each word (up to 3), uppercase
UPDATE households SET initials = (
    SELECT string_agg(upper(left(word, 1)), '')
    FROM (
        SELECT unnest(string_to_array(trim(name), ' ')) AS word
        LIMIT 3
    ) words
    WHERE word != ''
) WHERE initials = '';

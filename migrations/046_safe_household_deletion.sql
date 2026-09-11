-- A household is never the owner of a user account. The old cascade could
-- delete other accounts while deleting one user's sole-member household.
ALTER TABLE users DROP CONSTRAINT users_household_id_fkey;
ALTER TABLE users ADD CONSTRAINT users_household_id_fkey
    FOREIGN KEY (household_id) REFERENCES households(id) ON DELETE SET NULL;
ALTER TABLE users DROP CONSTRAINT users_active_household_id_fkey;
ALTER TABLE users ADD CONSTRAINT users_active_household_id_fkey
    FOREIGN KEY (active_household_id) REFERENCES households(id) ON DELETE SET NULL;

-- Existing generated links get a bounded transition window; fresh links are
-- one-use and expire after seven days. Permanent codes rotate on revocation.
UPDATE invites SET expires_at = NOW() + INTERVAL '7 days' WHERE expires_at IS NULL;
UPDATE invites SET max_uses = used_count + 1 WHERE max_uses <= 0;

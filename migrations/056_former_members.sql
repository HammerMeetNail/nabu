-- Account deletion keeps a household's shared history (a co-parent deleting
-- their account must not erase half of the baby's log). The departing user's
-- retained logs are reassigned to a per-household "former member" tombstone:
-- a negative ID drawn from this sequence. It has no row, email, name or avatar
-- and is not derivable from the deleted user's ID.
CREATE SEQUENCE IF NOT EXISTS former_member_seq;

-- Tombstoned logs reference no users row, so the cascade that used to delete
-- them with the account is replaced by explicit handling in DeleteUser.
ALTER TABLE chore_logs DROP CONSTRAINT IF EXISTS chore_logs_user_id_fkey;

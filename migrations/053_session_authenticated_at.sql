-- Existing sessions have no trustworthy record of their last credential proof.
-- They must sign in again before issuing an operator key.
ALTER TABLE sessions ADD COLUMN authenticated_at TIMESTAMPTZ;

-- Credential changes invalidate sessions and any password check still in flight.
ALTER TABLE users ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;

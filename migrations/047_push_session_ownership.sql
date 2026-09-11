-- Legacy registrations have no provable session owner. Clients re-register
-- their existing browser/device subscription after the next authenticated load.
DELETE FROM push_subscriptions;
DELETE FROM mobile_device_tokens;
ALTER TABLE push_subscriptions ADD COLUMN session_hash TEXT NOT NULL REFERENCES sessions(token_hash) ON DELETE CASCADE;
ALTER TABLE push_subscriptions ADD COLUMN binding_id TEXT NOT NULL;
ALTER TABLE push_subscriptions ADD COLUMN registration_id TEXT NOT NULL;
ALTER TABLE push_subscriptions DROP CONSTRAINT IF EXISTS push_subscriptions_user_id_endpoint_key;
CREATE UNIQUE INDEX push_subscriptions_endpoint_owner ON push_subscriptions(endpoint);
CREATE INDEX push_subscriptions_session ON push_subscriptions(session_hash);
ALTER TABLE mobile_device_tokens ADD COLUMN session_hash TEXT NOT NULL REFERENCES sessions(token_hash) ON DELETE CASCADE;
ALTER TABLE mobile_device_tokens ADD COLUMN registration_id TEXT NOT NULL;
CREATE INDEX mobile_device_tokens_session ON mobile_device_tokens(session_hash);

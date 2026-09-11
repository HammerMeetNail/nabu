-- Mail is committed with the account and retried independently of SMTP uptime.
-- Bodies contain temporary email capabilities; purge after delivery or expiry.
CREATE TABLE auth_mail_outbox (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_id TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX auth_mail_outbox_due_idx ON auth_mail_outbox (next_attempt, id);

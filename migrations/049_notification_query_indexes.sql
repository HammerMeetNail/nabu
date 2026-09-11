SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- An interrupted optional concurrent prebuild must not silently skip the real
-- index on startup. Resolve the actual table even with an empty first schema.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
        WHERE i.indrelid = 'notifications'::regclass
          AND c.relname IN ('idx_notifications_user_created', 'idx_notifications_unread')
          AND NOT i.indisvalid
    ) THEN
        RAISE EXCEPTION 'invalid notification index; drop and rebuild it before retrying';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread
    ON notifications (user_id) WHERE is_read = false;

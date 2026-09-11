-- Measured against 10 households / 500k logs. Bound startup work so a failed
-- migration leaves the previous app eligible for deployment rollback. Larger
-- installations can prebuild these exact indexes CONCURRENTLY (runbook).
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE INDEX IF NOT EXISTS idx_chore_logs_household_chore_latest
    ON chore_logs (household_id, chore_id, completed_at DESC, id DESC);

-- pg_trgm is trusted, but requires database CREATE privilege on first install.
-- Respect an existing extension's namespace instead of assuming search_path.
DO $migration$
DECLARE extension_schema TEXT;
BEGIN
    SELECT n.nspname INTO extension_schema
      FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace
      WHERE e.extname='pg_trgm';
    IF extension_schema IS NULL THEN
        CREATE EXTENSION pg_trgm WITH SCHEMA public;
        extension_schema := 'public';
    END IF;
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_chore_logs_search_trgm ON chore_logs USING GIN (note %I.gin_trgm_ops, title %I.gin_trgm_ops)',
                   extension_schema, extension_schema);
    IF EXISTS (SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
               WHERE i.indrelid='chore_logs'::regclass
               AND c.relname IN ('idx_chore_logs_household_chore_latest','idx_chore_logs_search_trgm')
               AND NOT i.indisvalid) THEN
        RAISE EXCEPTION 'history index is invalid; rebuild it before deployment';
    END IF;
END
$migration$;

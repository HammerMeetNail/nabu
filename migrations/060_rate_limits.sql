-- Rate-limit windows every instance shares, for the sign-in and join limits.
-- client is the truncated SHA-256 of the client's IP that request logs
-- already carry, never the address itself. A window only matters until it
-- ends, so the table is unlogged and every instance deletes ended windows
-- each minute.
CREATE UNLOGGED TABLE IF NOT EXISTS rate_limits (
    scope TEXT NOT NULL,
    client TEXT NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    count INTEGER NOT NULL,
    PRIMARY KEY (scope, client)
);
CREATE INDEX IF NOT EXISTS idx_rate_limits_window_end ON rate_limits (window_end);

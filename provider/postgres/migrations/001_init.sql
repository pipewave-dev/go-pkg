CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    last_heartbeat TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS active_connections (
    user_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    holder_id TEXT NOT NULL,
    connection_type SMALLINT NOT NULL DEFAULT 0,
    last_heartbeat TIMESTAMPTZ NOT NULL,
    ttl TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, session_id)
);

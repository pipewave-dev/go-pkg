CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    last_heartbeat TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS active_connections (
    user_id TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    holder_id TEXT NOT NULL,
    connection_type SMALLINT NOT NULL DEFAULT 0,
    last_heartbeat TIMESTAMPTZ NOT NULL,
    ttl TIMESTAMPTZ NOT NULL,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status SMALLINT NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, instance_id)
);

-- pending_messages stores pre-wrapped WebSocket response bytes for temporarily disconnected sessions.
-- session_key = user_id || ':' || instance_id
-- send_at is used as the ordering key (ascending).
-- expires_at drives row cleanup; application-level TTL matches MessageHub.TTL.
CREATE TABLE IF NOT EXISTS pending_messages (
    session_key TEXT NOT NULL,
    send_at     BIGINT NOT NULL,  -- Unix nano
    message     BYTEA NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (session_key, send_at)
);

CREATE INDEX IF NOT EXISTS idx_pending_messages_expires_at ON pending_messages (expires_at);

-- Ephemeral RP state. Canonical identity/grants remain owned by migration 75.
CREATE TABLE platform_browser_transaction (
 key_hash bytea PRIMARY KEY CHECK(octet_length(key_hash)=32),
 encrypted_state bytea NOT NULL CHECK(octet_length(encrypted_state)>28),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '5 minutes')
);
CREATE TABLE platform_browser_session (
 key_hash bytea PRIMARY KEY CHECK(octet_length(key_hash)=32),
 encrypted_state bytea NOT NULL CHECK(octet_length(encrypted_state)>28),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '8 hours')
);
CREATE INDEX platform_browser_transaction_expiry ON platform_browser_transaction(expires_at);
CREATE INDEX platform_browser_session_expiry ON platform_browser_session(expires_at);

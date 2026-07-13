CREATE TABLE client_invitations(
  id TEXT PRIMARY KEY,
  secret_hash BLOB NOT NULL UNIQUE,
  username TEXT NOT NULL,
  role INTEGER NOT NULL,
  validator_address TEXT NOT NULL,
  validator_pin TEXT NOT NULL,
  expires_unix_ms INTEGER NOT NULL,
  consumed_unix_ms INTEGER,
  revoked_unix_ms INTEGER,
  created_by TEXT REFERENCES users(id),
  created_unix_ms INTEGER NOT NULL
);
CREATE INDEX client_invitations_expires_idx ON client_invitations(expires_unix_ms);

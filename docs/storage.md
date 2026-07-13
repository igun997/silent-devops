# Validator storage

Validator uses SQLite with WAL, foreign keys, `synchronous=FULL`, 5-second busy timeout, transactional migrations, and one process-local writer connection.

## Data rules

SQLite stores password hashes, token hashes, public SSH keys, certificate metadata, jobs, audit metadata, sessions, and metrics. It never stores CA secrets, private keys, plaintext passwords or enrollment tokens. Command output remains `NULL` unless capture is explicitly authorized and capped by protocol limits.

Minute metrics and audit events use seven-day retention. Cleanup failure must block new privileged operations until persistence health recovers.

## Integrity

Stop validator, then run application integrity tooling backed by `PRAGMA integrity_check`. Never repair by deleting database automatically.

## Backup

Stop writes or place validator in maintenance mode. Checkpoint WAL, copy database to root-only destination, then test opening copy and run integrity check. `store.Backup` performs checkpoint and exclusive `0600` copy.

## Restore

1. Stop validator.
2. Preserve current database and WAL/SHM files.
3. Copy verified backup into configured path with owner-only permissions.
4. Start validator; migrations run transactionally.
5. Run integrity check and verify latest audit/job records.

Never restore CA material from SQLite; CA key backup is separate encrypted root-only procedure.

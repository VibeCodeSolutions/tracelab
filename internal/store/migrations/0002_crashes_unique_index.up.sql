-- Defense-in-depth uniqueness for crash dedup.
--
-- Today UpsertCrash runs SELECT-then-UPDATE/INSERT inside a single tx,
-- and the store enforces MaxOpenConns=1, so concurrent ingests for the
-- same (session_id, fingerprint) cannot race. This unique index makes
-- that invariant explicit at the schema level — if MaxOpenConns is ever
-- raised, the index turns a logic bug (duplicate row) into a constraint
-- error that UpsertCrash can detect and retry.
CREATE UNIQUE INDEX idx_crashes_session_fp ON crashes(session_id, fingerprint);

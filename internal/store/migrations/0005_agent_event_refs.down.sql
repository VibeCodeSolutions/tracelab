-- Reverse of 0005_agent_event_refs.up.sql — additive migration, so the
-- down-script is a single DROP TABLE. No column relaxation, no data
-- migration; the agent_event_refs rows are bounded to the new feature
-- and the rollback simply takes them with the table.
DROP INDEX IF EXISTS idx_agent_event_refs_event;
DROP INDEX IF EXISTS idx_agent_event_refs_spawn;
DROP INDEX IF EXISTS idx_agent_event_refs_uniq;
DROP TABLE IF EXISTS agent_event_refs;

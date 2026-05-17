-- Reverse of 0003_agents_schema.up.sql — drop in FK-respecting order
-- (children before parents). SQLite drops indexes implicitly when the
-- owning table is dropped, but we list them explicitly with IF EXISTS
-- to keep the down-script readable and idempotent (mirrors the style
-- of 0001_initial.down.sql).
DROP INDEX IF EXISTS idx_agent_edges_to;
DROP INDEX IF EXISTS idx_agent_edges_from;
DROP INDEX IF EXISTS idx_agent_edges_from_to_type_ts;
DROP TABLE IF EXISTS agent_mailbox_edges;

DROP INDEX IF EXISTS idx_agent_verdicts_spawn;
DROP INDEX IF EXISTS idx_agent_verdicts_spawn_verdict_ts;
DROP TABLE IF EXISTS agent_verdicts;

DROP INDEX IF EXISTS idx_agent_tokens_spawn;
DROP INDEX IF EXISTS idx_agent_tokens_spawn_ts_source;
DROP TABLE IF EXISTS agent_tokens;

DROP INDEX IF EXISTS idx_agent_spawns_started_at;
DROP INDEX IF EXISTS idx_agent_spawns_session_ref;
DROP INDEX IF EXISTS idx_agent_spawns_project;
DROP INDEX IF EXISTS idx_agent_spawns_parent;
DROP TABLE IF EXISTS agent_spawns;

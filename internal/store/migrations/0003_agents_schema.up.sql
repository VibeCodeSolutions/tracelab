-- Phase 2d agent-ingest layer schema.
--
-- Four tables persist the AI-agent observability domain alongside the
-- existing app-log domain (sessions/events/crashes/screenshots). The
-- agent domain has its own lifecycle (spawn → token-usage → verdict →
-- mailbox-edges) and is populated by three concurrent ingest sources
-- (SDK-hooks / transcript-tail / MCP-push) — see ADR-013 in docs/ARCH.md
-- for the multi-ingest rationale and the per-source-uniqueness contract.
--
-- Idempotency model: agent_spawns.id is the global identifier (ULID-shaped
-- 26-char string, supplied by the writer). The three child tables enforce
-- UNIQUE-tuples that include `source` (or the natural-event keys), so the
-- same event ingested twice from two different sources collapses to one
-- row rather than duplicating. See ADR-013 §Consequences.
--
-- This migration is non-destructive and additive: no existing column,
-- index, or constraint on sessions/events/crashes/screenshots is touched.

CREATE TABLE agent_spawns (
    id          TEXT PRIMARY KEY,
    parent_id   TEXT REFERENCES agent_spawns(id) ON DELETE SET NULL,
    skill       TEXT NOT NULL,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    project     TEXT NOT NULL,
    session_ref TEXT REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX idx_agent_spawns_parent      ON agent_spawns(parent_id);
CREATE INDEX idx_agent_spawns_project     ON agent_spawns(project);
CREATE INDEX idx_agent_spawns_session_ref ON agent_spawns(session_ref);
CREATE INDEX idx_agent_spawns_started_at  ON agent_spawns(started_at DESC);

CREATE TABLE agent_tokens (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    spawn_id      TEXT NOT NULL REFERENCES agent_spawns(id) ON DELETE CASCADE,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read    INTEGER NOT NULL DEFAULT 0,
    cache_write   INTEGER NOT NULL DEFAULT 0,
    ts            INTEGER NOT NULL,
    source        TEXT NOT NULL CHECK(source IN ('sdk-hook','transcript','mcp-push'))
);

CREATE UNIQUE INDEX idx_agent_tokens_spawn_ts_source ON agent_tokens(spawn_id, ts, source);
CREATE INDEX        idx_agent_tokens_spawn           ON agent_tokens(spawn_id);

CREATE TABLE agent_verdicts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    spawn_id     TEXT NOT NULL REFERENCES agent_spawns(id) ON DELETE CASCADE,
    verdict      TEXT NOT NULL CHECK(verdict IN ('freigabe','auflagen','rueckgabe','eskalation','none')),
    lerneffekt_md TEXT,
    ts           INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_agent_verdicts_spawn_verdict_ts ON agent_verdicts(spawn_id, verdict, ts);
CREATE INDEX        idx_agent_verdicts_spawn            ON agent_verdicts(spawn_id);

CREATE TABLE agent_mailbox_edges (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    from_spawn_id TEXT NOT NULL REFERENCES agent_spawns(id) ON DELETE CASCADE,
    to_spawn_id   TEXT NOT NULL REFERENCES agent_spawns(id) ON DELETE CASCADE,
    edge_type     TEXT NOT NULL CHECK(edge_type IN ('spawn','return','escalate','delegate')),
    ts            INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_agent_edges_from_to_type_ts ON agent_mailbox_edges(from_spawn_id, to_spawn_id, edge_type, ts);
CREATE INDEX        idx_agent_edges_from            ON agent_mailbox_edges(from_spawn_id);
CREATE INDEX        idx_agent_edges_to              ON agent_mailbox_edges(to_spawn_id);

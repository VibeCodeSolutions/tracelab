-- Phase 2d S5-Tail — agent_event_refs (ADR-014 Accepted, Option B).
--
-- Cross-domain bridge: links an agent_spawns row to an events row from
-- the app-log domain (sessions/events/crashes/screenshots, see
-- migration 0001). Both FK targets are stable primary keys:
--   - agent_spawns.id       — TEXT writer-supplied (ULID-shaped)
--   - events.id             — INTEGER PRIMARY KEY AUTOINCREMENT
--
-- ADR-014 rationale (verbatim from docs/ARCH.md §ADR-014 Decision):
--   1. Additive vs destructive migration — no column-relaxation on the
--      existing agent_mailbox_edges, rollback is a single DROP TABLE,
--      no data-loss risk.
--   2. Domain clarity preserved — agent_mailbox_edges stays
--      agent↔agent, agent_event_refs becomes agent↔app-log; read
--      consumers never filter on target_type.
--   3. /agents/edges wire-shape stays exactly as shipped in S5;
--      cross-references surface as /agents/event_refs sibling endpoint.
--
-- Idempotency: UNIQUE (spawn_id, event_id, ref_type, ts) lets the three
-- ingest sources (sdk-hook / transcript / mcp-push) all write the same
-- reference and collapse to one row via INSERT OR IGNORE — same
-- per-source-uniqueness contract as agent_mailbox_edges (ADR-013).
--
-- ref_type vocabulary is fixed by the CHECK constraint:
--   'observed'   — spawn passively observed this event in its window
--   'context'    — event was part of the spawn's input context
--   'caused-by'  — event was directly caused by this spawn's actions
--
-- CASCADE delete on both FKs: dropping a spawn or an event cleans the
-- reference row automatically (no orphan).

CREATE TABLE agent_event_refs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    spawn_id     TEXT    NOT NULL REFERENCES agent_spawns(id) ON DELETE CASCADE,
    event_id     INTEGER NOT NULL REFERENCES events(id)        ON DELETE CASCADE,
    ref_type     TEXT    NOT NULL CHECK(ref_type IN ('observed','context','caused-by')),
    ts           INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_agent_event_refs_uniq  ON agent_event_refs(spawn_id, event_id, ref_type, ts);
CREATE INDEX        idx_agent_event_refs_spawn ON agent_event_refs(spawn_id);
CREATE INDEX        idx_agent_event_refs_event ON agent_event_refs(event_id);

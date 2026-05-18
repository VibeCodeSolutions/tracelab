package agents

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// Handler bundles the agent-domain HTTP handlers. Today only one
// endpoint is live (POST /agents/ingest). Read endpoints land in S3+.
//
// The struct mirrors the internal/dashboard.Handler pattern: a
// dependency bundle constructed once at wire-up and passed to chi as
// method values.
type Handler struct {
	store *store.Store
	log   *slog.Logger
}

// NewHandler constructs the agent-handler bundle. Both store and log
// must be non-nil — a nil store would panic on the first request and
// is a programming error, not a runtime condition.
func NewHandler(st *store.Store, log *slog.Logger) *Handler {
	if st == nil {
		panic("agents.NewHandler: store must be non-nil")
	}
	if log == nil {
		log = slog.Default()
	}
	return &Handler{store: st, log: log}
}

// ingestResponse is the JSON body returned from POST /agents/ingest.
// The per-table counts are forensic: a 200 with zero counts across
// the board is a fully-idempotent repeat (the writer can use that to
// confirm a retry was harmless).
type ingestResponse struct {
	Ingested store.AgentInsertResult `json:"ingested"`
}

// Ingest implements POST /agents/ingest. It:
//   - parses + validates the JSON payload (400 on shape errors),
//   - persists into the agent_* tables via INSERT OR IGNORE,
//   - returns 200 with per-table counts so operators can audit
//     whether a repeat-call was a no-op.
//
// Bearer auth lives one layer up (internal/http/server.go registers
// this handler inside the bearer-protected sub-group), so 401 is
// already handled before we see the request.
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	payload, err := decodeIngest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid json: "+err.Error()))
		return
	}
	if err := payload.validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err.Error()))
		return
	}

	result, err := h.persist(r.Context(), payload)
	if err != nil {
		h.log.LogAttrs(r.Context(), slog.LevelError, "agents ingest persist failed",
			slog.String("source", string(payload.Source)),
			slog.Any("error", err),
		)
		// FK violations (parent spawn missing) and other store-level
		// rejections collapse to 500 — the contract is "writer
		// supplies a coherent payload"; FK breaks are real bugs.
		writeJSON(w, http.StatusInternalServerError, errorBody("persist failed"))
		return
	}

	h.log.LogAttrs(r.Context(), slog.LevelInfo, "agents ingest",
		slog.String("source", string(payload.Source)),
		slog.Int64("spawns", result.Spawns),
		slog.Int64("tokens", result.Tokens),
		slog.Int64("verdicts", result.Verdicts),
		slog.Int64("mailbox_edges", result.MailboxEdges),
		slog.Int64("event_refs", result.EventRefs),
	)
	writeJSON(w, http.StatusOK, ingestResponse{Ingested: result})
}

// persist is the dispatch loop: spawn first (so child rows can FK
// to it inside the same request), then tokens, then verdicts, then
// edges. All four use INSERT OR IGNORE, so a repeat-call is a no-op
// at every step — no per-step rollback bookkeeping needed.
//
// Returns the per-table affected-rows count for the forensic
// response.
func (h *Handler) persist(ctx context.Context, p IngestPayload) (store.AgentInsertResult, error) {
	var res store.AgentInsertResult

	if p.Spawn != nil {
		spawn := store.AgentSpawn{
			ID:         p.Spawn.ID,
			ParentID:   p.Spawn.ParentID,
			Skill:      p.Spawn.Skill,
			StartedAt:  nsToTime(p.Spawn.StartedAt),
			Project:    p.Spawn.Project,
			SessionRef: p.Spawn.SessionRef,
		}
		if p.Spawn.EndedAt != nil {
			t := nsToTime(*p.Spawn.EndedAt)
			spawn.EndedAt = &t
		}
		n, err := h.store.InsertAgentSpawn(ctx, spawn)
		if err != nil {
			return res, err
		}
		res.Spawns = n
	}

	for _, t := range p.Tokens {
		n, err := h.store.InsertAgentTokens(ctx, store.AgentTokenUsage{
			SpawnID:      t.SpawnID,
			InputTokens:  t.InputTokens,
			OutputTokens: t.OutputTokens,
			CacheRead:    t.CacheRead,
			CacheWrite:   t.CacheWrite,
			TS:           nsToTime(t.TS),
			Source:       string(p.Source),
		})
		if err != nil {
			return res, err
		}
		res.Tokens += n
	}

	for _, v := range p.Verdicts {
		n, err := h.store.InsertAgentVerdict(ctx, store.AgentVerdict{
			SpawnID:      v.SpawnID,
			Verdict:      v.Verdict,
			LerneffektMD: v.LerneffektMD,
			TS:           nsToTime(v.TS),
		})
		if err != nil {
			return res, err
		}
		res.Verdicts += n
	}

	for _, e := range p.MailboxEdges {
		n, err := h.store.InsertAgentMailboxEdge(ctx, store.AgentMailboxEdge{
			FromSpawnID: e.FromSpawnID,
			ToSpawnID:   e.ToSpawnID,
			EdgeType:    e.EdgeType,
			TS:          nsToTime(e.TS),
		})
		if err != nil {
			return res, err
		}
		res.MailboxEdges += n
	}

	// Phase 2d S5-Tail — agent_event_refs (ADR-014 Accepted, Option B).
	// Cross-domain bridge from this spawn to an events row from the
	// app-log domain. Additive to the persist loop; existing callers
	// that omit EventRefs are no-op'd here.
	for _, er := range p.EventRefs {
		n, err := h.store.InsertAgentEventRef(ctx, store.AgentEventRef{
			SpawnID: er.SpawnID,
			EventID: er.EventID,
			RefType: er.RefType,
			TS:      nsToTime(er.TS),
		})
		if err != nil {
			return res, err
		}
		res.EventRefs += n
	}
	return res, nil
}

// writeJSON is a tiny local mirror of internal/http.writeJSON — pulled
// in here to avoid a back-import on the http package (which would
// create a cycle if a future read-endpoint in this package were
// exposed via the same shared helper). 6 lines duplicated, the
// alternative is a shared utility package which is overkill for one
// helper today.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("agents writeJSON encode failed", slog.Any("error", err))
	}
}

// errorBody is a tiny shape wrapper so the JSON body is consistent
// with the rest of the API (`{"error": "..."}`).
func errorBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// sentinel for future use — keeps errors import live for read handlers.
var _ = errors.New

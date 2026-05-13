package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Ingest posts a batch of events to the hub's /ingest endpoint and
// returns the number of events the hub reports as ingested (its
// `{"ingested":N}` response).
//
// An empty events slice is allowed and is sent through unchanged — the
// hub answers 202 with `{"ingested":0}`, which mirrors the contract in
// internal/http/handlers.go.
func (c *Client) Ingest(ctx context.Context, sessionID string, events []Event) (int, error) {
	if sessionID == "" {
		return 0, errors.New("client: Ingest requires a non-empty sessionID")
	}
	wire := ingestReqWire{
		SessionID: sessionID,
		Events:    make([]ingestEventWire, len(events)),
	}
	for i, e := range events {
		w := ingestEventWire{
			TS:     e.TS,
			Source: e.Source,
			Level:  e.Level,
			Msg:    e.Msg,
		}
		if len(e.Meta) > 0 {
			raw, err := json.Marshal(e.Meta)
			if err != nil {
				return 0, fmt.Errorf("client: marshal event[%d].meta: %w", i, err)
			}
			w.Meta = raw
		}
		wire.Events[i] = w
	}
	var resp ingestRespWire
	err := c.doRequest(ctx, requestOpts{
		method:   http.MethodPost,
		path:     "/ingest",
		body:     wire,
		auth:     true,
		respInto: &resp,
	})
	if err != nil {
		return 0, err
	}
	return resp.Ingested, nil
}

package client

// Tail will subscribe to the hub's /tail WebSocket endpoint and invoke
// onEvent for every frame received (optionally filtered server-side by
// sessionFilter). It is intentionally NOT implemented in S2 — the WS
// surface is part of S4 (`tail` sub-command), where the gorilla
// dependency is wired in and a hub-side integration test guards the
// frame schema.
//
// Tracking: see docs/ARCH.md ADR-003 "Initial surface".
// TODO(P2a-S4): implement against gorilla/websocket + hub /tail handler.

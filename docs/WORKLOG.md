---
type: worklog
projekt: tracelab
status: phase-1-merged
last-updated: 2026-05-10
qs-letzter-lauf: qs-20260510-005
phase-1-merge-commit: cee7a5d
---

# WORKLOG — VibeCoding — Tracelab

> Auftragslogbuch für das Projekt **Tracelab** (Cross-Platform Test-Log-Hub, Go-Stack).
> **2026-05-10 Migration:** WORKLOG ist ab jetzt im Repo unter `docs/WORKLOG.md`. Vorgänger-Datei lag unter `~/.claude/projects/-home-kaik-Projekte-tracelab/worklogs/vc.md` (Project-Memory) und ist als Read-only-Archiv mit Migrations-Hinweis dort verblieben.
>
> **2026-05-10 PHASE 1 GEMERGED:** `feat/phase-1-mvp-hub` per `--ff-only` nach `main` gemerged (Merge-Commit `cee7a5d`), Branch lokal+remote gelöscht. MVP-Hub ist live auf `main`. Phase 2 (CLI / MCP / Dashboard) noch nicht definiert. Backlog M1-M12 wartet auf Tail-Sprint oder thematischen Touch.

## Offener Backlog (konsolidiert M1–M12)

Aus den QS-Läufen qs-20260510-001 bis -005. Alle Minor, kein Re-Lauf nötig — werden bei Phase-1-Tail-Aufräumer oder thematisch beim nächsten Touch des betroffenen Pakets aufgeräumt.

| ID | Quelle | Datei:Bereich | Beschreibung |
|---|---|---|---|
| **M1** | qs-001 (S4) | `README.md:51-57` | Endpoints-Tabelle erweitern: `GET /tail` + Hinweis Bearer-Header (nicht Query) |
| **M2** | qs-001 (S4) | `internal/http/tail_test.go:116` | `time.Sleep(50ms)` ersetzen durch deterministischen `waitForSubs`-Helper aus `internal/ws/handler_test.go:231` (CI-Flake-Schutz) |
| **M3** | qs-002 (S5) | `internal/crash/detect.go:232,238` | Magic numbers extrahieren: `fingerprintTopFrames=3`, `fingerprintHexLen=16` |
| **M4** | qs-002 (S5) | `internal/store/migrations/` | Migration 0002: `CREATE UNIQUE INDEX idx_crashes_session_fp ON crashes(session_id, fingerprint)` als Defense-in-Depth (heute durch `MaxOpenConns=1` implizit serialisiert) |
| **M5** | qs-002 (S5) | `internal/http/ingest_crash_test.go:139-203` | Test-Theater `TestIngestUpsertCrashFailureDoesNotBreakResponse` — umbenennen oder Store-Interface-Mock einbauen |
| **M6** | qs-003 (S5-Korr) | `internal/crash/detect.go` | Coverage-Lücke: Default-Rust-Runtime-Panic ohne `RUST_BACKTRACE=1` (`thread 'X' panicked at src/foo.rs:N:C:\noops\nnote: run with...`) wird vom neuen `isRust` nicht mehr erkannt. Bewusster Trade-off — entweder Branch (d) `header + reRustLineNo` mit Kommentar, oder als bekannte Lücke dokumentieren mit `t.Skip`-Probe-Test |
| **M7** | qs-004 (S6) | `internal/adb/adb.go` (Devices-Parser) | `no permissions`-State (zwei-Wort) sprengt das Standard-Splitting im Devices-Parser |
| **M8** | qs-004 (S6) | `internal/adb/adb.go` (LogcatStream Cancel) | Code-Kommentar verspricht Cross-Platform, Cancel-Pfad nutzt POSIX-Signals. Doc präzisieren oder Windows-Pfad implementieren |
| **M9** | qs-004 (S6) | `internal/adb/adb.go` | `SetBinary(name) string` ist Test-Override, leakt in Public-API. Lösung: `internal_test.go` mit nicht-exportierter Variante oder build-tag |
| **M10** | qs-004 (S6) | `internal/adb/adb_test.go` | `go.uber.org/goleak` für robustes Goroutine-Leak-Detection in TearDown der LogcatStream-Tests |
| **M11** | qs-005 (S7) | `internal/adb/bridge.go:299-300` ↔ `internal/http/handlers.go:159+165` ↔ `README.md:88-90` | Publish/Insert-Reihenfolge inkonsistent: ADB-Bridge published synchron beim Append, /ingest published nach Insert. README behauptet Symmetrie. Entscheidung im Tail-Sprint: entweder Reihenfolge angleichen (Latenz steigt auf Batch-Intervall) ODER README präzisieren ("Live-Stream konsistent zum Subprocess, DB kann dahinter zurückfallen") — letzteres hat Forensik-Vorteil |
| **M12** | qs-005 (S7) | `internal/adb/bridge_test.go:261` | Tautologischer Msg-Check vergleicht Element gegen sich selbst. Soll-Array `wantMsgs := []string{...}` analog `wantLevels` (Z. 253) |

**Zusätzlicher Hint (kein Finding):** `httplayer.Config.{Read,Write}Timeout`-Felder unbenutzt — beim nächsten http-Pkg-Touch aufräumen.

---

## AUFTRAG #008 — Tracelab P1-S7 ADB Daemon-Wireup

- **Timestamp:** 2026-05-10T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** ADB-Library aus #007 in den Daemon einbinden — aus dem Bookmark-Block von #007 wird ein echter Sprint vor Phase-1-Merge (Admin-Entscheidung 2026-05-10).
  - **TOML-Section** `[adb]` in `tracelab.toml`/`tracelab.toml.example`/`internal/config/config.go`: `enabled bool` (Default false), `device_serial string` (leer = first available aus `Devices()`), `tag_filter string` (Default leer = alle).
  - **Bridge-Goroutine** in `cmd/hub/main.go`: bei `[adb].enabled = true` parallel zum WS-Hub einen Logcat-Bridge starten. Lifecycle via `signal.NotifyContext`-ctx aus main, sauberer Stop **vor** `hub.Close()`.
  - **Session-Strategie:** beim Bridge-Start eine neue Session via `Store.CreateSession(label="adb-bridge: <serial>")`, deren ID im Bridge-State halten; bei Reconnect erneut starten (nicht dieselbe Session weiterverwenden, sonst landen reconnect-Lücken in derselben Session — sauberer für Forensik).
  - **Level-Mapping** `LogcatLine.Level` → event.level: `V/D` → `debug`, `I` → `info`, `W` → `warn`, `E/F` → `error`, `S` → ignorieren (silent ist Filter, kein Log-Level).
  - **Event-Konstruktion:** `source="adb"`, `level` aus Mapping, `msg = Tag + ": " + Message`, `meta = {pid, tid, timestamp, level_raw, device_serial}` als JSON.
  - **Schreibpfad:** `Store.InsertEvents([]Event{...})` in Batches (z.B. alle 50ms oder alle 50 Lines, je nachdem was zuerst greift) **UND** `ws.Hub.Publish(...)` analog `/ingest`-Pfad — damit `/tail`-Subscriber adb-Lines live sehen.
  - **Reconnect:** wenn der adb-Subprocess EOF't (daemon-Restart, Device-Disconnect): exponential-Backoff (1s/2s/5s/10s, dann konstant 10s), neu starten **mit neuer Session** (siehe oben). Nicht den ganzen Hub kippen, Bridge-Goroutine ist isoliert.
  - **README-Section** „ADB Bridge (optional)" mit toml-Beispiel + Smoke-Anleitung (`adb devices` zeigt das Gerät + `[adb].enabled=true` + Hub starten + `tail -f` auf `/tail?session=...`).
- **DoD:**
  - `go test -race ./internal/adb/... ./internal/http/... ./internal/store/... ./cmd/hub/...` grün
  - Bridge-Goroutine-Test mit fake-adb (analog `TestLogcatStream_*`-Suite): Lines landen in events-Tabelle und im ws-Hub
  - Reconnect-Test: fake-adb beendet sich nach N Zeilen, Bridge wartet Backoff-Periode, startet neu, neue Session
  - Level-Mapping-Test: alle 5 Stufen + S=skip
  - HTTP-Regression: `TestIngest*` muss grün bleiben
  - `go vet`/`build` clean
  - Smoke gegen Daemon (mit fake-adb-PATH-Override): Bridge startet bei `enabled=true`, schreibt Lines in events, `/tail`-Subscriber empfängt sie. SIGTERM stoppt Bridge **vor** Hub-Close (ordering verifizieren in slog).
  - README-Section drin, toml-Beispiel kommentiert
- **Status:** QS grün — Findings-Gate freigegeben (qs-20260510-005), 2 Minor → Backlog M11-M12
- **Verlauf:**
  - `2026-05-10` — Admin entscheidet: Daemon-Wireup wird P1-S7 vor Merge nachgezogen, kein Phase-1.5. Eröffnet aus Bookmark in #007. Klasse 🟡 feature, Worker-Spawn ballard.
  - `2026-05-10` — belanna übernommen. Mehrere Komponenten (config-extension, bridge-goroutine, reconnect-backoff, level-mapping, README) — Worker-Spawn ballard via Subagent (Konsistenz Sprint-Reihe, Code-Implementation mehrerer Files inkl. cmd/hub-Integration).
  - `2026-05-10` — QS-Lauf gestartet (tuvok, qs-20260510-005).
  - `2026-05-10` — QS-Lauf qs-20260510-005 abgeschlossen. `go vet ./...` + `go build ./...` clean, `go test -race -count=3 ./internal/adb/... ./cmd/hub/...` und `go test -race -count=1 ./...` grün. Bridge-Lifecycle-Sequenz (bridgeCancel+wait → hub.Close → srv.Shutdown) im Code und in Integration-Test via slog-Index verifiziert. Per-reconnect-Session, Backoff-Reset bei gotLines, 50ms/50-Lines-Batch ohne Doppel-Flush-Race, Detached-2s-Flush, Level-Mapping V/D/I/W/E/F/S vollständig — alles wie spezifiziert. Sicherheits-Spot-Check: TagFilter/DeviceSerial fließen als argv (kein Shell-Injection-Vektor). **Findings: 0 Blocker / 0 Major / 2 Minor.** VC-008-KOR-01: Bridge ruft `Hub.Publish` SYNCHRON beim append in den Batch-Puffer (bridge.go:299-300), `/ingest` publisht aber NUR NACH erfolgreichem `InsertEvents` (handlers.go:159+165). README Z. 88-90 behauptet Symmetrie ("Persistence and /tail fan-out work exactly like a normal /ingest POST"). Folge: bei `InsertEvents`-Fehler sind ADB-Lines live im Hub aber nicht in der DB — über /ingest unmöglich. VC-008-WAR-01: bridge_test.go Z. 261 hat einen tautologischen Msg-Check (`e.Msg != "T: "+got[i].Msg[len("T: "):]` vergleicht ein Element gegen sich selbst), prüft also faktisch nichts. Status: `freigabe`.
  - `2026-05-10` — **Findings-Gate (chakotay)**: M11 (Publish/Insert-Asymmetrie) ist sachlich — README präzisieren ist kleinerer Fix als Code-Refactor; Live-Stream-Konsistenz hat Forensik-Vorteil. M12 ist Test-Robustheit. **Freigabe**, M11+M12 → Backlog. Auftrag #008 geschlossen. **Phase 1 (S1-S7) damit feature-complete und mergebar.** Halte bei Admin an für Phase-1-Closure-Approval (FF-Merge + Branch-Cleanup + Backlog-Strategie).
  - `2026-05-10` — ballard übernommen. 4 Commits gepusht: `c572f02` config [adb], `7db5cd4` bridge.go+tests, `0ca3b12` cmd/hub wireup+integration-test, `2016669` README-Section. **Bridge-Architektur:** `adb.Bridge` mit `BridgeStore`/`BridgePublisher`-Interfaces (Test-Injection), `streamFunc` als unexported Test-Override für LogcatStream. Per-reconnect neue Session via `Store.CreateSession(...)`, Backoff 1s/2s/5s/10s+constant, Counter-Reset bei mind. 1 Line. Batch-Insert 50ms-OR-50-Lines (Timer-Reset bei Size-Flush). Level-Mapping: V/D→debug, I→info, W→warn, E/F→error, S→drop. Events parallel via `Hub.Publish` für /tail-Live-Stream. Detached 2s-Timeout-Context für Final-Flush+EndSession damit Shutdown-Lines noch landen. **Lifecycle in cmd/hub:** Bridge-ctx aus signal.NotifyContext-ctx abgeleitet, expliziter `bridgeCancel()`+wait-on-bridgeDone vor `hub.Close()` vor `srv.Shutdown()`. Slog-Marker `adb bridge stopped` → `websocket hub closed` → `http server stopped`. **Tests:** 5 neue bridge_test.go (level-mapping, all-levels-line-flow incl. metadata, reconnect-creates-new-session, backoff-between-attempts, stream-error, ctx-cancel cleanup) + 1 cmd/hub integration-test (echtes Binary + fake-adb-PATH-shim, verifiziert events-Tabelle + slog-Stop-Order). Race-clean, vet/build clean, full-suite grün. **Smoke:** Binary mit fake-adb gestartet, 5 Lines pro Reconnect-Session in events-Tabelle, slog-Stop-Reihenfolge wie spezifiziert verifiziert.

---

## AUFTRAG #007 — Tracelab P1-S6 ADB-Bridge

- **Timestamp:** 2026-05-10T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** ADB-Bridge im neuen Package `internal/adb/` (Verzeichnis vorhanden, leer). Minimal-Funktionalität:
  - `Devices() ([]Device, error)` — `adb devices -l` parsen (serial, state, transport)
  - `LogcatStream(ctx, deviceSerial, optionaler tag-filter) (<-chan LogcatLine, error)` — `adb -s <serial> logcat -v threadtime`-Subprocess starten, Zeilen line-by-line in Channel pushen, Cancel via ctx, sauberes Process-Cleanup
  - Interner Helper `runAdb(args...)` mit Timeout/Exit-Handling
  - Anbindung an /ingest aus dem Daemon-Lifecycle (cmd/hub) als Bookmark — siehe AUFTRAG #008
  - **NICHT in #007:** `Forward(...)` (port-forwarding), Screenshot-Capture, Touch-Injection — alles Phase-2-Material.
- **DoD:** `go test -race ./internal/adb/...` grün; `go vet`/`build` clean; Library-only OK.
- **Status:** QS grün — Findings-Gate freigegeben (qs-20260510-004), 4 Minor → Backlog M7-M10
- **Verlauf:**
  - `2026-05-10` — Auto-Continuation aus #006 nach Re-QS-Freigabe. Klasse 🟡 feature, Worker-Spawn ballard.
  - `2026-05-10` — belanna übernommen. Daemon-Wireup-Entscheidung an ballard delegiert (Stretch-Goal).
  - `2026-05-10` — ballard übernommen. `internal/adb/`-Package implementiert (`adb.go` 379 LOC + `adb_test.go` 408 LOC). Public API: `Device`, `LogcatLine`, `Devices(ctx)`, `LogcatStream(ctx, serial, tagFilter)`, `SetBinary(name)`. Interner `runAdb(ctx, args...)` mit `defaultRunTimeout=10s`. **Lifecycle:** eigene Cancel-Watcher-Goroutine mit SIGTERM→3s-Wait→SIGKILL-Eskalation (statt `exec.CommandContext`). Reader-Goroutine als single Channel-Owner. **Tests** (12 Cases, race-clean): Devices-Parser für 1/N/0 + daemon-preamble + nil + binary-not-found, parseLogcatLine für Basic/Banner-Reject/CurrentYear, LogcatStream für Streaming-Order, ctx-cancel (SIGTERM), SIGKILL-Eskalation (trap-ignored TERM), Goroutine-Leak (5 parallel), Argv-Rendering. **Stretch-Goal Daemon-Wireup:** in Bookmark verschoben (jetzt AUFTRAG #008). Commit `e63e7da` zu origin gepusht.
  - `2026-05-10` — QS-Lauf qs-20260510-004 abgeschlossen. Build/Vet/Test grün, Subprocess-Lifecycle-Design (Defer-LIFO) sauber, keine Goroutine-Leaks. **Findings: 0 Blocker / 0 Major / 4 Minor (M7-M10).** Status: `freigabe`.
  - `2026-05-10` — **Findings-Gate (chakotay)**: 4 Minor proportional, Subprocess-Lifecycle korrekt designed. **Freigabe**, M7-M10 → Backlog. Auftrag geschlossen.

---

## AUFTRAG #006 — Tracelab P1-S5 Crash-Detect

- **Timestamp:** 2026-05-10T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Stacktrace-Erkennung im `/ingest`-Pfad (Java/Kotlin/Go/Rust/Python) mit Fingerprint + Dedup-Insert in `crashes`. Logik in `internal/crash/`. Anbindung in `internal/http/handlers.go::ingest`.
- **DoD:** Tests grün (Pattern-Detect pro Sprache, Fingerprint-Stabilität, Dedup-Counter, NoMatch-NoCrashRow); vet/build clean; Smoke 3× identischer Stack → 1 crashes-Row count=3.
- **Status:** QS grün — Re-QS freigegeben (qs-20260510-003), Auflagen K1/K2 erledigt, 4 Minor → Backlog M3-M6
- **Verlauf:**
  - `2026-05-10` — Auto-Continuation aus #005. Klasse 🟡 feature, Worker-Spawn ballard.
  - `2026-05-10` — belanna übernommen. crashes-Tabelle bereits in Migration 0001 — kein Schema-Change.
  - `2026-05-10` — ballard übernommen. `internal/crash/`-Pkg (detect.go + Tests). Regex-Detection für 5 Sprachen mit defensivem Bias. Normalisierung schluckt Zeilennummern, goroutine-IDs, Hex-Offsets, Pointer-Args. Fingerprint = SHA256 über Top-3-Frames, hex-16. `Store.UpsertCrash(...)` mit SELECT+UPDATE/INSERT in einer Tx, Session-Existenz vorab geprüft, **kein** Schema-Change. `handlers.go::ingest` ruft `detectAndUpsertCrashes` NACH `InsertEvents` + Hub-Publish; Errors logging-only, /ingest bleibt 202. `go test -race ./...` grün, vet/build clean. Smoke (Port 18767): 3× Java-Stack → 1 crashes-Row count=3, fingerprint `6505143ba3be8213`. Commits b864903 + 23c279f + 2b3b8a7 gepusht.
  - `2026-05-10` — QS qs-20260510-002. Build/Vet/Test grün, Schema-Compliance ok. **0 Blocker / 2 Major (KOR-01/02 Rust-Detect zu locker, False-Positives auf Prod-Logs reproduziert) / 3 Minor (M3-M5).** Status: `auflagen`.
  - `2026-05-10` — **Findings-Gate**: K1/K2 zur Korrektur an belanna. S6 wartet bis Re-QS grün.
  - `2026-05-10` — ballard Korrektur (K1+K2). `isRust` umgebaut: matcht nur noch bei (a) Header + ≥1 `at <file>.rs:N`, (b) Literal `stack backtrace:` + ≥1 numerierter Frame, (c) ≥2 Frame-Pairs. Neue Probe-Tests + Regressions-Guards. Commit `ae5ab4f` gepusht.
  - `2026-05-10` — QS-Re-Lauf qs-20260510-003: K1/K2 verifiziert, Regressions intakt. **Aber Coverage-Trade-off:** Default-Rust-Runtime ohne `RUST_BACKTRACE=1` wird nicht mehr erkannt. **0 Blocker / 0 Major / 1 Minor (M6).** Status: `freigabe`.
  - `2026-05-10` — **Findings-Gate Re-QS**: K1/K2 erledigt, M6 als bewusster Trade-off → Backlog. **Freigabe**. AUFTRAG #006 geschlossen.

---

## AUFTRAG #005 — Tracelab P1-S4 WS /tail

- **Timestamp:** 2026-05-10T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** WebSocket-Live-Stream-Endpoint `GET /tail` — gorilla/websocket, Bearer-Auth, optionaler Session-Filter `?session=<id>`. Pub/Sub-Hub `internal/ws/`, Anbindung an `/ingest` per Channel-Fanout (kein DB-Polling). Heartbeat 30s. Graceful close.
- **DoD:** Tests grün (Subscribe/Receive/Filter/Auth-Reject/Disconnect); vet/build clean; Smoke 2 Clients <100ms; SIGTERM trennt sauber.
- **Status:** QS grün — Findings-Gate freigegeben (qs-20260510-001, 2 Minor → Backlog M1-M2)
- **Verlauf:**
  - `2026-05-10` — Phase-1-Restscope-Plan (S4-S6) durch Admin freigegeben (Auto-Continuation, Modus 5a). Klasse 🟡 feature.
  - `2026-05-10` — belanna übernommen, Worker-Spawn ballard. Tech-Defaults aus #001 weiter aktiv (gorilla/websocket etc.). `bearerAuth`-Middleware constant-time wird wiederverwendet.
  - `2026-05-10` — ballard übernommen. `internal/ws/`-Pkg (`hub.go` Pub/Sub mit per-subscriber buffered channel + non-blocking-send/drop-on-full, `handler.go` mit gorilla/websocket-Upgrade, ping(30s)/pong(60s)-Heartbeat, CloseGoingAway-Frame). `/ingest` published nach DB-Insert direkt in Hub. `/tail` in chi-Group mit `bearerAuth`; chi-`Timeout`-Middleware in Sub-Sub-Group verschoben (sonst inkompatibel mit Hijack). `cmd/hub/main.go` ownt Hub für Daemon-Lifetime, schließt vor `srv.Shutdown()`. **Tests:** 6 ws + 5 ws-Handler + 3 http-Tail. Smoke (Port 18766): 2 Clients fanout <1ms first/last; SIGTERM Close-Frame `1001 going away` in 341ms. Commits `d16bc9a` + `dd956f8` gepusht.
  - `2026-05-10` — QS qs-20260510-001. Build/Vet/Test grün, Concurrency-Audit clean (kein Doppel-close, kein Send-on-closed-channel, Drop-on-full dokumentiert, Heartbeat-Ordering korrekt, Hub.Close vor srv.Shutdown). **0 Blocker / 0 Major / 2 Minor (M1 README-Drift /tail, M2 50ms-Sleep statt waitForSubs).** Status: `freigabe`.
  - `2026-05-10` — **Findings-Gate**: 2 Minor proportional, sachlich. **Freigabe** → Backlog M1/M2. Auto-Continuation → AUFTRAG #006.

---

## AUFTRAG #004 — Tracelab P1-S3 HTTP-Layer

- **Timestamp:** 2026-05-09T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** HTTP-Layer mit `go-chi/chi` — Endpoints `POST /session/start`, `POST /session/end`, `POST /ingest` (Batch), `GET /healthz`, `GET /sessions`. Bearer-Auth gegen `tracelab.toml` `[auth].token`. Middleware (slog, recover, timeout, auth außer /healthz). Server in `internal/http/`. `cmd/hub/main.go` integriert mit graceful shutdown 5s.
- **DoD:** Tests grün (httptest-basiert, alle Endpoints + Auth-Reject); vet/build clean; curl-Smoke gegen Binary.
- **Status:** erledigt — DoD grün
- **Verlauf:**
  - `2026-05-09` — Auftrag eröffnet nach S2-Done. Toolchain-Floor 1.25 akzeptiert.
  - `2026-05-09` — ballard übernommen.
  - `2026-05-09` — `internal/http/` (server/auth/middleware/handlers + Tests) und `internal/config/` (BurntSushi/toml-Loader) implementiert. cmd/hub integriert Store+chi-Server mit 5s graceful shutdown und CHANGEME-Token-Refusal. Endpoints: GET /healthz (no-auth), POST /session/start, POST /session/end (204), POST /ingest (202 batch), GET /sessions?limit. Middleware: RequestID, Recoverer, slog-Request-Logger, Timeout(30s), bearerAuth (constant-time). vet/build clean, `go test -race` grün (5 Store + Healthz/Auth/Flow/Batch100/InvalidJSON). Smoke (Port 18765, Token "smoke-token-1234"): /healthz 200, /session/start ohne Token 401, mit Token 200+JSON, /ingest 2 events 202, /sessions zeigt Session, SIGTERM sauber. Commit `cc1260a` gepusht.

---

## AUFTRAG #003 — Tracelab P1-S2 SQLite-Store

- **Timestamp:** 2026-05-09T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** SQLite-Store in `internal/store/` mit `modernc.org/sqlite` (CGO-frei), Schema sessions/events/crashes/screenshots, Migrations, PRAGMA-Setup (WAL, foreign_keys=ON, busy_timeout). Public API: Open/Close/CreateSession/EndSession/InsertEvents/RecentEvents.
- **DoD:** Tests grün; vet clean; Migrations idempotent; README-Storage-Sektion.
- **Status:** erledigt — DoD grün
- **Verlauf:**
  - `2026-05-09` — Auftrag eröffnet nach S1-Done.
  - `2026-05-09` — `internal/store/` implementiert (sqlite.go + Tests + Migration 0001 up/down). Eigenbau-Migrator mit `schema_migrations`-Versionstabelle. Session-IDs als 26-char hex (lexsortable). modernc.org/sqlite v1.50.0 zog Toolchain-Selbstupgrade auf 1.25.10. vet/build clean, `go test -race` grün (5 Tests: OpenAndMigrate, SessionLifecycle, IdempotentMigrations, ForeignKeyCascade, InsertEventsRejectsUnknownSession). Commit `0108ab2` gepusht. README um Storage-Sektion erweitert.

---

## AUFTRAG #002 — Tracelab P1-S1 Projekt-Skeleton

- **Timestamp:** 2026-05-09T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Projekt-Skeleton — `go mod init`, `cmd/hub/main.go` (minimal Daemon mit graceful shutdown), `internal/`-Struktur (store/ingest/ws/adb/crash mit `.gitkeep`), `tracelab.toml.example`, Makefile, .gitignore, README-Build-Anleitung. Branch `feat/phase-1-mvp-hub`.
- **DoD:** vet/build clean, `go run ./cmd/hub` startet+beendet sauber, Branch+Commit+Push.
- **Status:** erledigt — DoD grün
- **Verlauf:**
  - `2026-05-09` — Admin-Freigabe Variante 2 (Skeleton-Etappenschritt). Worker-Spawn ballard.
  - `2026-05-09` — Skeleton angelegt (go.mod stdlib-only, cmd/hub/main.go, internal/{store,ingest,ws,adb,crash}/.gitkeep, tracelab.toml.example, Makefile, .gitignore, README). Branch + Commit `c45ad1a` gepusht. **Blocker:** `go` nicht im PATH — Eskalation an Belanna.
  - `2026-05-09` — Tooling-Block gelöst via Tarball (Go 1.22.12 → `~/go-toolchain/`, sudo-frei). DoD verifiziert: vet/build clean, `go run ./cmd/hub` startet mit slog-JSON-Start-Message und beendet bei SIGINT.

---

## AUFTRAG #001 — Tracelab Phase 1 (MVP-Hub) — Umbrella

- **Timestamp:** 2026-05-07T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → (Bedarfsanalyse Seven `bedarf-20260507-001`) → Belanna
- **Auftrag:** MVP-Hub bauen — Go-Daemon mit `POST /ingest`, `WS /tail`, `POST /session/start|end`, SQLite-Store auf `/run/media/kaik/AE62672C6266F88B/tracelab/`. Session-Marker-Schema. Konfig via `tracelab.toml`.
- **Vorausgesetzt:** Skill `lyndsay-ballard` (Go-Backend-Lead) durch Harry erstellt — Auftrag liegt parallel.
- **Erwarteter Output:** Lauffähiger Hub-Daemon, HTTP/WS-Endpoints, SQLite-Schema sessions+events. E2E-Verification: Test-Session starten, Event posten, via WS empfangen.
- **Status:** erledigt — Phase 1 gemerged
- **Verlauf:**
  - `2026-05-07` — Auftrag eröffnet, Skill-Schöpfung Lyndsay Ballard parallel angestoßen.
  - `2026-05-07` — Skill `ballard` erstellt. Persona-Notiz unter `XBrain/50_Personen/Ballard.md`.
  - `2026-05-07` — Belanna Paket-Schneidung (P1-S1..S6). Tech-Defaults bestätigt (chi / gorilla / modernc.org/sqlite / log/slog). Spawn pausiert wegen Token-Vorsicht.
  - `2026-05-09` — S1-S3 in voriger Session abgearbeitet (Skeleton, Store, HTTP-Layer).
  - `2026-05-10` — S4 (WS /tail), S5 (Crash-Detect) inkl. Korrektur, S6 (ADB-Library) abgearbeitet. Phase-1-Restscope komplett.
  - `2026-05-10` — Admin-Erweiterung: S7 (ADB Daemon-Wireup) wird vor Merge nachgezogen, kein Phase-1.5.
  - `2026-05-10` — S7 done, alle QS-Läufe grün (qs-001..005), 12 Minor im Backlog. **FF-Merge nach `main` (Merge-Commit `cee7a5d`), Branch `feat/phase-1-mvp-hub` lokal+remote gelöscht.** MVP-Hub ist live. AUFTRAG #001 erledigt.

---

## Vorlage für neue Aufträge

```
## AUFTRAG #<nr> — <Titel>

- **Timestamp:** <YYYY-MM-DDTHH:MM>
- **Von:** chakotay
- **An:** <chef-skill>
- **Quelle-Kette:** Admin → Chakotay → <chef-skill>
- **Auftrag:** <konkrete Aufgabe>
- **Erwarteter Output:** <DoD>
- **Status:** offen | in Arbeit | QS grün | QS rot | erledigt | Rückgabe
- **Verlauf:**
  - `<ts>` — <Statuswechsel oder Notiz>
```

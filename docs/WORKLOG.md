---
type: worklog
projekt: tracelab
status: phase-2b-laufend (S1 Skeleton + ARCH-Vorab in Arbeit)
last-updated: 2026-05-15
qs-letzter-lauf: qs-20260514-003
phase-1-merge-commit: cee7a5d
phase-1-tail-merge-commit: 60adf48
phase-2a-merge-commit: bdc3a0c
aktiver-auftrag: "#018 P2b-S1 Skeleton + ARCH-Vorab"
---

# WORKLOG — VibeCoding — Tracelab

> Auftragslogbuch für das Projekt **Tracelab** (Cross-Platform Test-Log-Hub, Go-Stack).
> **2026-05-10 Migration:** WORKLOG ist ab jetzt im Repo unter `docs/WORKLOG.md`. Vorgänger-Datei lag unter `~/.claude/projects/-home-kaik-Projekte-tracelab/worklogs/vc.md` (Project-Memory) und ist als Read-only-Archiv mit Migrations-Hinweis dort verblieben.
>
> **2026-05-10 PHASE 1 GEMERGED:** `feat/phase-1-mvp-hub` per `--ff-only` nach `main` gemerged (Merge-Commit `cee7a5d`), Branch lokal+remote gelöscht. MVP-Hub ist live auf `main`. Phase 2 (CLI / MCP / Dashboard) noch nicht definiert. Backlog M1-M12 wartet auf Tail-Sprint oder thematischen Touch.
>
> **2026-05-10 TAIL-SPRINT ERÖFFNET (AUFTRAG #009):** Phase-1-Tail räumt M1–M12 in vier thematischen Paketen ab (P1 Doku, P2 ADB-Polish, P3 Crash/Store, P4 Test+Konsistenz). Branch `chore/phase-1-tail`, Commit pro Paket, QS-Sammelgate am Ende. Auto-Stop erwartet bei M11 (Architektur-Entscheidung Publish/Insert-Reihenfolge).
>
> **2026-05-13 PHASE 2 ERÖFFNET (AUFTRAG #010, Phase 2a):** Tool-Kette baut auf MVP-Hub auf — Phase 2 = CLI → MCP → Dashboard (linear). Plan-File: `~/.claude/plans/tracelab-phase-2-roadmap.md` (Admin-bestätigt Block 1/2/3). Phase 2a startet jetzt: `tracelab` CLI mit Subkommandos `run`/`tail`/`sessions`/`adb`. Branch `feat/phase-2-cli` von `main`@e4eb434.
>
> **2026-05-14 ADR-005 ENTSCHIEDEN — Phase-2a-DoD-Anpassung (Admin grün):** Option C — `run` wird aus Phase 2a gestrichen. `tracelab-hub` bleibt Daemon-Start, CLI ist purer Consumer (`sessions`/`tail`/`adb`). Begründung Belanna (übernommen): Daemon-Management ist eigene Problemklasse, separat von Log-Konsumption; CLI+MCP zuerst in Userhand bekommen, `run` später revisit falls realer Bedarf. DoD von AUFTRAG #010 entsprechend reduziert auf S1-S5 (`run.go`-Stub bleibt cosmetic im Code mit Stage-Mapping „revisit later if needed", kann nach Phase-2a-Merge separat aufgeräumt werden). **Phase 2a ist mit S5-Findings-Gate effektiv abgeschlossen** — wartet auf Admin-Confirm für FF-Merge `feat/phase-2-cli` → `main`. Bookmarks für post-Merge / Backlog: (a) `tracelab.toml.example`-Doku-Update für `cfg.ADB.Enabled` mit DeviceSerial-Pflicht, (b) 200-OK-Discriminator-Body-Pattern als API-Convention-Section in `docs/ARCH.md`, (c) `run.go`-Stub-Refactor nach Phase-2a-Merge (entweder ganz raus oder klarer „not part of CLI scope"-Hinweis).

---

## AUFTRAG #018 — Tracelab P2b-S1 — Skeleton + ARCH-Vorab

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Erster Sub-Sprint Phase 2b. `cmd/mcp/main.go` Skeleton mit `mcp-go`, leere Tool-Stubs (sessions/tail/crashes/adb), `--version`-Flag analog hub+cli, Makefile-Target `mcp` mit Cross-Compile, ARCH-Vorab (ADR-006 voll + ADR-007 als Skelett).
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S1)
  - **Branch:** `feat/phase-2-mcp` von `main@9536b12`
- **DoD S1:**
  - `cmd/mcp/main.go` baut → `tracelab-mcp` Binary, cross-compiled Linux+Windows ohne CGO
  - mcp-go-Server-Init + 4 Tool-Stubs (Names placeholder, kein Behavior — S2 entscheidet Naming)
  - `--version`-Flag (LDFLAGS-Pattern wie hub+cli, gemeinsame `VERSION`-Variable im Makefile)
  - ARCH-Doku in `docs/ARCH.md` Phase-2b-Sektion:
    - **ADR-006** voll: Lib-Wahl `mcp-go` mit Considered/Rejected
    - **ADR-007 als Skelett**: 4 Tool-Sektionen Platzhalter, final-fill in S2; mit Vermerk **„S6-Risiko: `/crashes`-Hub-Endpoint fehlt → Hub-Schema-Change analog 2a-S5/ADR-004 nötig"** (Pre-Check belanna 2026-05-15 bestätigt)
  - `go vet ./...` clean, `go test -race ./...` repo-weit grün
  - mindestens 1 Smoke-Test für `cmd/mcp` (Build + `--version` Output)
- **Auto-Stop S1:** falls mcp-go-Lib substantielle Lücke (z.B. fehlende Streaming-Tool-Support für tail) → Eskalation an chakotay mit Alternativen.
- **Worker-Brief-Disziplin (PFLICHT):**
  - Trance-Bruch-Cross-Check-Scope EXPLIZIT breit: alle Dateien im Sprint-Touch-Scope inkl. Package-Doc + Const-Blocks + Smoke-Test-Doc (Lesson Cross-Check-Scope #013→#014→#015, 3× bestätigt)
  - Phase-1+2a-Code unangetastet außer Makefile (LDFLAGS-Erweiterung) und ggf. `docs/ARCH.md`
- **Status:** offen — Lead-Direktarbeit ADR-006 läuft, Worker-Spawn ballard folgt für Code
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — belanna: Auftrag empfangen, `/crashes`-Pre-Check Lead-Direktarbeit: Endpoint fehlt am Hub, Store-Tabelle vorhanden (`internal/store/sqlite.go:397` Kommentar „future /crashes API"). S6-Auto-Stop-Risiko in ADR-007 dokumentieren.

---

## AUFTRAG #017 — Tracelab Phase 2b — `tracelab-mcp` MCP-Server (Umbrella)

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Phase 2b der Phase-2-Roadmap — `tracelab-mcp` MCP-Server bauen. Wrappt Hub-API als MCP-Tools für Claude Code (sessions/tail/crashes/adb). Konsumiert dieselben HTTP+WS-Endpoints wie die CLI über shared `internal/client/`.
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Block 1/2/3 ✅ 2026-05-15, Admin-Approval „alle geplanten Phasen durcharbeiten" am 2026-05-15)
  - **Parent-Plan:** `~/.claude/plans/tracelab-phase-2-roadmap.md` (Phase-2-Roadmap, 3 Phasen 2a/2b/2c)
  - **Branch:** `feat/phase-2-mcp` von `main@9536b12` (bereits lokal angelegt)
  - **Sub-Sprint-Schnitt (Plan-Default, belanna kann anpassen):**
    - **S1** — Skeleton + ARCH-Vorab (cmd/mcp/main.go, mcp-go-Init, ADR-006 Lib-Wahl + ADR-007 Tool-Surface-Skelett)
    - **S2** — Tool-Schema-Surface-Cut (**Auto-Stop**: Naming, Tool-vs-Resource für tail, Auth-Strategie)
    - **S3** — `sessions`-Tool (list + get-by-id, reuse internal/client.ListSessions)
    - **S4** — `tail`-Tool/Resource (WS-Stream, reuse internal/client.Tail — Surface-Form folgt S2)
    - **S5** — `adb`-Tool (devices/start/stop, reuse internal/client ADB)
    - **S6** — `crashes`-Tool (**potenzieller Auto-Stop**: falls Hub-`/crashes`-Endpoint fehlt → ADR analog 2a-S5/ADR-004)
- **ARCH-Vorab (`docs/ARCH.md`, vor S1-Code):**
  - ADR-006 — Lib-Wahl `github.com/mark3labs/mcp-go` (Ballard-Stack-Default), Begründung + Considered/Rejected
  - ADR-007 — Tool-Surface-Liste (Skelett in S1, final in S2)
- **DoD Phase 2b:**
  - `cmd/mcp/main.go` baut → `tracelab-mcp` Binary, cross-compiled für Linux+Windows ohne CGO
  - 4 Tools (sessions/tail/crashes/adb) funktional gegen lokal-laufenden Hub
  - `internal/client/` wiederverwendet, kein Hub-API-Re-Implement
  - `go test -race ./...` repo-weit grün
  - `docs/ARCH.md` Phase-2b-Sektion ausgefüllt (ADR-006 + ADR-007 final)
  - tuvok release-qs Findings-Gate grün (keine Blocker, keine offenen Major) am Phasen-Ende
- **Auto-Continuation-Modus (5a-Default + Admin-Bekräftigung „alle geplanten Phasen durcharbeiten"):**
  - Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit
  - Per-Sub-Sprint-QS via tuvok → Findings-Gate über chakotay
  - Bei `freigabe` ohne Findings ≥ major → direkt nächsten Sub-Sprint routen (Auto-Chain)
  - Recovery-Patterns max 2 Versuche
  - FF-Merge nach `main` NACH Phasen-Done und Admin-Confirm
- **Auto-Stop-Trigger zusätzlich:**
  - **S2 Tool-Schema-Surface-Cut** (Naming, Tool-vs-Resource) — Admin-Confirm
  - **S6 Hub-Schema-Change** falls `/crashes`-Endpoint fehlt — Admin-Confirm vor Hub-Touch
  - **mcp-go Lib-Eignungs-Bruch** in S1 → Alternative-Lib-Diskussion
  - 🔴 Blocker-Findings, Architektur-Verzweigung, Token-Budget, Heartbeat-Fail (5a-Standard)
- **Status:** offen — S1 wird belanna eröffnet
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Umbrella + Plan-Briefing 5a (Admin-Approval „alle geplanten Phasen durcharbeiten") + Branch `feat/phase-2-mcp` von `main@9536b12` angelegt. Routing S1 an belanna folgt.

---

## AUFTRAG #016 — Tracelab Phase-2a-Closure + Backlog-Bookmarks

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna
- **Auftrag:** Phase-2a-Closure sequenziell:
  1. **FF-Merge** `feat/phase-2-cli` → `main` (Admin-Confirm 2026-05-15 via „Phase 2 closen"). Push `main` zum Remote.
  2. **Bookmarks abarbeiten** aus #015 + Header-Eintrag (a)-(c):
     - **(a)** `tracelab.toml.example`-Doku-Update für `cfg.ADB.Enabled=true` — `DeviceSerial` ist jetzt PFLICHT-Feld bei `Enabled=true` (Migration aus #015 S5).
     - **(b)** 200-OK-Discriminator-Body-Pattern als API-Convention-Section in `docs/ARCH.md` (started/already_running/stopped/not_running — scripted „ensure-running"/„ensure-stopped"-Pipelines branchen auf Body, nicht HTTP-Status). Vorlage für künftige Hub-Endpoints.
     - **(c)** `run.go`-Stub-Refactor: entweder ganz raus aus `cmd/cli/main.go` cobra-Tree, oder klarer „not part of Phase 2a CLI scope"-Hinweis im Short/Long-Description (ADR-005 = Option C konsequent durchziehen).
  3. **Branch-Cleanup** `feat/phase-2-cli` (lokal + remote `origin/feat/phase-2-cli` löschen) — Force-Op, **Admin-Confirm separat** über chakotay einholen **nach** Schritt 2.
- **Mandat:**
  - Belanna entscheidet, ob Bookmarks auf einem kleinen Doku-Branch oder direkt auf `main` landen (Doku-only, kein QS-Gate nötig — kein Code-Touch außer (c)).
  - Falls (c) Code-Touch beinhaltet (run-Stub-Refactor): kurzer Sanity-Check `go vet ./... && go test ./...`, kein Re-QS-Gate (cosmetic).
  - Commit-Schema:
    - FF-Merge-Commit-Message bleibt git-Default (`--ff-only`).
    - Bookmarks: einzelne Commits pro Bookmark oder ein Sammel-Commit `docs(arch): post-phase-2a backlog bookmarks (a,b,c)` — Belanna-Wahl.
- **DoD:**
  - `main` enthält Phase-2a-Code (S1-S5).
  - Drei Bookmarks (a)/(b)/(c) commited und gepusht.
  - WORKLOG-Sync-Commit (Modus G) auf `main` nach Bookmark-Abschluss: `chore(state): #016 phase-2a-closure done`.
  - Branch-Cleanup **noch nicht** ausgeführt — wartet auf Admin-Confirm (separat über chakotay).
- **Out of Scope:**
  - Phase 2b (MCP-Server) — separater Auftrag, nicht hier.
  - Bestehende Phase-1-Backlog-Items (M1-M12 längst durch Tail-Sprint).
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Auftrag angelegt, Mandat an belanna.
  - 2026-05-15T (Schritt 1 done) — belanna: FF-Merge `feat/phase-2-cli` → `main` per `--ff-only` (tip `bdc3a0c`), push origin/main. Vor-Merge wurde der WORKLOG-Eröffnungs-Commit `bdc3a0c` auf `origin/feat/phase-2-cli` gepusht, damit Branch-Tip vor Merge synchron mit remote ist (Standard-Hygiene für nachfolgendes Branch-Cleanup). main hatte zuvor noch den `e4eb434`-Phase-1-Tail-Tip — alle Phase-2a-Commits (S1-S5 + Findings-Gate-Commits + ADR-005-State-Commit + #016-Eröffnung) landen damit linear auf main.
  - 2026-05-15T (Schritt 2 done) — belanna: Bookmarks (a)+(b)+(c) abgearbeitet als zwei thematische Commits direkt auf `main` (kein separater Doku-Branch, da Doku-only bzw. cosmetic Code-Touch, kein QS-Gate nötig per Mandat).
    - **(a)+(b)** `docs: post-phase-2a backlog bookmarks (toml.example, ARCH API conventions)` — `tracelab.toml.example` [adb]-Section: `device_serial` als REQUIRED bei `enabled=true` dokumentiert (S5-Manager-Migration, kein implicit "first device" mehr); `docs/ARCH.md` neue Cross-Phase-Section "API Conventions" mit dem 200-OK + Discriminator-Body-Pattern (`started`/`already_running`/`stopped`/`not_running`), Begründung (scripted ensure-X-Pipelines branchen auf Body, nicht Status), Spiegelung Phase-1-`/ingest`-Pattern, Discriminator-Naming-Konvention (lowercase snake_case, past-tense für Transitions, present-tense für No-Ops), Client-Side-Mapping-Hinweis (Discriminator zu `nil` gefoldet, nicht surfaced — MCP-Future-Proof). Zusätzlich Bearer-Auth + Single-JSON-Helpers als reaffirmierte Konventionen aufgenommen.
    - **(c)** `refactor(cli): drop run sub-command per ADR-005=Option C` — `cmd/cli/run.go` komplett gelöscht (statt Stub-mit-Disclaimer-Variante); `cmd/cli/main.go` `AddCommand(newRunCmd())`-Aufruf entfernt + Package-Doc aktualisiert (Phase-2a final, kein S6, keine pending ADRs); `cmd/cli/main_test.go` `want`-Slice auf `[adb sessions tail]` reduziert + Kommentar mit ADR-005-Verweis. Begründung Lead: konsequente Umsetzung Option C — ein Stub, der "not implemented yet" druckt, widerspricht der CLI-Identität als purem Consumer im `--help`-Output und hinterlässt mentalen Müll. Bei realem Bedarf später eigener Sprint (Option B-Variante).
    - **Sanity-Check (Mandat-konform statt formales QS-Gate):** `go vet ./...` clean, `go test -count=1 ./...` repo-weit grün (10/10 Pakete ok). `cmd/cli` bleibt unter den bisherigen 0.027s. Test-Surface-Reduktion: 1 expected sub-command weniger im Smoke-Test, sonst keine Test-Adjustments nötig (run hatte nur den Smoke-Counter triggernden Eintrag, keinen eigenen Behavior-Test).
  - 2026-05-15T (Schritt 3 done) — chakotay: Admin-Confirm „ja" für Branch-Cleanup eingeholt. Pre-Delete-Verifikation `git merge-base --is-ancestor feat/phase-2-cli main` ⇒ OK (Branch vollständig in main enthalten, tip war `bdc3a0c`, kein Datenverlust-Risiko). `git branch -D feat/phase-2-cli` lokal + `git push origin --delete feat/phase-2-cli` remote. `git branch -a` bestätigt: nur noch `main` lokal + `origin/main` remote. Sync-Commit nach Modus G folgt mit diesem WORKLOG-Update.
- **Status:** ✅ erledigt. Phase 2a vollständig abgeschlossen, Repo auf `main` konsolidiert. Phase 2b (MCP-Server) wartet auf Admin-Eröffnung.

---

## AUFTRAG #015 — Tracelab P2a-S5 — `adb` Sub-Cmd (Hub-Schema-Change)

- **Timestamp:** 2026-05-14T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna
- **Auftrag:** S5 von Phase 2a — erstes Sub-Cmd mit Hub-Schema-Change. ADR-004 ist mit Admin-Confirm 2026-05-14 als **Option B (Hub-vermittelt)** entschieden — Hub bleibt Single-Source-of-Truth-Sammelpunkt. Drei neue HTTP-Endpoints am Hub + CLI thin client, der sie konsumiert. Erstes Mal in Phase 2 wird Phase-1-Code angefasst (Hub + `internal/adb/` + `internal/http/`).
  - **Repo/Branch:** `/home/kaik/Projekte/tracelab` · `feat/phase-2-cli`@ae06785 (post-#014-Abschluss)
  - **ARCH-Ref:** `docs/ARCH.md` ADR-004 (entschieden 2026-05-14, Option B begründet mit Sammelpunkt-Vision)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2-roadmap.md` (Phase 2a, Sub-Sprint S5)
- **Scope-Cuts (Minimum für S5):**
  - **Hub-Erweiterung** (neue Endpoints, additiv — keine bestehenden Endpoint-Schemas brechen):
    - `GET /adb/devices` — listet via `internal/adb/` die aktuellen ADB-Devices (Serial, State, Model wenn verfügbar)
    - `POST /adb/start` — startet ADB-Bridge-Recording für eine Device-Serial (Body: `{"serial": "...", "session": "<optional>"}`); Hub verdrahtet Logcat→Ingest-Pipeline
    - `POST /adb/stop` — stoppt aktive Bridge für eine Serial (Body: `{"serial": "..."}`)
    - Bearer-Auth wie bestehende Endpoints (kein neuer Auth-Pfad)
    - Errors: konsistent zu bestehenden Hub-Endpoints (Status-Codes + JSON-Body-Pattern)
  - **`internal/client/`-Erweiterung** (additiv): drei Methoden Mirror der neuen Endpoints — `ListADBDevices(ctx) ([]ADBDevice, error)`, `StartADBBridge(ctx, serial, sessionID string) error`, `StopADBBridge(ctx, serial string) error`. Bestehende HTTP+WS-Surface unangetastet.
  - **`cmd/cli/adb.go`** (Stub ersetzen):
    - `tracelab adb devices` — listet Devices als Tabelle/JSON nach `--format`
    - `tracelab adb start <serial> [--session=<id>]` — startet Bridge
    - `tracelab adb stop <serial>` — stoppt Bridge
    - Error-UX-Pfad: `translateClientError` wiederverwenden (DRITTER Konsument → jetzt ist Extraktion nach `cmd/cli/errors.go` legitim, Bookmark aus #014 fällig)
- **DoD S5:**
  - **Hub-side:**
    - 3 neue Endpoints registriert + funktional, Bearer-Auth, konsistente Error-Responses
    - Hub-Tests gegen `httptest.NewServer` für alle 3 Endpoints (Happy + Auth-Fehler + 4xx-Validierung)
    - `internal/adb/`-Integration: Bridge-Lifecycle via Start/Stop-Calls, idempotent (Stop auf nicht-laufende Bridge → kein Fehler oder klar definierter 404)
  - **Client-side:**
    - 3 neue Methoden mit Tests gegen `httptest.NewServer` (Happy + 401/403 → `ErrUnauthorized` via `errors.Is`)
    - DTOs (`ADBDevice` mindestens mit `Serial`, `State`, ggf. `Model`) Mirror der Wire-Types
  - **CLI-side:**
    - Alle 3 Sub-Sub-Cmds funktional (`devices`, `start`, `stop`), Help-Output sauber
    - `--format=table|json` für `devices` (analog `sessions`)
    - `start/stop` Status-Output knapp (z.B. „bridge started for emulator-5554")
    - `translateClientError`-Extraktion nach `cmd/cli/errors.go` (jetzt dritter Konsument — Bookmark aus #014 abgehakt), bestehende Tests in `sessions_test.go` + `tail_test.go` müssen weiterhin grün sein
  - **Repo-weit:**
    - `go vet ./...` + `go test -race ./...` repo-weit grün
    - `go mod tidy` Diff = 0 (keine neuen Top-Level-Deps)
    - Stubs `run` aus S1 mit Stage-Mapping unangetastet
    - **Trance-Bruch-Pre-Commit-Check** PFLICHT auf **ALLE** Dateien im Touch-Scope inkl. `cmd/hub/`, `internal/http/`, `internal/adb/` Package-Doc-Comments + Const-Blocks + Stubs (Lesson aus #014: Cross-Check-Scope explizit breit)
- **QS-Aufmerksamkeit (erhöht, kein Routine-Gate):** Erste Phase-1-Code-Mutation in Phase 2 — Hub-Schema-Change. Bei QS-Übergabe gezielt prüfen:
  - Endpoint-Konsistenz mit bestehenden Hub-Endpoints (Auth-Header-Behandlung, Error-Response-Shape, JSON-Body-Konventionen)
  - Bridge-Lifecycle: keine Race-Conditions zwischen Start/Stop/concurrent Calls auf dieselbe Serial, keine Goroutine-Leaks
  - Idempotenz: Start auf bereits laufende Bridge / Stop auf inaktive — definiertes Verhalten (kein Panic, klar dokumentiert)
  - `translateClientError`-Extraktion: bestehende Sub-Cmds (sessions, tail) müssen mit gleichem Error-Verhalten weiterlaufen — Regression-Check
  - Trance-Bruch-Cross-Check explizit auf NEUE Hub-Files erweitert
- **Auto-Continuation-Modus:** 5a-Default — Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit (Hub-Endpoints / Client-Methoden / CLI-Sub-Cmd / Tests / Error-Extraktion). Recovery max 2 Patterns.
- **Status:** QS grün — Findings-Gate freigabe (0 Findings), S5 abgeschlossen
- **Verlauf:**
  - 2026-05-14T (Eröffnung) — Chakotay routet S5 an belanna. Admin-Confirm zu ADR-004 Option B liegt vor, Sammelpunkt-Vision explizit bestätigt. ARCH.md ADR-004 als entschieden gekennzeichnet (Option B mit Begründung).
  - 2026-05-14T (Delegation) — belanna: Worker-Spawn an ballard (Klasse `feature` mit Cross-Stack-Touch). ballard kennt Phase-1-Hub-Code (Phase 1 Tail-Sprint) und S2-Client-Surface (eigene Arbeit), passt zum Sprint. Brief enthält explizit erweiterten Trance-Bruch-Cross-Check-Scope (ALLE Dateien im Touch-Scope inkl. NEUE Hub-Files Package-Doc + Const-Blocks + bestehende Stubs) als Anwendung der „Cross-Check-Scope-Bias"-Long-Term-Lesson aus #014. Bookmark `translateClientError`-Extraktion nach `cmd/cli/errors.go` ist Pflicht-Teil (3. Konsument).
  - 2026-05-14T (Worker-Return) — ballard: 4 Commits gepusht — `8575c27 feat(adb): P2a-S5 hub-mediated bridge endpoints (ADR-004)` + `67ce876 feat(adb): P2a-S5 client methods for /adb/* endpoints` + `3a459dd refactor(cli): extract translateClientError to errors.go` + `221630c feat(adb): P2a-S5 tracelab adb sub-command end-to-end`. **Hub-Schicht:** `internal/adb/manager.go` (BridgeManager mit per-Serial-Map, Mutex + done-Channel pro Bridge), `internal/http/adb.go` (3 Handler, additiv in `server.go` als optionale Config-Felder), `cmd/hub/main.go` migriert `cfg.ADB.Enabled`-Auto-Start auf `adbMgr.Start` (eine Lifecycle-Quelle für config-driven UND HTTP-getriebene Bridges). Slog-Marker umbenannt „adb bridge stopped" → „adb bridges stopped" (Manager-Close fan-out), main_test.go nachgezogen. **Idempotenz-Entscheid:** 200 OK + Discriminator-Body (`{"status":"started"|"already_running"|"stopped"|"not_running"}`) statt 409/404 — Begründung: scripted „ensure-running"/„ensure-stopped"-Pipelines branchen auf Body, nicht HTTP-Status; spiegelt Phase-1-`/ingest`-Pattern (immer 202). Race-frei via per-Serial-Mutex + done-Channel, getestet unter `-race`. **Client-Schicht:** `internal/client/adb.go` 3 Methoden + `ADBDevice`-DTO mit Wire-Type-Mirror, Sentinel-Reuse `ErrUnauthorized`/`ErrServerError`, Idempotenz-Cases zu `nil` gefoldet (Discriminator gelesen, nicht surfaced — MCP-Future-Proof). **CLI-Schicht:** `cmd/cli/adb.go` 3 Sub-Sub-Cmds (devices/start/stop) mit `--format=table|json`, `resolveADBClient`-Helper. **Refactor (eigener Commit):** `cmd/cli/errors.go` neu, hält `userErrorMsg`/`userError`/`asUserError`/`translateClientError`/`leafErrorMessage`; sessions.go + tail.go + adb.go rufen aus errors.go auf. sessions+tail-Tests unverändert grün (Regression-Check). **Test-Counts neu:** internal/adb +11 (Lifecycle/Idempotenz/Race-Hammer/Double-Close), internal/http +14 (happy/auth/bad-request/idempotency je Endpoint + conditional route registration), internal/client +16, cmd/cli +13 — Repo-Total +54. `go vet` clean, `go test -race ./...` repo-weit grün (10 Pakete), `go mod tidy` Diff = 0. **Trance-Bruch-Cross-Check** über 14 Files bestätigt (cmd/cli/main.go Stage 4→5, cmd/hub/main.go Phase 1/Stage 3→Phase 2a/Stage 5, Idempotenz-Doc-Strings exakt 1:1 mit Code, Slog-Marker-Konsistenz, cmd/cli/run.go byte-identisch zu HEAD). **Phase-1-Unangetastetheit verifiziert** außer den explizit erweiterten Pfaden (cmd/hub, internal/http, internal/adb): `internal/{crash,ingest,store,ws,config,cliconfig}` + bestehende client/cli-Files byte-identisch. Offen: `tracelab.toml.example`-Doc für `cfg.ADB.Enabled`-Migration (jetzt `DeviceSerial` PFLICHT bei `Enabled=true`) — kein S5-DoD-Item.
  - 2026-05-14T (QS-Trigger) — belanna: tuvok-Subagent (Klasse `standard`) für S5-QS. Erhöhte Aufmerksamkeit: Endpoint-Konsistenz zu Phase-1-Endpoints, Bridge-Lifecycle-Race-Freiheit + Idempotenz-Verhalten + Doc-Strings-1:1-mit-Code, `translateClientError`-Extraktion-Regression-Check (sessions+tail-Tests müssen unverändert grün sein), Hub-Migration-Sauberkeit (cfg.ADB.Enabled-Auto-Start → adbMgr.Start), Trance-Bruch-Scope-Vollständigkeit (zweite Anwendung der Lesson aus #014 — Promotion-Trigger bei drittem Beleg).
  - 2026-05-14T (QS-Return) — tuvok (qs-20260514-003): Status `freigabe` / Schweregrad `none`. **0 Findings.** DoD-Checkliste 1-23 alle erfüllt. Erhöhte-Aufmerksamkeit A-G alle grün: (A) Endpoint-Konsistenz 1:1 zu Phase-1 (writeJSON/decodeJSON/error-shape/bearer-auth wiederverwendet), (B) Race-Hammer 200 Ops/Serial unter `-race` grün, (C) Idempotenz-Doc-vs-Verhalten Drei-Punkt-Match (Code/Tests/Doc-Comments exakt `started`/`already_running`/`stopped`/`not_running`), (D) Migration `cfg.ADB.Enabled` → `adbMgr.Start` ein-Lifecycle-Quelle, kein paralleler State, (E) Empty-DeviceSerial-Reject vorhanden (kein dedizierter Test, kein Finding — 3 Zeilen Validierungslogik), (F) `translateClientError`-Refactor reine Bewegung (sessions_test.go + tail_test.go diff = 0, alle grün), (G) **Trance-Bruch-Cross-Check 0 Doku-Drift-Findings** — dritte Bestätigung der Cross-Check-Scope-Lesson aus #014, Promotion-Kandidat. Test-Counts: internal/adb +11, internal/http +14, internal/client +16, cmd/cli +13 = **+54 Repo-Total**. `go vet` clean, `go test -race ./...` repo-weit grün (10/10 Pakete ok), `go mod tidy` Diff = 0. **Tuvok-Empfehlung:** Freigabe ohne Findings. Strukturelle Anmerkungen (kein Finding): (1) Empty-DeviceSerial-Reject könnte optional Test bei nächster Aufräum-Welle bekommen; (2) 200-OK-Discriminator-Body-Pattern als API-Convention dokumentationswürdig in ARCH.md unter „API conventions" für Folge-Endpoints — Backlog, kein S5-Scope.
  - 2026-05-14T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf erste Phase-1-Code-Mutation in Phase 2 (1300+437+602 LOC + 54 Tests) ist beachtlich sauber — methodische Disziplin (Idempotenz-Pattern + Race-Test + Lifecycle-Migration auf eine Quelle + Refactor-Regression-null + Trance-Bruch-Cross-Check) hat substantiell getragen. Sub-Sprint S5 abnehmbar. **Promotion-Trigger erreicht** zur Cross-Check-Scope-Lesson (#013-Fix → #014-Worker → #015-ballard, drei Bestätigungen) — Aufnahme in Chakotay-Long-Term als agent-übergreifende Lesson. Bookmarks für Phase-2a-Closure: (1) `tracelab.toml.example`-Doc für `cfg.ADB.Enabled=true` mit DeviceSerial-Pflicht, (2) 200-OK-Discriminator-Body-Pattern als API-Convention in `docs/ARCH.md`. Beide Backlog, nicht blockierend.

---

## AUFTRAG #014 — Tracelab P2a-S4 — `tail` Sub-Cmd

- **Timestamp:** 2026-05-14T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna
- **Auftrag:** S4 (Read-Side-Abschluss) von Phase 2a. Schließt das CLI-Read-Pfad-Trio (S2 HTTP-Client + S3 `sessions`-Sub-Cmd + S4 `tail`-Sub-Cmd). WS-Loop in `internal/client/` (`Tail`-Methode aus ADR-003) implementieren + CLI-Consumer `tracelab tail --session=<id>` mit Color-by-Level und sauberer SIGINT-Beendigung. Erstmals echte WebSocket-Konsumtion, erstmals echte Nutzung von `[cli].tail_buffer` und `[cli].color` aus S3.
  - **Repo/Branch:** `/home/kaik/Projekte/tracelab` · `feat/phase-2-cli`@5ab891b (post-#013-Gate)
  - **ARCH-Ref:** `docs/ARCH.md` ADR-003 (Tail-Methode-Surface), Sub-Sprint S4 Spec
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2-roadmap.md` (Phase 2a, Sub-Sprint S4)
- **Vorab-Pflicht (Sub-Task A):** S3-Auflagen aus AUFTRAG #013 abarbeiten — direkter Fix durch ballard auf demselben Sprint-Branch, kein Re-QS:
  - **S3-001:** `cmd/cli/sessions.go` Z.166-169 `translateClientError` Generic-Connection-Path. Wahl Variante (b) aus tuvok-Empfehlung: geknappte Wrap-Form (z.B. via `errors.Unwrap`-Loop bis letztes Glied, oder `errors.Is(err, syscall.ECONNREFUSED)`-Map). Leak-Guard-Test um `dial tcp` und `Get "http` Substrings erweitern.
  - **S3-002:** `cmd/cli/sessions.go` Z.201-204 `writeSessionsJSON` Doc-Comment Ein-Zeilen-Fix — Aussage muss `omitempty`-Verhalten beschreiben (Feld weggelassen statt `null`-serialisiert).
  - Commit-Schema: `fix(cli): S3-001 + S3-002 doc-vs-behavior drift` oder zwei einzelne Commits — Implementer-Wahl.
- **Sub-Task B (Haupt-Implementation):**
  - `internal/client/`: `(*Client).Tail(ctx context.Context, sessionFilter string, onEvent func(Event)) error` aus ADR-003 implementieren. `gorilla/websocket` (im Repo schon Dep), Bearer-Auth-Header, Reconnect-Strategie NICHT in S4 (Backlog falls nötig).
  - `cmd/cli/tail.go`: `--session=<id>` (Pflicht-Flag), `--format=plain|json` (Default plain; JSON-Stream-NDJSON oder pro Event eine Zeile), Color-by-Level aus `[cli].color`-Setting (auto: TTY-Detect, always, never). Buffer-Größe aus `[cli].tail_buffer` (default 1024).
  - SIGINT-Clean-Close: WS-Close-Frame mit Status 1000, kein Stacktrace, Exit-Code 0 bei sauberer Beendigung, !=0 bei Auth/Connection-Fehler.
  - Error-UX-Disziplin (Lesson aus #013 S3-001): `translateClientError` aus S3 wiederverwenden / erweitern — KEIN paralleler Error-Pfad. Kommentare im Code GEGEN Verhalten cross-checken vor Commit (Trance-Bruch-Prävention).
- **DoD S4:**
  - **Vorab:** S3-Auflagen committet (vor S4-Code-Commits), `git log feat/phase-2-cli` zeigt Fix-Commit(s) vor S4-Implementation-Commits.
  - `tracelab tail --help` zeigt `--session`, `--format`, Color-Verhalten.
  - `tracelab tail --session=<id>` connectet WS gegen Hub `/ws/tail?session=<id>`, druckt Events live.
  - `--format=plain` color-formatted nach Level (ERROR rot, WARN gelb, INFO standard, DEBUG dim), Format-Choice respektiert `[cli].color`-Setting (auto/always/never).
  - `--format=json` druckt NDJSON (pro Event eine `json.Marshal`-Zeile).
  - SIGINT → sauberer Close, kein Stacktrace, Exit-Code 0.
  - Auth-Fehler (Token fehlt/falsch beim WS-Handshake) → klare Fehlermeldung + Exit-Code != 0.
  - Tests gegen `httptest.NewServer` mit `websocket.Upgrader` — Happy-Path (Events streamen), Error-Path (Auth-Fehler bei Handshake), SIGINT-Clean-Close (Context-Cancel als Stand-in).
  - `go vet ./...` + `go test -race ./...` repo-weit grün.
  - `go mod tidy` Diff = 0 (gorilla/websocket schon da, kein neuer Dep).
  - Phase-1-Pakete + Hub-`internal/ws/` unangetastet (Hub liefert die Endpoints schon).
  - S3-Surface in `internal/client/`: additive Erweiterung (Tail-Methode neu, kein Refactor bestehender HTTP-Methoden).
  - Sub-Cmd-Stubs `run` und `adb` aus S1 mit Stage-Mapping unangetastet.
- **QS-Aufmerksamkeit (erhöht, kein Routine-Gate):** WS-Loop ist erstmals echte Bidirektional-IO im Client + erstes Async-Heavy Sub-Cmd. Bei QS-Übergabe gezielt prüfen: SIGINT-Sauberkeit (kein leaking goroutine), WS-Close-Frame, Color-Detection-Disziplin, Error-Output-Pfad-Wiederverwendung (kein Code-Duplikat zu S3). **Kommentar-vs-Verhalten-Check** explizit als Pre-Commit-Disziplin im Worker-Brief (Trance-Bruch-Lesson aus #013).
- **Auto-Continuation-Modus:** 5a-Default — Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit (S3-Auflagen-Fix / Client-Tail-Methode / CLI-Sub-Cmd / Tests). Recovery max 2 Patterns.
- **Status:** erledigt — alle Auflagen gefixt, S4 schließbar
- **Verlauf:**
  - 2026-05-14T (Eröffnung) — Chakotay routet S4 (mit Vorab-Pflicht S3-Auflagen-Fix) an belanna. Admin-Approval: ja.
  - 2026-05-14T (Delegation) — belanna: Worker-Spawn an ballard (Klasse `feature` für Gesamt-Auftrag — Sub-A trivial im Commit-Trail vor Sub-B). Single-Spawn statt zwei separater Spawns: ballard kennt den Code aus S2+S3, Commit-Trail-Konsistenz besser, ein Spawn-Overhead gespart. Trance-Bruch-Lesson explizit im Brief: Kommentar-vs-Verhalten-Check als Pre-Commit-Disziplin in neuralgischen Funktionen.
  - 2026-05-14T (Worker-Return) — ballard: 3 Commits gepusht — Sub-A `df04efc fix(cli): S3-001 + S3-002 doc-vs-behavior drift` (strikt vor Sub-B), Sub-B `62dd595 feat(cli): P2a-S4 client Tail WebSocket method` + `0643ab7 feat(cli): P2a-S4 tail sub-command end-to-end`. **Sub-A:** S3-001 via Variante (i) — `errors.Unwrap`-Loop bis Leaf via Helper `leafErrorMessage` (stdlib-only, OS-agnostisch); S3-002 Doc-Comment erweitert um Konsumenten-Konsequenz. Leak-Guards `dial tcp` + `Get "http` ergänzt, +2 Tests. **Sub-B Implementer-Entscheidungen:** (1) `Event.SessionID` als Public-Field mit `omitempty` statt separater TailEvent-Type; (2) Watcher-Goroutine-Pattern statt gorilla `SetCloseHandler` (explicit owner, lesbarer Code-Pfad); (3) `tailDialError` mappet HTTP-Status auf bestehende Sentinels (HTTP+WS einheitliche `errors.Is`-Familie); (4) synchroner Send-in-onEvent + Printer-Goroutine mit Channel-Size aus `[cli].tail_buffer` (Hub-side drop-on-full greift, CLI droppt nie silent); (5) Color-Detection via `os.Stdout.Stat()+ModeCharDevice` stdlib-only, kein `golang.org/x/term`-Dep; (6) `translateClientError`-Wiederverwendung statt vorzeitige Extraktion (Bookmark: extract bei drittem Konsument S5/S6); (7) `signal.NotifyContext` statt manuelles `signal.Notify`+`os.Exit`; (8) **Endpoint-Korrektur:** Hub-Pfad ist `/tail` (verifiziert in `internal/http/server.go:78`), nicht `/ws/tail` wie im Brief vermutet. **Tests:** internal/client 17→24 (+7), cmd/cli 20→32 (+10 Tail + 2 Sub-A). `go vet` clean, `go test -race ./...` repo-weit grün, `go mod tidy` Diff = 0. Phase-1-Dirs + S2-HTTP-Surface unangetastet, Stubs `run`/`adb` unverändert. **Trance-Bruch-Pre-Commit-Check (Pflicht aus DoD) durchgeführt** — Kommentare in tail.go und Helpers gegen Code-Verhalten cross-gecheckt, keine Drift gefunden. Smoke: `tracelab tail` ohne `--session` → klare Error-Message + Exit 1. Offen: `translateClientError`-Extraktion bei 3. Konsument; Tail-Reconnect-Logik ADR-explizit deferred; Tail-Client-Heartbeat-Floor optional.
  - 2026-05-14T (QS-Trigger) — belanna: tuvok-Subagent (Klasse `standard`) für S4-QS. Erhöhte Aufmerksamkeit: SIGINT-Sauberkeit (no goroutine leak), WS-Close-Frame, Color-Detection-Disziplin, Error-Output-Pfad-Wiederverwendung kein Code-Duplikat zu S3, Kommentar-vs-Verhalten-Drift (Trance-Bruch-Lesson aus #013 — zweite Anwendung, Promotion-Trigger-Schwelle).
  - 2026-05-14T (Initialer Tuvok-Spawn API-Fail) — Transienter API-Socket-Drop beim ersten Tuvok-Spawn (`The socket connection was closed unexpectedly`). Kein Auftragsfehler, kein State-Schaden — Repo blieb clean auf `6ee4076`. Retry erfolgreich.
  - 2026-05-14T (QS-Return) — tuvok (qs-20260514-002): Status `auflagen` / Schweregrad `minor`. DoD 1-13 vollständig + Erhöhte-Aufmerksamkeit A/B/C/D/F grün (SIGINT-Sauberkeit unter `-race` zuverlässig, WS-Close-Frame mit Deadline + IsCloseError-Branch, Color-Detection stdlib-only deterministisch, `translateClientError`-Wiederverwendung ohne Duplikat, Backpressure-Pfad sauber). E (Kommentar-vs-Verhalten) produzierte 5 Minor — alle Doku-Drift: **S4-001** `cmd/cli/main.go:3-6` Package-Doc behauptet `tail remains stub` nach S4-Implementation; **S4-002** `cmd/cli/run.go:11-12` claimt „ships in S4 once ADR-005" widersprüchlich zu `main.go` (`run` ist S6); **S4-003** `cmd/cli/tail.go:20-27` Doc-Block + Const-Block-Mismatch (`formatJSON` lebt in `sessions.go`, hier deklariert nur `formatPlain` + `tailFormatTag`); **S4-004** `cmd/cli/tail.go:127-129` Kommentar verspricht `Config.Timeout` regle WS-Handshake, tatsächlich greift `dialer.HandshakeTimeout` in `internal/client/tail.go:59` (substantiellster Doku-Fix); **S4-005** `cmd/cli/tail.go:142-146` Kommentar „no silent local drop" relativiert sich nicht für den `ctx.Done()`-Bail-out-Pfad. Test-Counts verifiziert: internal/client 25→32 (+7), cmd/cli 17→28 (+11 — Worker-Self-Report leicht off-by-one bei Sub-A-Test-Counts, kein Finding). Tuvok-Empfehlung: Freigabe-mit-Auflagen, Sub-5-Min-Sammel-Fix durch belanna/ballard direkt, kein Re-QS. **Tuvok-Methodik-Lesson:** Trance-Bruch-Cross-Check muss Scope erweitern auf `alle Dateien im Sprint-Touch-Scope inkl. Package-Doc-Comments und Const-Block-Doku`, nicht nur Funktions-Kommentare in den Kerndateien. Lesson aus #013 hat halb gegriffen (Kerncode drift-frei, Periphere ausgespart) — Promotion-Trigger NICHT voll erreicht, neue Methodik-Sub-Lesson identifiziert.
  - 2026-05-14T (Findings-Gate) — chakotay: **Freigabe-mit-Auflagen.** Strategie/Proportion: 5 Minor auf 1250-Zeilen-Sprint mit WS-Komplexität proportional, alle Sub-5-Min-Fix, Code selbst hält Erhöhte-Aufmerksamkeit stand (nur Doku-Schicht betroffen). Keine Hochstufung. Tuvoks Methodik-Empfehlung (Cross-Check-Scope-Erweiterung) ist die strukturell wertvolle Lesson — wird in Belanna-Long-Term aufgenommen als Worker-Brief-Disziplin für künftige feature-Sprints. S4-001..-005 durch belanna/ballard direkt fixen auf `feat/phase-2-cli`, kein Re-QS. S5 (`adb`) ist NICHT direkt anschließbar — ADR-004 (Option A vs B mit Hub-Schema-Change) ist Auto-Stop-Trigger, braucht Admin-Entscheidung.
  - 2026-05-14T (Auflagen-Fix) — belanna: **Lead-Direktarbeit** (kein Worker-Spawn — reine Doku-Comment-Edits in 3 Files, klar im Lead-Direkt-Faustregel-Cluster). Live-Cross-Check während Edits hat einen eigenen Drift-Versuch sofort gefangen: meine erste Korrektur an `tailFormatTag` beschrieb es als „loop-source marker für translateClientError", `grep` zeigte aber `tailFormatTag` wird in `writeTailEvent` als Fallback-Source-Tag für Events ohne `Source`-Feld verwendet — Korrektur korrigiert, sauberer Cross-Check-Reflex hat live gegriffen. Alle 5 Fixes:
    - **S4-001** `main.go:3-6` — Stage 3 → Stage 4, stub-Liste auf `run`+`adb`, S5/S6-Targets benannt
    - **S4-002** `run.go:11-13` + Z.19 — Wechsel auf S6 + Stub-Fehlermeldung aktualisiert
    - **S4-003** `tail.go:20-23` — Doc-Block zugeschnitten auf `formatPlain` + `tailFormatTag` mit Verweis-Zeile auf `writeTailEvent`, `formatJSON`-Erwähnung mit Hinweis auf sessions.go als Quelle
    - **S4-004** `tail.go:127-131` — Kommentar verschoben auf „configures embedded http.Client, WS handshake uses own dialer-level HandshakeTimeout in internal/client/tail.go, kept for parity, no-op for tail"
    - **S4-005** `tail.go:142-149` — Halbsatz ergänzt „… except for the in-flight event during context cancellation, which is dropped by the select-on-Done bail-out"
    Sanity-Check: `go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün (10/10 Pakete ok). Commits: `fix(cli): S4-001..S4-005 doc-drift sweep` (Code-Files) + `docs(worklog): #014 auflagen-fix done` (WORKLOG-Sync). Push auf `feat/phase-2-cli`. **Meta-Lesson zur Aufnahme in Belanna-Long-Term:** Lesson-Internalisierung hat Scope-Bias; bei Anwendung einer Disziplin-Lesson aus Sprint N+1 Cross-Check-Scope explizit breit benennen (alle Dateien im Touch-Scope inkl. Package-Doc + Const-Block + peer-Stubs).

---

## AUFTRAG #013 — Tracelab P2a-S3 — `sessions` Sub-Cmd

- **Timestamp:** 2026-05-14T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna
- **Auftrag:** S3 (Erstes End-to-End-CLI) von Phase 2a. Verdrahtet das `sessions`-Sub-Cmd mit dem Client-Paket aus S2 (`internal/client/`) und etabliert dabei erstmals die Config-Discovery (5-Stufen-Reihenfolge aus ADR-002) plus die neue `[cli]`-Sektion in `tracelab.toml`. Erstes echtes CLI-UX — `--limit`, `--format=table|json`.
  - **Repo/Branch:** `/home/kaik/Projekte/tracelab` · `feat/phase-2-cli`@872dee6
  - **ARCH-Ref:** `docs/ARCH.md` ADR-001/-002/-003 (Admin-grün 2026-05-13)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2-roadmap.md` (Phase 2a, Sub-Sprint S3)
- **Scope-Cuts (Minimum für S3):**
  - Config-Discovery: `--config` → `$TRACELAB_CONFIG` → `./tracelab.toml` → `$XDG_CONFIG_HOME/tracelab/tracelab.toml` → `~/.config/tracelab/tracelab.toml` (Reihenfolge aus ADR-002)
  - Per-Invocation-Overrides: `--url`, `--token`, env `TRACELAB_URL` / `TRACELAB_TOKEN`
  - `[cli]`-Sektion (initial keys aus ADR-002): `default_format`, `color`, `tail_buffer` (letzteres erst in S4 echt benutzt — aber Parser muss es schon kennen, damit `tracelab.toml` mit `[cli]`-Block kein Fehler wirft)
  - `tracelab sessions --limit N --format table|json`:
    - Default-`limit` = sensibler Default (z.B. 20), Override via Flag
    - Default-Format aus `[cli].default_format`, Override via Flag
    - `table`: tab-aligned (ID, Label, Started, Ended/„running")
    - `json`: array von Session-DTOs, pretty-print
  - Exit-Codes: 0 grün, !=0 bei Fehler (Connection / Auth / Server)
  - Auth-Fehler-Output: knappe Fehlermeldung („unauthorized — check token") statt Go-Stacktrace
- **DoD S3:**
  - `tracelab sessions --help` zeigt beide Flags
  - `tracelab sessions` (ohne Flags) lädt Config in Discovery-Reihenfolge, kontaktiert Hub, druckt Tabelle
  - `--format=json` druckt validen JSON-Array
  - `--limit 5` limitiert auf 5 Einträge
  - `--url` / `--token` / env-Vars überschreiben Config-Werte (alle 3 Schichten getestet)
  - Auth-Fehler (Token fehlt / falsch) → klare Fehlermeldung + Exit-Code != 0, kein Stacktrace
  - Config-Discovery-Tests: pro Schicht mindestens 1 Test (`--config` explizit, env, cwd, XDG, default)
  - `go vet ./...` + `go test -race ./...` repo-weit grün
  - Keine neuen Deps außerhalb bestehender (cobra/chi/gorilla/stdlib) — TOML-Parser ist im Hub schon drin, dieselbe Lib nutzen
  - cmd/hub/ + Phase-1-Pakete unangetastet
  - `internal/client/` ggf. minimal ergänzt (z.B. ListSessions-Limit-Parameter), aber Surface aus S2 nicht refactored
- **QS-Aufmerksamkeit (erhöht, kein Routine-Gate):** S3 ist erster echter CLI-UX-Sprung — Config-Discovery, Format-Output, Error-Handling. Bei Gate-Bewertung Tabelle/JSON-Verhalten + Discovery-Reihenfolge + Error-Output prüfen, nicht nur Test-Counts.
- **Auto-Continuation-Modus:** 5a-Default — Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit (Config-Loader / Sub-Cmd / Tests), Recovery max 2 Patterns.
- **Status:** QS auflagen — Findings-Gate durch (Freigabe-mit-Auflagen), 2 Minor offen zum direkten Fix
- **Verlauf:**
  - 2026-05-14T (Eröffnung) — Chakotay routet S3 an belanna.
  - 2026-05-14T (Delegation) — belanna: Worker-Spawn an ballard (Klasse `feature`). Hub-TOML-Lib bestätigt: `BurntSushi/toml v1.6.0` (`internal/config/`) — wiederverwenden. Implementer-Autonomie für Layout-Detail (Erweiterung `internal/config/` mit `CLIConfig` vs. neues `internal/cliconfig/` für 5-Stufen-Discovery-Wrapper).
  - 2026-05-14T (Worker-Return) — ballard: 2 Commits (`f35149d` Config-Loader + `0f61bdc` Sub-Cmd), gepusht auf `feat/phase-2-cli`. **Hybrid-Layout:** `internal/config/` um `CLIConfig`-Sektion erweitert (Single-Source-of-Truth, Hub parst+ignoriert), `internal/cliconfig/` neu (5-Stufen-Discovery + Override-Resolver mit injizierbaren Hooks für hermetische Tests). 38 neue Tests (5 config + 20 cliconfig + 13 cmd/cli neu von 2 → 17), `go vet` clean, `go test -race ./...` repo-weit grün, `go mod tidy` Diff = 0 (kein neuer Dep). Phase-1-Pakete unangetastet. Auth-Fehler zentralisiert in `translateClientError()`, Stacktrace-Leak-Guard im Test (`!strings.Contains(msg, "goroutine")`). `bind = "0.0.0.0"` (Listener) → Connect-`127.0.0.1`-Umschreibung sauber. ListSessions(limit) war bereits in S2 final, C2 entfiel.
  - 2026-05-14T (QS-Trigger) — belanna: tuvok-Subagent (Klasse `standard`) für S3-QS. Erhöhte Aufmerksamkeit: Discovery-Reihenfolge ADR-002, Format-Output-UX, Error-Output-Disziplin.
  - 2026-05-14T (QS-Return) — tuvok (qs-20260514-001): Status `auflagen` / Schweregrad `minor`. Alle 13 DoD-Punkte + Erhöhte-Aufmerksamkeit A/B/D/E/F grün. 2 Minor-Findings (beide *Code-Kommentar widerspricht Code-Verhalten*, kein Code-Bug): **S3-001 (Grenzfall Minor/Major)** — `translateClientError` Z.166-168 Kommentar promises „no raw transport error", aber Z.169 `%v` leakt `dial tcp ...: connect: connection refused`-Detail; Korrekturvorschlag (b): geknappte Wrap-Form + Leak-Guard-Test um `dial tcp`/`Get "http`-Substrings erweitern. **S3-002** — `writeSessionsJSON` Z.201-204 Doc-Comment behauptet „EndedAt is serialised as null when still running", aber `omitempty` lässt das Feld weg (Test bestätigt key-absence). Empfehlung tuvok: Freigabe mit Korrekturen direkt durch belanna, kein Re-QS nötig. **Lesson (Tuvok):** nach 2× 0-Findings-Trance verlagert sich Risiko zu „Doku-Drift im Code selbst" — Kommentare gegen Verhalten cross-checken.
  - 2026-05-14T (Findings-Gate) — chakotay: **Freigabe-mit-Auflagen.** Strategie/Proportion: 2 Minor (beide Doku-Drift, Sub-5-Min-Fix) auf substantiellen Feature-Sprint (+1511 / 38 Tests) — proportional. S3-001-Hochstufung auf Major verworfen: Doku-Bug ≠ Code-Bug, kein Stacktrace im strikten Sinn (.go:/goroutine clean), kein Secret-Leak, Diagnose-Wert vorhanden. Tuvoks Empfehlung übernommen: Korrektur direkt durch belanna/ballard, kein Re-QS-Gate. S3-001 + S3-002 fixen vor S4-Start (oder im S4-Commit-Trail mitnehmen). Tuvoks Trance-Bruch-Lesson als Promotion-Kandidat in Bookmark (2× Wiederholung → Long-Term-Promotion).

---

- **Timestamp:** 2026-05-13T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** S2 (Client-Paket) von Phase 2a. Neues Paket `internal/client/` mit Hub-HTTP-API-Surface (ohne WebSocket-Tail — der folgt in S4 zusammen mit dem `tail`-Sub-Cmd). Wird in S3+ vom CLI konsumiert, in 2b vom MCP-Server. Eigene DTOs (Mirror der Hub-Response-Shapes), kein Import aus `internal/store/`.
  - **Repo/Branch:** `/home/kaik/Projekte/tracelab` · `feat/phase-2-cli`@d65115e
  - **ARCH-Ref:** `docs/ARCH.md` ADR-003 (Admin-grün)
- **Surface (Minimum für S2):**
  - `Config{ BaseURL, Token string; Timeout time.Duration }`, `New(cfg Config) (*Client, error)`
  - `(*Client).Health(ctx) error`
  - `(*Client).StartSession(ctx context.Context, label string) (id string, err error)`
  - `(*Client).EndSession(ctx, id string) error`
  - `(*Client).Ingest(ctx, id string, events []Event) (accepted int, err error)`
  - `(*Client).ListSessions(ctx, limit int) ([]Session, error)`
  - DTOs: `Event{Source, Level, Msg string; Meta map[string]any; TS int64 (optional)}`, `Session{ID, Label string; StartedAt, EndedAt int64}` — Mirror der Hub-Schemas; Felder mit `omitempty` wo angemessen.
- **DoD S2:**
  - `internal/client/` mit obiger Surface, Bearer-Auth-Header gesetzt, Default-Timeout 10s
  - Unit-Tests gegen `httptest.NewServer` — pro Endpoint mindestens 1 Happy-Path + 1 Error-Path (HTTP-Status ≠ 2xx → typisierter Fehler)
  - Auth-Fehler: 401/403 → erkennbarer Error-Typ (`var ErrUnauthorized`)
  - Context-Cancellation respektiert (ein Test mit cancellierter ctx)
  - `go vet ./...` + `go test -race ./...` repo-weit grün
  - Keine neuen Top-Level-Deps (nur stdlib + cobra/chi/gorilla — schon da)
  - cmd/cli/ wird NICHT angefasst (S3 verdrahtet später)
  - Phase-1-Code in cmd/hub/ + internal/{adb,crash,http,ingest,store,ws} unangetastet
- **Auto-Continuation-Modus:** 5a-Default — Lead-Autonomie für Standard-git-Ops.
- **Status:** QS grün — Findings-Gate freigegeben (chakotay)
- **Verlauf:**
  - 2026-05-13T (Eröffnung) — Auftrag an ballard via Worker-Subagent (Klasse `feature`)
  - 2026-05-13T (Worker-Return) — ballard: Commit `408465c` (11 Files, +1017). 6 Surface-Methoden + Tail-Placeholder. Sentinel-Errors (`ErrUnauthorized`/`ErrServerError`) + `HTTPError{Status,Endpoint,Body}` mit `Unwrap()`. Stdlib-only (kein neuer Dep). Wire-Types getrennt von Public-Types (`Event.Meta any` → intern `json.RawMessage`). `Session.EndedAt *int64` `omitempty` spiegelt Hub `sessionView`. `Health` ohne Auth, `New()` mit URL+Token+CHANGEME-Validation. `go vet` + `go test -race -count=1 ./...` repo-weit grün. cmd/cli/ + Phase-1-Pakete unangetastet.
  - 2026-05-13T (QS-Trigger) — tuvok-Subagent (Klasse `standard`) für S2-QS
  - 2026-05-13T (QS-Lauf) — tuvok: QS-Lauf gestartet
  - 2026-05-13T (QS-Return) — tuvok: 0 Blocker / 0 Major / 0 Minor — alle 12 DoD-Items grün, 24/24 Tests pass, `go vet` clean, `go mod tidy` clean, Wire-Format-Mirror zu Hub bestätigt. Bericht `qs-20260513-002`. Verdikt: **QS grün** (freigabe).
  - 2026-05-13T (Findings-Gate) — chakotay: **Freigabe**. Trivial-Gate (0 Findings, Surface ARCH-konform, Sentinel-Errors via `errors.Is` erreichbar, Wire-Mirror zu Hub belegt, stdlib-only). AUFTRAG #012 geschlossen.

---

## AUFTRAG #011 — Tracelab P2a-S1 — CLI Skeleton

- **Timestamp:** 2026-05-13T (Eröffnung)
- **Von:** belanna
- **An:** ballard
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** S1 (Skeleton) von Phase 2a. `cmd/cli/main.go` mit cobra-Root, leeren Sub-Cmd-Stubs für `run`/`tail`/`sessions`/`adb`. Makefile-Target `cli` mit Cross-Compile-Sub-Targets (Linux + Windows, CGO-frei). Keine Logik in den Subkommandos — nur `--help`-Output und passende Short-/Long-Descriptions. `--version`-Flag identisch zur Hub-Version (LDFLAGS-Pattern aus Makefile übernehmen).
  - **Repo/Branch:** `/home/kaik/Projekte/tracelab` · `feat/phase-2-cli`@94a05f7
  - **ARCH-Ref:** `docs/ARCH.md` ADR-001/-002/-003 (Admin-grün 2026-05-13)
- **DoD S1:**
  - `tracelab --help` zeigt alle 4 Sub-Cmds (run/tail/sessions/adb) mit Kurzbeschreibung
  - `tracelab --version` druckt Git-derived Version (gleiche Schiene wie Hub)
  - `make cli` baut Linux-Binary nach `dist/tracelab`
  - `make cli-windows` baut Windows-Binary nach `dist/tracelab.exe`, CGO-frei
  - `go vet ./...` und `go test -race ./...` repo-weit grün
  - Neue Deps: nur cobra-Familie (`spf13/cobra`); `go mod tidy` läuft sauber
- **Auto-Continuation-Modus:** 5a-Default — Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit, Recovery max 2 Patterns.
- **Status:** QS grün — Findings-Gate freigegeben (chakotay)
- **Verlauf:**
  - 2026-05-13T (Eröffnung) — Auftrag an ballard via Worker-Subagent (Klasse `feature`)
  - 2026-05-13T (Worker-Return) — ballard: Commit `f983a26` (9 Files, +214/-2). cobra v1.10.2 als einzige neue direct-Dep. cmd/cli/ mit Factory-Pattern + 4 Stubs (Exit 2 mit Stage-Mapping S3/S4/S5). Makefile `hub`/`cli`/`hub-windows`/`cli-windows`; `build` baut Linux-Hub+CLI. `go vet`/`go test -race` repo-weit grün. DoD-Smoke gegen `./dist/tracelab`: alle 4 Sub-Cmds gelistet, `--version` druckt git-derived, Windows-Binary PE32+ CGO-frei. Phase-1-Code nicht angefasst.
  - 2026-05-13T (QS-Trigger) — tuvok-Subagent (Klasse `standard`) für S1-QS
  - 2026-05-13T (QS-Lauf gestartet) — tuvok, qs-20260513-001
  - 2026-05-13T (QS: 0 Blocker / 0 Major / 0 Minor) — alle DoD-Items belegt (help/version/cli/cli-windows/vet/test/tidy/Phase-1-unangetastet/keine internal-Importe). Status `QS grün` — freigabe an chakotay.
  - 2026-05-13T (Findings-Gate) — chakotay: **Freigabe**. Trivial-Gate (0 Findings, sauberer Skeleton, Stage-Mapping konsistent zu ARCH, Phase-1-Isolation gewahrt). S1 abgeschlossen. AUFTRAG #011 geschlossen.

---

## AUFTRAG #010 — Tracelab Phase 2a — CLI

- **Timestamp:** 2026-05-13T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Phase 2a der Phase-2-Roadmap — `tracelab` CLI bauen. Subkommandos: `run` (Hub starten/managen), `tail` (Live-WS-Stream konsumieren mit optional `--session=<id>`), `sessions` (GET `/sessions` listen, lim/format-Optionen), `adb` (Devices listen, Logcat-Stream-Test via Hub-Bridge). Konsumiert Hub-HTTP+WS-API mit Bearer-Auth aus `tracelab.toml`.
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2-roadmap.md` (Block 1/2/3 ✅, Status `briefing-bereit`)
  - **Branch:** `feat/phase-2-cli` von `main`@e4eb434
  - **Sub-Sprint-Schnitt:** belanna entscheidet (analog Phase 1 — ARCH-Vorab erst, dann ggf. S1 Skeleton, S2 HTTP-Client, S3 WS-Tail-Subkommando, S4 Sessions-Subkommando, S5 ADB-Subkommando, S6 `run`-Subkommando). Final cut bleibt Lead.
  - **ARCH-Vorab (`docs/ARCH.md` ergänzen, vor S1):**
    - CLI-Framework: cobra-Default oder Alternative — belanna+ballard begründen
    - Config-Sharing: `tracelab.toml` Sektion `[cli]` mit Server-URL+Token, oder separate `.tracelab-cli.toml` mit Default-Discovery — Entscheidung dokumentieren
    - Shared HTTP-Client (Bearer-Auth, Retries) als neues `internal/client/` Paket
- **DoD Phase 2a:**
  - `cmd/cli/main.go` baut → `tracelab` Binary, cross-compiled für Linux+Windows ohne CGO
  - Alle vier Subkommandos funktional gegen lokal laufenden Hub
  - `go test -race ./...` repo-weit grün
  - `docs/ARCH.md` enthält CLI-Sektion (Framework-Wahl, Config-Strategie, Client-Paket)
  - README Endpoints-Tabelle um CLI-Konsumenten-Sicht ergänzt (kurz)
  - tuvok release-qs Findings-Gate grün (keine Blocker, keine offenen Major)
- **Auto-Continuation-Modus (5a-Default):** Lead-Autonomie für Standard-git-Ops, Commit pro logischer Einheit, Recovery-Patterns max 2 Versuche, FF-Merge nach `main` NACH Phasen-Done und Admin-Confirm.
- **Auto-Stop-Trigger zusätzlich:**
  - jedes Hub-Schema-Change (rückwirkt auf Phase-1-Code) → Admin-Confirm
  - Blocker-Findings aus Zwischen-QS
  - ARCH-Vorab-Entscheidung mit Tragweite jenseits CLI-Framework-Wahl (z.B. eigenes Daemon-Management-Konzept für `run`-Subkommando)
- **Status:** offen — pausiert nach S2-Gate, S3 (sessions-Sub-Cmd) pending bei belanna
- **Verlauf:**
  - 2026-05-13T (Eröffnung) — WORKLOG-Open + Plan-File-Status `phase-2a-laufend` + Sync-Commit
  - 2026-05-13T (Branch) — `feat/phase-2-cli` von `main`@db0370d angelegt
  - 2026-05-13T (ARCH-Vorab-Entwurf) — belanna: `docs/ARCH.md` angelegt mit ADR-001 (cobra), ADR-002 (shared toml + `[cli]`), ADR-003 (`internal/client/`). Sub-Sprint-Schnitt S1–S6 vorgeschlagen. **Auto-Stop:** ADR-004 (`adb`-Scope, hub-Endpoint vs. local) und ADR-005 (`run`-Semantik, drop/foreground/daemon) brauchen Admin-Entscheidung vor S5/S6. Empfehlungen liegen im File. S1–S4 sind nach ADR-001/-002/-003-Approval startklar.
  - 2026-05-13T (Admin-Approval) — Admin: ADR-001/-002/-003 akzeptiert, S1 (Skeleton) startklar. ADR-004 + ADR-005 bleiben pending bis vor S5/S6.
  - 2026-05-13T (S1 done) — AUFTRAG #011 geschlossen, Findings-Gate `d65115e`.
  - 2026-05-13T (S2 done) — AUFTRAG #012 geschlossen, Findings-Gate `872dee6`.
  - 2026-05-13T (Admin-Pause) — Pause vor S3. Branch `feat/phase-2-cli`@`872dee6` bleibt stehen. Wiedereinstieg: S3 (`sessions`-Sub-Cmd) — erstes End-to-End des Clients, erstmals Config-Discovery aus `tracelab.toml [cli]`-Sektion (ARCH-002), Format-Output table/json.

---

- **Timestamp:** 2026-05-10T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard
- **Auftrag:** Phase-1-Tail-Sprint — alle 12 Minor-Backlog-Items (M1–M12) aus den QS-Läufen qs-20260510-001 bis -005 abräumen. Details pro Item siehe Backlog-Tabelle oben in dieser Datei.
  - **Branch:** `chore/phase-1-tail` von `main` (commit `a5d1a10`)
  - **Paketierung** (sequenziell, Commit pro Paket):
    - **P1 Doku** — M1 (README-Endpoints-Tabelle: `/tail` + Bearer-Header), M8 (adb-Cross-Platform-Doc präzisieren oder Windows-Cancel implementieren — Doku-Variante bevorzugt für Tail)
    - **P2 ADB-Polish** — M7 (`no permissions` Parser-Fix), M9 (`SetBinary` aus Public-API raus, `internal_test.go` oder build-tag), M10 (`goleak` für LogcatStream-Tests)
    - **P3 Crash/Store** — M3 (Magic-Numbers extrahieren), M4 (Migration 0002 UNIQUE INDEX als Defense-in-Depth), M5 (Test-Theater umbenennen/Store-Mock), M6 (Default-Rust-Panic Coverage-Lücke: entweder Branch d ergänzen oder als bekannte Lücke `t.Skip`-Probe)
    - **P4 Test+Konsistenz** — M2 (`time.Sleep(50ms)` → `waitForSubs`-Helper), **M11 (Architektur-Entscheidung, Auto-Stop)**, M12 (`wantMsgs`-Soll-Array)
  - **M11 — Auto-Stop:** Publish/Insert-Reihenfolge zwischen `internal/adb/bridge.go:299-300` und `internal/http/handlers.go:159+165`. Belanna legt die zwei Wege vor (a: Reihenfolge angleichen, Latenz steigt | b: README präzisieren, „Live-Stream konsistent zum Subprocess, DB kann zurückfallen") — Admin entscheidet, dann M11 ausführen.
- **DoD:**
  - alle M1–M12 erledigt oder explizit als „bekannte Lücke" dokumentiert (M6, M8 ggf.)
  - `go test -race ./...` grün auf dem Tail-Branch
  - QS-Sammelgate durch tuvok (release-qs) am Ende
  - keine neuen Findings ohne Eintrag in vorgemerkter Backlog-Tabelle
- **Auto-Continuation-Modus (5a Default):** feature-branch, Commit pro Paket, Lead-Autonomie für Standard-git-Ops und Recovery-Patterns, kein FF-Merge ohne Admin-Confirm.
- **Auto-Stop-Trigger zusätzlich:** M11 (Architektur-Verzweigung), jede Blocker-Finding aus Zwischen-Check.
- **Status:** offen — bei belanna
- **Verlauf:**
  - 2026-05-10T (Eröffnung) — Branch `chore/phase-1-tail` von `main`@a5d1a10 angelegt, WORKLOG-Open committet (`72ba335`)
  - 2026-05-10T (P1 done) — M1 README + M8 adb-Doc — Lead-Direktarbeit, commit `dbb9040`, `go vet ./...` grün
  - 2026-05-10T (P2 start) — ADB-Polish (M7/M9/M10) an ballard via Subagent
  - 2026-05-10T (P2 done) — ballard liefert: M7 multi-word-state Parser + M9 SetBinary→export_test.go + M10 goleak. Commit `5ba6a6e`, `go test -race ./...` repo-weit grün, goleak ohne Leak-Report.
  - 2026-05-10T (P3 start) — Crash/Store (M3/M4/M5/M6) an ballard via Subagent
  - 2026-05-10T (P3 done) — ballard liefert: M3 Konstanten + M4 Migration 0002 unique-index + M5 Test-Rename (Variante a, gegen Interface-Extraktion: 7-Methoden-Surface zu groß) + M6 Skip-Probe (Variante d, gegen Heuristik-Erweiterung: würde K1 re-öffnen). Commit `5c0ce33`, `go test -race ./...` repo-weit grün. M5/M6-Follow-ups als Bookmarks dokumentiert.
  - 2026-05-10T (P4 Auto-Stop) — M11 Architektur-Entscheidung wartet auf Admin (Publish/Insert-Reihenfolge bridge.go ↔ handlers.go ↔ README)
  - 2026-05-10T (M11 Admin-Entscheidung) — Variante (b): README präzisieren, Code bleibt. Forensik-Vorteil zwei unabhängiger Audit-Kanäle ist gewollt.
  - 2026-05-10T (P4 done) — ballard liefert: M2 waitForSubs-Helper (beide Sleep-Stellen ersetzt) + M11 README-Section ADB Bridge neu formuliert (alte „exactly like /ingest"-Behauptung weg) + M12 wantMsgs-Soll-Array, kein Tautologie-Vergleich. Commit `2f65e50`, `go test -race ./...` grün, kein Mapping-Auffälligkeit.
  - 2026-05-10T (QS-Sammelgate) — tuvok release-qs an gesamtem Tail-Sprint via Subagent
  - 2026-05-10T (QS-Bericht) — qs-20260510-006: alle 12 Items grün, freigabe/none, Pattern-Wahlen 1:1 mit dokumentierten Begründungen, M11 Code-Diff = 0 Zeilen, repo-weit `go test -race ./...` grün
  - 2026-05-10T (Findings-Gate) — chakotay: **Freigabe**. Strategie/Proportion sauber (12 Items / 4 Pakete proportional, M11-Auto-Stop diszipliniert, Pattern-Begründungen tragend, keine Geschmacksfindings). Sprint #009 ist QS-grün und FF-merge-ready zu `main`.
  - 2026-05-11T (FF-Merge) — Tail-Sprint per `--ff-only` nach `main` gemerged (Tip `60adf48`), Branch `chore/phase-1-tail` lokal gelöscht. Phase-1-Tail erledigt, Backlog M1-M12 vollständig abgearbeitet. AUFTRAG #009 geschlossen.

---

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

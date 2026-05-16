---
type: worklog
projekt: tracelab
status: phase-2c-4-von-5-durch — S5 (Polish + Agents-Tab-Stub + Sammel-Gate) eröffnet 2026-05-16 (S4 freigegeben eef2a6e, S1+S2+S3 fc38460/cf850f8/220f445); AUFTRAG #029 aktiv
last-updated: 2026-05-16
qs-letzter-lauf: qs-20260516-004
phase-1-merge-commit: cee7a5d
phase-1-tail-merge-commit: 60adf48
phase-2a-merge-commit: bdc3a0c
phase-2b-merge-commit: cb249bd
aktiver-auftrag: "#029 — P2c-S5 Polish + Agents-Tab-Stub + Sammel-Gate"
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

## AUFTRAG #029 — Tracelab P2c-S5 — Polish + Agents-Tab-Stub + Sammel-Gate (Phase-2c-Closure-Vorbereitung)

- **Timestamp:** 2026-05-16T (Eröffnung, Auto-Continuation aus #028-Findings-Gate-Freigabe)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin („weiter" nach #028-Gate-Freigabe + S5-Confirm-Frage) → Chakotay (S4 freigegeben `eef2a6e`, S5-Eröffnung mit Sammel-Gate-Charakter — letzter Sub-Sprint vor Phase-2c-Closure) → belanna
- **Auftrag:** S5 von Phase 2c — **Polish + Agents-Tab-Stub + Sammel-Gate** (letzter Sub-Sprint, Phase-2c-Closure-Vorbereitung vor FF-Merge nach `main`). Drei Scope-Bausteine:
  1. **Layout-Polish:** Empty-States vereinheitlichen, Error-States für die 4 Tabs, mobile-readable (Responsive-Check), CSS-Konsolidierung wo doppelt.
  2. **Agents-Tab-Stub:** 4. Tab „Agents" rendern mit „Phase 2d — coming soon"-Hinweis (kein toter Link, kein 404). Schließt die DoD-Lücke aus Plan-Briefing 2c Block 1 (e).
  3. **Defense-in-Depth-Rückport nach S3-`SessionDetailHandler`** (Bookmark aus #028): Triple-Layer-Check (chi-Wildcard-strip + raw-Validation + ParseInt → 404) aus S4-`CrashDetailHandler` rückportieren — kleine Posture-Verbesserung, S3+S4 dann konsistent.
  4. **Sammel-Gate-QS** durch Tuvok über die gesamte Phase 2c (S1+S2+S3+S4+S5 zusammen) — Cross-Sub-Sprint-Konsistenz-Check, Wire-Stability, ADR-Closure-Status (ADR-011/012 Accepted), Cross-Compile-Final, Branch-Push-Ready-Verifikation vor FF-Merge.
  - **Umbrella-Ref:** Phase 2c (5 Sub-Sprints, S1+S2+S3+S4 ✅)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2c-dashboard.md` (Sub-Sprint S5)
  - **ADR-Stand:** ADR-011 + ADR-012 Accepted (beide aus S1/S2). Kein neuer ADR erwartet — S5 ist Polish + Closure-Vorbereitung.
  - **Branch:** `feat/phase-2-dashboard` @ `eef2a6e` (S1+S2+S3+S4 darauf, gepusht)
- **DoD S5:**
  - **Polish:**
    - Empty-States in allen 4 Tabs konsistent (Sessions / Live-Tail / Crashes / Agents) — gleiche Klassen, gleicher CTA-Stil, Session-Ref wo sinnvoll.
    - Error-States (z.B. Store-nil-Pfad → einheitliche „Daemon not configured"-Meldung; htmx-Fehler → einheitliche Fehler-Card).
    - Mobile-Readable: Tabellen sind scrollbar oder responsive collapsen, Schrift > 14px, Tap-Targets > 44px (Apple/Material-Konvention). Smoke-Test im Browser-DevTools auf mobile viewport.
    - CSS-Konsolidierung: `dashboard.css` durchgehen, doppelte Selektoren mergen, Top-Frames-Ellipsis-Pattern wo wiederverwendbar.
  - **Agents-Tab-Stub:**
    - `web/templates/tab_agents.gohtml` (neu oder erweitert) mit „Phase 2d — Agents Stack" Header + „coming soon"-Body + Beschreibung was Phase 2d bringen wird (Skill-Spawn-Tree, Token-Usage, Mailbox-Traffic — Bezug auf Plan-File Sektion „Phase 2d").
    - Tab-Navigation zeigt „Agents" als 4. Tab, nicht visuell deaktiviert — Klick führt zum Stub-Render. Stub ist gerendert, kein 404, kein leerer Container.
    - Keine Backend-Logik — pure statische Template-Render mit hardgekodetem Inhalt. Test: `TestAgentsTabRendersComingSoon` (1 Test reicht).
  - **Defense-in-Depth-Rückport nach S3:**
    - `internal/dashboard/sessions.go` `SessionDetailHandler` bekommt analog zu S4-`CrashDetailHandler` den Triple-Layer-Check: chi-Wildcard-strip + `raw==""||strings.Contains(raw,"/")`-Check + Session-ID-Validation (ggf. Format-Check, abhängig von Session-ID-Schema).
    - Bestehende S3-Tests bleiben grün, neuer Test `TestSessionDetailDefenseInDepth` (empty/embedded-slash/non-format) ergänzt.
  - **Sammel-Gate-QS:**
    - Tuvok release-qs über **Phase 2c gesamt** (S1+S2+S3+S4+S5 zusammen, nicht nur S5).
    - DoD aus Plan-Briefing 2c Block 1: (a) `/dashboard` rendert, (b) Live-Tail E2E, (c) Session-Browser-Tabelle+Detail, (d) Crash-Inspector dedup+Fingerprint+Top-Frames, (e) Agents-Tab gerendert, (f) `go test -race ./...` grün + Sammel-Gate ohne offene Major.
    - Cross-Sub-Sprint-Konsistenz: alle 4 Tabs verwenden gleiche Empty/Error-State-Pattern, gleiche htmx-Trigger-Konventionen, gleiche Pagination-Marker.
    - Wire-Stability: keine breaking changes an Hub-Endpoints durch die Phase (S1-S5 alle additiv).
    - Cross-Compile-Final: `make hub mcp mcp-windows hub-windows` produziert 4 saubere Binaries CGO-frei.
    - Branch `feat/phase-2-dashboard` push-ready, `git merge-base --is-ancestor main HEAD` = false (kein behind), Repo clean.
  - **Verifikation:** `go vet ./...` clean, `go test -race -count=1 ./...` über 12+ Pakete grün, `go mod tidy` Diff=0, `make hub mcp mcp-windows hub-windows` clean.
  - **E2E-Smoke** auf Daemon mit Test-Sessions + Test-Crashes: alle 4 Tabs lädt, Mobile-Viewport-Smoke (DevTools 375px) hat keine Layout-Breaks, Agents-Tab zeigt „coming soon"-Stub.
- **Mandat:**
  - Worker-Spawn ballard (Klasse `feature`, 3 Bausteine — Polish/Stub/Rückport, S3-/S4-Patterns als Vorlage).
  - Sammel-Gate-Spawn an Tuvok (Klasse `release-qs`, Cross-Phase-Konsistenz).
  - **Cross-Check-Scope (13. Anwendung):** bestehende Pakete `cmd/{hub,cli,mcp}`, `internal/{adb,client,config,cliconfig,crash,ingest,ws}` müssen 0 Lines bleiben — `internal/dashboard/` (Polish + Defense-in-Depth-Rückport S3, Agents-Tab-Handler trivial), `web/templates/` (tab_agents.gohtml neu, Empty-/Error-States polish), `web/static/dashboard.css` (Konsolidierung + responsive), `internal/http/server.go` (ggf. Agents-Route falls eigener Endpoint — sonst nur Wildcard-Tab-Dispatch).
- **Auto-Stop S5:** keine eingeplant. Falls Polish-Scope ausufert (mehr als Empty/Error/Mobile/CSS-Konsolidierung) — Stopp + Admin-Confirm. Falls Sammel-Gate Major-Findings über die Phase findet — Stopp + Findings-Gate, kein blindes Auto-Chain zur FF-Merge-Approval-Frage.
- **Nach S5-QS-grün (Sammel-Gate):** Bericht an Admin mit FF-Merge-Approval-Frage (`feat/phase-2-dashboard` → `main`, `--ff-only`, Branch-Cleanup nach Merge). **Kein Auto-FF-Merge** — Admin-Confirm-pflichtig laut Default-Modus 5a.
- **Status:** offen (eröffnet)
- **Verlauf:**
  - 2026-05-16T (Eröffnung) — chakotay: Admin „weiter" auf S5-Confirm-Frage. S5 = Sammel-Gate-Sub-Sprint mit 3 Polish-Bausteinen (Layout-Polish + Agents-Tab-Stub + Defense-in-Depth-Rückport) + Cross-Phase-Sammel-Gate-QS. Routet an belanna mit Mandat. Letzter Sub-Sprint vor Phase-2c-Closure mit FF-Merge nach `main`.

---

## AUFTRAG #028 — Tracelab P2c-S4 — Crash-Inspector (deduplicated Crashes pro Session + Fingerprint + Top-Frames + Stacktrace-View)

- **Timestamp:** 2026-05-16T (Eröffnung, neue Session)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin („weiter mit tracelab" + „ja" zu S4-Routing, 2026-05-16) → Chakotay (Session-Resume nach Pause, Plan-Briefing 2c bleibt aktiv inkl. Default-Modus + Auto-Continuation) → belanna
- **Auftrag:** S4 von Phase 2c — **Crash-Inspector-Tab end-to-end**. Deduplicated Crashes pro Session anzeigen mit Fingerprint, Count, Timestamp, Top-3-Frames. Klick → vollständige Stacktrace-Detail-View. Reuse 2b-S6-Endpoint `/crashes` (Store-Method `CrashesBySession` aus P2b-S6 ist Bestand).
  - **Umbrella-Ref:** Phase 2c (5 Sub-Sprints, S1+S2+S3 ✅)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2c-dashboard.md` (Sub-Sprint S4)
  - **ADR-Stand:** ADR-011 Accepted (Render-Stack), ADR-012 Accepted (SSE). Kein neuer ADR erwartet — Crash-Inspector ist Server-Render-Layer auf bestehendem `/crashes`-Endpoint.
  - **Branch:** `feat/phase-2-dashboard` @ `5763028` (aktiv, S1+S2+S3 darauf)
- **DoD S4:**
  - **Crash-Inspector-Tab** in `web/templates/tab_crashes.gohtml` voll implementiert: Tabelle pro Session mit Spalten (Fingerprint/Count/Erst-Timestamp/Letzt-Timestamp/Top-3-Frames-Preview), gruppiert pro Session.
  - **Session-Filter-Dropdown:** Auswahl einer Session zeigt nur deren Crashes (oder "Alle Sessions" für globale Sicht). htmx-Trigger=change auf `/dashboard/tab/crashes?session=…`.
  - **Detail-View:** Klick auf Crash-Row öffnet htmx-Swap mit vollständiger Stacktrace (alle Frames, nicht nur Top-3). Optional: Back-Button mit Query-Preserve analog S3.
  - **Bestehende Endpoints reuse:** `/crashes?session=…` für gefilterte Liste. Falls aggregierte Counts/Fingerprint-Gruppierung am Server gewünscht (statt im Template), additiv eine `CrashesGroupedByFingerprint`-Store-Methode — Lead-Empfehlung in Pre-Inspection notieren, autonome Entscheidung wenn additiv-trivial (analog S3-Pattern).
  - **Defensive Patterns:** Unknown-Session/Crash → 404 in Detail-View, leere Result-Sets → „No crashes found"-Empty-State, Crash ohne Stacktrace → graceful „—" statt blank.
  - **Tests:** `internal/dashboard/crashes_test.go` (neu, mindestens 5 Tests: Tab-Render-Empty, Tab-Render-with-Crashes-Mock, Session-Filter-Param-Forwarding, Fingerprint-Gruppierung, Detail-View-Render, Detail-404). Bestehende Tests grün.
  - **`go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün, `go mod tidy` Diff=0, `make hub mcp mcp-windows hub-windows` clean.
  - **E2E-Smoke:** Daemon mit Test-Sessions inkl. realistischen Crash-Patterns (Java-Stacktrace, das den Detector tatsächlich triggert — Folge-Hinweis aus S3-Bericht beachten), Browser `/dashboard` → Crashes-Tab → Liste sichtbar mit Fingerprint-Gruppierung, Session-Filter funktioniert, Klick öffnet vollen Stacktrace.
- **Mandat:**
  - Worker-Spawn ballard (Klasse `feature`, neuer Crash-Inspector-Tab mit htmx-Templating + Tests, S3-Pattern als Vorlage).
  - Falls Fingerprint-Gruppierung server-seitig (additive Store-Methode) sinnvoller als template-seitig: additive Server-Erweiterung, im Bericht begründen, Cross-Check-Scope-Touch dokumentieren.
  - **Cross-Check-Scope (12. Anwendung):** bestehende Pakete `cmd/{hub,cli,mcp}`, `internal/{adb,client,config,cliconfig,crash,ingest,ws}` müssen 0 Lines bleiben — `internal/store` ggf. additiv (neue Aggregat-Methode), `internal/http/server.go` ggf. Route-Add wenn neuer Endpoint, `internal/dashboard/` + `web/templates/tab_crashes.gohtml` + Tests erwartet.
  - **Crash-Detector-Pattern-Sample:** Realistisches Java-Stacktrace-Sample für E2E-Smoke verwenden (Hinweis aus S3-Bericht), damit Detector tatsächlich Crashes erkennt.
- **Auto-Stop S4:** keine eingeplant. Falls Fingerprint-Gruppierung nicht trivial-additiv: Stopp + Admin-Confirm via chakotay.
- **Nach S4-QS-grün:** Auto-Chain zu S5 (Polish + Agents-Tab-Stub + Sammel-Gate) — letzter Sub-Sprint vor Phase-2c-Closure. Token-Budget der Hauptsession bei S5-Eintritt prüfen (Konservatismus aus S3-Outro).
- **Status:** QS grün — wartet auf Chakotay-Findings-Gate (qs-20260516-004 freigabe / none / 0 Findings)
- **Verlauf:**
  - 2026-05-16T (Eröffnung, neue Session) — chakotay: Resume nach Session-Pause. S1-S3 alle QS-grün, S4 wartet seit Pause (`5763028`). Admin-„ja" auf S4-Routing-Frage. Plan-Briefing 2c bleibt aktiv (Default-Modus + Auto-Continuation), keine S4-spezifischen Auto-Stops im Plan. S4 routet an belanna mit Crash-Inspector-Tab + Fingerprint-Gruppierung + Stacktrace-Detail-View. `/crashes`-Endpoint + `CrashesBySession`-Store-Method aus P2b-S6 sind Bestand. Folge-Hinweis aus S3: realistisches Crash-Pattern für E2E-Smoke verwenden.
  - 2026-05-16T (Annahme + Delegation) — belanna: Auftrag angenommen, Klasse 🟡 feature (Crash-Inspector-Tab voll mit Fingerprint-Gruppierung + Stacktrace-Detail-View, ggf. additive Store-Methode für Aggregation). Branch aktiv (`feat/phase-2-dashboard` @ `4a1c817`). Worker-Spawn ballard analog S3 (Go-Render-Handler + Templates + Tests + ggf. trivial-additive Store-Method). Pre-Inspection-Mandat: ballard prüft ob bestehende `CrashesBySession` für Fingerprint-Gruppierung ausreicht (Template-seitig) oder ob `CrashesGroupedByFingerprint`-Variante trivial-additiv angelegt werden soll (Default-Hypothese: trivial-additiv mit GROUP BY fingerprint + MIN/MAX-Timestamp + COUNT, kein Schema-Change-Auto-Stop). Falls nicht trivial → Stopp via chakotay. Auto-Stop-Liste: Schema-Change-Auto-Stop bei nicht-trivialer Aggregat-Strategie. E2E-Smoke-Pflicht: realistisches Java-Stacktrace-Sample (S3-Hinweis), das den Detector tatsächlich triggert.
  - 2026-05-16T (erledigt — ballard): Pre-Inspection-Verdikt **trivial-additiv** — das `crashes`-Schema enforced bereits UNIQUE(session_id, fingerprint) via `idx_crashes_session_fp`, `UpsertCrash` bumpt `count`+`ts` für Duplikate, jede Zeile in der Tabelle ist also bereits der deduplizierte Repräsentant. Kein GROUP BY nötig, kein Schema-Change. Lieferung:
    - **Store (additiv):** `ListCrashes(ctx, ListCrashesOpts)` mit typed `CrashSort`-Enum (Whitelist: ts_desc/asc, count_desc, fingerprint_asc), `FilterSessionID`-Param, Limit+Offset. `CountCrashes(ctx, filterSessionID)` für Pagination-Total. `CrashByID(ctx, id)` mit sql.ErrNoRows-Sentinel für Detail-View-404. `ListSessionIDsWithCrashes(ctx)` für Session-Filter-Dropdown.
    - **Dashboard-Handler (additiv):** Neue `CrashesHandler` + `CrashDetailHandler` in `internal/dashboard/crashes.go`. `renderTabBody` dispatcht via neuem `crashes`-case auf den datengetriebenen Body, `renderEmptyCrashesBody` für nil-Store-Skeleton-Tests. Handler-Bundle um `crashesTpl` + `crashDetailTpl` erweitert.
    - **Templates:** `tab_crashes.gohtml` voll implementiert (Sort-Dropdown + Session-Filter-Dropdown via htmx hx-trigger=change, Tabelle mit Fingerprint/Session/Count/Last-seen/Top-3-Frames-Preview, klickbare Rows, Pagination, Empty-State). Neues `tab_crash_detail.gohtml` (Header mit Fingerprint/Session/Count/Last-seen, volle Stacktrace als <pre class="tl-stacktrace">, Back-Link mit RawQuery-Preserve, graceful Empty-Stacktrace).
    - **Router:** `internal/http/server.go` ergänzt um 2 explizite Routes (`/dashboard/tab/crashes` + `/dashboard/tab/crashes/{id}`) vor dem Wildcard, identisch zum S3-Pattern.
    - **CSS:** `web/static/dashboard.css` erweitert (Crashes-Controls, Top-Frames-Preview-Liste mit ellipsis, Crash-Detail-Header, Stacktrace-Block in dunklem Mono-Style — gleiche Designsprache wie S1+S2+S3).
    - **Tests:** `internal/dashboard/crashes_test.go` neu mit 8 Tests (Empty, Seeded-with-Dedup, Session-Filter-Forwarding, Sort-Whitelist mit Injection-Check, Detail-Render-with-Full-Stack, Detail-404-Unknown-und-Non-Numeric, BackURL-Preserve, Empty-Stacktrace-Graceful, Pagination). `handler_test.go` zwei `crashes`-Marker-Cases auf datengetriebene Tab-Panel-Klasse aktualisiert.
    - **Verifikation:** `go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün (12 Pakete), `go mod tidy` Diff=0, `make hub mcp mcp-windows hub-windows` clean (CGO-frei).
    - **E2E-Smoke:** Daemon `127.0.0.1:28765` mit 2 Sessions, ingest mit 2 Java-Stacks (NullPointer 2× + IllegalArgument 1× in S1) + 1 Kotlin-Stack (S2) — Detector triggerte für alle 3, Dedup wirkte (NullPointer count=2), `/dashboard/tab/crashes` zeigt 3 Fingerprints, Session-Filter S1 versteckt korrekt das Kotlin-Crash aus S2, Detail-View rendert volle Stacktrace inkl. deep-frame App.java:7, Back-Link mit `sort=count_desc&page=1` preserved, unknown id `999` → HTTP 404.
    - **Cross-Check-Scope:** `git diff cf850f8..HEAD` für `cmd/{cli,mcp}` + `internal/{adb,client,config,cliconfig,crash,ingest,ws}` = 0 Bytes; `cmd/hub` 0 Bytes addiert (Signatur seit S3 erweitert).
    - **Status:** erledigt — wartet auf Belanna-QS-Trigger (Tuvok-Findings-Gate analog #026/#027).
  - 2026-05-16T (QS-Lauf) — tuvok, qs-20260516-004: release-qs-Spawn durch Belanna gestartet — Cross-Check-Scope 12. Anwendung, Pre-Inspection-Verdikt trivial-additiv, 2-Commit-Split `77f8520`+`bb5a42b`.
  - 2026-05-16T (QS-Lauf abgeschlossen) — tuvok, qs-20260516-004: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 8 DoD-Punkte live-verifiziert: Pre-Inspection-Verdikt „trivial-additiv" am Code bestätigt — `idx_crashes_session_fp` UNIQUE-Index aus Migration `0002_crashes_unique_index.up.sql` + `UpsertCrash`-Tx-Logic (`SELECT id … LIMIT 1` → bump count/refresh ts || INSERT count=1) machen jede Row zum dedup-Repräsentanten, kein GROUP BY nötig; Store-Erweiterung typed `CrashSort`-Enum + switch-mit-default-Fallback in `ListCrashes` (SQL-Injection mechanisch unmöglich) + `sql.ErrNoRows`-Sentinel bei `CrashByID` + `ListSessionIDsWithCrashes` für Filter-Dropdown; Handler-Whitelist-Loop über `allowedCrashSortKeys` + Length-Cap 128 char auf `session=` + Page-Clamp bei past-the-end (`if total>0 && offset>=total → maxPage`) + nil-Store-Skeleton-Pfad via `renderEmptyCrashesBody` + Detail-Handler-Defence-in-Depth (raw==""||embedded-slash → 404, ParseInt-Fail → 404); Templates htmx-Trigger-Disziplin korrekt (`change from:select[name=sort], change from:select[name=session]`, klickbare Rows `hx-get=/dashboard/tab/crashes/{id}`, Empty-State mit Session-Ref, Pagination mit muted-Span bei last page, Detail `<pre class="tl-stacktrace">` mit graceful `eq (len .StackLines) 0`-Fallback); 2 neue Routes explizit VOR Wildcard `/dashboard/tab/*` registriert (`internal/http/server.go:136-138` — identisch S3-Pattern); CSS additiv (Crashes-Controls, Top-Frames-Ellipsis, Crash-Detail-Meta-Grid, Dark-Bg-Mono-Stacktrace); 8 Dashboard-Crashes-Tests + 5 Store-Crashes-Tests grün — insbesondere `TestCrashesTabSortParamWhitelist/sql_inject_DROP` verifiziert Injection-Leak-Defence (Raw-String erscheint NICHT in Body), `TestCrashDetailUnknownID404` mit 99999+non-numeric, `TestCrashDetailBackURLPreservesQuery` für sort+page+session Round-Trip, `TestCrashDetailEmptyStacktraceGraceful` für leere Stacktrace; `go vet` clean, `go test -race -count=1 ./...` über 12 Pakete grün, `go mod tidy` Diff=0 (stdlib only), `make hub mcp mcp-windows hub-windows` clean — 4 Binaries gebaut (ELF + PE32+, CGO-frei verifiziert). **E2E-Smoke live re-verifiziert** auf `127.0.0.1:28766`: 2 Sessions, realistischer Java-NullPointerException-Stacktrace 2× in S1 (Detector triggerte, Dedup `count=2`, Fingerprint `b0438603eb833e06`) + IllegalArgumentException 1× in S2 (Fingerprint `42bed93aac74134f`), `/dashboard/tab/crashes` rendert beide Rows + Top-Frames-Preview (Foo.java:42 + Other.java:11), Session-Filter `?session=S1` versteckt korrekt S2-Crash, Detail-View id=1 zeigt deep frame `App.java:7` (jenseits Top-3), Back-Link `href="/dashboard/tab/crashes?sort=count_desc&amp;page=1"` (RawQuery preserved + HTML-Escape), id=999 → HTTP 404, id=`nope` → HTTP 404, Sort-Injection-URL-Encoded `ts_desc%27%20OR%201%3D1` rendert Tab weiter ohne Injection-Leak im Body. **Cross-Check-Scope 12. erfolgreiche Anwendung** — `git diff 220f445..HEAD -- cmd/{cli,mcp,hub} internal/{adb,client,config,cliconfig,crash,ingest,ws}` = 0 Bytes; Touches strikt auf 7 erwartete Pfade beschränkt (internal/store, internal/dashboard ×3, internal/http/server.go +4 Lines, web/templates ×2, web/static, docs/WORKLOG). Empfehlung Freigabe + Methodik-Beobachtung (außerhalb Findings): **Defense-in-Depth-Layering bei CrashDetailHandler** (Path-Parse vor Store-Lookup mit empty/embedded-slash-Check) ist eine kleine Verbesserung gegenüber S3-SessionDetail, gleiche Posture könnte rückportiert werden — nicht S4-Scope, Folge-Hinweis. Status `QS grün` — wartet auf Chakotay-Findings-Gate.

---

## AUFTRAG #027 — Tracelab P2c-S3 — Session-Browser (Tabelle + Sort/Filter/Pagination + Detail-View)

- **Timestamp:** 2026-05-16T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin (Plan-Briefing 2c „y" 2026-05-16, Default-Modus aktiv inkl. Auto-Continuation) → Chakotay (#026 Findings-Gate freigabe via `cf850f8`, Auto-Chain ohne neue Admin-Approval — Plan-Briefing sieht keine S3-Auto-Stops vor) → belanna
- **Auftrag:** S3 von Phase 2c — **Session-Browser-Tab end-to-end**. Tabelle aller Sessions mit Sort/Filter/Pagination, Klick → Detail-View mit allen Events der Session. Reuse bestehender Endpoints: `/sessions` (Liste) + `/events?session=…` (Detail). Falls Detail-View einen neuen Server-Endpoint braucht (z.B. `/sessions/{id}` mit aggregierten Counts), additiv in `internal/http` ergänzen + Hub-Schema-Stable-Pattern wie 2a/2b.
  - **Umbrella-Ref:** Phase 2c (5 Sub-Sprints, S1+S2 ✅)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2c-dashboard.md` (Sub-Sprint S3)
  - **ADR-Stand:** ADR-011 Accepted (Render-Stack + Auth permanent-Loopback), ADR-012 Accepted (SSE Live-Tail). Kein neuer ADR erwartet (Session-Browser ist pure Server-Render + htmx-Boost ohne neue Mechanik).
  - **Branch:** `feat/phase-2-dashboard` @ `cf850f8` (aktiv, S1+S2 darauf)
- **DoD S3:**
  - **Session-Browser-Tab** in `web/templates/tab_sessions.gohtml` voll implementiert: Tabelle mit Spalten (Session-ID/Start/End/Event-Count/Crash-Count), Klick auf Row öffnet Detail-View (htmx-Swap auf Container).
  - **Sort:** by start_at desc (default), by session_id, by event-count. htmx-Get auf `/dashboard/tab/sessions?sort=…` mit hx-swap=outerHTML auf den Tab-Inhalt.
  - **Filter:** Free-Text-Search über session_id-Substring (htmx-Trigger=keyup-delayed-300ms), Datums-Filter (start_at zwischen X und Y).
  - **Pagination:** Limit/Offset über htmx-Boost mit page-Param. Default-Limit 50.
  - **Detail-View:** Tabelle aller Events der Session (Spalten: timestamp/level/source/msg), htmx-Boost mit Back-Button.
  - **Bestehende Endpoints reuse:** `/sessions` für Liste, `/events?session=…` für Detail. Falls aggregierte Counts gewünscht (Event-Count + Crash-Count pro Session in der Liste), eventuell additiv ein `/sessions?with_counts=1`-Query-Param oder ein neuer `/sessions/{id}/stats`-Endpoint — Lead-Empfehlung in Pre-Inspection notieren, autonome Entscheidung wenn additiv-trivial.
  - **Defensive Patterns:** Unknown-Session-ID → 404 in Detail-View, leere Result-Sets → „No sessions found"-Empty-State im Tab.
  - **Tests:** `internal/dashboard/sessions_test.go` (neu, mindestens 5 Tests: Tab-Render-Empty, Tab-Render-with-Sessions-Mock, Sort-Param-Forwarding, Filter-Substring-Match, Detail-View-Render). Bestehende Tests grün.
  - **`go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün, `go mod tidy` Diff=0, `make hub mcp mcp-windows hub-windows` clean.
  - **E2E-Smoke:** Daemon mit ein paar Test-Sessions, Browser `/dashboard` → Sessions-Tab → Liste sichtbar, Sort/Filter/Pagination funktionieren, Klick auf Session öffnet Detail-View mit Events.
- **Mandat:**
  - Worker-Spawn ballard (Klasse `feature`, neuer Session-Browser-Tab mit htmx-Templating + Tests).
  - Falls aggregierte Counts in `/sessions` nötig: additive Server-Erweiterung, im Bericht begründen, Cross-Check-Scope-Touch dokumentieren.
  - **Cross-Check-Scope (11. Anwendung):** bestehende Pakete `cmd/{hub,cli,mcp}`, `internal/{adb,client,config,cliconfig,crash,ingest,ws}` müssen 0 Lines bleiben — `internal/store` ggf. additiv (neue Query-Methode falls Aggregat-Counts), `internal/http/server.go` ggf. Route-Add wenn neuer Endpoint, `internal/dashboard/` + `web/templates/tab_sessions.gohtml` + Tests erwartet.
- **Auto-Stop S3:** keine eingeplant. Falls Schema-Change am Hub für aggregierte Counts nicht trivial-additiv: Stopp + Admin-Confirm via chakotay.
- **Nach S3-QS-grün:** Auto-Chain zu S4 (Crash-Inspector) — keine weiteren Auto-Stops bis S5 (Sammel-Gate).
- **Status:** offen (eröffnet)
- **Verlauf:**
  - 2026-05-16T (Eröffnung) — chakotay: Auto-Chain nach #026 Freigabe (`cf850f8`). Default-Modus aktiv, keine S3-Auto-Stops im Plan-Briefing 2c. S3 routet an belanna mit Session-Browser-Tab + Sort/Filter/Pagination + Detail-View. Bestehende Endpoints werden reused (`/sessions` + `/events`), neue aggregierte Counts nur falls additiv-trivial. ADR-Bedarf nicht erwartet — pure Render-Layer-Sprint.
  - 2026-05-16T (Annahme + Delegation) — belanna: Auftrag angenommen, Klasse 🟡 feature (Session-Browser-Tab voll mit Server-side-Templating + ggf. additive Store-Query für Aggregat-Counts). Branch bereits aktiv (`feat/phase-2-dashboard` @ `b466daa`). Worker-Spawn ballard — klassische feature-Domain (Go-Render-Handler + Templates + Tests + ggf. trivial-additive Store-Method). Pre-Inspection-Mandat: ballard prüft zuerst ob `internal/store.ListSessions` Counts schon liefert oder ob `WithCounts`-Variante trivial-additiv anzulegen ist (Default-Hypothese: trivial-additiv mit COUNT-Subquery, kein Schema-Change-Auto-Stop). Falls nicht trivial → Stopp via chakotay. Auto-Stop-Liste: nur Schema-Change-Auto-Stop bei nicht-trivialer Aggregat-Strategie. Methodik-Bookmark beachtet: ADR-Konventionen-Promotion (3× stabil) bleibt Folge-Auftrag bei Phase-2c-Sammel-Gate-Outro, nicht in S3-Scope.
  - 2026-05-16T (erledigt — ballard): Pre-Inspection-Verdikt **trivial-additiv** — `ListSessions` lieferte nur Metadaten, COUNT-Subquery via existierende Indizes (`idx_events_session_ts`, `idx_crashes_session_ts`) ist trivial, kein neuer ADR, kein Schema-Change. Lieferung:
    - **Store (additiv):** `ListSessionsWithCounts(ctx, ListSessionsOpts)` mit typed `SessionSort`-Enum (Whitelist: started_at_desc/asc, session_id, event_count_desc), `FilterIDSubstring` via parameterisiertes LIKE, Limit+Offset. `CountSessions(ctx, filter)` für Pagination-Total. `SessionByID(ctx, id)` mit sql.ErrNoRows-Sentinel für Detail-View-404.
    - **Dashboard-Handler (additiv):** `NewHandler(version, log, store)` (Signatur erweitert um Store-Param, nil bleibt für Skeleton-Tests akzeptiert). Neue `SessionsHandler` + `SessionDetailHandler` in `internal/dashboard/sessions.go`. `LayoutHandler`/`TabHandler` dispatchen via `renderTabBody` zum datengetriebenen Sessions-Body.
    - **Templates:** `tab_sessions.gohtml` voll implementiert (Sort-Dropdown via htmx hx-trigger=change, Filter mit keyup delay 300ms, Tabelle mit klickbaren Rows, Pagination Prev/Next + Page-Indicator, Empty-State). Neues `tab_session_detail.gohtml` (Header + Events-Tabelle Limit 200 + Crashes + Back-Link mit RawQuery-Preserve).
    - **Router:** `internal/http/server.go` ergänzt um 2 explizite Routes (`/dashboard/tab/sessions` + `/dashboard/tab/sessions/{id}`) vor dem Wildcard.
    - **CSS:** `web/static/dashboard.css` erweitert (Tabellen, Pagination, Detail-Header — gleiche Designsprache wie S1+S2).
    - **Tests:** `internal/store/sqlite_test.go` +5 Tests (HappyPath/Sort/Filter+Pagination/Count/ByID). `internal/dashboard/sessions_test.go` neu, 9 Tests (Empty, Seeded, Sort-Whitelist inkl. Injection-Defence, Filter-Substring, Pagination 3-Seiten + Last-Page-muted-Next, Detail-Render, Detail-404, BackURL-Query-Preserve, Page-Clamp). Bestehende Tests grün: handler_test/stream_test/dashboard_wireup_test angepasst (NewHandler-3-arg-Signatur, Body-Marker auf `tl-tab-panel tl-sessions`).
    - **Verifikation:** `go vet ./...` clean; `go test -race -count=1 ./...` repo-weit grün; `go mod tidy` Diff=0; `make hub mcp mcp-windows hub-windows` clean (CGO-frei); E2E-Smoke gegen Daemon `127.0.0.1:28765` mit 3 Test-Sessions (S1=2 Events, S2=5 Events, S3=21 Events + Crash-Pattern) ergab korrekte Sort-/Filter-/Pagination-/Detail-/404-Outputs.
    - Status `erledigt`, Auto-QS-Trigger via belanna folgt.
  - 2026-05-16T (Code-Commits) — belanna: 2-Commit-Split analog P2c-S2-Pattern. `f9620c2 feat(store): P2c-S3 ListSessionsWithCounts + CountSessions + SessionByID additive`. `6276e27 feat(dashboard): P2c-S3 session-browser tab E2E`. Push `feat/phase-2-dashboard` zu origin clean. Status `in QS` — Tuvok-Spawn standard folgt. **Crash-Count-Beobachtung aus E2E-Smoke** (konstruierter Java-Stacktrace triggerte den Detector nicht): nicht S3-Scope (Detail-Counts laufen über bestehende `CrashesBySession` aus P2b-S6, Code funktional korrekt) — Hinweis an Tuvok im QS-Spawn für realistischeres Pattern-Sample.
  - 2026-05-16T (QS-Lauf) — tuvok, qs-20260516-003: QS-Lauf gestartet.
  - 2026-05-16T (QS-Lauf abgeschlossen) — tuvok, qs-20260516-003: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 7 DoD-Punkte live-verifiziert: Store-Erweiterung trivial-additiv (typed `SessionSort`-Enum + parameterisiertes LIKE + `sql.ErrNoRows`-Sentinel bei `SessionByID`); Handler Whitelist-Validation für sort (Loop über `allowedSortKeys`) + Filter-Length-Cap 128 char + Page-Clamp bei past-the-end (`if total>0 && offset>=total → maxPage`) + Back-URL preserve `r.URL.RawQuery`; Templates htmx-Trigger-Disziplin korrekt (`change from:select[name=sort]`, `keyup changed delay:300ms from:input[name=filter]`, klickbare Rows `hx-get=/dashboard/tab/sessions/{id}`, Empty-State, Pagination-Indicator + muted-Span bei last page); 2 neue Routes explizit VOR Wildcard `/dashboard/tab/*` registriert (`internal/http/server.go:127-130`); 9 Dashboard-Tests + 5 Store-Tests alle grün — insbesondere `TestSessionsTabSortParamWhitelist/sql_inject_DROP` verifiziert Injection-Defence (Raw-String darf NICHT im Body landen), `TestSessionsTabClampsPageBeyondLast` verifiziert past-the-end-Snap-Back, `TestSessionDetailBackURLPreservesQuery` verifiziert sort+page+filter Round-Trip im Back-Link, `TestSessionByID` verifiziert `errors.Is(err, sql.ErrNoRows)`-Sentinel; `NewHandler(version, log, store)` 3-Arg-Signatur über alle 4 Caller migriert (handler_test/stream_test/dashboard_wireup_test/dashboard_stream_wireup_test), nil-Store-Pfad via `renderEmptySessionsBody`; `go vet` clean, `go test -race -count=1 ./...` über 13 Pakete grün, `go mod tidy` Diff=0, Cross-Compile Linux+Windows CGO-frei bestätigt (20M ELF + 20M PE32+). **Cross-Check-Scope 11. erfolgreiche Anwendung** — `git diff cf850f8..HEAD -- cmd/{cli,mcp} internal/{adb,client,config,cliconfig,crash,ingest,ws}` = 0 Bytes; Touches ausschließlich erwartet (`internal/store`, `internal/dashboard`, `internal/http/server.go` +7 Lines, `web/templates`, `web/static/dashboard.css`, `cmd/hub/main.go` 1 semantische Line, `docs/WORKLOG.md`). Crash-Count-Beobachtung des Worker-Berichts geprüft: nicht S3-Scope (`CrashesBySession` ist P2b-S6-Bestand, S3 reused rein lesend) — kein Finding, allenfalls Folge-Hinweis für späteren Sub-Sprint mit realistischerem Stacktrace-Sample. Empfehlung Freigabe. Status `QS grün` — wartet auf Chakotay-Findings-Gate.
  - 2026-05-16T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf substantieller additive Store-Erweiterung + neuer Render-Layer, Whitelist-Disziplin sauber, SQL-Injection-Defence + Page-Clamp testgesichert. **Cross-Check-Scope 11. erfolgreiche Anwendung** sauber. Status `QS grün`. **Auto-Chain S4 NICHT ausgelöst** — Token-Budget-Hinweis von Belanna übernommen: 3 Sub-Sprints in einer Hauptsession, S4+S5 = 4 weitere Spawns wären riskant. Plan-Phasen-Auto-Chain-Konvention sagt explizit „Token-Budget Hauptsession > 30%?" — konservativ Stop. Admin-Status-Push folgt mit S4-Confirm-Frage. **NICHT in main mergen jetzt** — Sub-Sprint, Phase 2c noch S4+S5 offen.

---

## AUFTRAG #026 — Tracelab P2c-S2 — Live-Tail (SSE + htmx-Stream-Append + Filter/Auto-Scroll/Pause)

- **Timestamp:** 2026-05-16T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin (AskUserQuestion-Block 2026-05-16: ADR-012 = Option A SSE ✅ + Auth = Loopback-Default permanent ✅) → Chakotay (#025-Findings-Gate freigabe via `fc38460`, Auto-Continuation phase-by-phase, beide S2-Auto-Stops aus Plan-Briefing damit aufgelöst) → belanna
- **Auftrag:** S2 von Phase 2c — **Live-Tail-Tab end-to-end**. SSE auf neuem `/dashboard/stream?session=…`-Endpoint mit Subscriber-Bridge zum WS-Hub (oder direkter Hub-Subscriber-Reuse), Browser-seitig htmx `hx-ext="sse"` mit Stream-Append in den Live-Tail-Tab, Session-Filter-Dropdown + Auto-Scroll + Pause/Resume.
  - **Umbrella-Ref:** Phase 2c (5 Sub-Sprints S1–S5, S1 ✅)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2c-dashboard.md` (Sub-Sprint S2)
  - **ADR-Ref bestehend:** ADR-011 (Render-Stack + Embedding, Accepted) — `/dashboard*` als Sub-Router; ADR-012 (Live-Tail-Mechanik, Proposed) — Decision-Block leer, Lead-Empfehlung Option A SSE.
  - **Branch:** `feat/phase-2-dashboard` (bereits aktiv, S1 darauf gemerged)
- **DoD S2:**
  - **ADR-012 Decision-Block ausfüllen VOR Code-Touch** — Option A bestätigt (SSE auf `/dashboard/stream`), Status `Proposed` → `Accepted`, Datum + „Admin-Confirm 2026-05-16 via AskUserQuestion-Block (Chakotay)" als Ref.
  - **ADR-011 Consequences-Note upgraden** — Auth-Posture von „awaits decision" auf „permanently Loopback-only" (Admin-Confirm 2026-05-16). 3-fach-Doku (Config-Field-Doc + README-Footnote¹ + ARCH-Consequences) entsprechend angleichen.
  - **SSE-Endpoint** `GET /dashboard/stream?session=<id>` als neuer Sub-Sub-Router-Endpoint unter `/dashboard*`. Subscriber-Bridge zum WS-Hub (`ws.Hub.Subscribe` analog `/tail`-WS-Pfad) → SSE-Encoding (`Content-Type: text/event-stream`, `data: <json>\n\n`-Format, Heartbeat-Comments alle ~15s). Drop-on-full-Slow-Subscriber-Pattern aus ws.Hub übernehmen.
  - **`internal/dashboard/handler.go`** erweitern um `StreamHandler` (oder neues File `stream.go`). Defensive Patterns: Session-ID-Validation, Hub-nil-Check (analog Layout-Handler-Posture), Heartbeat-Ticker, Context-Cancel-Cleanup.
  - **Live-Tail-Tab-Template** `web/templates/tab_live_tail.gohtml` upgraden: htmx `hx-ext="sse"` + `hx-sse="connect:/dashboard/stream?session=..."` + `hx-swap="beforeend"` auf ein Ziel-Element. Session-Filter-Dropdown (`<select>` mit Sessions aus `/sessions`-Endpoint, htmx-Hot-Swap des Stream-Targets bei Filter-Wechsel). Auto-Scroll als kleine CSS-/JS-Logik (overflow-anchor oder scrollIntoView in der Event-Append-Hook). Pause/Resume-Toggle (Button, der das `<div>` aus dem SSE-Listening rausnimmt via `hx-swap="none"` oder Connection-Close).
  - **Tests:** `internal/dashboard/stream_test.go` (neu) — SSE-Format-Konformität (Content-Type, data-Lines, Heartbeat), Slow-Subscriber-Drop-Behavior, Context-Cancel-Cleanup, Session-Filter-Forwarding. Bestehende Tests grün, neue Tests >= 6 für Stream-Pfad.
  - **`go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün, `go mod tidy` Diff=0, `make hub mcp mcp-windows hub-windows` clean (CGO-frei bestätigt).
  - **E2E-Smoke:** Daemon mit aktiver Session, im Browser `/dashboard` → Live-Tail-Tab → Events strömen real-time rein, Session-Filter switcht Stream-Target, Auto-Scroll funktioniert, Pause hält Stream an, Resume nimmt wieder auf.
- **Mandat:**
  - Worker-Spawn ballard (Klasse `feature`, SSE-Endpoint + Subscriber-Bridge + Browser-Wireup = substantieller Stream-Layer).
  - **ADR-Updates VOR Code-Touch** (ADR-012-Decision-Block + ADR-011-Consequences-Note).
  - **Cross-Check-Scope (10. Anwendung):** bestehende Pakete `cmd/{hub,cli,mcp}`, `internal/{adb,client,config,cliconfig,crash,http,ingest,store}` müssen 0 Lines bleiben — AUSSER `internal/http/server.go` (`/dashboard/stream`-Route-Add) und ggf. `internal/ws/hub.go` falls Subscribe-API noch nicht stream-bridge-fähig (in dem Fall: additive Erweiterung, dokumentieren). Falls weitere Pakete touchiert werden müssen, im Bericht explizit begründen.
- **Auto-Stop S2:**
  - **ws.Hub.Subscribe-API-Erweiterung** falls nötig — wenn die bestehende Subscribe-Signatur nicht ohne weiteres SSE-Bridge-tauglich ist (z.B. Channel-Pattern incompatible), kurz an chakotay mit Lead-Empfehlung. Sonst (additive Erweiterung trivial): autonom durchziehen, im ADR-012-Body als Implementation-Detail notieren.
  - **Slow-Subscriber-Strategy bei Browser** — wenn Browser-SSE-Buffer voll, Behavior abklären (drop-events vs. close-connection vs. backpressure-signal). Default = drop-events analog ws.Hub. Bei abweichender Strategy: kurz an chakotay.
- **Nach S2-QS-grün:** Auto-Chain zu S3 (Session-Browser) — keine weiteren Auto-Stops im Plan-Briefing für S3-S4. S5 hat Sammel-Gate-QS als natürlichen Stop.
- **Status:** in QS (worker-erledigt, Tuvok-Spawn standard läuft)
- **Verlauf:**
  - 2026-05-16T (Eröffnung) — chakotay: Beide S2-Auto-Stops aus Plan-Briefing per AskUserQuestion-Block aufgelöst: ADR-012 Option A (SSE) + Auth Loopback-Default-permanent. S2 routet an belanna mit ADR-Updates-Mandat vor Code-Touch + SSE-Endpoint + Live-Tail-Browser-Wireup. Default-Modus weiter aktiv (Lead-Autonomie für trivial-Folge-Schritte, Auto-Continuation zu S3 nach QS-grün ohne neue Approval-Frage).
  - 2026-05-16T (Annahme + Delegation) — belanna: Auftrag angenommen, Klasse 🟡 feature (SSE-Stream-Layer + Subscriber-Bridge zum ws.Hub + Browser-htmx-Wireup). Branch bereits aktiv (`feat/phase-2-dashboard` @ `277463c`). Worker-Spawn ballard für Implementation — eindeutig kein Lead-Direkt-Korridor (substantieller Code mit SSE-Encoding-Compliance, Hub-Subscriber-Bridge mit Drop-on-full-Pattern aus ws.Hub, Heartbeat-Ticker, Context-Cancel-Cleanup, Browser-htmx-SSE-Wireup mit Session-Filter-Hot-Swap + Auto-Scroll + Pause/Resume). ADR-Updates VOR Code-Touch im Mandat (ADR-012-Decision-Block-Fill + ADR-011-Auth-Consequences-Upgrade auf „permanently Loopback-only"). Pre-Inspection-Hint für Ballard: ws.Hub.Subscribe-Signatur prüfen (Channel-Pattern? Subscriber-Struct? — falls additive Erweiterung nötig, im ADR-012-Body als Implementation-Detail notieren statt Auto-Stop).
  - 2026-05-16T (worker-erledigt) — ballard: Implementation in 11 Files (5 neu, 6 mod). **ADR-012 Accepted** mit Decision „Option A — Server-Sent Events on a new `/dashboard/stream` endpoint" + Datum + Admin-Confirm-Ref + Consequences-Block. **ADR-011 Consequences-Note** upgraded auf „permanently Loopback-only (Admin-Confirm 2026-05-16)". **3-fach-Auth-Posture-Doku-Angleichung** (ARCH-Consequences + Config.Dashboard-Field-Doc + README-Footnote¹). **Neu:** `internal/dashboard/stream.go` (StreamHandler + HeartbeatInterval-Var + writeSSEEvent-Helper, defensive Patterns: session=leer→400, Flusher-Type-Assert→500, Context-Cancel-Cleanup via deferred Hub-Cancel) + `internal/dashboard/stream_test.go` (+8 Tests: ContentType, DataFormat, Heartbeat, SlowSubscriberDrop, ContextCancelCleanup, UnknownSession, SessionRequired, HubNil) + `internal/http/dashboard_stream_wireup_test.go` (+4 Tests: RegisteredWhenHubAndDashboardPresent, NotRegisteredWhenHubNil, NoBearerRequired, EventEndToEnd) + `web/static/htmx-ext-sse.js` v2.2.2 (8896 byte SHA256 `83eca6fa...875611`) + `web/static/htmx-ext-sse.version.txt`. **Modifiziert:** `docs/ARCH.md`, `internal/http/server.go`, `README.md`, `internal/dashboard/handler_test.go` (4 Assertions auf neuen Stable-Marker `id="live-tail-output"`), `web/templates/{base.gohtml, tab_live_tail.gohtml}` (base lädt htmx-ext-sse mit defer, tab komplett neu mit Controls + IIFE-Scoped-JS + Pause/Resume via DOM-Mutation), `web/static/dashboard.css` (Live-Tail-Styles mit overflow-anchor:none gegen Browser-Scroll-Anchoring). **ws.Hub UNANGETASTET** — Pre-Inspection-Audit bestätigte Subscribe-API bereits SSE-bridge-tauglich (im ADR-012-Body als Implementation-Detail dokumentiert wie gewünscht). Repo-weit `go test -race -count=1 ./...` über 12 Pakete grün, +12 Tests (dashboard +8, http +4), 0 Regressionen. `go vet` clean, `go mod tidy` Diff=0 (stdlib only — `net/http` hat SSE-Support nativ via `http.Flusher`), `make hub mcp mcp-windows hub-windows` clean (CGO-frei bestätigt). E2E-Smoke gegen Daemon `127.0.0.1:28765`: `/dashboard` rendert neues Live-Tail-Markup, `/dashboard/static/htmx-ext-sse.js` 200 + 8896 byte, `/dashboard/stream` ohne session → 400 „session required", mit gestarteter Session + parallelem `/ingest`-Push kommen `data: {...}\n\n`-Frames durch, Pause/Resume funktioniert via DOM-Mutation. **Cross-Check-Scope 10. erfolgreiche Anwendung:** `git diff main..HEAD -- cmd/{cli,mcp} internal/{adb,client,config,cliconfig,crash,ingest,store,ws}` = 0 Lines. **Unknown-Session-Entscheidung:** 200 OK + leerer Stream (nur Heartbeats), analog `/events`-Verhalten (im Test-Doc-Kommentar dokumentiert: session-Existenz ist `/sessions`-Concern, nicht Stream-Endpoint-Concern; Browser-UX sauberer als 404).
  - 2026-05-16T (Code-Commits) — belanna: 2-Commit-Split analog P2c-S1-Pattern. `8ff69fd docs(arch): P2c-S2 ADR-012 Accepted (SSE) + ADR-011 Auth permanent-Loopback`. `d356270 feat(dashboard): P2c-S2 SSE /dashboard/stream + live-tail tab E2E`. Push `feat/phase-2-dashboard` zu origin clean (`277463c..d356270`). Status `in QS` — Tuvok-Spawn standard folgt.
  - 2026-05-16T (QS-Lauf abgeschlossen) — tuvok, qs-20260516-002: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 7 DoD-Punkte live-verifiziert: ADR-012 Accepted mit Decision-Wortlaut „Option A — SSE on /dashboard/stream" + Datum + Admin-Confirm-Ref + 6-Punkt-Consequences-Block; ADR-011-Consequences-Note „permanently Loopback-only" 3-fach-Sync drift-frei (ARCH:1057 + server.go:63 + README:68 verwenden exakt das Wortlaut-Tripel); SSE-Encoding-Konformität (Content-Type/Cache-Control/Connection/X-Accel-Buffering vor erstem Flush, `data: %s\n\n`, `: heartbeat\n\n` alle 15s); defensive Patterns (session=leer→400, Flusher-Type-Assert→500, Hub-nil→Route nicht registriert); 12 neue Tests (dashboard +8 inkl. SlowSubscriberDrop-Publisher-Non-Block-Contract auf 1000 Events Buffer-2 ohne Body-Read + ContextCancelCleanup-runtime.NumGoroutine-Delta≤2 nach 5 Open/Close-Zyklen, http +4 Wireup); `go vet` clean, `go mod tidy` Diff=0 (stdlib only), Cross-Compile Linux+Windows CGO-frei bestätigt. **Cross-Check-Scope 10. erfolgreiche Anwendung** — `git diff main..HEAD -- cmd/{cli,mcp} internal/{adb,client,config,cliconfig,crash,ingest,store,ws}` = 0 Bytes, ws.Hub absolut unangetastet. htmx-ext-sse v2.2.2 SHA-Pin `83eca6fa...875611` + 8896-Byte-Match live verifiziert. Empfehlung Freigabe + Methodik-Hinweis (außerhalb Findings): **ADR-Decision-Block-Disziplin 3× stabil bestätigt** (ADR-007, ADR-009, ADR-012) → Promotion-Reife erreicht für `XBrain/30_Wissen/ADR-Konventionen.md` (Belanna-Folge-Auftrag). Analoges Pattern „3-fach-Auth-Posture-Doku-Sync" bei 1. Anwendung — beobachten. Status `QS grün` — wartet auf Chakotay-Findings-Gate.
  - 2026-05-16T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf SSE-Stream-Layer mit 2 ADR-Updates, ADR-Disziplin tadellos (Decision-Block-Disziplin 3. Anwendung — Promotion-Trigger für Belanna), Cross-Check-Scope 10. Anwendung sauber (ws.Hub unangetastet trotz SSE-Bridge-Bedarf — Subscribe-API trivial-additiv-tauglich Pre-Inspection-bestätigt). Status `QS grün`. Auto-Chain zu S3 (Session-Browser) freigegeben — Plan-Briefing 2c sieht keine weiteren Auto-Stops bis S5 (Sammel-Gate) vor, Default-Modus aktiv. — Tracelab P2c-S1 — ARCH-Vorab (ADR-011/012) + Skeleton + `/dashboard`-Route + Tab-Layout

- **Timestamp:** 2026-05-16T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin (Stack-Vorbesprechung am 2026-05-16 in Chakotay-Session: 2 Runden AskUserQuestion-Block-Dialog, Plan-Briefing 5a y'd) → Chakotay (Modus H2 Block-Dialog + Plan-File `~/.claude/plans/tracelab-phase-2c-dashboard.md` + Auto-Continuation-Default-Modus aktiv) → belanna
- **Auftrag:** S1 von Phase 2c — **erster Sub-Sprint, Foundation-Layer**. ARCH-Vorab in `docs/ARCH.md` vor Code-Touch + Skeleton-Implementierung mit `/dashboard`-Route in `tracelab-hub` und Tab-Navigation-Layout (4 Tabs: Live-Tail · Sessions · Crashes · Agents-Placeholder).
  - **Umbrella-Ref:** Phase 2c (5 Sub-Sprints S1–S5)
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2c-dashboard.md` (Sub-Sprint S1)
  - **Stack-Entscheidungen (Admin-confirmed):** htmx + `html/template` + SSE-or-WS (genauer Live-Tail-Mechanik in ADR-012 zu entscheiden), embedded in `tracelab-hub` als `/dashboard`-Route, Assets via `go:embed`, CGO-frei bleibt, keine Node-Toolchain.
  - **Branch:** `feat/phase-2-dashboard` von `main@bf69888` (neu zu erstellen)
- **DoD S1:**
  - **ADR-011 in `docs/ARCH.md` VOR Code-Touch** — Render-Stack + Embedding-Pfad. Decisions: (a) htmx + `html/template` als Render-Stack, (b) `tracelab-hub` als Host (`/dashboard`-Route, kein eigenes Binary), (c) `go:embed` für Templates + Static-Assets, (d) `web/`-Top-Level-Paket-Layout. Considered/Rejected: Templ, Vue/Svelte-SPA, separate `tracelab-dashboard`-Binary. Begründungen kurz aber konkret (CGO-frei-Constraint, eine Build-Pipeline, Cross-Compile-Story).
  - **ADR-012 in `docs/ARCH.md` VOR S2-Start** — Live-Tail-Mechanik. Trade-off-Analyse: SSE auf `/dashboard/stream?session=…` vs. WS-Reuse von `/tail` direkt aus Browser. Entscheidung benötigt Admin-Confirm (Auto-Stop laut Plan-Briefing) — in S1 nur ADR-012-Skelett mit Optionen + Empfehlung, finaler Decision-Block nach Admin-Confirm.
  - **`/dashboard`-Route** in `internal/http/server.go` (Sub-Router, ggf. eigene Auth-Strategie zu klären — Auto-Stop falls nicht trivial aus Bearer ableitbar).
  - **`web/`-Paket-Layout:** `web/templates/*.gohtml` (Layout + Page-Templates für die 4 Tabs als leere Stubs), `web/static/` (htmx-Distribution + minimal CSS), `internal/dashboard/` für Handler.
  - **Layout + Tab-Navigation rendert E2E** — `/dashboard` zeigt im Browser Layout-Frame mit 4 Tabs („Live-Tail" / „Sessions" / „Crashes" / „Agents — Phase 2d coming soon"), navigation funktioniert (htmx-swap auf Tab-Klick), Tab-Inhalte sind leer/Placeholder.
  - **`go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün** — neue Tests für Dashboard-Handler (Layout-Render, Tab-Routing-Smoke, Static-Asset-Embed).
- **Mandat:**
  - Worker-Spawn ballard empfohlen (Klasse `feature`, neuer `web/`-Top-Level + ARCH-Vorab + `go:embed`-Setup = substantielle Foundation).
  - **ADR-011 vor Code-Touch** (analog ADR-006/008 als Schreib-vor-Code-Pattern), ADR-012-Skelett mit klarer Trade-off-Tabelle für Admin-Confirm.
  - **Cross-Check-Scope:** dies ist Greenfield-Foundation, neue Pakete (`web/`, `internal/dashboard/`). Cross-Check vs `main@bf69888`-Baseline: alle bestehenden Pakete (`cmd/{hub,cli,mcp}`, `internal/{adb,client,config,cliconfig,crash,http,ingest,store,ws}`) sollten 0 Lines bleiben **außer** `internal/http/server.go` (`/dashboard`-Sub-Router-Registrierung) und `cmd/hub/main.go` falls Wireup-Touch nötig.
- **Auto-Stop S1:**
  - **ADR-012-Entscheidung SSE-vs-WS-Reuse** — Admin-Confirm nach Belannas Trade-off-Analyse (Belanna soll konkrete Empfehlung im ADR-012-Entwurf hinterlegen, ich routet das an Admin).
  - **Auth-Modell fürs Dashboard** — falls nicht trivial aus Bearer ableitbar (Cookie-Wrap), Stopp + Admin-Confirm.
- **Nach S1-QS-grün:** S2 (Live-Tail) chained automatisch via Auto-Chain, **außer** ADR-012-Entscheidung blockt.
- **Status:** in QS (worker-erledigt, Tuvok-Spawn release-qs läuft)
- **Verlauf:**
  - 2026-05-16T (Eröffnung) — chakotay: Plan-Briefing 2c approved (Admin „y" auf Stack htmx+html/template+SSE in hub-Binary, Scope Live-Tail + Sessions + Crashes + Agents-Placeholder, Sub-Sprints S1–S5). S1 routet an belanna mit ARCH-Vorab-Mandat (ADR-011 vor Code-Touch, ADR-012-Skelett mit Trade-off-Tabelle). Default-Modus aktiv (Lead-Autonomie für trivial-Folge-Schritte). Phase-2d-Bookmark dokumentiert im Plan-File (KI-Agenten-Stack nach 2c, eigener H2-Dialog).
  - 2026-05-16T (Annahme + Branch) — belanna: Auftrag angenommen, Klasse 🟡 feature (Greenfield mit ARCH-Pflicht). Branch `feat/phase-2-dashboard` von `main@0372ff0` angelegt (sitzt 1 Commit über `bf69888` durch chakotays Eröffnungs-Sync). Worker-Spawn ballard für Implementation — eindeutig kein Lead-Direkt-Korridor (neuer `web/`-Layout + go:embed-Setup + 2 ADRs + neue Pakete). ARCH-Vorab-Reihenfolge: ADR-011 voll ausgeschrieben **vor** Code-Touch, ADR-012-Skelett mit Trade-off-Tabelle + Empfehlung, Decision-Block bleibt leer (Auto-Stop für Admin-Confirm via chakotay).
  - 2026-05-16T (Delegation) — belanna: Worker-Spawn ballard via Agent-Tool gestartet (Klasse `feature`, ARCH-Vorab + Skeleton + Tab-Layout). Self-Test Lead-Direkt-vs-Spawn: Spawn klar gerechtfertigt — substantieller Go-Code mit neuen Top-Level-Paketen, ADR-Schreibpflicht, Test-Suite-Anlage. Kein Token-Wallet-Risiko (vergleichbar mit P2b-S1 #018).
  - 2026-05-16T (Empfang) — ballard: Auftrag angenommen, Pflicht-Init gelesen (Persona-Bookmarks bekannt, Plan-File `tracelab-phase-2c-dashboard.md` durchgelesen, ADR-009-Format als Schablone, server.go-Sub-Router-Pattern und Bearer-30s-Group-Konstruktion identifiziert). Reihenfolge: (1) ADR-011 voll in `docs/ARCH.md` schreiben, (2) ADR-012-Skelett mit leerem Decision-Block, (3) Commit (a) `docs(arch)`, (4) `web/` Top-Level + `internal/dashboard/` + Handler + Templates + go:embed, (5) `/dashboard`-Sub-Router in server.go, (6) Tests, (7) `go vet` + `go test -race` + Cross-Compile-Smoke, (8) Commit (b) `feat(dashboard)`, (9) Push. Auth-Modell-Eval: Bearer-via-Cookie braucht Cookie-Setup-Endpoint + CSRF-Erwägung → siehe Bericht ## Offen, S1 rendert `/dashboard` ungeschützt im Local-Loopback-Default (Single-User-Dev-Host), Auth-Härtung folgt Admin-Confirm.
  - 2026-05-16T (worker-erledigt) — ballard: Implementation in 22 Files (15 neu, 7 mod). **ADR-011 Accepted mit 4 Decisions** (htmx+html/template, hub-embedded `/dashboard`, `go:embed`-Assets, Top-Level `web/`-Layout) inkl. Considered/Rejected (Templ, Vue/Svelte/React-SPA, separate Binary). **ADR-012 Proposed mit 10-Kriterien-Trade-off-Tabelle** SSE-vs-WS-Reuse, Decision-Block bleibt leer (wartet auf Admin-Confirm). **Lead-Empfehlung Option A (SSE auf neuem `/dashboard/stream`)** mit 5-Punkt-Begründung im ADR-Body. **Neu:** `web/embed.go` + 5 Templates + `web/static/{dashboard.css, htmx.min.js v2.0.4 SHA-pinned, htmx.version.txt}` + `internal/dashboard/{handler.go, handler_test.go}` + `internal/http/dashboard_wireup_test.go`. **Modifiziert:** `docs/ARCH.md` (+250 Lines ADR-011/012), `internal/http/server.go` (`/dashboard*`-Sub-Router außerhalb Bearer-Group), `cmd/hub/main.go` (Wireup), `README.md` (Components + Endpoints-Tabelle + Auth-Footnote¹). Repo-weit `go test -race -count=1 ./...` grün (12 Pakete, +12 Tests: dashboard +8, http +4). `go vet` clean, `go mod tidy` Diff=0 (stdlib only), `make hub mcp mcp-windows hub-windows` clean (CGO-frei bestätigt). E2E-Smoke gegen Daemon `127.0.0.1:28765`: `/dashboard` 200 mit 4 Tabs sichtbar, htmx-Swap funktioniert, Static-Assets servable. **Cross-Check-Scope sauber:** `git diff main -- internal/{adb,client,config,cliconfig,crash,ingest,store,ws} cmd/{cli,mcp}` = 0 Lines (9. erfolgreiche Anwendung). **Zwei Auto-Stops als Offen-Punkte:** (1) ADR-012-Decision SSE-vs-WS (Lead-Empfehlung Option A), (2) Auth-Modell `/dashboard*` (S1-Posture: unauthenticated im Loopback-Default, 3-fach dokumentiert).
  - 2026-05-16T (Code-Commits) — belanna: 2-Commit-Split analog P2b-Pattern. `4906766 docs(arch): P2c-S1 ADR-011 dashboard stack + ADR-012 live-tail skeleton` (302 insert). `8f4d234 feat(dashboard): P2c-S1 skeleton + /dashboard route + tab layout` (945 insert, 15 files). Push `feat/phase-2-dashboard` zu origin clean. Status `in QS` — Tuvok-Spawn release-qs folgt.
  - 2026-05-16T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf Foundation-Sprint mit 2 ADRs + neuen Top-Level-Paketen, ADR-Disziplin tadellos (Decision-Block-Disziplin 2. Anwendung), htmx-Vendor-Pin korrekt, Template-Hygiene sauber. **9. Cross-Check-Scope-Anwendung** ohne Drift. Auto-Chain zu S2 bewusst unterdrückt — Plan-Briefing 5a hat ADR-012-Decision + Auth-Modell als explizite Auto-Stops vorgesehen, das sind keine Mängel. Status `QS grün` auf Sub-Sprint, Phase 2c weiterhin offen. Bericht an Admin mit 2-Frage-Block (ADR-012 + Auth-Modell) folgt. **NICHT in main mergen jetzt** — Sub-Sprint, Phase noch nicht durch.
  - 2026-05-16T (QS-Lauf abgeschlossen) — tuvok, qs-20260516-001: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 7 DoD-Punkte live-verifiziert: ADR-011 4 Decisions voll ausgeschrieben mit konkreten Considered/Rejected-Begründungen + Consequences-Block inkl. Auth-Posture-Defer-Note; ADR-012 10-Kriterien-Trade-off-Tabelle + 5-Punkt-Lead-Empfehlung + Decision-Block bei Z.1150-1153 explizit `intentionally blank — awaits Admin-Confirm` (Disziplin tadellos, Empfehlung sauber als Empfehlung gelabelt nicht als Decision verkleidet); Sub-Router außerhalb Bearer-Group; Auth-Posture-3-fach-Doku (ARCH-Consequences + Config-Field-Doc + README-Footnote¹) verifiziert; htmx.min.js v2.0.4 SHA256-Pin `e209dda5...8fb447` + Bytes `50917` exakt verifiziert; NewHandler parst Templates bei Init (template.ParseFS) mit non-nil-Error-Path bei Parse-Fail; 12 neue Tests (dashboard +8, http +4); `go test -race -count=1 ./...` über 12 Pakete grün, `go vet` clean, `go mod tidy` Diff=0, Cross-Compile Linux+Windows CGO-frei bestätigt. **Cross-Check-Scope 9. erfolgreiche Anwendung** — `git diff main..HEAD -- internal/{adb,client,config,cliconfig,crash,ingest,store,ws} cmd/{cli,mcp}` = 0 Files. **Template-Hygiene:** kein Inline-CSS/JS, keine externen url()/@import, kein CDN, Assets ausschließlich Loopback-Path. Empfehlung Freigabe — 2 Worker-Bericht-Auto-Stops sind keine QS-Findings, sondern Admin-Confirm-Pflichten für S2-Start (ADR-012-Decision + Dashboard-Auth-ADR). Pflicht-Outro Tuvok: keine neue Lesson (ADR-Decision-Block-Disziplin 2. Anwendung nach ADR-007 — bei 3. Promotion-Kandidat, noch nicht jetzt). Status `QS grün` — wartet auf Chakotay-Findings-Gate.

---

## AUFTRAG #024 — Tracelab Phase-2b-Closure — FF-Merge + Branch-Cleanup + State-Sync

- **Timestamp:** 2026-05-15T (Eröffnung + Ausführung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin („beides" — P2a-Drift-Klärung + P2b-Closure als Tail-Bündel) → Chakotay → belanna
- **Auftrag:** Phase-2b-Sammel-Gate-Closure. FF-Merge `feat/phase-2-mcp` → `main` mit anschließendem Branch-Cleanup (lokal + remote) und State-Sync. Drift-Kontext: Chakotays initialer Status-Check zeigte Vault-ST.md auf veraltetem „P2a wartet auf Merge"-Stand, tatsächlich war P2a bereits gemerged (`bdc3a0c`) — effektiver Restscope ausschließlich P2b-Closure.
- **DoD:**
  1. ✅ FF-Merge `feat/phase-2-mcp` → `main` (Fast-Forward, kein Merge-Commit)
  2. ✅ Push `main` → `origin/main`
  3. ✅ Branch-Cleanup: lokal (`-D` da `feat/phase-2-mcp` ahead of `origin/feat/phase-2-mcp` durch Sammel-Gate-Commit `cb249bd`, der nie auf Branch gepusht wurde — Admin-Pauschal-Confirm deckt `-D`) UND remote (`git push origin --delete`)
  4. ✅ State-Sync-Commit auf `main` mit Frontmatter-Update (`phase-2b-merge-commit: cb249bd`, status auf „phase-2b-gemerged", aktiver-auftrag auf „—") + AUFTRAG #024 als Closure-Eintrag
  5. ✅ Push final-state-commit
  6. ✅ Verifikation: `git branch -a` zeigt nur `main` + `origin/main`, kein verwaister Feature-Branch
- **Mandat:** Lead-Direktarbeit (Klasse ⚪ continuation/trivial). Kein Worker-Spawn — alle Operationen sind explizit unter Belanna-Lead-Autonomie gelistet (FF-Merge mit `--ff-only`, Branch-Cleanup nach Merge, WORKLOG-Updates, Standard-Push). Admin-Pauschal-Confirm deckt die Force-relevante `-D`-Operation.
- **Pre-Check (vor FF-Merge):** `git merge-base feat/phase-2-mcp main` = `9536b12` = `git rev-parse main` → FF-Linearität bestätigt (ANCESTOR_OK).
- **Status:** ✅ Abgeschlossen
- **Verlauf:**
  - 2026-05-15T (Eröffnung + Ausführung) — belanna: Pre-Check linear (`main@9536b12` ≤ `feat/phase-2-mcp@cb249bd`). `git checkout main && git merge --ff-only feat/phase-2-mcp` → 32 Files, +5121/-50 Lines fast-forward von `9536b12` auf `cb249bd`. `git push origin main` → `9536b12..cb249bd  main -> main` clean. `git branch -d` schlug fehl (Branch ahead of `refs/remotes/origin/feat/phase-2-mcp`, weil Sammel-Gate-Commit `cb249bd` nur lokal war + dann direkt nach main gemerged statt zur Feature-Branch gepusht) → `git branch -D feat/phase-2-mcp` (Force-Delete unter Admin-Pauschal-Confirm). `git push origin --delete feat/phase-2-mcp` → `[deleted] feat/phase-2-mcp` remote sauber. Verifikation: `git branch -a` zeigt `main` + `origin/HEAD` + `origin/main`, kein Feature-Branch mehr. `git log --oneline -5` Tip: `cb249bd chore(qs): findings-gate #023 — freigabe` → `496efeb feat(mcp): P2b-S6 crashes_list-Tool` → `54b81a6 feat(store): P2b-S6 CrashesBySession limit param + ADR-009` → `9d31461 docs(worklog): #023 P2b-S6 eröffnet` → `c88ec9d chore(qs): findings-gate #022 — freigabe`. Tracelab Phase 2b damit vollständig auf `main`: 6 Sub-Sprints (S1 surface-cut + S2 ADR-007 + S3 sessions_list + S4 tail_since/ADR-008 + S5 adb-Tools + S6 crashes_list/ADR-009), 4 ADRs Admin-confirmed (007/008/009/010), 6 MCP-Tools live (`adb_devices, adb_start, adb_stop, crashes_list, sessions_list, tail_since`) stub-frei, 0 Findings über die gesamte Phase. State-Sync-Commit als finaler Touch des Auftrags: Frontmatter (status/phase-2b-merge-commit/aktiver-auftrag) + dieser AUFTRAG-#024-Eintrag.

---

## AUFTRAG #023 — Tracelab P2b-S6 — `crashes_list`-Tool + Hub-`/crashes`-Endpoint + ADR-009

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin (Pauschal-Confirm „weiter solange context unter 45%" deckt S6-Hub-Schema-Change) → Chakotay (Auto-Chain nach #022-Freigabe) → belanna
- **Auftrag:** S6 von Phase 2b — **letzter Sub-Sprint vor Phase-2b-Closure**. `crashes_list`-MCP-Tool + neuer Hub-`GET /crashes`-Endpoint. Additive Hub-Schema-Mutation analog ADR-004/008-Pattern. Store-Layer existiert bereits (`crashes`-Tabelle + `UpsertCrash` aus Phase-1-S5), nur HTTP-Wrapper fehlt.
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S6)
  - **ADR-Ref bestehend:** ADR-007 (Tool-Surface Admin-confirmed) — `crashes_list` `{ session_id: string, limit?: number }` → `{ crashes: CrashEvent[] }` / `client.CrashesList` (neu), Bearer required, Hub-Endpoint `GET /crashes?session=…&limit=…` (neu). **ADR-009 NEU** für Hub-`/crashes`-Endpoint-Form (Query-Params, Response-Shape, Bearer-Check, Default/Max-Limit) — Pattern analog ADR-008 (Considered/Rejected, Wire-Compat-Statement falls additiv am Hub).
  - **Branch:** `feat/phase-2-mcp` (aktiv, tip `c88ec9d`)
- **DoD S6:**
  - **ADR-009 in `docs/ARCH.md` VOR Code-Touch** — neuer ADR-Block, dokumentiert: Hub-`/crashes`-Endpoint-Form (Query-Params session required + limit optional, Response `{ crashes: CrashEvent[] }`, Bearer-Check, Default + Max-Limit). Considered/Rejected (analog ADR-008-Struktur).
  - **Hub-Endpoint** `GET /crashes?session=<id>&limit=<n>` in `internal/http/server.go` (Bearer-30s-Sub-Router-Group) + Handler in `internal/http/handlers.go`. Default/Max-Limit als Const-Block.
  - **Store-Query-Methode** neu in `internal/store/sqlite.go` (Vorschlag: `CrashesBySession(ctx, sessionID, limit)` o.ä. — Tabelle + UpsertCrash existieren bereits seit Phase-1-S5, nur Query-Methode + Tests neu). Default-Limit-Logik analog `EventsSince`.
  - **Client-Methode** `internal/client.CrashesList(ctx, sessionID, limit)` (neu) — Bearer-Plumbing wie bestehende Methoden, httptest-Tests (happy, auth-fail, empty, malformed).
  - **MCP-Tool** `cmd/mcp/crashes.go` analog `cmd/mcp/sessions.go`/`tail.go`/`adb.go`-Pattern (4. Anwendung des Tool-Templates). Input-Schema `{ session_id, limit? }`, defensive nil-slice für Empty-Stability.
  - **Stub-Removal:** `crashes_stub` aus `stubTools`-Slice in `cmd/mcp/main.go` raus (Slice ist danach leer/wird entfernt). `want`-Slice in `main_test.go` aktualisiert auf finale Liste (alle 6 echten Tools: `adb_devices, adb_start, adb_stop, crashes_list, sessions_list, tail_since` sortiert — kein _stub mehr).
  - **mcp-go-Smoke-Tests + Hub-Endpoint-Integration-Tests** analog S4 (Registered, Description, Schema, Tripwire, HandlerCallsHub, AuthFail, Empty-Stability + Hub-Test mit echter Store-Layer)
  - `go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün
  - **CrashEvent-DTO** in `internal/client/types.go` ergänzen falls nicht vorhanden (Felder analog `internal/store.Crash`-Struct)
- **Mandat:**
  - Worker-Spawn ballard empfohlen (Klasse `feature`, Hub+Store-Query+Client+MCP-Tool = substantiell, 4. Anwendung Tool-Template)
  - **ADR-009-Skelett vor Code-Touch** (analog ADR-008 in #021)
  - Cross-Check-Scope voll wie #020/#021/#022 (Lesson 7× bestätigt). Diesmal sind `internal/http`/`internal/store`/`cmd/hub` **wieder beteiligt** (S6 ist Hub-Mutation), aber `cmd/cli`/`internal/adb`/`internal/crash`/`internal/ws`/`internal/ingest`/`internal/cliconfig`/`internal/config` MÜSSEN 0 Lines bleiben.
  - **Tool-Template-Promotion-Trigger erreicht (Belanna L7 sagt: bei 4. Anwendung → T5-Auslagerung in `30_Wissen/MCP-Tool-Skeleton.md`).** Belanna entscheidet, ob T5-Note in diesem Sprint oder als Folge-Aufgabe.
- **Auto-Stop S6:** keine zusätzlichen über 5a-Default hinaus — Hub-Schema-Change ist mit diesem Auftrag bereits Admin-confirmed (Pauschal-Confirm).
- **Nach S6-QS-grün:** Phase-2b-Sammel-Gate (zusätzliches `release-qs` über alle Sub-Sprints zusammen) + Admin-Confirm für FF-Merge `feat/phase-2-mcp` → `main`. Plan-Block-3 sieht das explizit so vor.
- **Status:** ✅ QS grün — wartet auf Chakotay-Findings-Gate. Code committed (`54b81a6` + `496efeb`) + gepusht. **Phase-2b-Sammel-Gate jetzt eröffnungsreif** (Tuvok-Empfehlung).
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Auto-Chain nach #022-Freigabe unter Admin-Pauschal-Confirm. S6 routet an belanna mit Hub-Touch-Mandat + ADR-009-Schreibauftrag. Tool-Template-Promotion-Trigger erreicht (4. Anwendung).
  - 2026-05-15T (Delegation) — belanna: Worker-Spawn ballard (Klasse `feature`, Hub-Mutation analog #021). **Vor-Inspektion** mit drei Befunden: (a) `Store.CrashesBySession(ctx, sessionID) ([]CrashRow, error)` **existiert bereits** in `internal/store/sqlite.go:448` mit Doc-Kommentar „Used by tests and the future /crashes API" — Limit-Param fehlt → **additive Signatur-Erweiterung** `(ctx, sessionID, limit int)` analog #022 Discriminator-Pattern; 7 Test-Aufrufe in `internal/http/ingest_crash_test.go` + `internal/store/sqlite_test.go` als Co-Touch im Sprint-Scope (legitim). (b) `crashes`-Schema vollständig (id, session_id, ts, fingerprint, stacktrace, count) mit Indices `idx_crashes_session_ts` + `idx_crashes_fingerprint` — keine Migration 0003 nötig, ADR-009 kann „kein neuer Index" mit EXPLAIN-Beleg dokumentieren. (c) `CrashEvent`-DTO **fehlt** in `internal/client/types.go` — muss neu angelegt werden, Felder analog `CrashRow` mit JSON-Tags (`session_id`, `ts` als int64 unix-nanos analog Event, `fingerprint`, `stacktrace`, `count`). **T5-Auslagerung in `30_Wissen/MCP-Tool-Skeleton.md` als Folge-Aufgabe nach Phase-2b-Closure** (nicht in S6) — sauberer wenn 4. Pattern empirisch beobachtet ist und Auto-Chain nicht unterbrochen wird.
  - 2026-05-15T (worker-erledigt) — ballard: Implementation in 16 Files (5 neu, 11 mod). **ADR-009 mit 4 Decisions** (Reuse-mit-Limit, List-Read-vs-Forward-Cursor-Begründung „events sind Streaming-Log, Crashes sind Triage-Digest", CrashEvent-Greenfield-DTO, kein-neuer-Index-mit-EXPLAIN-Beleg + ADR-010-Tripwire). **Neu:** `internal/http/crashes_test.go` (7 Hub-Integration-Tests), `internal/client/crashes.go` (`Client.CrashesList` mit Bearer+omits-zero+fail-fast), `internal/client/crashes_test.go` (8 Tests), `cmd/mcp/crashes.go` (`crashes_list`-Tool, 4. Anwendung Tool-Template), `cmd/mcp/crashes_test.go` (9 Tests inkl. NoStub-Regression-Guard + Tripwire + LimitForwardedToHub). **Modifiziert:** `docs/ARCH.md` (ADR-009-Block ~220 Lines), `internal/store/sqlite.go` (`CrashesBySession`-Signatur additiv mit Default-500 im Store, Cap im HTTP-Layer), `internal/store/sqlite_test.go` (3 Bestands-Aufrufe gepatcht + 4 neue Tests), `internal/http/ingest_crash_test.go` (4 Bestands-Aufrufe trivial mit `, 0`-Append), `internal/http/handlers.go` (`listCrashes`+`crashView`+Const-Block), `internal/http/server.go` (`/crashes`-Route in Bearer-30s-Sub-Router), `internal/http/server_test.go` (Auth-Test erweitert), `internal/client/types.go` (`CrashEvent` greenfield), `cmd/mcp/main.go` (`crashes_list` in AddTools, `stubTools = []stubTool{}` geleert mit Doc-Kommentar als Ready-To-Extend-Pattern, S6-Status), `cmd/mcp/main_test.go` (final `want` = `[adb_devices, adb_start, adb_stop, crashes_list, sessions_list, tail_since]` alphabetisch), `README.md` (Endpoints-Tabelle `/crashes`-Row). Repo-weit `go test -race -count=1 ./...` grün (11 Pakete, +29 Tests: store +4, client +8, http +7, mcp +10). `go vet` clean, `go mod tidy` Diff=0, `make hub mcp mcp-windows` clean (ELF+ELF+PE32+).
  - 2026-05-15T (Code-Commits) — belanna: 2-Commit-Split analog #022. `54b81a6 feat(store): P2b-S6 CrashesBySession limit param + ADR-009` (5 Files, +379 Lines — Store-Signatur-Erweiterung + ADR-009 + Test-Patches). `496efeb feat(mcp): P2b-S6 crashes_list-Tool + Hub-/crashes-Endpoint` (12 Files, +1039 Lines — Hub + Client + MCP-Tool + Stub-Removal + README). Push `feat/phase-2-mcp` zu origin (`9d31461..496efeb`).
  - 2026-05-15T (QS-Lauf abgeschlossen) — tuvok, qs-20260515-005: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 9 DoD-Punkte (A-I) live-verifiziert. ADR-009 strukturell stabil zu ADR-008-Vorbild (separates Wire-Compat-Statement, EXPLAIN-Plan-Beleg, ADR-010-Tripwire). List-Read-Semantik (unknown session = 200 + `[]`) konsistent zu /events. Limit-Cap-Konsistenz (silent-cap 5000). Tool-Template 4. Anwendung strukturell 100% konsistent. **Cross-Check-Scope 8. erfolgreiche Anwendung** — 0 Lines in 7 unbeteiligten Paketen + `cmd/hub` 0 Lines. Tuvok-Empfehlung: Freigabe + **Phase-2b-Sammel-Gate jetzt eröffnungsreif** (alle 6 Sub-Sprints QS-grün, alle 4 ADRs Admin-confirmed, alle 6 MCP-Tools live, stub-frei).
  - 2026-05-15T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf letztem Sub-Sprint, ADR-Pattern 2× stabil (ADR-008+009), Tool-Template 4× wiederverwendet, additive-API-Pattern 2× bestätigt. **Cross-Check-Scope-Lesson jetzt 8× bestätigt** — Promotion-Schuld zu `30_Wissen/Worker-Brief-Konventionen.md` überdeutlich. **Phase-2b-Sammel-Gate-Eröffnung an Admin gestellt** (Token-Hinweis Belannas berücksichtigt — Auto-Chain pausiert vor Sammel-Gate, Admin entscheidet ob jetzt oder neue Session). ADR-009 4 Decisions vor Code-Touch mit Considered/Rejected je Decision, EXPLAIN-Plan-Beleg, ADR-010-Tripwire — strukturell stabil zu ADR-008-Vorbild. Hub `GET /crashes` korrekt in Bearer-30s-Sub-Sub-Router, Store-Query strict-correct, 7 Bestands-Call-Sites trivial `, 0` gepatcht, 28 neue Tests (4 store + 7 http + 8 client + 9 mcp inkl. NoStub-Regression-Guard), Stub-Removal vollständig (`stubTools = []stubTool{}` mit Doc, want-Slice alphabetisch `[adb_devices, adb_start, adb_stop, crashes_list, sessions_list, tail_since]`), `CrashEvent`-DTO greenfield bestätigt (kein bestehender Crash-Read-Endpoint). Repo-weit `go vet` clean, 11 Pakete `go test -race -count=1` grün, `go mod tidy` Diff=0, `make hub mcp mcp-windows` ELF+ELF+PE32+. Cross-Check vs S6-Baseline `9d31461`: 7 unbeteiligte Pakete + cmd/hub alle 0 Diff-Lines. Cross-Check-Scope-Lesson 8× stabil — Promotion-Schuld zu `30_Wissen/Worker-Brief-Konventionen.md` überdeutlich (Belannas Aufgabe). Status `QS grün`.

---

## AUFTRAG #022 — Tracelab P2b-S5 — adb-MCP-Tools (`adb_devices`/`adb_start`/`adb_stop`)

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin (Pauschal-Confirm „weiter solange context unter 45%") → Chakotay (Auto-Chain nach #021-Freigabe) → belanna
- **Auftrag:** S5 von Phase 2b — drei MCP-Tools für ADB-Bridge-Steuerung. Reine MCP-Layer-Wiring, kein Hub-Touch, alle Client-Methoden existieren seit Phase-1-S6/S7.
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S5)
  - **ADR-Ref:** ADR-007 (Admin-confirmed 2026-05-15) — `adb_devices` `{}` → `{ devices: ADBDevice[] }` / `client.ADBDevices`; `adb_start` `{ device_serial, tag_filter? }` → `{ status: "started"|"already_running" }` / `client.ADBStart`; `adb_stop` `{ device_serial }` → `{ status: "stopped"|"not_running" }` / `client.ADBStop`. Bearer required, alle Hub-Endpoints existing.
  - **Branch:** `feat/phase-2-mcp` (aktiv, tip `5792111`)
- **DoD S5:**
  - Drei Tools `adb_devices`/`adb_start`/`adb_stop` in `cmd/mcp/adb.go` (Single-File-Sub-Sprint analog Belannas Plan-Cut-Mandate; oder gesplittet falls Belanna Reviewability-Argument hat) — Pattern aus `cmd/mcp/sessions.go` (S3) und `cmd/mcp/tail.go` (S4) 1:1 wiederverwendbar
  - Input-Schemas mit mcp-go-Schema-Builder (Required-Felder + optionale `tag_filter`)
  - Handler ruft existing `client.ADBDevices`/`ADBStart`/`ADBStop` 1:1
  - JSON-encoded TextContent-Output, defensive nil-slice-Normalisierung (für `adb_devices` bei empty)
  - mcp-go-Smoke-Tests pro Tool (Registered, Description, Schema-Variants, WrongTypes-Tripwire, HandlerCallsHub, AuthFail, Empty-Stability)
  - Stub-Removal: `adb_stub` aus `stubTools`-Slice in `cmd/mcp/main.go` raus, `want`-Slice in `main_test.go` aktualisiert (`[adb_devices, adb_start, adb_stop, crashes_stub, sessions_list, tail_since]`)
  - `go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün
- **Mandat:**
  - Worker-Spawn ballard empfohlen (Klasse `feature`, Konsistenz mit S3+S4 — 3 Tools sind substantiell, auch wenn jedes einzelne dünn ist)
  - Single-Commit ODER 3 Commits (1 pro Tool) — Belannas Entscheidung nach Reviewability-Argument
  - Cross-Check-Scope voll wie #020/#021 (Lesson 6× bestätigt): `git diff main -- internal/adb internal/crash internal/ws internal/ingest internal/cliconfig internal/config cmd/cli cmd/hub` MUSS 0 Lines bleiben
- **Auto-Stop S5:** keine zusätzlichen über 5a-Default hinaus. **Reine MCP-Layer-Wiring.**
- **Status:** ✅ QS grün — wartet auf Chakotay-Findings-Gate. Code committed (`d9be90d` + `69d4d4c`) + gepusht. Tool-Template aus S3/S4 7. Anwendung sauber durchgegangen.
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Auto-Chain nach #021-Freigabe unter Admin-Pauschal-Confirm „weiter solange context unter 45% bleibt". S5 routet an belanna mit Mandat „Tool-Template aus S3/S4 1:1, kein Hub-Touch".
  - 2026-05-15T (Delegation) — belanna: Worker-Spawn ballard (Klasse `feature`). **Vor-Inspektion** `internal/client/adb.go`: Methoden-Namen sind `ListADBDevices`/`StartADBBridge`/`StopADBBridge` (nicht ADR-007's `ADBStart`/`ADBStop`-Kurzform — Drift bekannt, ADR-007 ist Tool-Surface-Spec, keine Client-API-Spec, Aliasing okay). **Status-Discriminator-Lücke:** Hub returnt `{status: "started"|"already_running"}` bzw. `{status: "stopped"|"not_running"}`, Client schluckt aktuell den Status (return type ist nur `error`). ADR-007-Tool-Output `{ status: "started"|"already_running" }` braucht den Discriminator → **additive Signatur-Erweiterung** an `StartADBBridge`/`StopADBBridge` auf `(status string, err error)`. **Co-Touch `cmd/cli/adb.go:176+193`** (trivial: `_, err := c.Start...`) ist API-bedingt unvermeidbar → kein Cross-Check-Scope-Verstoß, aber im Worker-Brief explizit ausgewiesen. Caller-Verifikation: `grep "Start|StopADBBridge"` zeigt nur `cmd/cli/adb.go` als non-test-Caller, alles andere ist `internal/client/adb_test.go`. Keine ADR-Erweiterung nötig (additive Public-API-Mutation an existierender Methode, kein Wire-Change, kein Hub-Touch).
  - 2026-05-15T (worker-erledigt) — ballard: Implementation in 7 Files (2 neu, 5 mod). **Neu:** `cmd/mcp/adb.go` (3 Tools `adb_devices`/`adb_start`/`adb_stop` mit `newXxxTool`/`xxxHandler`-Pattern, `translateHubError`-Reuse, defensive Nil-Slice, tag_filter accept-and-log-warn via `slog.Warn` mit Tool-Name+Serial+TagFilter-Value), `cmd/mcp/adb_test.go` (21 Tests / 599 Lines: Registered+NoStubRegression, DescriptionPresent+Knob-Mention, InputSchemaAccepts-2-Varianten, WrongTypesTolerated-Tripwire, MissingRequiredFailsFast-3+2-Case-Loops, HandlerCallsHub-Bearer+Method+Path+Body, AlreadyRunning/NotRunning-Discriminator, TagFilterAcceptedAndIgnored-Hub-Request-Body-Leak-Assertion, AuthFail, EmptyResultEmitsArray). **Modifiziert:** `internal/client/adb.go` (Start/Stop-Signatur additiv `(status string, err error)`, Empty-String-on-Error-Convention dokumentiert), `internal/client/adb_test.go` (Bestands-Tests gepatched + 2 neue Discriminator-Pass-Through-Tests), `cmd/cli/adb.go` (Z.176+193 Co-Touch `_, err :=` — exakt 2 semantische Lines, idempotent-ensure-Semantic unverändert), `cmd/mcp/main.go` (stubTools 2→1 ohne adb_stub, AddTools um 3 ADB-Factories erweitert, Package-Doc S5-Status current/S4 historisch), `cmd/mcp/main_test.go` (`want` = `[adb_devices, adb_start, adb_stop, crashes_stub, sessions_list, tail_since]` sortiert). Repo-weit `go test -race -count=1 ./...` grün (11 Pakete: cmd/mcp 54 Run-Lines (+32), internal/client 61 Run-Lines (+5), Rest unverändert). `go vet ./...` clean, `go mod tidy` Diff=0. `make hub mcp mcp-windows` clean (ELF+ELF+PE32+). **Cross-Check-Scope 7. erfolgreiche Anwendung:** `grep -rn "adb_stub" cmd/mcp/` 5 Treffer nur Test-Doc/Regression-Guard, `git diff 1497d2a..HEAD -- internal/adb internal/crash internal/ws internal/ingest internal/cliconfig internal/config internal/http internal/store cmd/hub` = **0 Lines** in allen 9 unbeteiligten Paketen. `grep -rn "StartADBBridge\|StopADBBridge"` Total-Check: alle Non-Test-Caller migriert, kein vergessener Fixture. Tool-Template strukturell konsistent zu sessions.go/tail.go.
  - 2026-05-15T (Code-Commits) — belanna: 2-Commit-Split für Reviewability. `d9be90d feat(client): P2b-S5 surface ADB start/stop status discriminator` (3 Files, 77/28 Insertions/Deletions — Client-Signatur + CLI-Co-Touch + Client-Tests, isolierte API-Mutation). `69d4d4c feat(mcp): P2b-S5 adb-MCP-Tools — devices/start/stop` (4 Files, 860/21 Insertions/Deletions — produktive Verwendung + Stub-Removal). Push `feat/phase-2-mcp` zu origin (`5792111..69d4d4c`).
  - 2026-05-15T (QS-Lauf abgeschlossen) — tuvok, qs-20260515-004: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 8 DoD-Punkte (A-H) live-verifiziert. Discriminator-Pass-Through korrekt (Hub-Status `"already_running"`/`"not_running"` 1:1 im Tool-Output). tag_filter-Test pinnt aktiv Hub-Request-Body-Leak-Prevention (kein `tag_filter` und kein `MyTag`-String im Body). Empty-String-on-Error-Convention dokumentiert + getestet. CLI-Co-Touch exakt 2 semantische Lines verifiziert (4 inkl. Kontext). Tool-Template-Konsistenz zu S3/S4 bestätigt (Const-Block-Stil, Result-Struct-Pattern, translateHubError-Reuse, Defensive-Nil-Slice). Tuvok-Empfehlung: Freigabe — S6 kann nach Findings-Gate starten (crashes_stub-Retirement, Hub-`/crashes`-Endpoint, ADR-009).
  - 2026-05-15T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings, 3 Tools + additive API-Mutation + CLI-Co-Touch sauber diszipliniert, Tool-Template etabliert sich (3× wiederverwendet), API-Mutations-Pattern dokumentiert. **Cross-Check-Scope-Lesson jetzt 7× bestätigt** (#013-#014-#015-#018-#020-#021-#022) — Promotion-Schuld in `30_Wissen/Worker-Brief-Konventionen.md` jetzt überdeutlich. **Auto-Chain zu S6 unter Admin-Pauschal-Confirm „weiter solange context unter 45%"** — S6 hat Hub-Schema-Change (`/crashes`-Endpoint additiv analog ADR-004/008), ADR-009 von Belanna/ballard vor Code-Touch zu schreiben. S6 ist **letzter Sub-Sprint** vor Phase-2b-Sammel-Gate + FF-Merge.

---

## AUFTRAG #021 — Tracelab P2b-S4 — `tail_since`-Tool + Hub-`/events`-Endpoint

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna (Admin-Confirm für Hub-Schema-Change erteilt nach #020-Freigabe)
- **Auftrag:** S4 von Phase 2b — `tail_since`-Tool + neuer Hub-`/events`-Endpoint. Erste Hub-Mutation in 2b, additive Erweiterung analog 2a-S5/ADR-004-Pattern.
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S4)
  - **ADR-Ref:** ADR-007 (tail-Tool-Shape, Admin-confirmed 2026-05-15) — `tail_since(session, since_seq?, limit?)` → `{ events: Event[], next_since_seq: number }`, Bearer required. **ADR-008 NEU:** Hub-`GET /events?session=…&since_seq=…&limit=…`-Endpoint (Schema-Change, additiv) — analog ADR-004-Pattern, von belanna in S4 zu schreiben vor Code-Touch.
  - **Branch:** `feat/phase-2-mcp` (aktiv, tip `929dd24`)
- **DoD S4:**
  - **ADR-008** in `docs/ARCH.md` vor Code-Touch — dokumentiert Hub-`/events`-Endpoint-Form (Query-Params, Response-Shape, Bearer-Check, seq-Semantik, Considered/Rejected analog ADR-004)
  - **Hub-Endpoint** `GET /events?session=<id>&since_seq=<n>&limit=<n>` in `cmd/hub/` (bzw. wo die Route-Registry liegt) implementiert. Default-Limit + Max-Limit dokumentiert. Bearer-Auth wie alle anderen Endpoints. Response `{ events: [...], next_since_seq: <n> }`
  - **Store-Reuse:** `internal/store/` hat bereits Event-Persistenz inkl. `seq` — kein Schema-Change am Store nötig, nur neue Query-Methode (`EventsSince(sessionID string, sinceSeq int64, limit int)` o.ä.). Falls Store-Lücke gefunden → ADR-Note.
  - **Client-Methode** `internal/client.EventsSince(ctx, session, sinceSeq, limit)` (neu) — Bearer-Plumbing wie existierende Methoden, Unit-Tests via `httptest`-Fake-Hub
  - **MCP-Tool** `tail_since` in `cmd/mcp/tail.go` (analog `cmd/mcp/sessions.go`-Pattern aus S3) — Input-Schema mit mcp-go-Schema-Builder, Handler ruft `client.EventsSince`, Output `{ events, next_since_seq }` als JSON-encoded TextContent
  - **Stub-Removal:** `tail_stub` aus `stubTools`-Slice in `cmd/mcp/main.go` raus, `want`-Slice in `main_test.go` aktualisiert (`[adb_stub, crashes_stub, sessions_list, tail_since]`)
  - **mcp-go-Smoke-Test:** Tool registriert, Schema-Validation (input mit/ohne optional fields), Handler-Test mit `httptest`-Fake-Hub, leerer Result → Array-Stability, `next_since_seq`-Cursor-Korrektheit
  - **Hub-Endpoint-Test:** Integration-Test gegen reale Store-Layer, Bearer-Auth-Fail-Case, seq-Filter-Korrektheit, Empty-Result-Shape
  - `go vet ./...` clean, `go test -race -count=1 ./...` repo-weit grün
- **Mandat:**
  - Belanna entscheidet Worker-Spawn ballard (Hub + Client + MCP-Tool = substantielle Implementation, klar Worker-Klasse `feature` empfohlen) ODER Lead-Direktarbeit
  - **ADR-008-Skelett vor Code-Touch** (analog wie ADR-004 in 2a-S5 vor Hub-Touch geschrieben wurde) — Considered/Rejected, Query-Param-Form, seq-Semantik
  - **Cross-Check-Scope:** voll wie #020 — alle Files im Touch-Scope inkl. Package-Doc, Const-Blocks, Tool-Description-Strings, Smoke-Test-Doc-Comments (Lesson 5× bestätigt)
  - **Reihenfolge-Empfehlung:** (1) ADR-008-Entwurf zur Admin-Sichtung optional, (2) Hub-Endpoint + Store-Query + Hub-Test, (3) Client-Methode + Client-Test, (4) MCP-Tool + Stub-Removal + MCP-Test, (5) repo-weit Tests, Commit pro logischer Einheit ODER Sammel-Commit nach Belanna-Cut
- **Auto-Stop S4:** keine zusätzlichen über 5a-Default hinaus — Hub-Schema-Change ist mit diesem Auftrag bereits Admin-confirmed. Bei substantieller Lücke in `internal/store` (z.B. seq-Index fehlt) → Eskalation.
- **Status:** ✅ QS grün — wartet auf Chakotay-Findings-Gate. Code committed (`047cd97`) + gepusht. ADR-008 (4 Decisions: events.id-Cursor, /events-Shape, SeqID-omitempty-additiv, default=500/max=5000-kein-neuer-Index).
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Admin-Confirm für Hub-`/events`-Schema-Change erhalten. AUFTRAG #021 eröffnet, S4 routet an belanna mit Hub-Touch-Mandat + ADR-008-Schreibauftrag.
  - 2026-05-15T (Delegation) — belanna: Worker-Spawn ballard (Klasse `feature`, analog S3-Konsistenz und Hub+Client+MCP-Multi-Layer). **Vor-Inspektion Hub-Routes** (`internal/http/server.go:90-127`): `/events` existiert nicht, additiv zu bauen. **Vor-Inspektion Store-Schema** (`internal/store/migrations/0001_initial.up.sql`): `events.id INTEGER PRIMARY KEY AUTOINCREMENT` ist global-monoton — **nicht** pro Session-`seq`-Spalte. → ADR-008-Designentscheidung statt Eskalation: `events.id` als opaker Cursor (analog ADR-004-Pattern, additiv, keine Schema-Migration). `client.Event` braucht zusätzliches Feld `SeqID int64 \`json:"seq_id,omitempty"\`` (additiv, kompatibel zu bestehenden Hub-Endpoints, weil omitempty). Index-Optionalität: `idx_events_session_id_id` könnte additiv via Migration 0003 helfen — ballards Entscheidung in ADR-008 (perf-rationale dokumentieren). Vorbild-Pattern aus `cmd/mcp/sessions.go` (S3-Tool-Skeleton mit mcp-go-Schema-Builder, since-Filter client-side, JSON-encoded TextContent).
  - 2026-05-15T (worker-erledigt) — ballard: Implementation in 15 Files (5 neu, 10 mod). **Neu:** `cmd/mcp/tail.go` (tail_since-Tool, mcp-go-Schema-Builder, fail-fast-leeres-session, defensive nil-slice→`[]client.Event{}` für stable JSON), `cmd/mcp/tail_test.go` (9 Tests: Registered, NoTailStubRegression, DescriptionPresent, SchemaAccepts-4-Variants, WrongTypesTolerated-Tripwire, MissingSessionFailsFast-3-Cases, HandlerCallsHub-Bearer-Check, CursorAdvances-2-RTT-Walk, AuthFail, EmptyResultEmitsArray-StableNext), `internal/client/events.go` (`EventsSince(ctx, session, sinceSeq, limit) → ([]Event, int64, error)` mit omits-zero-Query-Encoding), `internal/client/events_test.go` (8 Tests), `internal/http/events_test.go` (7 Top-Level + 6 Sub-Tests gegen httptest+SQLite). **Modifiziert:** `docs/ARCH.md` (ADR-008-Block mit 4 Decisions + Considered/Rejected + Wire-Compat-Statement + ADR-009-Tripwire für composite-Index), `internal/store/sqlite.go` (`EventsSince(ctx, sessionID, sinceID, limit)` strict-gt SQL `WHERE session_id=? AND id>? ORDER BY id ASC LIMIT ?` Default 500), `internal/store/sqlite_test.go` (+4 Tests CursorAdvances/Empty-Unknown+At-Max/LimitDefault/CrossSessionIsolation), `internal/http/handlers.go` (`listEvents`-Handler + `eventView`/`listEventsResp` + Const-Block default=500/cap=5000, silent-cap statt 400), `internal/http/server.go` (Route `/events` in Bearer-30s-Sub-Router-Group `:112` analog `/sessions`/`/ingest`), `internal/http/server_test.go` (Auth-Test um `/events?session=x` erweitert), `internal/client/types.go` (`Event.SeqID int64 json:"seq_id,omitempty"` additiv + Wire-Types `eventsSinceEventWire`/`eventsSinceRespWire`), `cmd/mcp/main.go` (stubTools 3→2 ohne tail_stub, `s.AddTools(newSessionsListTool, newTailSinceTool)`, Package-Doc S4-Status), `cmd/mcp/main_test.go` (`want` = `[adb_stub, crashes_stub, sessions_list, tail_since]`), `README.md` (Endpoints-Tabelle um `/events`-Row erweitert — deckt M1-Backlog implizit ab). Repo-weit `go test -race -count=1 ./...` grün (11 Pakete: cmd/mcp 22 (+9), internal/client 56 (+8), internal/http 34 (+13), internal/store 14 (+4), Rest unverändert). `go vet ./...` clean, `go mod tidy` Diff=0. `make hub mcp mcp-windows` clean (`file`-verifiziert ELF+ELF+PE32+). **Cross-Check-Scope 6. erfolgreiche Anwendung:** `grep -rn "tail_stub" cmd/mcp/` nur Test-Doc/Regression-Guard, `git diff main -- internal/adb internal/crash internal/ws internal/ingest internal/cliconfig internal/config cmd/cli cmd/hub` = 0 Lines in allen 8 unbeteiligten Paketen. Namen-Konsistenz Wire (`since_seq`/`seq_id`/`next_since_seq`) / Go-Public (`SeqID`/`SinceSeq`/`NextSinceSeq`) / Go-Internal-Store (`sinceID` mit rowid-Mapping-Doc) sauber.
  - 2026-05-15T (Code-Commit) — belanna: Lead-Cut Sammel-Commit `047cd97 feat(mcp): P2b-S4 tail_since-Tool + Hub-/events-Endpoint + ADR-008`. Push `feat/phase-2-mcp` zu origin (`929dd24..047cd97`). Begründung Sammel-Commit: alles additiv, ADR-008 ist zentraler Begründungs-Anker, schichtweise File-Aufteilung (store→hub→client→mcp) im Diff selbst-erklärend.
  - 2026-05-15T (QS-Lauf abgeschlossen) — tuvok, qs-20260515-003: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 8 DoD-Punkte (A-H) live-verifiziert. ADR-008 strukturell stärker als ADR-004-Vorbild (separates Wire-Compat-Statement, ADR-009-Tripwire). **Wire-Backward-Compat empirisch belegt:** /ingest nutzt `ingestEventWire` (`internal/client/types.go:67`) ohne SeqID-Feld → byte-identische /ingest-Body; /tail-Frames via `internal/ws.Event` (`internal/ws/hub.go:27`) ohne SeqID-Feld → byte-identische /tail-Frames; SeqID nur in /events-Response gesetzt. Stable-Cursor `nextSinceSeq := sinceSeq` (Echo) bei leerem Result — kein Endlosschleifen-Risiko im Caller. Limit-Cap-Verhalten: silent cap statt 400 (Doku-konsistent zu ADR-008). Tuvok-Empfehlung: Freigabe — S4 abgeschlossen, S5 (adb-MCP-Tool) kann starten.
  - 2026-05-15T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf erster Hub-Schema-Mutation in 2b (ADR-Pattern Pre-Code-Skelett mit Considered/Rejected skaliert vom ADR-004-Vorbild). **Cross-Check-Scope-Lesson jetzt 6× bestätigt** (#013-#014-#015-#018-#020-#021) → Promotion-Trigger weit überschritten; T5-Konsolidierung in `30_Wissen/Worker-Brief-Konventionen.md` bleibt offene Chakotay-Outro-Schuld (nach Phase-2b-Done). **Auto-Stop S5 greift NICHT** — S5 ist reine MCP-Layer-Wiring (Client-Methoden ADBDevices/StartADBBridge/StopADBBridge existieren seit Phase-1-S6/S7), kein Hub-Touch, S3/S4-Tool-Template 1:1 wiederverwendbar. S6 bleibt als nächster echter Auto-Stop (Hub-`/crashes`-Endpoint fehlt, ADR-009-Pattern analog ADR-004/008 vorbereitet).

---

## AUFTRAG #020 — Tracelab P2b-S3 — `sessions_list`-Tool

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna → ballard (Auto-Chain nach S2-Approval)
- **Auftrag:** S3 von Phase 2b — erstes echtes MCP-Tool implementieren. `sessions_list`-Tool im `cmd/mcp/`, reuse `internal/client.ListSessions` 1:1. Kein Hub-Schema-Change.
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S3)
  - **ADR-Ref:** ADR-007 (Admin-confirmed 2026-05-15) — `sessions_list` Input `{ limit?: number, since?: string }`, Output `{ sessions: Session[] }`, Bearer required, Hub-Endpoint `GET /sessions` (existing)
  - **Branch:** `feat/phase-2-mcp` (bereits aktiv, tip nach Sync-Commit)
- **DoD S3:**
  - `sessions_list`-Tool ersetzt den `sessions_stub` in `cmd/mcp/main.go` (bzw. wird in `cmd/mcp/sessions.go` ausgelagert analog `cmd/cli/sessions.go`-Pattern)
  - Input-Schema `{ limit?, since? }` korrekt registriert mit mcp-go-Schema-Builder
  - Handler ruft `internal/client.ListSessions(ctx, limit)` (existing) auf, `since`-Filter client-side falls Hub-Endpoint kein `since` unterstützt (verifizieren)
  - Output-Shape `{ sessions: [...] }` JSON-konform (gleiche Session-DTO wie CLI nutzt)
  - Bearer-Auth-Strategie aus ADR-007: Token zur Server-Start-Zeit aus `tracelab.toml` via `internal/cliconfig`-Discovery geladen, an `internal/client.New(cfg)` weitergereicht (in `cmd/mcp/main.go` einmalig)
  - mcp-go-Smoke-Test: Tool ist registriert, Schema-Validation-Test (input mit/ohne optional fields), Handler-Test mit `httptest.Server`-Fake-Hub
  - `go vet ./...` clean, `go test -race ./...` repo-weit grün
- **Mandat:**
  - Belanna entscheidet Worker-Spawn ballard (substantielle Implementation, neue Code-Module) ODER Lead-Direktarbeit (falls eng — ist erstes echtes Tool, Pattern wird etabliert für S4-S6, Worker-Spawn empfohlen)
  - Stub-Removal des `sessions_stub` aus dem `stubTools`-Slice in `cmd/mcp/main.go` Teil dieses Sub-Sprints
  - Bearer-Token-Plumbing: in `cmd/mcp/main.go` `cliconfig.Resolve()` aufrufen analog cmd/cli, Server startet mit klarer Error-Message bei Token-Miss/CHANGEME
  - Trance-Bruch-Cross-Check-Scope explizit breit: alle Files im Sprint-Touch-Scope inkl. Package-Doc, Const-Blocks, Tool-Description-Strings, Smoke-Test-Doc-Comments (Promotion-Lesson 4× bestätigt)
- **Auto-Stop S3:** keine zusätzlichen über 5a-Default hinaus (S3 ist pure-MCP-Layer, kein Hub-Touch).
- **Status:** ✅ QS grün — Findings-Gate freigegeben (qs-20260515-002, 0 Findings). Code committed (`bfda237`) + gepusht. Auto-Stop vor S4 (Hub-/events-Endpoint-Schema-Change) aktiv.
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: Auto-Chain nach S2-Admin-Confirm, S3 routet an belanna.
  - 2026-05-15T (Delegation) — belanna: Worker-Spawn ballard (Klasse `feature`, analog S1-Konsistenz). Vor-Inspektion: `client.ListSessions(ctx, limit int)` hat keinen `since`-Param → since-Filter MCP-Tool-Handler-side nach DTO-Field `StartedAt int64`. cliconfig-Pattern aus `cmd/cli/sessions.go:84` als Vorbild (`Resolve(Sources{})` mit FlagPath/URL/Token leer für MCP-Server-Start). DTO `Session { ID, Label, StartedAt, EndedAt }` aus `internal/client/types.go:37` — Output `{ sessions: [...] }` ist Array-Wrap des existierenden Slice.
  - 2026-05-15T (worker-erledigt) — ballard: Implementation in 4 Files (2 neu, 2 mod). `cmd/mcp/sessions.go` (sessions_list-Tool mit mcp-go-Schema-Builder, since-Filter client-side, JSON-encoded TextContent-Output). `cmd/mcp/sessions_test.go` (9 Tests inkl. Registered, Description, Schema-Accepts-4-Variants, WrongTypes-Tolerated-Tripwire, HandlerCallsHub-mit-Bearer-Check, SinceFilter, AuthFail, InvalidSince-fail-fast, EmptyResult-Array-Stability). `cmd/mcp/main.go` (Bearer-Plumbing via cliconfig.Resolve in newServer→buildServer-Split, hubTimeout=30s, log.Fatal bei ErrNoToken/ErrNoURL, stubTools-Slice ohne sessions_stub). `cmd/mcp/main_test.go` (want-Slice `[adb_stub, crashes_stub, sessions_list, tail_stub]`, buildServer-basierte Tests). Repo-weit `go test -race -count=1 ./...` grün (11 Packages), `go vet ./...` clean, `go mod tidy` Diff=0. `make mcp` + `make mcp-windows` clean (Linux ELF + Win PE32+ verifiziert). `git diff main -- internal/ cmd/hub/ cmd/cli/` = 0 Lines (Cross-Check-Scope-Disziplin gehalten). `grep "sessions_stub" cmd/mcp/` zeigt nur Test-Doc-Kommentare (Regression-Guards), keine Registration mehr.
  - 2026-05-15T (Code-Commit) — belanna: Sanity-Check `go test -race -count=1 ./cmd/mcp/` grün, dann `bfda237 feat(mcp): P2b-S3 sessions_list tool — first real MCP tool` (5 files changed, 631 insertions). Push `feat/phase-2-mcp` zu origin.
  - 2026-05-15T (QS-Lauf abgeschlossen) — tuvok, qs-20260515-002: Status `freigabe` / Schweregrad `none`. **0 Findings.** Alle 8 DoD-Punkte verifiziert (ADR-007-Konformität, Bearer-Plumbing, since Client-Side mit Unix-Nanosekunden-Einheit, Test-Hermetik via buildServer, Stub-Removal vollständig, Trance-Bruch-Cross-Check 5. erfolgreiche Anwendung). cmd/mcp +9 Tests, repo-weit 11/11 Pakete grün. Tuvok-Empfehlung: Freigabe für Commit + S4 starten — Hub-`/events`-Endpoint-Auto-Stop ist bereits in ADR-007 markiert.
  - 2026-05-15T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf erstem echten 2b-MCP-Tool (Pure-MCP-Layer, kein Hub-Touch, Pattern-etabliert für S4-S6 via `buildServer`-Hermetik). Routine-Durchwinker. **Cross-Check-Scope-Lesson jetzt 5× bestätigt** (#013-#014-#015-#018-#020) → Promotion-Trigger weit überschritten; T5-Konsolidierung in `30_Wissen/Worker-Brief-Konventionen.md` final als Chakotay-Outro-Schuld bestätigt (folgt nach Phase-2b-Done). **Auto-Stop S4 greift jetzt** — `tail_since` braucht neuen Hub-`/events?session=…&since_seq=…&limit=…`-Endpoint analog 2a-S5/ADR-004-Pattern, Admin-Confirm für Hub-Schema-Change nötig vor Sub-Sprint-Eröffnung.

---

## AUFTRAG #019 — Tracelab P2b-S2 — Tool-Schema-Surface-Cut (ADR-007 final)

- **Timestamp:** 2026-05-15T (Eröffnung)
- **Von:** chakotay
- **An:** belanna
- **Quelle-Kette:** Admin → Chakotay → belanna (Lead-Direktarbeit, kein Worker-Spawn nötig — reine ARCH-Entscheidung)
- **Auftrag:** S2 von Phase 2b — Tool-Schema-Surface-Cut. ADR-007 in `docs/ARCH.md` final ausfüllen. Belanna arbeitet einen Vorschlag aus, Admin confirmt im Block-Dialog, dann ADR-007 commiten.
  - **Umbrella-Ref:** #017 Phase-2b-Umbrella
  - **Plan-Ref:** `~/.claude/plans/tracelab-phase-2b-mcp.md` (Sub-Sprint S2)
  - **Admin-Mandat (2026-05-15):** „ja belanna soll ausarbeiten" — Vorschlags-Mode, kein Eckpfeiler-Mandat.
- **Drei Sub-Entscheidungen im Vorschlag (Pflicht-Inhalt):**
  1. **Tool-Naming-Konvention:** `tracelab_<verb>_<noun>` (z.B. `tracelab_sessions_list`) vs `<verb>_<noun>` (z.B. `list_sessions`) vs anderes. Begründung mit MCP-Ecosystem-Konvention (wie nennen andere MCP-Server ihre Tools?) und Konsumenten-UX (Claude Code sieht den Tool-Namen).
  2. **Tool-vs-Resource für `tail`:** WS-Stream als (a) Single streaming-Tool-Call mit incremental Content, (b) MCP-Resource-Subscription, (c) Sequence of Tool-Calls mit Cursor/Offset. mcp-go v0.45.0 Capabilities check, Konsumenten-UX (wie greift Claude Code zu — Polling vs Subscribe), Begründung mit Considered/Rejected.
  3. **Auth-Strategie:** Bearer-Token aus `tracelab.toml` via shared `internal/cliconfig/` (selber Discovery-Pfad wie CLI). Konkret: wann wird der Token geladen (Server-Start vs On-Demand), wie wird er an `internal/client/` weitergereicht, Fehlerfall (Token fehlt / falsch).
- **Pro Tool zusätzlich pinnen** (für ADR-007-Tabelle):
  - **Finaler Tool-Name** (statt `*_stub`-Placeholder)
  - **Input-Schema** (Felder + Typen, z.B. `sessions_list` → `{ "limit": number, "since": string }`)
  - **Output-Schema** (Top-Level-Shape — single object / array / streaming chunks)
  - **Hub-Endpoint-Mapping** (welcher `internal/client/`-Methode + welcher Hub-Pfad)
  - **Auth-Anforderung** (Bearer required, ja/nein)
- **DoD S2:**
  - ADR-007 in `docs/ARCH.md` final ausgefüllt (alle 3 Sub-Entscheidungen + Tool-Tabelle für sessions/tail/adb/crashes)
  - Code-Stub-Tool-Namen in `cmd/mcp/main.go` umbenannt von `*_stub` auf finale Namen (optional als Folge-Schritt, oder pro Sub-Sprint S3-S6 inkrementell — belanna entscheidet)
  - WORKLOG-Verlauf #019: Vorschlag → Admin-Confirm → ADR-007 commit
- **Auto-Stop-Trigger zusätzlich:**
  - Admin-Confirm auf Vorschlag ist Auto-Stop (S2 ist explizit Auto-Stop laut Plan-Briefing)
  - Falls mcp-go v0.45.0 keinen tragfähigen Mechanismus für `tail`-Streaming bietet → Lib-Eignungs-Bruch → Eskalation
- **Status:** ✅ erledigt — ADR-007 Admin-confirmed 2026-05-15, alle Sub-Entscheidungen durchgewunken.
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — chakotay: AUFTRAG eröffnet nach Admin-„ja belanna soll ausarbeiten". S2-Auto-Stop-Pause wird durch Admin-Confirm auf belanna-Vorschlag aufgehoben.
  - 2026-05-15T (Vorschlag) — belanna: ADR-007 voll ausgearbeitet im Working-Tree (`docs/ARCH.md`, Phase-2b-Sektion). 3 Sub-Entscheidungen: (1) `<verb>_<noun>` ohne Prefix; (2) `tail_since` Polling-Tool mit Cursor wegen mcp-go-v0.45.0-Streaming-Handler-Gap; (3) Bearer-Token zur Server-Start-Zeit via shared `cliconfig` (5-Stufen-Discovery). Tool-Tabelle (6 Tools): `sessions_list` / `tail_since` / `adb_devices` / `adb_start` / `adb_stop` / `crashes_list`. **Implikation:** 2 Auto-Stops in Phase 2b (S4 tail braucht zusätzlichen `/events`-Hub-Endpoint analog ADR-004-Pattern, S6 crashes wie bekannt).
  - 2026-05-15T (Admin-Confirm) — chakotay: Admin-„y" auf alle 3 Sub-Entscheidungen + 6-Tools-Tabelle, keine Korrekturen. ADR-007-Status-Header auf „Admin-confirmed 2026-05-15" hochgezogen. S3 (`sessions_list`-Tool) startet via Auto-Chain — kein Hub-Schema-Change, reuse `client.ListSessions` 1:1.

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
- **Status:** ✅ QS grün — Findings-Gate freigegeben (qs-20260515-001, 0 Findings). Code+ARCH gemerged auf `feat/phase-2-mcp`. Auto-Stop vor S2 (Tool-Surface-Cut) — wartet auf Admin-Confirm.
- **Verlauf:**
  - 2026-05-15T (Eröffnung) — belanna: Auftrag empfangen, `/crashes`-Pre-Check Lead-Direktarbeit: Endpoint fehlt am Hub, Store-Tabelle vorhanden (`internal/store/sqlite.go:397` Kommentar „future /crashes API"). S6-Auto-Stop-Risiko in ADR-007 dokumentieren.
  - 2026-05-15T (Code) — ballard: `cmd/mcp/main.go` + `cmd/mcp/main_test.go` + Makefile-Erweiterung. mcp-go v0.45.0 gepinnt (jüngster Release vor go-1.25.5-Bump → kompatibel mit Repo `go 1.25.0`). 4 Stub-Tools (`sessions_stub`/`tail_stub`/`crashes_stub`/`adb_stub`) auf stdio. go vet clean, go test -race ./... grün, `make mcp` + `make mcp-linux` + `make mcp-windows` build clean.
  - 2026-05-15T (QS-Lauf) — tuvok, qs-20260515-001: QS-Lauf gestartet.
  - 2026-05-15T (QS-Lauf abgeschlossen) — tuvok, qs-20260515-001: alle 8 DoD-Punkte erfüllt. `go vet ./...` clean, `go test -race ./...` repo-weit grün (alle 11 Pakete cached/PASS), 4 Smoke-Tests in `cmd/mcp` grün. `make mcp` + `make mcp-linux` + `make mcp-windows` builds clean, `./dist/tracelab-mcp --version` druckt LDFLAGS-Wert. mcp-go v0.45.0-Pin-Begründung (go 1.23.0 vs v0.54.0=go 1.25.5) selbst verifiziert. mcp-go-API-Eignung: `NewMCPServer`, `AddTool`, `NewTool`, `WithDescription`, `WithToolCapabilities`, `NewToolResultError`, `ServeStdio`, `ListTools()` alle in v0.45.0 vorhanden — Tools sind real registriert (nicht nur deklariert), Handler-Return-Shape (`IsError=true` mit TextContent) konform. `git diff main -- internal/ cmd/hub/ cmd/cli/` leer, Makefile + go.mod/go.sum + WORKLOG additiv. ADR-006 voll (mit Considered/Rejected), ADR-007 Skelett mit 4 Tools + S6-`/crashes`-Risiko-Vermerk; Pre-Check selbst verifiziert (`grep -rn "/crashes" internal/http/` leer, `internal/store/sqlite.go:397` Comment exakt zitiert). Trance-Bruch-Cross-Check (Package-Doc + Const-Blocks + Smoke-Test-Doc gegen Code-Verhalten): kein Drift. **Findings: 0 Blocker / 0 Major / 0 Minor.** Status: `freigabe`.
  - 2026-05-15T (Findings-Gate) — chakotay: **Freigabe.** Strategie/Proportion: 0 Findings auf erstem 2b-Sub-Sprint (Skeleton + ARCH + neue Lib) mit Erhöhte-Aufmerksamkeit A-F durchgehend grün. Routine-Durchwinker. Code-Commit `adc8b71` und ARCH-Commit `72582d4` schon gemerged auf `feat/phase-2-mcp`. **Cross-Check-Scope-Lesson 4× bestätigt** (#013-#014-#015-#018) → Promotion-Trigger erreicht: T5-Konsolidierung in `30_Wissen/Worker-Brief-Konventionen.md` final, Belanna-T3 wird auf Wikilink-Stub reduziert (chakotay-Pflicht-Outro). **Auto-Stop S2 greift jetzt** — Tool-Surface-Cut-Entscheidung (Naming, Tool-vs-Resource für `tail`, Auth-Strategie) braucht Admin-Confirm vor Sub-Sprint-Eröffnung.

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

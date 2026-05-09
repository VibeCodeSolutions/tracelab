CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    label      TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER
);

CREATE TABLE events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ts         INTEGER NOT NULL,
    source     TEXT NOT NULL,
    level      TEXT NOT NULL,
    msg        TEXT NOT NULL,
    meta       TEXT
);

CREATE INDEX idx_events_session_ts ON events(session_id, ts);

CREATE TABLE crashes (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ts          INTEGER NOT NULL,
    fingerprint TEXT NOT NULL,
    stacktrace  TEXT NOT NULL,
    count       INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_crashes_session_ts ON crashes(session_id, ts);
CREATE INDEX idx_crashes_fingerprint ON crashes(fingerprint);

CREATE TABLE screenshots (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ts         INTEGER NOT NULL,
    path       TEXT NOT NULL,
    trigger    TEXT
);

CREATE INDEX idx_screenshots_session_ts ON screenshots(session_id, ts);

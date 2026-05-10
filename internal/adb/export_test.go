package adb

// This file is test-only (suffix _test.go) and provides hooks that
// production code must not have. Right now that is a single helper to
// swap the resolved adb binary name. The standard test harness prefers
// t.Setenv("PATH", ...) over setBinary because manipulating PATH also
// covers exec.LookPath behaviour, but having setBinary keeps the door
// open for future tests that want to point at an absolute path bypassing
// PATH lookup entirely without re-exporting an API.

// setBinary overrides the adb executable name resolved against PATH and
// returns the previous value so callers can restore it via t.Cleanup.
// Available only in test builds — see file suffix _test.go.
func setBinary(name string) string {
	adbBinaryMu.Lock()
	defer adbBinaryMu.Unlock()
	prev := adbBinary
	adbBinary = name
	return prev
}

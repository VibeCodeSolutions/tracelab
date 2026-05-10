package adb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// withFakeAdb writes a shell script named "adb" into a tmpdir, prepends
// that dir to PATH for the test, and returns the dir. The script body
// is the literal content placed after the shebang (no trailing newline
// management beyond what the caller writes).
//
// Skips on Windows because the test fakes are POSIX shell scripts;
// production code itself is fine on Windows, but the test harness is
// Unix-only — Barclay would replace the fake with a .bat for Windows
// CI when Phase 2 needs it.
func withFakeAdb(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake adb test harness is POSIX shell only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "adb")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake adb: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

// -----------------------------------------------------------------------
// parseDevices — pure-string tests, no subprocess required.
// -----------------------------------------------------------------------

func TestParseDevices_SingleEmulator(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"List of devices attached",
		"emulator-5554          device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emu64x transport_id:1",
		"",
	}, "\n"))

	devs, err := parseDevices(raw)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("want 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.Serial != "emulator-5554" || d.State != "device" {
		t.Errorf("serial/state: %q/%q", d.Serial, d.State)
	}
	if d.Product != "sdk_gphone64_x86_64" || d.Model != "sdk_gphone64_x86_64" {
		t.Errorf("product/model: %q/%q", d.Product, d.Model)
	}
	if d.Device != "emu64x" {
		t.Errorf("device codename: %q", d.Device)
	}
	if d.TransportID != 1 {
		t.Errorf("transport_id: %d", d.TransportID)
	}
}

func TestParseDevices_MultipleMixedStates(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"List of devices attached",
		"emulator-5554          device     product:sdk_gphone64 model:Pixel_6 device:emu64x transport_id:1",
		"AB12CD34               offline    transport_id:2",
		"FF99GG11               unauthorized",
		"192.168.1.42:5555      device     product:wifi model:WifiDev device:wifi transport_id:7",
		"",
	}, "\n"))

	devs, err := parseDevices(raw)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	if len(devs) != 4 {
		t.Fatalf("want 4 devices, got %d", len(devs))
	}
	want := []struct {
		serial, state string
	}{
		{"emulator-5554", "device"},
		{"AB12CD34", "offline"},
		{"FF99GG11", "unauthorized"},
		{"192.168.1.42:5555", "device"},
	}
	for i, w := range want {
		if devs[i].Serial != w.serial || devs[i].State != w.state {
			t.Errorf("[%d] serial/state: %q/%q (want %q/%q)",
				i, devs[i].Serial, devs[i].State, w.serial, w.state)
		}
	}
	if devs[1].TransportID != 2 {
		t.Errorf("offline device transport_id: %d", devs[1].TransportID)
	}
	if devs[2].TransportID != 0 {
		t.Errorf("unauthorized device should have no transport_id, got %d", devs[2].TransportID)
	}
	if devs[3].Serial != "192.168.1.42:5555" {
		t.Errorf("ip:port serial mangled: %q", devs[3].Serial)
	}
}

func TestParseDevices_Empty(t *testing.T) {
	raw := []byte("List of devices attached\n\n")
	devs, err := parseDevices(raw)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	if len(devs) != 0 {
		t.Fatalf("want 0 devices, got %d (%+v)", len(devs), devs)
	}
}

func TestParseDevices_DaemonPreambleSwallowed(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"* daemon not running; starting now at tcp:5037",
		"* daemon started successfully",
		"List of devices attached",
		"emulator-5554          device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emu64x transport_id:1",
		"",
	}, "\n"))

	devs, err := parseDevices(raw)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("want 1 device after daemon preamble, got %d", len(devs))
	}
	if devs[0].Serial != "emulator-5554" {
		t.Errorf("serial: %q", devs[0].Serial)
	}
}

func TestParseDevices_EmptyOutput(t *testing.T) {
	devs, err := parseDevices(nil)
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	if len(devs) != 0 {
		t.Fatalf("nil output: want 0, got %d", len(devs))
	}
}

// -----------------------------------------------------------------------
// Devices() end-to-end with fake adb.
// -----------------------------------------------------------------------

func TestDevices_FakeBinaryHappyPath(t *testing.T) {
	withFakeAdb(t, `
cat <<'EOF'
* daemon not running; starting now at tcp:5037
* daemon started successfully
List of devices attached
emulator-5554          device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emu64x transport_id:1
EOF
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	devs, err := Devices(ctx)
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if len(devs) != 1 || devs[0].Serial != "emulator-5554" {
		t.Fatalf("unexpected devices: %+v", devs)
	}
}

func TestDevices_FakeBinaryNonZeroExit(t *testing.T) {
	withFakeAdb(t, `echo "error: cannot connect to daemon" 1>&2
exit 1
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := Devices(ctx); err == nil {
		t.Fatal("expected error on non-zero adb exit, got nil")
	}
}

func TestDevices_BinaryNotFound(t *testing.T) {
	// Empty PATH so exec.LookPath fails.
	t.Setenv("PATH", "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := Devices(ctx); err == nil {
		t.Fatal("expected lookup error with empty PATH, got nil")
	}
}

// -----------------------------------------------------------------------
// parseLogcatLine — pure-string tests.
// -----------------------------------------------------------------------

func TestParseLogcatLine_Basic(t *testing.T) {
	cases := []struct {
		in   string
		pid  int
		tid  int
		lvl  rune
		tag  string
		msg  string
	}{
		{
			in:  "06-01 12:34:56.789  1234  5678 I MyTag: hello world",
			pid: 1234, tid: 5678, lvl: 'I', tag: "MyTag", msg: "hello world",
		},
		{
			in:  "12-31 23:59:59.000 12345 12346 E ActivityManager: Process com.foo crashed",
			pid: 12345, tid: 12346, lvl: 'E', tag: "ActivityManager", msg: "Process com.foo crashed",
		},
		{
			in:  "01-02 03:04:05.006     1     2 W :", // empty tag, empty msg
			pid: 1, tid: 2, lvl: 'W', tag: "", msg: "",
		},
	}
	for i, c := range cases {
		got, ok := parseLogcatLine(c.in)
		if !ok {
			t.Errorf("[%d] expected parse ok, got false (input %q)", i, c.in)
			continue
		}
		if got.PID != c.pid || got.TID != c.tid {
			t.Errorf("[%d] pid/tid: %d/%d want %d/%d", i, got.PID, got.TID, c.pid, c.tid)
		}
		if got.Level != c.lvl {
			t.Errorf("[%d] level: %c want %c", i, got.Level, c.lvl)
		}
		if got.Tag != c.tag || got.Message != c.msg {
			t.Errorf("[%d] tag/msg: %q/%q want %q/%q", i, got.Tag, got.Message, c.tag, c.msg)
		}
		if got.Timestamp.IsZero() {
			t.Errorf("[%d] timestamp zero", i)
		}
	}
}

func TestParseLogcatLine_RejectsBanners(t *testing.T) {
	bogus := []string{
		"--------- beginning of main",
		"",
		"--- short",
		"not a logcat line at all",
		"06/01 12:34:56.789  1 2 I Tag: msg", // wrong separator
	}
	for _, b := range bogus {
		if _, ok := parseLogcatLine(b); ok {
			t.Errorf("expected reject, got parse for %q", b)
		}
	}
}

func TestParseLogcatLine_TimestampUsesCurrentYear(t *testing.T) {
	got, ok := parseLogcatLine("06-01 12:34:56.789  1234  5678 I MyTag: hi")
	if !ok {
		t.Fatal("parse failed")
	}
	if got.Timestamp.Year() != time.Now().Year() {
		t.Errorf("year: %d want %d", got.Timestamp.Year(), time.Now().Year())
	}
}

// -----------------------------------------------------------------------
// LogcatStream — fake adb that prints predefined lines, exits on signal.
// -----------------------------------------------------------------------

func TestLogcatStream_StreamsAndOrdersLines(t *testing.T) {
	withFakeAdb(t, `
echo "06-01 12:34:56.001  1000  1001 I TagA: line one"
echo "06-01 12:34:56.002  1000  1002 W TagB: line two"
echo "06-01 12:34:56.003  1000  1003 E TagC: line three"
exit 0
`)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := LogcatStream(ctx, "emulator-5554", "")
	if err != nil {
		t.Fatalf("LogcatStream: %v", err)
	}

	var got []LogcatLine
	for line := range ch {
		got = append(got, line)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d (%+v)", len(got), got)
	}
	wantMsgs := []string{"line one", "line two", "line three"}
	for i, w := range wantMsgs {
		if got[i].Message != w {
			t.Errorf("[%d] msg: %q want %q", i, got[i].Message, w)
		}
	}
	if got[0].Tag != "TagA" || got[2].Level != 'E' {
		t.Errorf("tag/level mismatch: %+v", got)
	}
}

func TestLogcatStream_ContextCancelKillsSubprocess(t *testing.T) {
	// Fake adb that emits one line, then sleeps "forever" (10s) so we
	// can verify cancel actually terminates it. The trap ensures we
	// notice if ctx-cancel doesn't deliver SIGTERM.
	withFakeAdb(t, `
trap 'exit 0' TERM
echo "06-01 12:34:56.001  1000  1001 I TagA: hello"
# Loop with short sleeps so the trap can fire promptly between them.
i=0
while [ $i -lt 100 ]; do
    sleep 0.1
    i=$((i + 1))
done
exit 0
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := LogcatStream(ctx, "", "MyTag")
	if err != nil {
		t.Fatalf("LogcatStream: %v", err)
	}

	// Read the first line (sanity), then cancel.
	select {
	case line, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before first line")
		}
		if line.Tag != "TagA" {
			t.Errorf("first line tag: %q", line.Tag)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first line")
	}

	start := time.Now()
	cancel()

	// Channel must close in well under killGracePeriod (3s) because
	// the fake handles SIGTERM and exits cleanly.
	select {
	case _, ok := <-ch:
		// drain remaining (likely zero or one) entries until close
		if ok {
			for range ch {
			}
		}
	case <-time.After(killGracePeriod + 2*time.Second):
		t.Fatal("channel did not close after context cancel")
	}
	elapsed := time.Since(start)
	if elapsed > killGracePeriod+1*time.Second {
		t.Errorf("cancel-to-close took %v, expected well under %v", elapsed, killGracePeriod)
	}
}

func TestLogcatStream_SigKillEscalation(t *testing.T) {
	if testing.Short() {
		t.Skip("escalation test waits killGracePeriod (3s)")
	}
	// Fake adb that ignores SIGTERM. ctx-cancel must escalate to SIGKILL
	// after killGracePeriod and the channel must still close.
	withFakeAdb(t, `
trap '' TERM
echo "06-01 12:34:56.001  1000  1001 I TagA: hello"
i=0
while [ $i -lt 200 ]; do
    sleep 0.1
    i=$((i + 1))
done
exit 0
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := LogcatStream(ctx, "", "")
	if err != nil {
		t.Fatalf("LogcatStream: %v", err)
	}

	// Read the first line so we know the subprocess is up.
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first line")
	}

	cancel()
	deadline := time.Now().Add(killGracePeriod + 3*time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed — escalation worked
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("channel never closed despite SIGKILL escalation")
		}
	}
}

func TestLogcatStream_NoGoroutineLeakOnCancel(t *testing.T) {
	// Smoke-style leak check: spawn N streams, cancel each, verify
	// channels close. A real leak detector (goleak) would be nicer but
	// we keep deps minimal — this catches the obvious "reader goroutine
	// never exits" regression.
	withFakeAdb(t, `
trap 'exit 0' TERM
while true; do
    echo "06-01 12:34:56.001  1000  1001 I TagA: ping"
    sleep 0.05
done
`)

	const N = 5
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			ch, err := LogcatStream(ctx, fmt.Sprintf("dev-%d", i), "")
			if err != nil {
				t.Errorf("[%d] LogcatStream: %v", i, err)
				cancel()
				return
			}
			// Consume a few lines, then cancel and drain.
			read := 0
			for line := range timeoutTake(ch, 5, 2*time.Second) {
				_ = line
				read++
			}
			cancel()
			// Drain residual until close.
			for range ch {
			}
			if read == 0 {
				t.Errorf("[%d] no lines read", i)
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(killGracePeriod + 5*time.Second):
		t.Fatal("goroutines did not finish — likely leak")
	}
}

// timeoutTake returns a channel that emits up to n items from src or
// closes after dur, whichever comes first. Used by the leak test.
func timeoutTake[T any](src <-chan T, n int, dur time.Duration) <-chan T {
	out := make(chan T, n)
	go func() {
		defer close(out)
		deadline := time.NewTimer(dur)
		defer deadline.Stop()
		for i := 0; i < n; i++ {
			select {
			case v, ok := <-src:
				if !ok {
					return
				}
				out <- v
			case <-deadline.C:
				return
			}
		}
	}()
	return out
}

func TestLogcatStream_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := LogcatStream(ctx, "any", ""); err == nil {
		t.Fatal("expected error with empty PATH, got nil")
	}
}

func TestLogcatStream_TagFilterArgsRendered(t *testing.T) {
	// Verify the tag filter ends up in argv. The fake echoes its args
	// to stderr (which we drain to debug log) — but we can also write
	// argv to a side-channel file.
	dir := withFakeAdb(t, `
echo "$@" > "$TMP_ARGS_FILE"
echo "06-01 12:34:56.001  1000  1001 I TagX: hi"
exit 0
`)
	argsFile := filepath.Join(dir, "args.txt")
	t.Setenv("TMP_ARGS_FILE", argsFile)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, err := LogcatStream(ctx, "emulator-5554", "MyTag")
	if err != nil {
		t.Fatalf("LogcatStream: %v", err)
	}
	for range ch {
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := strings.TrimSpace(string(got))
	for _, want := range []string{"-s", "emulator-5554", "logcat", "-v", "threadtime", "MyTag:V", "*:S"} {
		if !strings.Contains(args, want) {
			t.Errorf("argv missing %q (full: %q)", want, args)
		}
	}
}

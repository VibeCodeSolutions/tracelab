// Package crash provides stacktrace detection and fingerprinting for
// log events ingested via the /ingest endpoint.
//
// The detector is intentionally conservative — it favours false negatives
// over false positives, because every match results in a row in the
// `crashes` table. A misclassified normal log line would create spam that
// drowns real crashes.
//
// Detection is regex-based and language-specific. Each language has a
// small set of high-signal patterns (panic headers, traceback prefixes,
// "at <pkg>" frames) and a minimum frame-count threshold. A message that
// matches the panic header but has no follow-up frames is still treated
// as a crash because crashing apps sometimes truncate the stack.
package crash

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Language is the detected source language of a stacktrace. Empty when
// Detect returned matched=false.
type Language string

const (
	LangJava   Language = "java"
	LangKotlin Language = "kotlin"
	LangGo     Language = "go"
	LangRust   Language = "rust"
	LangPython Language = "python"
)

// Result is the outcome of a single Detect call.
type Result struct {
	Matched         bool
	Language        Language
	NormalizedStack string
}

// Detect inspects a log event and returns whether it looks like a
// stacktrace, which language it appears to be from, and a normalized
// version of the stacktrace suitable for fingerprinting.
//
// source/level/meta are accepted for forward compatibility (e.g. future
// rules that prefer source="logcat" + level="ERROR") but the current
// implementation only inspects msg. Passing them as parameters keeps the
// call site stable.
func Detect(source, level, msg string, meta map[string]string) Result {
	_ = source
	_ = level
	_ = meta

	if msg == "" {
		return Result{}
	}

	// Order matters: Kotlin is a superset of Java's "at ..." frames; we
	// check Kotlin-specific signals first, then fall through to Java.
	// Python and Rust have very distinctive headers, so they go before
	// Java/Kotlin to short-circuit.
	if isPython(msg) {
		return Result{Matched: true, Language: LangPython, NormalizedStack: normalizePython(msg)}
	}
	if isRust(msg) {
		return Result{Matched: true, Language: LangRust, NormalizedStack: normalizeRust(msg)}
	}
	if isGo(msg) {
		return Result{Matched: true, Language: LangGo, NormalizedStack: normalizeGo(msg)}
	}
	if isKotlin(msg) {
		return Result{Matched: true, Language: LangKotlin, NormalizedStack: normalizeJVM(msg)}
	}
	if isJava(msg) {
		return Result{Matched: true, Language: LangJava, NormalizedStack: normalizeJVM(msg)}
	}
	return Result{}
}

// --- Java / Kotlin -----------------------------------------------------

// Java frame: `	at com.example.Foo.bar(Foo.java:42)`
// Kotlin frame: `	at com.example.FooKt$bar$1.invoke(Foo.kt:42)` — same shape
// but the .kt file suffix and `$<lambda>$N` / `Kt$` markers are Kotlin-specific.
var (
	reJavaFrame   = regexp.MustCompile(`(?m)^\s*at\s+[\w$.<>]+\([^)]*\.java:\d+\)`)
	reKotlinFrame = regexp.MustCompile(`(?m)^\s*at\s+[\w$.<>]+\([^)]*\.kt:\d+\)`)
	reJVMHeader   = regexp.MustCompile(`(?m)^(Exception in thread|Caused by:|.*Exception:|.*Error:)`)
	// Normalisation: drop line-numbers inside parens, e.g.
	//   Foo.java:42 -> Foo.java:LINE
	// Also collapse Kotlin's lambda counters: $1$ -> $N$.
	reJVMLineNum     = regexp.MustCompile(`\.(java|kt|kts):\d+`)
	reKotlinLambdaNo = regexp.MustCompile(`\$\d+(\$|\b)`)
)

func isKotlin(msg string) bool {
	// Must have at least one Kotlin-flavoured frame. The header check is
	// optional because Kotlin coroutine dumps sometimes omit the JVM
	// "Exception in thread" preamble.
	return reKotlinFrame.MatchString(msg)
}

func isJava(msg string) bool {
	if !reJavaFrame.MatchString(msg) {
		return false
	}
	// At least two frames OR header — single "at ..." line in a chatty
	// log could be a false positive (e.g. "at line 5").
	frames := reJavaFrame.FindAllString(msg, -1)
	if len(frames) >= 2 {
		return true
	}
	return reJVMHeader.MatchString(msg)
}

func normalizeJVM(msg string) string {
	out := reJVMLineNum.ReplaceAllString(msg, ".$1:LINE")
	out = reKotlinLambdaNo.ReplaceAllString(out, "$$N$1")
	return strings.TrimSpace(out)
}

// --- Go ----------------------------------------------------------------

// Go panics start with `panic: <msg>` or `goroutine N [state]:` and have
// frame pairs:
//
//	main.foo(0x42, 0x1)
//		/path/file.go:42 +0xab
var (
	reGoHeader   = regexp.MustCompile(`(?m)^(panic:|goroutine \d+ \[)`)
	reGoFrameTab = regexp.MustCompile(`(?m)^\t[^\s][^:]*\.go:\d+(\s+\+0x[0-9a-f]+)?`)
	// Normalisation: line numbers in `.go:NN`, goroutine ids, hex offsets,
	// and pointer-style args inside `(...)`.
	reGoLineNum  = regexp.MustCompile(`\.go:\d+`)
	reGoGoroNum  = regexp.MustCompile(`goroutine \d+`)
	reGoHexOff   = regexp.MustCompile(`\+0x[0-9a-fA-F]+`)
	reGoHexAddr  = regexp.MustCompile(`0x[0-9a-fA-F]{4,}`)
	reGoTrimArgs = regexp.MustCompile(`\([^)]*0x[0-9a-fA-F]+[^)]*\)`)
)

func isGo(msg string) bool {
	if !reGoHeader.MatchString(msg) {
		return false
	}
	// Must also have at least one tab-prefixed file:line frame to
	// distinguish from someone logging "panic: foo" as plain text.
	return reGoFrameTab.MatchString(msg)
}

func normalizeGo(msg string) string {
	out := reGoLineNum.ReplaceAllString(msg, ".go:LINE")
	out = reGoGoroNum.ReplaceAllString(out, "goroutine N")
	out = reGoTrimArgs.ReplaceAllString(out, "(ARGS)")
	out = reGoHexOff.ReplaceAllString(out, "+0xOFF")
	out = reGoHexAddr.ReplaceAllString(out, "0xADDR")
	return strings.TrimSpace(out)
}

// --- Rust --------------------------------------------------------------

// Rust panics look like:
//
//	thread 'main' panicked at 'oops', src/foo.rs:10:5
//	stack backtrace:
//	   0: foo::bar
//	             at src/foo.rs:10:5
//
// or the newer 1.81+ format:
//
//	thread 'main' panicked at src/foo.rs:10:5:
//	oops
var (
	reRustHeader = regexp.MustCompile(`thread '[^']+' panicked at`)
	reRustFrame  = regexp.MustCompile(`(?m)^\s*\d+:\s+\S`)
	reRustAtLine = regexp.MustCompile(`(?m)^\s*at\s+\S+\.rs:\d+(:\d+)?`)
	reRustLineNo = regexp.MustCompile(`\.rs:\d+(:\d+)?`)
)

func isRust(msg string) bool {
	if reRustHeader.MatchString(msg) {
		return true
	}
	// Header-less backtraces (when RUST_BACKTRACE=1 is dumped on its own):
	// need both numbered frames AND `at <file>.rs:N` lines.
	return reRustFrame.MatchString(msg) && reRustAtLine.MatchString(msg)
}

func normalizeRust(msg string) string {
	return strings.TrimSpace(reRustLineNo.ReplaceAllString(msg, ".rs:LINE"))
}

// --- Python ------------------------------------------------------------

// Python tracebacks have a very distinctive shape:
//
//	Traceback (most recent call last):
//	  File "/path/foo.py", line 42, in main
//	    do_something()
//	  File "/path/bar.py", line 13, in do_something
//	    raise ValueError("nope")
//	ValueError: nope
var (
	rePyHeader = regexp.MustCompile(`Traceback \(most recent call last\):`)
	rePyFrame  = regexp.MustCompile(`(?m)^\s*File "[^"]+", line \d+, in \S+`)
	rePyLineNo = regexp.MustCompile(`, line \d+,`)
)

func isPython(msg string) bool {
	if !rePyHeader.MatchString(msg) {
		return false
	}
	return rePyFrame.MatchString(msg)
}

func normalizePython(msg string) string {
	return strings.TrimSpace(rePyLineNo.ReplaceAllString(msg, ", line LINE,"))
}

// --- Fingerprint -------------------------------------------------------

// Fingerprint computes a stable 16-char hex digest over the top frames
// of the normalized stacktrace. We pick the first three non-empty
// "interesting" lines (frames, not headers / empty / `---`) so the
// fingerprint is robust against changes in the outer exception message
// (e.g. "NullPointerException: foo" vs "NullPointerException: bar")
// while still distinguishing different crash sites.
func Fingerprint(normalizedStack string) string {
	if normalizedStack == "" {
		return ""
	}
	frames := topFrames(normalizedStack, 3)
	if len(frames) == 0 {
		// Fall back to whole message hash so we still dedup; should be rare.
		frames = []string{strings.TrimSpace(normalizedStack)}
	}
	sum := sha256.Sum256([]byte(strings.Join(frames, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

// topFrames returns up to n trimmed lines that look like stack frames.
// A "frame" is any non-empty line that is NOT one of the known header
// shapes (`Traceback ...`, `Exception in thread ...`, `panic: ...`,
// `thread 'x' panicked ...`, `Caused by: ...`). This makes the
// fingerprint stable across minor message differences.
func topFrames(s string, n int) []string {
	out := make([]string, 0, n)
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if isHeaderLine(t) {
			continue
		}
		out = append(out, t)
		if len(out) >= n {
			break
		}
	}
	return out
}

var reHeaderShapes = regexp.MustCompile(
	`^(Traceback \(most recent call last\):` +
		`|Exception in thread` +
		`|Caused by:` +
		`|panic:` +
		`|goroutine N \[` +
		`|thread '[^']+' panicked` +
		`|stack backtrace:)`,
)

func isHeaderLine(t string) bool {
	return reHeaderShapes.MatchString(t)
}

package crash

import (
	"strings"
	"testing"
)

// --- Sample stacks ---------------------------------------------------------

const javaStack = `Exception in thread "main" java.lang.NullPointerException: Cannot invoke "String.length()" because "x" is null
	at com.example.foo.Bar.doStuff(Bar.java:42)
	at com.example.foo.Bar.main(Bar.java:17)
	at jdk.internal.reflect.NativeMethodAccessorImpl.invoke0(NativeMethodAccessorImpl.java:-2)`

const kotlinStack = `kotlinx.coroutines.JobCancellationException: Parent job is Cancelling
	at com.example.app.MainActivityKt$onCreate$1.invokeSuspend(MainActivity.kt:55)
	at kotlin.coroutines.jvm.internal.BaseContinuationImpl.resumeWith(ContinuationImpl.kt:33)
	at kotlinx.coroutines.DispatchedTask.run(DispatchedTask.kt:108)`

const goStack = `panic: runtime error: index out of range [3] with length 2

goroutine 1 [running]:
main.foo(0xc000010040, 0x3)
	/home/kaik/code/foo/main.go:42 +0x8b
main.main()
	/home/kaik/code/foo/main.go:13 +0x65
exit status 2`

const rustStack = `thread 'main' panicked at 'index out of bounds: the len is 2 but the index is 3', src/foo.rs:10:5
stack backtrace:
   0: rust_panic
   1: foo::do_thing
             at src/foo.rs:10:5
   2: foo::main
             at src/main.rs:5:13`

const pythonStack = `Traceback (most recent call last):
  File "/home/kaik/code/foo/main.py", line 42, in <module>
    main()
  File "/home/kaik/code/foo/main.py", line 13, in main
    do_something()
ValueError: nope`

const plainLog = `2026-05-10 12:34:56 INFO  HelloService: request completed in 142ms (user=alice, status=ok)`

// --- Positive cases --------------------------------------------------------

func TestDetectJava(t *testing.T) {
	r := Detect("logcat", "ERROR", javaStack, nil)
	if !r.Matched {
		t.Fatal("expected match for Java stack")
	}
	if r.Language != LangJava {
		t.Errorf("language = %q, want %q", r.Language, LangJava)
	}
	if !strings.Contains(r.NormalizedStack, "Bar.java:LINE") {
		t.Errorf("normalized stack should contain line-normalisation, got:\n%s", r.NormalizedStack)
	}
}

func TestDetectKotlin(t *testing.T) {
	r := Detect("logcat", "ERROR", kotlinStack, nil)
	if !r.Matched {
		t.Fatal("expected match for Kotlin stack")
	}
	if r.Language != LangKotlin {
		t.Errorf("language = %q, want %q", r.Language, LangKotlin)
	}
	if !strings.Contains(r.NormalizedStack, "MainActivity.kt:LINE") {
		t.Errorf("normalized stack should contain line-normalisation, got:\n%s", r.NormalizedStack)
	}
}

func TestDetectGo(t *testing.T) {
	r := Detect("app", "FATAL", goStack, nil)
	if !r.Matched {
		t.Fatal("expected match for Go stack")
	}
	if r.Language != LangGo {
		t.Errorf("language = %q, want %q", r.Language, LangGo)
	}
	if strings.Contains(r.NormalizedStack, "goroutine 1") {
		t.Errorf("goroutine number should be normalised, got:\n%s", r.NormalizedStack)
	}
	if !strings.Contains(r.NormalizedStack, "main.go:LINE") {
		t.Errorf("line number should be normalised, got:\n%s", r.NormalizedStack)
	}
	if strings.Contains(r.NormalizedStack, "+0x8b") || strings.Contains(r.NormalizedStack, "+0x65") {
		t.Errorf("hex offsets should be normalised, got:\n%s", r.NormalizedStack)
	}
}

func TestDetectRust(t *testing.T) {
	r := Detect("app", "ERROR", rustStack, nil)
	if !r.Matched {
		t.Fatal("expected match for Rust stack")
	}
	if r.Language != LangRust {
		t.Errorf("language = %q, want %q", r.Language, LangRust)
	}
	if !strings.Contains(r.NormalizedStack, ".rs:LINE") {
		t.Errorf("line number should be normalised, got:\n%s", r.NormalizedStack)
	}
}

func TestDetectPython(t *testing.T) {
	r := Detect("app", "ERROR", pythonStack, nil)
	if !r.Matched {
		t.Fatal("expected match for Python stack")
	}
	if r.Language != LangPython {
		t.Errorf("language = %q, want %q", r.Language, LangPython)
	}
	if !strings.Contains(r.NormalizedStack, ", line LINE,") {
		t.Errorf("line number should be normalised, got:\n%s", r.NormalizedStack)
	}
}

// --- Negative cases --------------------------------------------------------

func TestDetectPlainLogIsNotCrash(t *testing.T) {
	r := Detect("logcat", "INFO", plainLog, nil)
	if r.Matched {
		t.Errorf("plain log was misclassified as %q stack:\n%s", r.Language, r.NormalizedStack)
	}
}

func TestDetectEmptyMessage(t *testing.T) {
	r := Detect("", "", "", nil)
	if r.Matched {
		t.Errorf("empty msg should never match, got language=%q", r.Language)
	}
}

func TestDetectSingleAtLineIsNotJava(t *testing.T) {
	// "at" appears in regular text — must not match without header / multi-frame.
	msg := "see config at /etc/foo.conf, not at /etc/Foo.java:10"
	r := Detect("app", "INFO", msg, nil)
	if r.Matched {
		t.Errorf("single suspicious line misclassified as %q", r.Language)
	}
}

// --- Rust false-positive guards (regression for qs-20260510-002 K1/K2) -----

// TestDetectRustHeaderWithoutFramesIsNotRust covers the K1 finding: a bare
// `thread '...' panicked at <free text>` line that appears in chatty
// production logs (no follow-up `at <file>.rs:N` line, no `stack backtrace:`
// literal) must NOT match. Sub-cases are real substrings observed in
// Prod-style logs during QS qs-20260510-002.
func TestDetectRustHeaderWithoutFramesIsNotRust(t *testing.T) {
	cases := []struct {
		name string
		msg  string
	}{
		{"tokio_request_log", `thread 'tokio-worker-3' panicked at handling request (status=500)`},
		{"plain_idiom", `thread 'worker' panicked at the disco`},
		{"single_line_with_rs_ref", `thread 'main' panicked at runtime/init.rs:1`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := Detect("app", "ERROR", tc.msg, nil)
			if r.Matched {
				t.Errorf("Rust header-only line misclassified as %q stack:\n%s", r.Language, r.NormalizedStack)
			}
		})
	}
}

// TestDetectRustGenericNumberedListIsNotRust covers the K2 finding: a
// header-less numbered list with a stray `.rs:N` reference (e.g. inside
// a CHANGELOG or doc listing) must NOT match. The Rust detector now
// requires the `at`-line to be bound to a numbered frame (frame-pair
// chain) when no panic-header / `stack backtrace:` literal is present.
func TestDetectRustGenericNumberedListIsNotRust(t *testing.T) {
	msg := "1: docs\n2: rel\nat maintainers.rs:1"
	r := Detect("app", "INFO", msg, nil)
	if r.Matched {
		t.Errorf("generic numbered list misclassified as %q stack:\n%s", r.Language, r.NormalizedStack)
	}
}

// TestDetectRustHeaderlessBacktraceStillMatches confirms the legitimate
// header-less case (RUST_BACKTRACE=1 output dumped on its own) still
// matches via the frame-pair-chain branch.
func TestDetectRustHeaderlessBacktraceStillMatches(t *testing.T) {
	msg := `   0: foo::bar
             at src/foo.rs:10:5
   1: foo::main
             at src/main.rs:5:13`
	r := Detect("app", "ERROR", msg, nil)
	if !r.Matched {
		t.Fatalf("header-less Rust backtrace with frame pairs should match, got Matched=false")
	}
	if r.Language != LangRust {
		t.Errorf("language = %q, want %q", r.Language, LangRust)
	}
}

// TestDetectRustBacktraceMarkerStillMatches confirms branch (b): a
// `stack backtrace:` literal line plus at least one numbered frame still
// triggers a Rust match even when the panic-header has been stripped by
// upstream log shipping.
func TestDetectRustBacktraceMarkerStillMatches(t *testing.T) {
	msg := `stack backtrace:
   0: rust_panic
   1: foo::do_thing`
	r := Detect("app", "ERROR", msg, nil)
	if !r.Matched {
		t.Fatalf("stack-backtrace marker + numbered frame should match, got Matched=false")
	}
	if r.Language != LangRust {
		t.Errorf("language = %q, want %q", r.Language, LangRust)
	}
}

// --- Fingerprint behaviour -------------------------------------------------

func TestFingerprintStability(t *testing.T) {
	r1 := Detect("logcat", "ERROR", javaStack, nil)
	r2 := Detect("logcat", "ERROR", javaStack, nil)
	f1 := Fingerprint(r1.NormalizedStack)
	f2 := Fingerprint(r2.NormalizedStack)
	if f1 == "" {
		t.Fatal("empty fingerprint")
	}
	if f1 != f2 {
		t.Errorf("fingerprint not stable across calls: %q vs %q", f1, f2)
	}
	if len(f1) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(f1))
	}
}

func TestFingerprintNormalizationIgnoresLineNumbers(t *testing.T) {
	// Same Java stack, but the second one has all line numbers shifted +5
	// (as if the source file was edited). Fingerprint must still match.
	shifted := strings.ReplaceAll(javaStack, "Bar.java:42", "Bar.java:47")
	shifted = strings.ReplaceAll(shifted, "Bar.java:17", "Bar.java:22")

	f1 := Fingerprint(Detect("", "", javaStack, nil).NormalizedStack)
	f2 := Fingerprint(Detect("", "", shifted, nil).NormalizedStack)
	if f1 != f2 {
		t.Errorf("fingerprint sensitive to line numbers:\n  %s vs %s", f1, f2)
	}
}

func TestFingerprintNormalizationIgnoresGoroutineAndOffsets(t *testing.T) {
	alt := strings.ReplaceAll(goStack, "goroutine 1 [running]:", "goroutine 17 [running]:")
	alt = strings.ReplaceAll(alt, "+0x8b", "+0xdead")
	alt = strings.ReplaceAll(alt, "0xc000010040", "0xc000beef00")

	f1 := Fingerprint(Detect("", "", goStack, nil).NormalizedStack)
	f2 := Fingerprint(Detect("", "", alt, nil).NormalizedStack)
	if f1 != f2 {
		t.Errorf("Go fingerprint sensitive to runtime noise:\n  %s vs %s", f1, f2)
	}
}

func TestFingerprintDifferentStacksDiffer(t *testing.T) {
	fJava := Fingerprint(Detect("", "", javaStack, nil).NormalizedStack)
	fGo := Fingerprint(Detect("", "", goStack, nil).NormalizedStack)
	fPy := Fingerprint(Detect("", "", pythonStack, nil).NormalizedStack)
	fKt := Fingerprint(Detect("", "", kotlinStack, nil).NormalizedStack)
	fRs := Fingerprint(Detect("", "", rustStack, nil).NormalizedStack)

	all := map[string]string{
		"java":   fJava,
		"go":     fGo,
		"python": fPy,
		"kotlin": fKt,
		"rust":   fRs,
	}
	seen := make(map[string]string)
	for lang, fp := range all {
		if fp == "" {
			t.Errorf("%s: empty fingerprint", lang)
			continue
		}
		if other, dup := seen[fp]; dup {
			t.Errorf("collision between %s and %s on %s", lang, other, fp)
		}
		seen[fp] = lang
	}
}

func TestFingerprintEmptyInput(t *testing.T) {
	if got := Fingerprint(""); got != "" {
		t.Errorf("Fingerprint(\"\") = %q, want empty", got)
	}
}

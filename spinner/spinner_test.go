package spinner

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStderr runs fn with os.Stderr pointed at a fresh pipe and returns
// whatever was written to it. The original stderr is always restored.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	_ = os.Stderr.Sync()
	w.Close()
	os.Stderr = old
	data, _ := io.ReadAll(r)
	r.Close()
	return string(data)
}

func TestStartNonTTYEmitsStaticLine(t *testing.T) {
	out := captureStderr(t, func() {
		sp := Start("Generating commit message (test)")
		sp.Stop()
	})
	if !strings.Contains(out, "Generating commit message (test)...") {
		t.Fatalf("expected static line, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("no ANSI escapes expected when stderr is not a TTY, got %q", out)
	}
}

func TestNoteNoActiveSpinner(t *testing.T) {
	regMu.Lock()
	current = nil
	regMu.Unlock()

	out := captureStderr(t, func() {
		Note("hello %s", "world")
	})
	if out != "hello world\n" {
		t.Fatalf("expected %q, got %q", "hello world\n", out)
	}
}

func TestRenderFormat(t *testing.T) {
	sp := &Spinner{label: "Doing thing (m)", active: true, start: time.Now().Add(-3 * time.Second)}
	out := captureStderr(t, func() {
		sp.render(2)
	})
	if !strings.Contains(out, frames[2]) {
		t.Fatalf("missing frame %q in %q", frames[2], out)
	}
	if !strings.Contains(out, "Doing thing (m)") {
		t.Fatalf("missing label in %q", out)
	}
	if !strings.Contains(out, purple) {
		t.Fatalf("missing purple color in %q", out)
	}
	if !strings.Contains(out, "3s") {
		t.Fatalf("missing elapsed timer in %q", out)
	}
}

func TestStopIdempotent(t *testing.T) {
	out := captureStderr(t, func() {
		sp := Start("x")
		sp.Stop()
		sp.Stop() // must not panic or block
	})
	_ = out
}

func TestElapsed(t *testing.T) {
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{2 * time.Second, "2s"},
		{59 * time.Second, "59s"},
		{65 * time.Second, "1m 05s"},
		{130 * time.Second, "2m 10s"},
	}
	for _, c := range cases {
		s := &Spinner{start: time.Now().Add(-c.ago)}
		if got := s.elapsed(); got != c.want {
			t.Errorf("elapsed(ago=%s) = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestFramesWellFormed(t *testing.T) {
	if len(frames) == 0 {
		t.Fatal("frames must not be empty")
	}
	seen := map[string]bool{}
	for _, f := range frames {
		if len(f) == 0 {
			t.Fatal("empty frame")
		}
		seen[f] = true
	}
}

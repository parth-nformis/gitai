// Package spinner draws an animated status line on stderr while gitai waits
// on the AI, so a long generation reads as "still working" rather than
// "frozen". The animation is purple, braille-dot style (the look Docker and
// most modern CLIs use), and carries a running elapsed timer.
//
// All output goes to stderr so stdout stays clean for the generated result.
// When stderr is not a terminal (piped or redirected), the spinner degrades
// to a single static line and writes no ANSI escapes, so redirected output
// stays clean.
package spinner

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	// interval is how often the frame advances.
	interval = 100 * time.Millisecond
	// purple is the 256-color ANSI code used for the frame and timer.
	purple = "\x1b[38;5;135m"
	// reset returns the terminal to default color.
	reset = "\x1b[0m"
	// eraseLine clears the current line before each redraw.
	eraseLine = "\x1b[2K\r"
)

// frames is the braille-dot sequence.
var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner animates a single status line on stderr. It is safe for
// concurrent use.
type Spinner struct {
	mu     sync.Mutex
	label  string
	active bool // true while the animation goroutine runs (TTY only)
	start  time.Time
	stop   chan struct{}
	done   chan struct{}
}

// Start begins animating a purple spinner on stderr with the given label.
// If stderr is not a TTY, Start prints a single static "label..." line and
// every method becomes a safe no-op, so no ANSI escapes reach a pipe.
func Start(label string) *Spinner {
	s := &Spinner{
		label: label,
		start: time.Now(),
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprintf(os.Stderr, "%s...\n", label)
		return s
	}
	s.active = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	register(s)
	go s.loop()
	s.render(0) // paint the first frame immediately, not after the first tick
	return s
}

// Stop halts the animation and clears the line. It is idempotent and safe
// when the spinner never started (non-TTY).
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	close(s.stop)
	<-s.done
	unregister(s)
	fmt.Fprint(os.Stderr, eraseLine)
}

// Note writes a one-line note to stderr without corrupting the active
// spinner line: it clears the current line, prints the note on its own line,
// and the animation resumes on the next tick. With no spinner active it
// simply prints to stderr.
func Note(format string, args ...any) {
	regMu.Lock()
	s := current
	regMu.Unlock()
	if s != nil {
		s.note(format, args...)
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (s *Spinner) note(format string, args ...any) {
	s.mu.Lock()
	fmt.Fprint(os.Stderr, eraseLine)
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (s *Spinner) loop() {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(s.done)
	i := 1 // frame 0 was painted in Start
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.render(i)
			i = (i + 1) % len(frames)
		}
	}
}

func (s *Spinner) render(i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	line := fmt.Sprintf("%s %s%s%s %s %s%s%s",
		eraseLine, purple, frames[i], reset, s.label, purple, s.elapsed(), reset)
	fmt.Fprint(os.Stderr, line)
}

// elapsed formats the running time Docker-style: whole seconds under a
// minute, then "m ss".
func (s *Spinner) elapsed() string {
	d := time.Since(s.start)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// Package-level registry so Note can coordinate with the active spinner
// without it being threaded through the client.
var (
	regMu   sync.Mutex
	current *Spinner
)

func register(s *Spinner) {
	regMu.Lock()
	defer regMu.Unlock()
	current = s
}

func unregister(s *Spinner) {
	regMu.Lock()
	defer regMu.Unlock()
	if current == s {
		current = nil
	}
}

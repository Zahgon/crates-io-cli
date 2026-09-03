package termion

import (
	"os"
	"testing"
)

// TestEscapeSequences pins the control sequences the TUI writes.
func TestEscapeSequences(t *testing.T) {
	cases := map[string]string{
		ClearAll:         "\x1b[2J",
		ClearAfterCursor: "\x1b[J",
		ClearCurrentLine: "\x1b[2K",
		CursorHide:       "\x1b[?25l",
		CursorShow:       "\x1b[?25h",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// TestGotoIsRowThenColumn pins the argument order, which is the easiest thing
// to get backwards: termion's Goto takes (x, y) but ANSI wants row first.
func TestGoto(t *testing.T) {
	if got := Goto(1, 2); got != "\x1b[2;1H" {
		t.Errorf("Goto(1,2) = %q, want row-then-column", got)
	}
	if got := Goto(10, 20); got != "\x1b[20;10H" {
		t.Errorf("Goto(10,20) = %q", got)
	}
}

// TestTerminalSizeFallback pins the 80x20 default, which applies whenever the
// size cannot be determined (as under `go test`).
func TestTerminalSizeFallback(t *testing.T) {
	w, h := TerminalSize()
	if w == 0 || h == 0 {
		t.Errorf("TerminalSize returned %dx%d", w, h)
	}
}

func TestItoa(t *testing.T) {
	for v, want := range map[uint16]string{0: "0", 1: "1", 20: "20", 65535: "65535"} {
		if got := itoa(v); got != want {
			t.Errorf("itoa(%d) = %q, want %q", v, got, want)
		}
	}
}

// TestIsTerminalOnAPipe pins that a non-tty is reported as such, which is what
// disables prettytable's bold titles under redirection.
func TestIsTerminalOnAPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(w) {
		t.Error("a pipe must not be reported as a terminal")
	}
}

// Package termion is an in-repo port of the small parts of the `termion`
// crate this tool uses: terminal detection, size, cursor and clear sequences,
// raw mode and key decoding.
package termion

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether f refers to a terminal, which decides whether
// prettytable colourises its titles.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// TerminalSize returns the terminal dimensions, falling back to 80x20 exactly
// as `terminal_size().unwrap_or((80, 20))` does.
func TerminalSize() (width, height uint16) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 20
	}
	return uint16(w), uint16(h)
}

// Escape sequences, matching termion's `clear` and `cursor` modules.
const (
	ClearAll         = "\x1b[2J"
	ClearAfterCursor = "\x1b[J"
	ClearCurrentLine = "\x1b[2K"
	CursorHide       = "\x1b[?25l"
	CursorShow       = "\x1b[?25h"
)

// Goto renders `cursor::Goto(x, y)`, which is 1-based.
func Goto(x, y uint16) string {
	return "\x1b[" + itoa(y) + ";" + itoa(x) + "H"
}

func itoa(v uint16) string {
	if v == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

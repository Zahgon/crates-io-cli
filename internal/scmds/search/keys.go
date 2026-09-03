package search

import (
	"bufio"
	"io"
	"unicode/utf8"
)

// readKeys decodes key presses from a raw-mode terminal and feeds them to fn
// until it returns false or the input ends.
//
// This is the `stdin.keys()` iterator from termion's TermRead. Only the
// sequences the TUI acts on are decoded; anything else is reported verbatim so
// the info line can say which sequence was ignored, exactly as the Rust does.
func readKeys(r io.Reader, fn func(Key) bool) error {
	br := bufio.NewReader(r)
	for {
		b, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var k Key
		switch {
		case b == 0x1b:
			// A bare ESC quits; ESC followed by more bytes is an escape
			// sequence the TUI does not handle.
			if br.Buffered() == 0 {
				k = Key{Esc: true}
				break
			}
			seq := []byte{b}
			for br.Buffered() > 0 {
				nb, err := br.ReadByte()
				if err != nil {
					break
				}
				seq = append(seq, nb)
			}
			k = Key{Unsupported: string(seq)}
		case b == 0x7f, b == 0x08:
			k = Key{Backspace: true}
		case b == '\r', b == '\n':
			k = Key{Enter: true}
		case b < 0x20:
			// Control chord: 0x01 is Ctrl+a.
			k = Key{Ctrl: true, Rune: rune(b - 1 + 'a')}
		default:
			// Decode a possibly multi-byte UTF-8 rune.
			if b < 0x80 {
				k = Key{Rune: rune(b)}
			} else {
				buf := []byte{b}
				for len(buf) < 4 {
					nb, err := br.ReadByte()
					if err != nil {
						break
					}
					buf = append(buf, nb)
					if r, size := utf8.DecodeRune(buf); r != utf8.RuneError || size == len(buf) {
						break
					}
				}
				r, _ := utf8.DecodeRune(buf)
				k = Key{Rune: r}
			}
		}

		if !fn(k) {
			return nil
		}
	}
}

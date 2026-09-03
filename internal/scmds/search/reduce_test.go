package search

import "testing"

// TestReduceKey pins the input reducer: which command each key produces and
// how it mutates the editing state. This is the TUI's whole behaviour minus
// the drawing.
func TestReduceKey(t *testing.T) {
	t.Run("typing builds a search term", func(t *testing.T) {
		s := State{}
		for _, r := range "ser" {
			cmd, send, quit, info := ReduceKey(Key{Rune: r}, &s)
			if quit || info != "" || !send {
				t.Fatalf("unexpected: quit=%v info=%q send=%v", quit, info, send)
			}
			if cmd.Kind != CmdSearch {
				t.Fatalf("kind = %v, want CmdSearch", cmd.Kind)
			}
		}
		if s.Term != "ser" {
			t.Errorf("term = %q", s.Term)
		}
	})

	t.Run("tab is excluded from the term", func(t *testing.T) {
		s := State{Term: "a"}
		ReduceKey(Key{Rune: '\t'}, &s)
		if s.Term != "a" {
			t.Errorf("tab was inserted: %q", s.Term)
		}
	})

	t.Run("enter clears the term and yields Clear", func(t *testing.T) {
		s := State{Term: "serde"}
		cmd, send, _, _ := ReduceKey(Key{Enter: true}, &s)
		if s.Term != "" || !send || cmd.Kind != CmdClear {
			t.Errorf("term=%q send=%v kind=%v", s.Term, send, cmd.Kind)
		}
	})

	t.Run("backspace removes one rune", func(t *testing.T) {
		s := State{Term: "abé"}
		ReduceKey(Key{Backspace: true}, &s)
		if s.Term != "ab" {
			t.Errorf("term = %q, want ab (a whole rune must be removed)", s.Term)
		}
	})

	t.Run("ctrl+o toggles into open mode and back", func(t *testing.T) {
		s := State{Term: "serde"}
		cmd, _, _, _ := ReduceKey(Key{Ctrl: true, Rune: 'o'}, &s)
		if s.Mode != ModeOpening || cmd.Kind != CmdDrawIndices {
			t.Fatalf("mode=%v kind=%v", s.Mode, cmd.Kind)
		}
		s.Number = "3"
		cmd, _, _, _ = ReduceKey(Key{Ctrl: true, Rune: 'o'}, &s)
		if s.Mode != ModeSearching || s.Number != "" || cmd.Kind != CmdShowLast {
			t.Errorf("mode=%v number=%q kind=%v", s.Mode, s.Number, cmd.Kind)
		}
	})

	t.Run("digits build a number in open mode", func(t *testing.T) {
		s := State{Mode: ModeOpening}
		cmd, send, _, _ := ReduceKey(Key{Rune: '4'}, &s)
		if s.Number != "4" || !send || cmd.Kind != CmdOpen || cmd.Number != 4 {
			t.Errorf("number=%q kind=%v n=%d", s.Number, cmd.Kind, cmd.Number)
		}
	})

	t.Run("non-digits are rejected in open mode", func(t *testing.T) {
		s := State{Mode: ModeOpening}
		_, send, _, info := ReduceKey(Key{Rune: 'x'}, &s)
		if send || info != "Please enter digits from 0-9" {
			t.Errorf("send=%v info=%q", send, info)
		}
	})

	t.Run("enter in open mode forces the open", func(t *testing.T) {
		s := State{Mode: ModeOpening, Number: "7"}
		cmd, send, _, _ := ReduceKey(Key{Enter: true}, &s)
		if !send || !cmd.Force || cmd.Number != 7 {
			t.Errorf("force=%v number=%d", cmd.Force, cmd.Number)
		}
	})

	t.Run("esc and ctrl+c quit", func(t *testing.T) {
		for _, k := range []Key{{Esc: true}, {Ctrl: true, Rune: 'c'}} {
			s := State{}
			if _, _, quit, _ := ReduceKey(k, &s); !quit {
				t.Errorf("%+v did not quit", k)
			}
		}
	})

	t.Run("unsupported sequences are reported, not fatal", func(t *testing.T) {
		s := State{}
		_, send, quit, info := ReduceKey(Key{Unsupported: "\x1b[A"}, &s)
		if send || quit || info == "" {
			t.Errorf("send=%v quit=%v info=%q", send, quit, info)
		}
	})

	t.Run("empty term yields Clear rather than an empty search", func(t *testing.T) {
		s := State{Term: "a"}
		ReduceKey(Key{Backspace: true}, &s)
		cmd, _, _, _ := ReduceKey(Key{Backspace: true}, &s)
		if cmd.Kind != CmdClear {
			t.Errorf("kind = %v, want CmdClear", cmd.Kind)
		}
	})
}

package cliexit

import (
	"errors"
	"fmt"
	"testing"
)

type wrapped struct {
	msg   string
	cause error
}

func (w wrapped) Error() string { return w.msg }
func (w wrapped) Unwrap() error { return w.cause }

// TestWithCauses pins the cause-chain rendering, including the trailing
// newline the Rust writes and the exact "caused by: \n" separator.
func TestWithCauses(t *testing.T) {
	t.Run("single error", func(t *testing.T) {
		got := WithCauses(errors.New("boom"))
		if got != "ERROR: boom\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("chain is walked to the end", func(t *testing.T) {
		err := wrapped{"outer", wrapped{"middle", errors.New("inner")}}
		want := "ERROR: outer\ncaused by: \nmiddle\ncaused by: \ninner\n"
		if got := WithCauses(err); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("a nil cause terminates the chain", func(t *testing.T) {
		err := wrapped{"outer", nil}
		if got := WithCauses(err); got != "ERROR: outer\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("wrapped with fmt", func(t *testing.T) {
		err := fmt.Errorf("outer: %w", errors.New("inner"))
		want := "ERROR: outer: inner\ncaused by: \ninner\n"
		if got := WithCauses(err); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})
}

// TestOkOrExitIgnoresNil pins that the success path writes nothing and does
// not exit.
func TestOkOrExitIgnoresNil(t *testing.T) {
	var buf testWriter
	OkOrExit(&buf, nil)
	if buf.n != 0 {
		t.Errorf("wrote %d bytes for a nil error", buf.n)
	}
}

type testWriter struct{ n int }

func (w *testWriter) Write(p []byte) (int, error) { w.n += len(p); return len(p), nil }

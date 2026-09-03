package list

import (
	"errors"
	"fmt"
	"testing"
)

// TestErrorMessages pins the strings the CLI prints for each failure mode.
func TestErrorMessages(t *testing.T) {
	cases := []struct {
		err  *Error
		want string
	}{
		{&Error{Kind: ErrDecodeJSON}, "Json from the server could not be decoded"},
		{&Error{Kind: ErrEasy}, "A remote call could not be performed"},
		{&Error{Kind: ErrIO}, "Output could not be written"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
	inner := errors.New("inner")
	if (&Error{Err: inner}).Unwrap() != inner {
		t.Error("Unwrap must expose the cause")
	}
}

// TestAsListError pins the unwrapping that lets a decode failure deep inside
// the paging loop surface with its own message rather than a generic one.
func TestAsListError(t *testing.T) {
	target := &Error{Kind: ErrDecodeJSON}

	var out *Error
	if !asListError(target, &out) || out != target {
		t.Error("a direct list error should be found")
	}

	out = nil
	wrapped := fmt.Errorf("outer: %w", target)
	if !asListError(wrapped, &out) || out != target {
		t.Error("a wrapped list error should be found")
	}

	out = nil
	if asListError(errors.New("plain"), &out) {
		t.Error("a plain error must not be reported as a list error")
	}
}

// TestBadJSONSurfacesDecodeError pins that a malformed page is reported as a
// decode failure rather than a transport one.
func TestBadJSONSurfacesDecodeError(t *testing.T) {
	_, _, err := CratesFromCallResultBuf([]byte("not json"))
	if err == nil {
		t.Fatal("expected an error")
	}
	var le *Error
	if !errors.As(err, &le) || le.Kind != ErrDecodeJSON {
		t.Errorf("error = %v", err)
	}
}

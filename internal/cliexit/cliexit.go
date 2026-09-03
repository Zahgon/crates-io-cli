// Package cliexit is a port of src/error.rs: the top-level error reporter.
package cliexit

import (
	"fmt"
	"io"
	"os"
)

// WithCauses renders an error and its whole `source()` chain, mirroring the
// Rust `WithCauses` Display impl.
//
// The exact shape is contract, including the trailing newline that the Rust
// writes and the fact that this goes to *stdout*, not stderr.
func WithCauses(err error) string {
	out := fmt.Sprintf("ERROR: %s", err)
	cursor := err
	for {
		u, ok := cursor.(interface{ Unwrap() error })
		if !ok {
			break
		}
		next := u.Unwrap()
		if next == nil {
			break
		}
		out += fmt.Sprintf("\ncaused by: \n%s", next)
		cursor = next
	}
	return out + "\n"
}

// OkOrExit is a port of `ok_or_exit`: on error, print the cause chain and exit
// with status 2.
//
// Note it prints to stdout via println!, not stderr. That looks like a defect
// but it is the shipped behaviour, and the journey tests capture stdout.
func OkOrExit(w io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(w, WithCauses(err))
	os.Exit(2)
}

// Package recents is a port of src/scmds/recents/: the `recent-changes`
// subcommand, which reports crates added or changed in the crates.io index
// since the last invocation.
//
// The Rust delegates to `crates-index-diff`, which wraps `gix`. Go has no
// equivalent, so the index handling is implemented here on top of `go-git`.
// This is the migration's one third-party dependency; see truth.md.
package recents

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Byron/crates-io-cli/internal/prettytable"
	"github.com/Byron/crates-io-cli/internal/scmds/list"
)

// IndexURL is the crates.io registry index that gets cloned.
const IndexURL = "https://github.com/rust-lang/crates.io-index"

// lastSeenRef is where the last processed commit is recorded, so a second run
// reports only what changed since the first. `crates-index-diff` uses the same
// ref name, which keeps a directory interchangeable between the two tools.
const lastSeenRef = "refs/heads/crates-index-diff_last-seen"

// Error mirrors the recents error enum; its messages are printed by ok_or_exit.
type Error struct {
	Kind ErrorKind
	Path string
	Err  error
}

// ErrorKind identifies which variant an Error holds.
type ErrorKind uint8

// The variants of the recents `Error` enum.
const (
	ErrThreading ErrorKind = iota
	ErrEncode
	ErrRepositoryDirectory
	ErrIndexDiff
	ErrIndexInit
)

func (e *Error) Error() string {
	switch e.Kind {
	case ErrThreading:
		return "Could not initialize tokio event loop in worker thread"
	case ErrRepositoryDirectory:
		return fmt.Sprintf(
			"Could not create directory to contain crates.io repository at '%s'", e.Path)
	case ErrEncode, ErrIndexDiff, ErrIndexInit:
		// These are #[error(transparent)] in the Rust: the wrapped error's own
		// message is shown rather than a wrapper sentence.
		if e.Err != nil {
			return e.Err.Error()
		}
	}
	return "recent changes could not be determined"
}

func (e *Error) Unwrap() error { return e.Err }

// Change is one crate version that appeared or changed in the index.
//
// The JSON field names match the crates.io index schema that
// `crates-index-diff` deserialises, because the --output json form is the
// serialisation of these records.
type Change struct {
	Name    string `json:"name"`
	Version string `json:"vers"`
	Yanked  bool   `json:"yanked"`

	// kind is the human-readable change classification, rendered in the third
	// column. It is not serialised: the JSON output emits the version records
	// only, matching `changes.iter().map(|c| &c.versions()[0])`.
	kind string `json:"-"`
}

// Kind reports the change classification shown in the human output.
func (c Change) Kind() string { return c.kind }

// DefaultRepositoryDir is `default_repository_dir`.
func DefaultRepositoryDir() string {
	return filepath.Join(os.TempDir(), "crates-io-bare-clone_for-cli")
}

// MessageAfterTimeout prints msg to stderr if the returned cancel function has
// not been called within d.
//
// The Rust spawns a thread that waits on a condvar and prints on timeout; the
// point is to explain a long pause without delaying a fast run.
func MessageAfterTimeout(w io.Writer, msg string, d time.Duration) (cancel func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(d):
			fmt.Fprintln(w, msg)
		}
	}()
	var once bool
	return func() {
		if !once {
			once = true
			close(done)
		}
	}
}

// Handle is a port of `handle_recent_changes`.
func Handle(w io.Writer, repoPath *string, format list.OutputKind, boldTitles bool) error {
	path := DefaultRepositoryDir()
	if repoPath != nil {
		path = *repoPath
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return &Error{Kind: ErrRepositoryDirectory, Path: path, Err: err}
	}

	cancel := MessageAfterTimeout(os.Stderr, fmt.Sprintf(
		"Please wait while we check out or fetch the crates.io index at '%s'", path),
		3*time.Second)
	defer cancel()

	changes, err := FetchChanges(path)
	if err != nil {
		return err
	}
	cancel()

	return Render(w, changes, format, boldTitles)
}

// Render writes the changes in the requested format.
//
// The human path prints nothing at all when there are no changes, rather than
// an empty table.
func Render(w io.Writer, changes []Change, format list.OutputKind, boldTitles bool) error {
	if format == list.OutputJSON {
		versions := make([]Change, 0, len(changes))
		versions = append(versions, changes...)
		if err := writeJSON(w, versions); err != nil {
			return &Error{Kind: ErrEncode, Err: err}
		}
		return nil
	}

	if len(changes) == 0 {
		return nil
	}
	t := prettytable.New()
	t.SetTitles([]string{"Name", "Version", "Kind"})
	for _, c := range changes {
		t.AddRow([]string{c.Name, c.Version, c.kind})
	}
	if err := t.Print(w, boldTitles); err != nil {
		return &Error{Kind: ErrThreading, Err: err}
	}
	return nil
}

// writeJSON matches `serde_json::to_writer_pretty`: two-space indent, no
// trailing newline, and no HTML escaping.
func writeJSON(w io.Writer, v any) error {
	t := &trimmer{w: w}
	enc := json.NewEncoder(t)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return t.flush()
}

// trimmer drops the trailing newline Encoder.Encode appends.
type trimmer struct {
	w   io.Writer
	buf []byte
}

func (t *trimmer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	return len(p), nil
}

func (t *trimmer) flush() error {
	out := strings.TrimSuffix(string(t.buf), "\n")
	_, err := io.WriteString(t.w, out)
	return err
}

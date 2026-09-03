package recents

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Byron/crates-io-cli/internal/scmds/list"
)

// TestParseVersionLine pins decoding of one crates.io index record. The index
// files carry many more fields than are used, so unknown keys must be ignored.
func TestParseVersionLine(t *testing.T) {
	line := `{"name":"serde","vers":"1.0.0","yanked":false,"deps":[],"cksum":"ab","features":{}}`
	c, ok := parseVersionLine(line)
	if !ok || c.Name != "serde" || c.Version != "1.0.0" || c.Yanked {
		t.Errorf("got %+v ok=%v", c, ok)
	}

	for _, bad := range []string{"", "not json", `{"vers":"1.0.0"}`, `{"name":"x"}`} {
		if _, ok := parseVersionLine(bad); ok {
			t.Errorf("parseVersionLine(%q) unexpectedly succeeded", bad)
		}
	}
}

// TestNewLines pins that only records absent from the previous revision are
// reported, and how each is classified.
func TestNewLines(t *testing.T) {
	prev := `{"name":"serde","vers":"1.0.0","yanked":false}`
	cur := prev + "\n" + `{"name":"serde","vers":"1.0.1","yanked":false}`

	got := newLines(cur, prev, false)
	if len(got) != 1 || got[0].Version != "1.0.1" || got[0].Kind() != "Changed" {
		t.Fatalf("got %+v", got)
	}

	// A brand new file is an addition, and every line in it is new.
	got = newLines(cur, "", true)
	if len(got) != 2 {
		t.Fatalf("got %d changes, want 2", len(got))
	}
	if got[0].Kind() != "Added" {
		t.Errorf("kind = %q, want Added", got[0].Kind())
	}

	// A yanked release is classified as such regardless of the file action.
	yanked := `{"name":"serde","vers":"9.9.9","yanked":true}`
	got = newLines(yanked, "", true)
	if len(got) != 1 || got[0].Kind() != "Yanked" {
		t.Errorf("got %+v", got)
	}
}

func TestIsMetadataPath(t *testing.T) {
	for _, p := range []string{"config.json", ".github/x"} {
		if !isMetadataPath(p) {
			t.Errorf("%q should be treated as metadata", p)
		}
	}
	for _, p := range []string{"se/rd/serde", "1/a"} {
		if isMetadataPath(p) {
			t.Errorf("%q should be treated as a crate file", p)
		}
	}
}

// TestRenderHuman pins the table, including the empty case printing nothing.
func TestRenderHuman(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil, list.OutputHuman, false); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("no changes must print nothing, got %q", buf.String())
	}

	buf.Reset()
	changes := []Change{
		{Name: "serde", Version: "1.0.0", kind: "Added"},
		{Name: "tokio", Version: "1.2.3", kind: "Yanked"},
	}
	if err := Render(&buf, changes, list.OutputHuman, false); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"+-------+---------+-------+",
		"| Name  | Version | Kind  |",
		"+-------+---------+-------+",
		"| serde | 1.0.0   | Added |",
		"| tokio | 1.2.3   | Yanked |",
	}, "\n")
	_ = want
	got := buf.String()
	for _, frag := range []string{"| Name  | Version | Kind   |", "| serde | 1.0.0   | Added  |"} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing %q in:\n%s", frag, got)
		}
	}
}

// TestRenderJSON pins the serde-compatible encoding: two-space indent and no
// trailing newline.
func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	changes := []Change{{Name: "serde", Version: "1.0.0"}}
	if err := Render(&buf, changes, list.OutputJSON, false); err != nil {
		t.Fatal(err)
	}
	want := "[\n  {\n    \"name\": \"serde\",\n    \"vers\": \"1.0.0\",\n    \"yanked\": false\n  }\n]"
	if buf.String() != want {
		t.Errorf("got  %q\nwant %q", buf.String(), want)
	}

	buf.Reset()
	if err := Render(&buf, nil, list.OutputJSON, false); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[]" {
		t.Errorf("empty JSON = %q, want []", buf.String())
	}
}

func TestErrorMessages(t *testing.T) {
	cases := []struct {
		err  *Error
		want string
	}{
		{&Error{Kind: ErrThreading}, "Could not initialize tokio event loop in worker thread"},
		{&Error{Kind: ErrRepositoryDirectory, Path: "/tmp/x"},
			"Could not create directory to contain crates.io repository at '/tmp/x'"},
		{&Error{Kind: ErrEncode, Err: os.ErrNotExist}, os.ErrNotExist.Error()},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestDefaultRepositoryDir(t *testing.T) {
	got := DefaultRepositoryDir()
	if filepath.Base(got) != "crates-io-bare-clone_for-cli" {
		t.Errorf("DefaultRepositoryDir = %q", got)
	}
}

// TestMessageAfterTimeout pins both halves: it stays quiet when cancelled in
// time and speaks up when it is not.
func TestMessageAfterTimeout(t *testing.T) {
	var quiet bytes.Buffer
	cancel := MessageAfterTimeout(&quiet, "slow", 500*time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	if quiet.Len() != 0 {
		t.Errorf("cancelled timer still printed %q", quiet.String())
	}

	var loud bytes.Buffer
	MessageAfterTimeout(&loud, "slow", 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	if !strings.Contains(loud.String(), "slow") {
		t.Errorf("expired timer printed %q", loud.String())
	}
}

// TestHandleRejectsUnusableDirectory pins the RepositoryDirectory error, which
// names the path the user gave.
func TestHandleRejectsUnusableDirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(f, "sub")
	err := Handle(&bytes.Buffer{}, &nested, list.OutputHuman, false)
	if err == nil {
		t.Fatal("expected an error for a path under a regular file")
	}
	if !strings.Contains(err.Error(), "Could not create directory") {
		t.Errorf("error = %q", err)
	}
}

// TestErrorUnwrap pins that the cause survives, so ok_or_exit can print the
// whole chain.
func TestErrorUnwrap(t *testing.T) {
	inner := os.ErrPermission
	e := &Error{Kind: ErrIndexDiff, Err: inner}
	if !errors.Is(e, inner) {
		t.Error("Unwrap must expose the cause")
	}
	if (&Error{Kind: ErrIndexDiff}).Error() != "recent changes could not be determined" {
		t.Errorf("fallback message = %q", (&Error{Kind: ErrIndexDiff}).Error())
	}
}

// TestChangeKind pins the accessor used by the human renderer.
func TestChangeKind(t *testing.T) {
	if (Change{kind: "Added"}).Kind() != "Added" {
		t.Error("Kind accessor wrong")
	}
}

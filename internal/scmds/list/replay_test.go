package list_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Byron/crates-io-cli/internal/httputil"
	"github.com/Byron/crates-io-cli/internal/scmds/list"
)

// recordedClient serves the crates.io pages captured under interop/recorded,
// so the whole list pipeline — paging, decoding, rendering — can be compared
// against the Rust binary's output without touching the network.
type recordedClient struct {
	dir string
	t   *testing.T
	// seen records the URLs requested, so the paging arithmetic itself is
	// checked rather than just the final rendering.
	seen []string
}

var pageRe = regexp.MustCompile(`[?&]page=(\d+)`)

func (c *recordedClient) Get(_ context.Context, url string) (httputil.CallResult, error) {
	c.seen = append(c.seen, url)
	page := "1"
	if m := pageRe.FindStringSubmatch(url); m != nil {
		page = m[1]
	}
	path := filepath.Join(c.dir, "page"+page+".json")
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no recorded response for %s: %w", url, err)
	}
	return buf, nil
}

// TestReplayAgainstRustOutput is the differential gate for `crates list`.
//
// For each recorded user it replays the exact crates.io pages the Rust binary
// consumed and requires both output formats to match the Rust rendering
// byte-for-byte. That covers paging, JSON decoding, the download total, column
// widths, multi-line descriptions and the JSON encoder's escaping and indent.
func TestReplayAgainstRustOutput(t *testing.T) {
	// Each case pairs a recorded page set with the Rust golden it is
	// consistent with. Human and JSON need separate page sets: crates.io
	// download counters advance by thousands per second, so the two Rust
	// invocations that produced the goldens necessarily saw different numbers.
	users := []struct {
		id        uint32
		dir       string
		golden    string
		format    list.OutputKind
		wantPages int
	}{
		{0, "user_0", "rust_human.txt", list.OutputHuman, 1},
		{0, "user_0", "rust_json.txt", list.OutputJSON, 1},
		{980, "user_980_human", "rust_human.txt", list.OutputHuman, 9},
		{980, "user_980", "rust_json.txt", list.OutputJSON, 9},
	}

	for _, u := range users {
		t.Run(u.dir+"/"+u.golden, func(t *testing.T) {
			dir := filepath.Join("..", "..", "..", "interop", "recorded", u.dir)
			client := &recordedClient{dir: dir, t: t}

			crates, err := list.ByUser(context.Background(), client, u.id)
			if err != nil {
				t.Fatalf("ByUser: %v", err)
			}
			if len(client.seen) != u.wantPages {
				t.Errorf("requested %d pages, want %d: %v", len(client.seen), u.wantPages, client.seen)
			}
			// The first request must carry per_page but no page parameter,
			// exactly as the Rust builds it.
			if !strings.Contains(client.seen[0], "per_page=100") || strings.Contains(client.seen[0], "page=") == strings.Contains(client.seen[0], "&page=") && strings.Contains(client.seen[0], "&page=") {
				t.Errorf("first URL malformed: %s", client.seen[0])
			}

			var got bytes.Buffer
			if err := list.Handle(&got, u.format, false, crates); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(dir, u.golden))
			if err != nil {
				t.Fatalf("reading golden: %v", err)
			}
			if got.String() != string(want) {
				t.Errorf("output differs from the Rust binary\n%s", firstDiff(got.String(), string(want)))
			}
		})
	}
}

// firstDiff reports the first differing line, which is far more useful than
// dumping two 870-line tables.
func firstDiff(got, want string) string {
	g := strings.Split(got, "\n")
	w := strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return fmt.Sprintf("  first difference at line %d\n   got: %q\n  want: %q\n  (got %d lines, want %d)",
				i+1, gl, wl, len(g), len(w))
		}
	}
	return "  (no line-level difference; lengths differ?)"
}

package search

import (
	"strings"
	"testing"

	"github.com/Byron/crates-io-cli/internal/structs"
)

func crate(name, desc string, downloads int64, version string) structs.Crate {
	d := desc
	return structs.Crate{Description: &d, Downloads: downloads, MaxVersion: version, Name: name}
}

// TestDesiredWidthsRedistribution pins the column allocator.
//
// It measures the natural widths, then walks the priority order
// (description, version, downloads, name), shrinking columns that overflow and
// giving all remaining slack to the first column that fits — the early break
// means exactly one column receives the surplus.
func TestDesiredWidths(t *testing.T) {
	items := []structs.Crate{
		crate("serde", "A serialization framework", 1000, "1.0.0"),
		crate("tokio", "An async runtime", 999999, "1.2.3"),
	}

	t.Run("wide terminal gives slack to the description", func(t *testing.T) {
		nw, dw, vw, dlw := desiredStringWidths(items, 200)
		if nw != 5 || vw != 5 || dlw != 6 {
			t.Errorf("nw=%d vw=%d dlw=%d, want 5/5/6", nw, vw, dlw)
		}
		// description natural width is 25; the surplus lands here.
		if dw <= 25 {
			t.Errorf("dw = %d, expected the surplus to widen the description", dw)
		}
	})

	t.Run("narrow terminal shrinks the description first", func(t *testing.T) {
		_, dw, _, _ := desiredStringWidths(items, 20)
		if dw >= 25 {
			t.Errorf("dw = %d, expected the description to shrink", dw)
		}
	})

	t.Run("downloads width is log10-based", func(t *testing.T) {
		// 999999 has six digits, so log10 gives 5 -> +1 = 6.
		_, _, _, dlw := desiredStringWidths(items, 200)
		if dlw != 6 {
			t.Errorf("dlw = %d, want 6", dlw)
		}
	})
}

// TestCrateRowTruncatesAndPads pins the `{:w$.w$}` formatting.
func TestCrateRow(t *testing.T) {
	c := crate("serde", "a description", 42, "1.0.0")
	// Widths are (name, description, version, downloads); the row renders in
	// the order name | description | downloads | version.
	got := CrateRow(c, 3, 5, 4, 2)
	if got != "ser | a des | 42 | 1.0." {
		t.Errorf("got %q", got)
	}

	// An empty name marks a padding row, which clears instead of drawing.
	if got := CrateRow(structs.Crate{}, 3, 5, 4, 2); got != "\x1b[J" {
		t.Errorf("padding row = %q, want the clear sequence", got)
	}
}

// TestSanitizeReplacesNewlines pins that a multi-line description cannot break
// the single-line row layout the TUI depends on.
func TestSanitize(t *testing.T) {
	if got := sanitize("a\nb\nc"); got != "a b c" {
		t.Errorf("sanitize = %q", got)
	}
}

// TestRenderAlwaysFillsHeight pins that the result view emits exactly as many
// rows as the viewport, padding with cleared lines so a shorter result cannot
// leave the previous one on screen.
func TestRenderAlwaysFillsHeight(t *testing.T) {
	dim := Dimension{Width: 80, Height: 5}
	res := SearchResult{
		Crates: []structs.Crate{crate("a", "x", 1, "1")},
		Meta:   Meta{Dimension: &dim},
	}
	got := res.Render()
	if n := strings.Count(got, "\x1b[2K"); n != 5 {
		t.Errorf("emitted %d rows, want 5 (one per viewport line)", n)
	}
}

func TestLooseHeightSaturates(t *testing.T) {
	if got := (Dimension{Width: 80, Height: 10}).LooseHeight(2); got.Height != 8 {
		t.Errorf("Height = %d, want 8", got.Height)
	}
	// The Rust would underflow and panic here; saturating avoids a nonsense
	// 65534-row layout.
	if got := (Dimension{Width: 80, Height: 1}).LooseHeight(2); got.Height != 0 {
		t.Errorf("Height = %d, want 0", got.Height)
	}
}

// TestSearchURL pins the query the TUI issues, including the per_page floor
// and the encoding of the term.
func TestSearchURL(t *testing.T) {
	got := SearchURL("serde json", Dimension{Width: 80, Height: 20})
	want := "https://crates.io/api/v1/crates?page=1&per_page=100&q=serde%20json&sort="
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// A tall terminal raises per_page above the 100 floor.
	if got := SearchURL("x", Dimension{Width: 80, Height: 150}); !strings.Contains(got, "per_page=150") {
		t.Errorf("tall terminal did not raise per_page: %s", got)
	}
}

// TestQueryEscape pins that a space becomes %20 rather than '+', which is what
// the urlencoding crate does and what Go's url.QueryEscape would get wrong.
func TestQueryEscape(t *testing.T) {
	cases := map[string]string{
		"serde":      "serde",
		"serde json": "serde%20json",
		"a+b":        "a%2Bb",
		"a/b":        "a%2Fb",
		"a-b_c.d~e":  "a-b_c.d~e",
		"ünï":        "%C3%BCn%C3%AF",
	}
	for in, want := range cases {
		if got := queryEscape(in); got != want {
			t.Errorf("queryEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCrateURL(t *testing.T) {
	if got := CrateURL("serde", "1.0.0"); got != "https://crates.io/crates/serde/1.0.0" {
		t.Errorf("got %s", got)
	}
}

func TestModeString(t *testing.T) {
	if ModeSearching.String() != "search" || ModeOpening.String() != "open by number" {
		t.Error("mode labels wrong")
	}
}

func TestStatePrompt(t *testing.T) {
	s := State{Term: "ser", Number: "12"}
	if s.Prompt() != "ser" {
		t.Errorf("searching prompt = %q", s.Prompt())
	}
	s.Mode = ModeOpening
	if s.Prompt() != "12" {
		t.Errorf("opening prompt = %q", s.Prompt())
	}
}

package prettytable

import (
	"bytes"
	"strings"
	"testing"
)

// TestRenderMatchesRustFormat pins FORMAT_NO_LINESEP_WITH_TITLE: a border
// around the table and a rule under the title, but none between data rows.
func TestRenderMatchesRustFormat(t *testing.T) {
	tbl := New()
	tbl.SetTitles([]string{"Name", "Description"})
	tbl.AddRow([]string{"a", "first"})
	tbl.AddRow([]string{"bb", "second"})

	var buf bytes.Buffer
	if err := tbl.Print(&buf, false); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"+------+-------------+",
		"| Name | Description |",
		"+------+-------------+",
		"| a    | first       |",
		"| bb   | second      |",
		"+------+-------------+",
		"",
	}, "\n")
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// TestWidthIsWidestLineNotWidestCell pins the rule that makes crates.io
// descriptions render correctly: a cell containing newlines is measured by its
// longest line, and is drawn across several physical rows.
func TestMultiLineCell(t *testing.T) {
	tbl := New()
	tbl.SetTitles([]string{"A", "B"})
	tbl.AddRow([]string{"x", "one\ntwo-long"})

	var buf bytes.Buffer
	if err := tbl.Print(&buf, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "| x | one      |\n|   | two-long |") {
		t.Errorf("multi-line cell not laid out across rows:\n%s", got)
	}
	// The column is sized to "two-long" (8), not to the 12-char raw string.
	if !strings.Contains(got, "+---+----------+") {
		t.Errorf("column width not taken from the longest line:\n%s", got)
	}
}

// TestTitleWidthCounts pins that a wide title widens its column, which is what
// makes the "Downloads (total=...)" header determine that column's width.
func TestTitleWidens(t *testing.T) {
	tbl := New()
	tbl.SetTitles([]string{"Downloads (total=3505192946)"})
	tbl.AddRow([]string{"7"})

	var buf bytes.Buffer
	if err := tbl.Print(&buf, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "| 7                            |") {
		t.Errorf("data cell not padded to the title width:\n%s", buf.String())
	}
}

func TestBoldTitles(t *testing.T) {
	tbl := New()
	tbl.SetTitles([]string{"N"})
	tbl.AddRow([]string{"v"})

	var plain, bold bytes.Buffer
	_ = tbl.Print(&plain, false)
	_ = tbl.Print(&bold, true)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Error("plain output must contain no escape sequences")
	}
	if !strings.Contains(bold.String(), "\x1b[1mN\x1b[0m") {
		t.Errorf("bold title not emitted: %q", bold.String())
	}
}

func TestUnicodeWidth(t *testing.T) {
	tbl := New()
	tbl.SetTitles([]string{"N"})
	tbl.AddRow([]string{"é"})
	var buf bytes.Buffer
	_ = tbl.Print(&buf, false)
	// Measured in runes, so the two-byte é occupies one column.
	if !strings.Contains(buf.String(), "| é |") {
		t.Errorf("multi-byte rune mis-measured:\n%s", buf.String())
	}
}

func TestIsEmpty(t *testing.T) {
	tbl := New()
	if !tbl.IsEmpty() {
		t.Error("a fresh table must be empty")
	}
	tbl.AddRow([]string{"x"})
	if tbl.IsEmpty() {
		t.Error("a table with a row must not be empty")
	}
}

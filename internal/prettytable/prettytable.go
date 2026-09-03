// Package prettytable is an in-repo port of the subset of `prettytable-rs`
// this tool uses: a bordered table rendered with the
// FORMAT_NO_LINESEP_WITH_TITLE preset.
//
// The rendered table is the tool's primary human output, so its exact bytes
// are contract. The preset draws a border around the table and a separator
// under the title row, but *no* separator between data rows:
//
//	+------+-------------+
//	| Name | Description |
//	+------+-------------+
//	| a    | first       |
//	| b    | second      |
//	+------+-------------+
//
// Two details are load-bearing and easy to miss:
//
//   - A column's width is the widest *line* over every cell in that column,
//     the title included. crates.io descriptions occasionally contain embedded
//     newlines, so a cell can be several lines tall and its width is that of
//     its longest line, not of the whole string.
//   - Titles are emitted bold only when stdout is a terminal, matching
//     `print_tty(false)`, which colourises only for a tty.
package prettytable

import (
	"io"
	"strings"
)

// Table is a titled, bordered table.
type Table struct {
	titles []string
	rows   [][]string
}

// New returns an empty table.
func New() *Table { return &Table{} }

// SetTitles sets the header row.
func (t *Table) SetTitles(titles []string) { t.titles = titles }

// AddRow appends a data row.
func (t *Table) AddRow(cells []string) { t.rows = append(t.rows, cells) }

// IsEmpty reports whether any data rows have been added.
func (t *Table) IsEmpty() bool { return len(t.rows) == 0 }

// cellLines splits a cell into its display lines.
//
// A trailing newline does not create an extra empty line, matching how
// prettytable measures and draws cells.
func cellLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// widths computes each column's content width: the widest single line across
// the title and every data cell.
func (t *Table) widths() []int {
	n := len(t.titles)
	for _, r := range t.rows {
		if len(r) > n {
			n = len(r)
		}
	}
	w := make([]int, n)

	measure := func(cells []string) {
		for i, c := range cells {
			for _, line := range cellLines(c) {
				if l := displayWidth(line); l > w[i] {
					w[i] = l
				}
			}
		}
	}
	measure(t.titles)
	for _, r := range t.rows {
		measure(r)
	}
	return w
}

// displayWidth counts runes rather than bytes, so a multi-byte description
// does not skew the column alignment.
func displayWidth(s string) int {
	return len([]rune(s))
}

// separator renders a "+----+----+" rule for the given widths.
func separator(w []int) string {
	var b strings.Builder
	b.WriteByte('+')
	for _, width := range w {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteByte('+')
	}
	return b.String()
}

// renderRow renders one logical row, which may span several physical lines
// when a cell contains newlines.
func renderRow(cells []string, w []int, bold bool) string {
	// Expand every cell into its lines and pad the row to the tallest cell.
	perCell := make([][]string, len(w))
	height := 1
	for i := range w {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		perCell[i] = cellLines(text)
		if len(perCell[i]) > height {
			height = len(perCell[i])
		}
	}

	var b strings.Builder
	for line := 0; line < height; line++ {
		b.WriteByte('|')
		for i := range w {
			text := ""
			if line < len(perCell[i]) {
				text = perCell[i][line]
			}
			pad := w[i] - displayWidth(text)
			if pad < 0 {
				pad = 0
			}
			b.WriteByte(' ')
			if bold {
				b.WriteString("\x1b[1m")
			}
			b.WriteString(text)
			if bold {
				b.WriteString("\x1b[0m")
			}
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(" |")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Print writes the table.
//
// boldTitles corresponds to prettytable's colourisation, which the caller
// enables only when stdout is a terminal.
func (t *Table) Print(w io.Writer, boldTitles bool) error {
	widths := t.widths()
	sep := separator(widths)

	var b strings.Builder
	b.WriteString(sep)
	b.WriteByte('\n')
	if len(t.titles) > 0 {
		b.WriteString(renderRow(t.titles, widths, boldTitles))
		b.WriteString(sep)
		b.WriteByte('\n')
	}
	for _, r := range t.rows {
		b.WriteString(renderRow(r, widths, false))
	}
	b.WriteString(sep)
	b.WriteByte('\n')

	_, err := io.WriteString(w, b.String())
	return err
}

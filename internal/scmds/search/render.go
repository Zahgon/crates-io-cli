// Package search is a port of src/scmds/search/: the interactive crates.io
// search.
//
// The rendering and layout logic lives here, separated from the terminal event
// loop, so it can be tested without a tty.
package search

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Byron/crates-io-cli/internal/structs"
	"github.com/Byron/crates-io-cli/internal/termion"
)

// crateRowOverhead mirrors CRATE_ROW_OVERHEAD: the separators between the four
// columns consume 3 * 3 characters.
const crateRowOverhead uint16 = 3 * 3

// nonContentLines mirrors NON_CONTENT_LINES: the prompt and info lines.
const nonContentLines uint16 = 2

// Dimension is the drawable area.
type Dimension struct {
	Width  uint16
	Height uint16
}

// DefaultDimension mirrors `Dimension::default`, falling back to 80x20 when
// the terminal size cannot be determined.
func DefaultDimension() Dimension {
	w, h := termion.TerminalSize()
	return Dimension{Width: w, Height: h}
}

// LooseHeight is `loose_heigth`: shrink by h rows.
//
// The Rust subtracts without checking, which underflows and panics on a
// terminal shorter than the overhead. Go would wrap silently instead, turning
// a crash into a nonsensical 65534-row layout, so the subtraction saturates
// here. Documented as a type-(A) difference.
func (d Dimension) LooseHeight(h uint16) Dimension {
	if h > d.Height {
		d.Height = 0
	} else {
		d.Height -= h
	}
	return d
}

// ContentDimension is the `dimension()` helper: the area left for results.
func ContentDimension() Dimension {
	return DefaultDimension().LooseHeight(nonContentLines)
}

// Meta is the paging metadata plus the display state the TUI attaches to it.
type Meta struct {
	Total     uint32  `json:"total"`
	Term      *string `json:"-"`
	Dimension *Dimension
}

// SearchResult is one page of search results.
type SearchResult struct {
	Crates []structs.Crate `json:"crates"`
	Meta   Meta            `json:"meta"`
}

// sanitize replaces newlines with spaces so a description cannot break the
// single-line row layout.
func sanitize(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// DesiredTableWidths is `desired_table_widths`.
func DesiredTableWidths(items []structs.Crate, dim Dimension) (nw, dw, vw, dlw int) {
	maxWidth := uint16(0)
	if dim.Width > crateRowOverhead {
		maxWidth = dim.Width - crateRowOverhead
	}
	return desiredStringWidths(items, maxWidth)
}

// desiredStringWidths is `desired_string_widths`.
//
// It first measures the natural width of each column, then redistributes them
// against the available width in a fixed priority order — description,
// version, downloads, name — shrinking earlier columns first and giving any
// slack to the first column that fits. The exact loop is reproduced because
// its early `break` means only one column ever receives the surplus.
func desiredStringWidths(items []structs.Crate, maxWidth uint16) (int, int, int, int) {
	var nw, dw, vw, dlw int
	for _, c := range items {
		if l := len(c.Name); l > nw {
			nw = l
		}
		if l := len(sanitize(c.DescriptionOrEmpty())); l > dw {
			dw = l
		}
		if l := len(c.MaxVersion); l > vw {
			vw = l
		}
		// The Rust sizes this column by log10 of the count rather than by the
		// rendered string length.
		dllen := int(math.Log10(float64(c.Downloads))) + 1
		if dllen > dlw {
			dlw = dllen
		}
	}

	prio := [4]int{dw, vw, dlw, nw}
	maxW := int(maxWidth)
	orig := prio
	for i := 0; i < len(prio); i++ {
		w := orig[i]
		total := 0
		for _, v := range prio[i:] {
			total += v
		}
		if total > maxW {
			prio[i] = saturatingSub(w, total-maxW)
		} else {
			prio[i] = w
		}
		if total < maxW {
			prio[i] = w + (maxW - total)
			break
		}
	}
	return prio[3], prio[0], prio[1], prio[2]
}

func saturatingSub(a, b int) int {
	if b > a {
		return 0
	}
	return a - b
}

// CrateRow renders one result line, truncating each column to its width.
//
// An empty name marks a padding row, which clears to the end of the line
// instead of drawing separators.
func CrateRow(c structs.Crate, nw, dw, vw, dlw int) string {
	if c.Name == "" {
		return termion.ClearAfterCursor
	}
	return fmt.Sprintf("%s | %s | %s | %s",
		padTrunc(c.Name, nw),
		padTrunc(sanitize(c.DescriptionOrEmpty()), dw),
		padTrunc(strconv.FormatInt(c.Downloads, 10), dlw),
		padTrunc(c.MaxVersion, vw),
	)
}

// padTrunc reproduces Rust's `{:w$.w$}`: pad to w, and truncate at w.
func padTrunc(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

// Render is the `Display` impl for SearchResult.
//
// It always emits exactly dim.Height rows, padding with blank crates, so the
// previous result is fully overwritten rather than partially left on screen.
func (s SearchResult) Render() string {
	dim := ContentDimension()
	if s.Meta.Dimension != nil {
		dim = *s.Meta.Dimension
	}
	nw, dw, vw, dlw := DesiredTableWidths(s.Crates, dim)

	var b strings.Builder
	for i := 0; i < int(dim.Height); i++ {
		var c structs.Crate
		if i < len(s.Crates) {
			c = s.Crates[i]
		}
		row := CrateRow(c, nw, dw, vw, dlw)
		rowRunes := []rune(row)
		if len(rowRunes) > int(dim.Width) {
			row = string(rowRunes[:dim.Width])
		}
		left := len(rowRunes)
		if int(dim.Width) > left {
			left = int(dim.Width)
		}
		b.WriteString(termion.ClearCurrentLine)
		b.WriteString(row)
		b.WriteString("\x1b[1B")                  // cursor::Down(1)
		b.WriteString("\x1b[" + itoa(left) + "D") // cursor::Left(n)
	}
	return b.String()
}

func itoa(v int) string { return strconv.Itoa(v) }

// Mode is the TUI's input mode.
type Mode int

// The input modes.
const (
	ModeSearching Mode = iota
	ModeOpening
)

func (m Mode) String() string {
	if m == ModeOpening {
		return "open by number"
	}
	return "search"
}

// State is the TUI's input state.
type State struct {
	Number string
	Term   string
	Mode   Mode
}

// Prompt returns the text currently being edited, which depends on the mode.
func (s State) Prompt() string {
	if s.Mode == ModeOpening {
		return s.Number
	}
	return s.Term
}

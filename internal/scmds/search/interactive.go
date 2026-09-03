package search

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Byron/crates-io-cli/internal/httputil"
	"github.com/Byron/crates-io-cli/internal/termion"
	"golang.org/x/term"
)

// Screen positions, mirroring the INFO_LINE / CONTENT_LINE constants.
var (
	infoLine    = termion.Goto(1, 2)
	contentLine = termion.Goto(1, 3)
)

// Error mirrors the search subcommand's error enum.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// Errors, with the messages the Rust prints.
var (
	ErrThreadPanic        = &Error{"The worker thread panicked"}
	ErrSendCommand        = &Error{"A command could not be transmitted to the worker thread"}
	ErrMissingRawTerminal = &Error{"Standard output could not be put into raw mode"}
)

// CommandKind identifies a TUI command.
type CommandKind int

// The commands the input loop can produce.
const (
	CmdSearch CommandKind = iota
	CmdShowLast
	CmdOpen
	CmdDrawIndices
	CmdClear
)

// Command is one instruction for the worker.
type Command struct {
	Kind   CommandKind
	Term   string
	Force  bool
	Number int
}

// Key is a decoded key press.
type Key struct {
	// Rune is the character for a printable key.
	Rune rune
	// Ctrl is set for a control chord, with Rune holding the letter.
	Ctrl bool
	// Backspace, Esc and Enter are the named keys the TUI handles.
	Backspace bool
	Esc       bool
	Enter     bool
	// Unsupported carries the raw bytes of a sequence the TUI ignores.
	Unsupported string
}

// isSpecial mirrors `is_special`: tab is the only character excluded from a
// search term.
func isSpecial(c rune) bool { return c == '\t' }

// ReduceKey is a port of `handle_key`'s pure part: given a key and the current
// state, it updates the state and returns the command to dispatch.
//
// Separating this from the terminal loop is what makes the TUI's behaviour
// testable; the Rust interleaves it with writes to stdout.
func ReduceKey(k Key, state *State) (cmd Command, send bool, quit bool, info string) {
	forceOpen := false
	showLast := false

	switch {
	case k.Enter:
		switch state.Mode {
		case ModeSearching:
			state.Term = ""
		default:
			forceOpen = true
			state.Mode = ModeOpening
		}
	case k.Backspace:
		if state.Mode == ModeSearching {
			state.Term = dropLastRune(state.Term)
		} else {
			state.Number = dropLastRune(state.Number)
		}
	case k.Ctrl && k.Rune == 'o':
		if state.Mode == ModeSearching {
			state.Mode = ModeOpening
		} else {
			state.Number = ""
			showLast = true
			state.Mode = ModeSearching
		}
	case k.Esc, k.Ctrl && k.Rune == 'c':
		return Command{}, false, true, ""
	case k.Rune != 0 && !k.Ctrl:
		if state.Mode == ModeSearching {
			if !isSpecial(k.Rune) {
				state.Term += string(k.Rune)
			}
		} else {
			if k.Rune >= '0' && k.Rune <= '9' {
				state.Number += string(k.Rune)
			} else {
				return Command{}, false, false, "Please enter digits from 0-9"
			}
		}
	default:
		return Command{}, false, false,
			fmt.Sprintf("unsupported key sequence: %s", k.Unsupported)
	}

	switch {
	case state.Mode == ModeSearching:
		if state.Term == "" {
			cmd = Command{Kind: CmdClear}
		} else if showLast {
			cmd = Command{Kind: CmdShowLast}
		} else {
			cmd = Command{Kind: CmdSearch, Term: state.Term}
		}
	case len(state.Number) > 0:
		n, err := strconv.Atoi(state.Number)
		if err != nil {
			state.Number = ""
			return Command{}, false, false, err.Error()
		}
		cmd = Command{Kind: CmdOpen, Force: forceOpen, Number: n}
	default:
		cmd = Command{Kind: CmdDrawIndices}
	}
	return cmd, true, false, ""
}

func dropLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// SearchURL builds the crates.io query, mirroring `handle_command`'s Search arm.
//
// The per_page floor of 100 is deliberate: a tall terminal asks for more rows
// than a default page holds.
func SearchURL(term string, dim Dimension) string {
	perPage := uint16(100)
	if dim.Height > perPage {
		perPage = dim.Height
	}
	return fmt.Sprintf(
		"https://crates.io/api/v1/crates?page=1&per_page=%d&q=%s&sort=",
		perPage, queryEscape(term))
}

// queryEscape matches the `urlencoding` crate, which percent-encodes
// everything outside the unreserved set — including the space, which Go's
// url.QueryEscape would render as '+'.
func queryEscape(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// CrateURL is the page opened for a selected result.
func CrateURL(name, version string) string {
	return fmt.Sprintf("https://crates.io/crates/%s/%s", name, version)
}

// RenderIndexed is the `Indexed` Display impl: the numbered overlay drawn over
// the result list when choosing a crate to open.
func RenderIndexed(s SearchResult) string {
	dim := ContentDimension()
	if s.Meta.Dimension != nil {
		dim = *s.Meta.Dimension
	}
	nw, _, _, _ := DesiredTableWidths(s.Crates, dim)
	center := nw + 1

	var b strings.Builder
	b.WriteString(termion.CursorHide)
	b.WriteString("\x1b[" + strconv.Itoa(center) + "C") // cursor::Right
	n := len(s.Crates)
	if n > int(dim.Height) {
		n = int(dim.Height)
	}
	for i := 0; i < n; i++ {
		rendered := fmt.Sprintf("|#%3d #|", i)
		b.WriteString(rendered)
		b.WriteString("\x1b[" + strconv.Itoa(len(rendered)) + "D")
		b.WriteString("\x1b[1B")
	}
	return b.String()
}

// UsageText and the other fixed strings the TUI shows on the info line.
const (
	UsageText   = "(<ESC> to quit, <enter> to clear, Ctrl+o to open) Please enter your search term."
	IndicesText = "(<ESC> to quit, Ctrl+o to cancel, <enter> to confirm) Type the number of the " +
		"crate to open."
	NothingToOpenSearchFirst  = "There is nothing to open - conduct a search first."
	NothingToOpenSearchFirst2 = "There is nothing to open - conduct a search first"
	NoPreviousResult          = "There is no previous result - conduct a search first."
	Searching                 = "searching ..."
)

// Info renders the info line, returning the length of the message so the
// caller can position text after it.
func Info(w io.Writer, msg string) int {
	fmt.Fprintf(w, "%s%s%s%s", termion.CursorHide, infoLine, termion.ClearCurrentLine, msg)
	return len(msg)
}

// Prompt renders the prompt line.
func Prompt(w io.Writer, state State) {
	fmt.Fprintf(w, "%s%s%s %s: %s",
		termion.CursorShow, termion.Goto(1, 1), termion.ClearCurrentLine,
		state.Mode, state.Prompt())
}

// browserCommand returns the platform's URL opener, mirroring what the `open`
// crate does. Split out from openURL so the choice can be tested without
// actually launching a browser.
func browserCommand(url string) (name string, args []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// openURL is `open::that`.
//
// It is a variable so tests can observe the call without spawning a browser.
var openURL = func(url string) error {
	name, args := browserCommand(url)
	return exec.Command(name, args...).Start()
}

// HandleInteractive is a port of `handle_interactive_search`.
//
// It puts stdout into raw mode, then runs an input loop that dispatches
// commands to a worker goroutine. Searches are versioned and cancellable: a
// keystroke that starts a new search flags the previous one, and a result
// whose version is stale is discarded rather than drawn over the newer one.
func HandleInteractive(ctx context.Context) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ErrMissingRawTerminal
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ErrMissingRawTerminal
	}
	defer term.Restore(fd, oldState)

	out := os.Stdout
	state := State{}

	fmt.Fprintf(out, "%s%s", termion.Goto(1, 1), termion.ClearAll)
	Prompt(out, state)
	Info(out, UsageText)

	client := httputil.NewHTTPClient()
	commands := make(chan Command, 16)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runWorker(ctx, out, client, commands)
	}()

	err = readKeys(os.Stdin, func(k Key) bool {
		cmd, send, quit, info := ReduceKey(k, &state)
		if quit {
			return false
		}
		if info != "" {
			Info(out, info)
			return true
		}
		Prompt(out, state)
		if send {
			commands <- cmd
		}
		return true
	})
	close(commands)
	wg.Wait()
	return err
}

// runWorker is the reducer goroutine: it owns the current result and the
// cancellation flag for the in-flight search.
func runWorker(ctx context.Context, out io.Writer, client httputil.Client, commands <-chan Command) {
	var current *SearchResult
	version := 0
	var cancel *atomic.Bool
	results := make(chan versionedResult, 4)

	for {
		select {
		case cmd, ok := <-commands:
			if !ok {
				if cancel != nil {
					cancel.Store(true)
				}
				return
			}
			switch cmd.Kind {
			case CmdSearch:
				version++
				if cancel != nil {
					cancel.Store(true)
				}
				c := &atomic.Bool{}
				cancel = c
				Info(out, Searching)
				go func(term string, v int, c *atomic.Bool) {
					res, err := runSearch(ctx, client, term, c)
					results <- versionedResult{v, res, err}
				}(cmd.Term, version, c)
			case CmdClear:
				version++
				if cancel != nil {
					cancel.Store(true)
					cancel = nil
				}
				current = applyCommand(out, cmd, current, nil, nil)
			default:
				current = applyCommand(out, cmd, current, nil, nil)
			}
		case r := <-results:
			if r.version != version {
				continue
			}
			cancel = nil
			if r.err != nil {
				Info(out, r.err.Error())
				continue
			}
			current = applyCommand(out, Command{Kind: CmdSearch}, current, r.result, nil)
		}
	}
}

type versionedResult struct {
	version int
	result  *SearchResult
	err     error
}

// runSearch performs the paged crates.io query for a term.
func runSearch(ctx context.Context, client httputil.Client, term string, cancel *atomic.Bool) (*SearchResult, error) {
	dim := ContentDimension()
	limit := uint32(dim.Height)
	url := SearchURL(term, dim)

	merge := func(acc *SearchResult, c httputil.CallResult) (*SearchResult, error) {
		next, err := decodeSearchResult(c, dim)
		if err != nil {
			return nil, err
		}
		acc.Crates = append(acc.Crates, next.Crates...)
		return acc, nil
	}
	extract := func(c httputil.CallResult) (httputil.CallMetaData, *SearchResult, error) {
		res, err := decodeSearchResult(c, dim)
		if err != nil {
			return httputil.CallMetaData{}, nil, err
		}
		return httputil.CallMetaData{
			Total: res.Meta.Total,
			Items: uint32(len(res.Crates)),
		}, res, nil
	}

	res, err := httputil.PagedCratesIoRemoteCall(
		ctx, client, url, &limit, (*SearchResult)(nil), merge, extract)
	if err != nil {
		return nil, err
	}
	t := term
	res.Meta.Term = &t
	return res, nil
}

// applyCommand is a port of `handle_future_result`: it draws the effect of a
// completed command and reports the new current result.
func applyCommand(out io.Writer, cmd Command, current *SearchResult, incoming *SearchResult, _ any) *SearchResult {
	switch cmd.Kind {
	case CmdDrawIndices:
		if current == nil {
			Info(out, NothingToOpenSearchFirst)
			return current
		}
		Info(out, IndicesText)
		fmt.Fprintf(out, "%s%s", contentLine, RenderIndexed(*current))
	case CmdOpen:
		if current == nil {
			Info(out, NothingToOpenSearchFirst2)
			return current
		}
		if cmd.Number >= len(current.Crates) {
			Info(out, fmt.Sprintf("No crate #%d! Try using <backspace> ...", cmd.Number))
			return current
		}
		c := current.Crates[cmd.Number]
		// A number that could still be extended by another digit waits for
		// confirmation, unless the user forced it with <enter>.
		if cmd.Number == 0 || cmd.Number*10 >= len(current.Crates) || cmd.Force {
			if err := openURL(CrateURL(c.Name, c.MaxVersion)); err != nil {
				Info(out, err.Error())
			}
		} else {
			Info(out, fmt.Sprintf("Hit <enter> to open crate #%d or keep typing ...", cmd.Number))
		}
	case CmdClear:
		Info(out, UsageText)
		dim := ContentDimension()
		empty := SearchResult{Meta: Meta{Dimension: &dim}}
		fmt.Fprintf(out, "%s%s", contentLine, empty.Render())
		return nil
	case CmdShowLast:
		if current == nil {
			Info(out, NoPreviousResult)
			return current
		}
		fmt.Fprintf(out, "%s%s", contentLine, current.Render())
	case CmdSearch:
		if incoming == nil {
			return current
		}
		height := uint16(0)
		if incoming.Meta.Dimension != nil {
			height = incoming.Meta.Dimension.Height
		}
		termStr := ""
		if incoming.Meta.Term != nil {
			termStr = *incoming.Meta.Term
		}
		Info(out, fmt.Sprintf("%d results for '%s' in total, showing %d max",
			incoming.Meta.Total, termStr, height))
		if len(incoming.Crates) == 0 {
			last := Info(out, UsageText)
			suffix := ""
			if current != nil && current.Meta.Term != nil {
				suffix = fmt.Sprintf("Showing results for '%s'", *current.Meta.Term)
			}
			fmt.Fprintf(out, "%s - nothing found.%s", termion.Goto(uint16(last), 2), suffix)
			return current
		}
		fmt.Fprintf(out, "%s%s", contentLine, incoming.Render())
		return incoming
	}
	return current
}

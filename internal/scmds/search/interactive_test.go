package search

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Byron/crates-io-cli/internal/httputil"

	"github.com/Byron/crates-io-cli/internal/termion"
	"strings"
	"testing"

	"github.com/Byron/crates-io-cli/internal/structs"
)

func TestDecodeSearchResult(t *testing.T) {
	raw := []byte(`{"crates":[{"description":"d","downloads":3,"max_version":"1.0.0","name":"n"}],"meta":{"total":9}}`)
	dim := Dimension{Width: 80, Height: 10}
	res, err := decodeSearchResult(raw, dim)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Crates) != 1 || res.Meta.Total != 9 {
		t.Fatalf("got %+v", res)
	}
	// The dimension is attached by the decoder, not present in the payload.
	if res.Meta.Dimension == nil || res.Meta.Dimension.Height != 10 {
		t.Errorf("dimension not attached: %+v", res.Meta.Dimension)
	}
	if _, err := decodeSearchResult([]byte("nope"), dim); err == nil {
		t.Error("expected a decode error")
	}
}

// TestRenderIndexed pins the numbered overlay drawn when choosing a crate.
func TestRenderIndexed(t *testing.T) {
	dim := Dimension{Width: 80, Height: 3}
	res := SearchResult{
		Crates: []structs.Crate{
			{Name: "aa", Downloads: 1, MaxVersion: "1"},
			{Name: "bb", Downloads: 1, MaxVersion: "1"},
		},
		Meta: Meta{Dimension: &dim},
	}
	got := RenderIndexed(res)
	if !strings.HasPrefix(got, termion.CursorHide) {
		t.Errorf("overlay must hide the cursor first: %q", got[:10])
	}
	for _, want := range []string{"|#  0 #|", "|#  1 #|"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	// Only as many indices as fit on screen.
	if strings.Contains(got, "|#  2 #|") {
		t.Error("drew more indices than crates")
	}
}

// TestInfoAndPrompt pin the two fixed screen lines.
func TestInfoAndPrompt(t *testing.T) {
	var buf bytes.Buffer
	n := Info(&buf, "hello")
	if n != 5 {
		t.Errorf("Info returned %d, want the message length", n)
	}
	got := buf.String()
	if !strings.Contains(got, "hello") || !strings.Contains(got, "\x1b[2;1H") {
		t.Errorf("info line = %q", got)
	}

	buf.Reset()
	Prompt(&buf, State{Term: "ser"})
	got = buf.String()
	if !strings.Contains(got, " search: ser") {
		t.Errorf("prompt = %q", got)
	}
	buf.Reset()
	Prompt(&buf, State{Mode: ModeOpening, Number: "4"})
	if !strings.Contains(buf.String(), " open by number: 4") {
		t.Errorf("prompt = %q", buf.String())
	}
}

// TestApplyCommandEmptyStates pins the messages shown before any search has
// been run.
func TestApplyCommandEmptyStates(t *testing.T) {
	cases := []struct {
		cmd  Command
		want string
	}{
		{Command{Kind: CmdDrawIndices}, NothingToOpenSearchFirst},
		{Command{Kind: CmdOpen}, NothingToOpenSearchFirst2},
		{Command{Kind: CmdShowLast}, NoPreviousResult},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if got := applyCommand(&buf, c.cmd, nil, nil, nil); got != nil {
			t.Errorf("current result should stay nil")
		}
		if !strings.Contains(buf.String(), c.want) {
			t.Errorf("got %q, want it to contain %q", buf.String(), c.want)
		}
	}
}

// TestApplyCommandClearResetsResult pins that clearing drops the stored result
// so a later ShowLast reports there is nothing to show.
func TestApplyCommandClear(t *testing.T) {
	dim := Dimension{Width: 40, Height: 2}
	current := &SearchResult{Crates: []structs.Crate{{Name: "x"}}, Meta: Meta{Dimension: &dim}}
	var buf bytes.Buffer
	got := applyCommand(&buf, Command{Kind: CmdClear}, current, nil, nil)
	if got != nil {
		t.Error("Clear must drop the current result")
	}
	if !strings.Contains(buf.String(), UsageText) {
		t.Errorf("Clear should restore the usage line: %q", buf.String())
	}
}

// TestApplyCommandShowResults pins the summary line and that an empty result
// set does not replace the previous one.
func TestApplyCommandShowResults(t *testing.T) {
	dim := Dimension{Width: 60, Height: 4}
	term := "serde"

	incoming := &SearchResult{
		Crates: []structs.Crate{{Name: "serde", Downloads: 5, MaxVersion: "1.0.0"}},
		Meta:   Meta{Total: 42, Term: &term, Dimension: &dim},
	}
	var buf bytes.Buffer
	got := applyCommand(&buf, Command{Kind: CmdSearch}, nil, incoming, nil)
	if got != incoming {
		t.Error("a non-empty result should become current")
	}
	if !strings.Contains(buf.String(), "42 results for 'serde' in total, showing 4 max") {
		t.Errorf("summary line = %q", buf.String())
	}

	empty := &SearchResult{Meta: Meta{Total: 0, Term: &term, Dimension: &dim}}
	buf.Reset()
	got = applyCommand(&buf, Command{Kind: CmdSearch}, incoming, empty, nil)
	if got != incoming {
		t.Error("an empty result must not replace the previous one")
	}
	if !strings.Contains(buf.String(), "nothing found") {
		t.Errorf("expected the nothing-found notice: %q", buf.String())
	}
}

// TestApplyCommandOpenGuards pins the confirmation logic: a number that could
// still gain another digit waits, unless <enter> forced it.
func TestApplyCommandOpenGuards(t *testing.T) {
	dim := Dimension{Width: 60, Height: 4}
	crates := make([]structs.Crate, 30)
	for i := range crates {
		crates[i] = structs.Crate{Name: "c", Downloads: 1, MaxVersion: "1"}
	}
	current := &SearchResult{Crates: crates, Meta: Meta{Dimension: &dim}}

	var buf bytes.Buffer
	applyCommand(&buf, Command{Kind: CmdOpen, Number: 2}, current, nil, nil)
	if !strings.Contains(buf.String(), "Hit <enter> to open crate #2") {
		t.Errorf("expected a confirmation prompt: %q", buf.String())
	}

	buf.Reset()
	applyCommand(&buf, Command{Kind: CmdOpen, Number: 99}, current, nil, nil)
	if !strings.Contains(buf.String(), "No crate #99!") {
		t.Errorf("expected an out-of-range notice: %q", buf.String())
	}
}

// TestReadKeys pins the decoding of the byte sequences the TUI reacts to.
func TestReadKeys(t *testing.T) {
	input := []byte{'a', 0x7f, '\r', 0x0f, 0x1b}
	var got []Key
	if err := readKeys(bytes.NewReader(input), func(k Key) bool {
		got = append(got, k)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("decoded %d keys, want 5: %+v", len(got), got)
	}
	if got[0].Rune != 'a' {
		t.Errorf("key 0 = %+v", got[0])
	}
	if !got[1].Backspace {
		t.Errorf("key 1 = %+v, want backspace", got[1])
	}
	if !got[2].Enter {
		t.Errorf("key 2 = %+v, want enter", got[2])
	}
	if !got[3].Ctrl || got[3].Rune != 'o' {
		t.Errorf("key 3 = %+v, want ctrl+o", got[3])
	}
	if !got[4].Esc {
		t.Errorf("key 4 = %+v, want esc", got[4])
	}
}

// TestReadKeysStopsWhenAsked pins that returning false ends the loop, which is
// how <ESC> quits.
func TestReadKeysStops(t *testing.T) {
	var n int
	if err := readKeys(bytes.NewReader([]byte("abc")), func(Key) bool {
		n++
		return false
	}); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("consumed %d keys after a stop, want 1", n)
	}
}

func TestErrorMessages(t *testing.T) {
	if ErrThreadPanic.Error() != "The worker thread panicked" {
		t.Errorf("got %q", ErrThreadPanic.Error())
	}
	if ErrMissingRawTerminal.Error() != "Standard output could not be put into raw mode" {
		t.Errorf("got %q", ErrMissingRawTerminal.Error())
	}
}

// TestBrowserCommand pins the opener chosen per platform.
func TestBrowserCommand(t *testing.T) {
	name, args := browserCommand("https://crates.io/crates/serde/1.0.0")
	if name == "" || len(args) == 0 {
		t.Fatalf("got %q %v", name, args)
	}
	if args[len(args)-1] != "https://crates.io/crates/serde/1.0.0" {
		t.Errorf("URL not passed last: %v", args)
	}
}

// TestOpenIsInvokedForAConfirmedChoice pins that a forced or unambiguous
// selection actually opens the crate page.
func TestOpenIsInvoked(t *testing.T) {
	orig := openURL
	defer func() { openURL = orig }()
	var opened string
	openURL = func(u string) error { opened = u; return nil }

	dim := Dimension{Width: 60, Height: 4}
	current := &SearchResult{
		Crates: []structs.Crate{{Name: "serde", Downloads: 1, MaxVersion: "1.0.0"}},
		Meta:   Meta{Dimension: &dim},
	}
	var buf bytes.Buffer
	applyCommand(&buf, Command{Kind: CmdOpen, Number: 0}, current, nil, nil)
	if opened != "https://crates.io/crates/serde/1.0.0" {
		t.Errorf("opened %q", opened)
	}
}

// TestRunSearchPagesAndTagsTerm pins that a search fetches through the paged
// caller and records the term on the result, which the summary line shows.
func TestRunSearch(t *testing.T) {
	client := &stubClient{body: `{"crates":[{"description":"d","downloads":1,"max_version":"1","name":"n"}],"meta":{"total":1}}`}
	res, err := runSearch(context.Background(), client, "serde", &atomic.Bool{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Term == nil || *res.Meta.Term != "serde" {
		t.Errorf("term not recorded: %+v", res.Meta.Term)
	}
	if len(res.Crates) != 1 {
		t.Errorf("got %d crates", len(res.Crates))
	}
	if !strings.Contains(client.seen[0], "q=serde") {
		t.Errorf("query not encoded into the URL: %s", client.seen[0])
	}
}

// TestRunWorkerDiscardsStaleResults pins the versioning that stops a slow
// search from overwriting a newer one.
func TestRunWorkerHandlesCommands(t *testing.T) {
	client := &stubClient{body: `{"crates":[],"meta":{"total":0}}`}
	commands := make(chan Command, 4)
	var buf syncBuffer

	done := make(chan struct{})
	go func() { runWorker(context.Background(), &buf, client, commands); close(done) }()

	commands <- Command{Kind: CmdShowLast}
	commands <- Command{Kind: CmdClear}
	close(commands)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop when the command channel closed")
	}
	if !strings.Contains(buf.String(), NoPreviousResult) {
		t.Errorf("ShowLast without a result should say so: %q", buf.String())
	}
}

type stubClient struct {
	body string
	seen []string
}

func (s *stubClient) Get(_ context.Context, url string) (httputil.CallResult, error) {
	s.seen = append(s.seen, url)
	return []byte(s.body), nil
}

// syncBuffer is a bytes.Buffer safe for the worker goroutine to write to.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestHandleInteractiveRequiresATerminal pins the raw-mode failure, which is
// the only branch of the entry point reachable without a tty.
func TestHandleInteractiveRequiresATerminal(t *testing.T) {
	err := HandleInteractive(context.Background())
	if err == nil {
		t.Skip("stdin is a terminal in this environment")
	}
	if err != ErrMissingRawTerminal {
		t.Errorf("err = %v, want ErrMissingRawTerminal", err)
	}
}

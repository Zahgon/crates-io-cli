package httputil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeClient serves synthetic pages and records the URLs requested.
type fakeClient struct {
	total int
	seen  []string
}

func (f *fakeClient) Get(_ context.Context, url string) (CallResult, error) {
	f.seen = append(f.seen, url)
	// Each page returns up to 100 items, numbered globally.
	page := 1
	if i := strings.Index(url, "&page="); i >= 0 {
		fmt.Sscanf(url[i:], "&page=%d", &page)
	}
	start := (page - 1) * 100
	items := []int{}
	for i := start; i < start+100 && i < f.total; i++ {
		items = append(items, i)
	}
	body, _ := json.Marshal(map[string]any{"items": items, "total": f.total})
	return body, nil
}

type payload struct {
	Items []int `json:"items"`
	Total int   `json:"total"`
}

func run(t *testing.T, total int, maxItems *uint32) ([]int, []string) {
	t.Helper()
	c := &fakeClient{total: total}
	merge := func(acc []int, r CallResult) ([]int, error) {
		var p payload
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, err
		}
		return append(acc, p.Items...), nil
	}
	extract := func(r CallResult) (CallMetaData, []int, error) {
		var p payload
		if err := json.Unmarshal(r, &p); err != nil {
			return CallMetaData{}, nil, err
		}
		return CallMetaData{Total: uint32(p.Total), Items: uint32(len(p.Items))}, p.Items, nil
	}
	got, err := PagedCratesIoRemoteCall(context.Background(), c,
		"https://example/api?x=1", maxItems, []int(nil), merge, extract)
	if err != nil {
		t.Fatalf("paged call: %v", err)
	}
	return got, c.seen
}

// TestPagingWalksEveryPage pins the page arithmetic, which decides how many
// requests crates.io receives.
func TestPagingWalksEveryPage(t *testing.T) {
	cases := []struct {
		total     int
		wantItems int
		wantPages int
	}{
		{0, 0, 1},
		{1, 1, 1},
		{100, 100, 1},
		{101, 101, 2},
		{200, 200, 2},
		{201, 201, 3},
		{866, 866, 9},
	}
	for _, c := range cases {
		got, seen := run(t, c.total, nil)
		if len(got) != c.wantItems {
			t.Errorf("total=%d: got %d items, want %d", c.total, len(got), c.wantItems)
		}
		if len(seen) != c.wantPages {
			t.Errorf("total=%d: made %d requests, want %d", c.total, len(seen), c.wantPages)
		}
	}
}

// TestFirstRequestShape pins that page 1 carries per_page but no page
// parameter, and later ones start at page=2.
func TestFirstRequestShape(t *testing.T) {
	_, seen := run(t, 250, nil)
	if !strings.HasSuffix(seen[0], "&per_page=100") {
		t.Errorf("first URL = %q", seen[0])
	}
	if !strings.Contains(seen[1], "&page=2&per_page=100") {
		t.Errorf("second URL = %q", seen[1])
	}
	if !strings.Contains(seen[2], "&page=3&per_page=100") {
		t.Errorf("third URL = %q", seen[2])
	}
}

// TestMaxItemsCapsPageSizeAndCount pins the search path, which asks for only
// as many rows as fit on screen.
func TestMaxItemsCaps(t *testing.T) {
	limit := uint32(20)
	_, seen := run(t, 1000, &limit)
	if len(seen) != 1 {
		t.Errorf("made %d requests, want 1 when max_items is below one page", len(seen))
	}
	if !strings.Contains(seen[0], "per_page=20") {
		t.Errorf("page size not capped to max_items: %q", seen[0])
	}
}

// TestSaturatingSubtraction guards the underflow the Rust avoids explicitly:
// crates.io can report a total lower than the items already returned.
func TestSaturatingSubtraction(t *testing.T) {
	if got := saturatingSub(5, 9); got != 0 {
		t.Errorf("saturatingSub(5,9) = %d, want 0", got)
	}
	if got := saturatingSub(9, 5); got != 4 {
		t.Errorf("saturatingSub(9,5) = %d, want 4", got)
	}
}

// TestHTTPClientSendsUserAgent pins the User-Agent crates.io sees, which the
// Rust sets on every request.
func TestHTTPClientSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewHTTPClient()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotUA != UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, UserAgent)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
	if c.C.Timeout != Timeout {
		t.Errorf("timeout = %v, want %v", c.C.Timeout, Timeout)
	}
}

// TestHTTPClientWrapsTransportErrors pins that a transport failure surfaces as
// a RemoteCallError, whose message the CLI prints.
func TestHTTPClientWrapsTransportErrors(t *testing.T) {
	c := NewHTTPClient()
	_, err := c.Get(context.Background(), "http://127.0.0.1:1/nope")
	if err == nil {
		t.Fatal("expected an error")
	}
	var rce *RemoteCallError
	if !errors.As(err, &rce) {
		t.Fatalf("error is not a RemoteCallError: %T", err)
	}
	if rce.Error() != "A remote call could not be performed" {
		t.Errorf("message = %q", rce.Error())
	}
	if rce.Unwrap() == nil {
		t.Error("Unwrap should expose the transport error")
	}

	if _, err := c.Get(context.Background(), "://bad"); err == nil {
		t.Error("expected an error for a malformed URL")
	}
}

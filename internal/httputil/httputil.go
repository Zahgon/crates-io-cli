// Package httputil is a port of src/http_utils.rs: the paged crates.io caller.
package httputil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxItemsPerPage mirrors MAX_ITEMS_PER_PAGE.
const maxItemsPerPage uint32 = 100

// UserAgent and Timeout mirror `request_new`.
const (
	UserAgent = "crates.io-cli (https://crates.io/crates/crates-io-cli)"
	Timeout   = 15 * time.Second
)

// CallResult is the body of one completed request, standing in for the Rust
// `(Arc<Mutex<Vec<u8>>>, Easy)` pair. The Go side only ever reads the body, so
// the handle is not carried along.
type CallResult []byte

// CallMetaData mirrors the Rust struct of the same name.
type CallMetaData struct {
	// Total is the item count crates.io reports in `meta.total`.
	Total uint32
	// Items is the number of items seen in the current call.
	Items uint32
}

// RemoteCallError wraps a transport failure, mirroring `RemoteCallError`.
type RemoteCallError struct {
	Err error
}

func (e *RemoteCallError) Error() string { return "A remote call could not be performed" }
func (e *RemoteCallError) Unwrap() error { return e.Err }

// Client performs the HTTP calls. It is an interface so tests can serve
// recorded fixtures instead of reaching crates.io.
type Client interface {
	Get(ctx context.Context, url string) (CallResult, error)
}

// HTTPClient is the production Client.
type HTTPClient struct{ C *http.Client }

// NewHTTPClient builds a client configured like `request_new`.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{C: &http.Client{Timeout: Timeout}}
}

// Get performs one GET and returns the whole body.
func (h *HTTPClient) Get(ctx context.Context, url string) (CallResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &RemoteCallError{Err: err}
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := h.C.Do(req)
	if err != nil {
		return nil, &RemoteCallError{Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RemoteCallError{Err: err}
	}
	return body, nil
}

// PagedCratesIoRemoteCall is a port of `paged_crates_io_remote_call`.
//
// It fetches the first page to learn the total, then walks the remaining pages
// and folds them together. The paging arithmetic is reproduced exactly,
// including the saturating subtractions: crates.io can report a total smaller
// than the number of items already returned, and an unsigned underflow there
// would ask for billions of pages.
func PagedCratesIoRemoteCall[T any](
	ctx context.Context,
	client Client,
	url string,
	maxItems *uint32,
	initial T,
	merge func(T, CallResult) (T, error),
	extract func(CallResult) (CallMetaData, T, error),
) (T, error) {
	var zero T

	limit := ^uint32(0)
	if maxItems != nil {
		limit = *maxItems
	}
	pageSize := maxItemsPerPage
	if limit < pageSize {
		pageSize = limit
	}

	first, err := client.Get(ctx, fmt.Sprintf("%s&per_page=%d", url, pageSize))
	if err != nil {
		return zero, err
	}
	meta, result, err := extract(first)
	if err != nil {
		return zero, err
	}

	remainingItems := saturatingSub(meta.Total, meta.Items)
	if byLimit := saturatingSub(limit, meta.Items); byLimit < remainingItems {
		remainingItems = byLimit
	}

	remainingPages := uint32(0)
	if pageSize > 0 {
		// div_ceil
		remainingPages = (remainingItems + pageSize - 1) / pageSize
	}

	for page := uint32(0); page < remainingPages; page++ {
		call, err := client.Get(ctx, fmt.Sprintf("%s&page=%d&per_page=%d", url, page+2, pageSize))
		if err != nil {
			return zero, err
		}
		result, err = merge(result, call)
		if err != nil {
			return zero, err
		}
	}
	_ = initial
	return result, nil
}

func saturatingSub(a, b uint32) uint32 {
	if b > a {
		return 0
	}
	return a - b
}

// ErrNotFound is returned when a page cannot be decoded at all.
var ErrNotFound = errors.New("not found")

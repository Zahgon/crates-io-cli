package search

import (
	"encoding/json"

	"github.com/Byron/crates-io-cli/internal/httputil"
)

// decodeSearchResult is `SearchResult::from_data`: decode a page and attach
// the display dimension the caller is rendering at.
func decodeSearchResult(buf httputil.CallResult, dim Dimension) (*SearchResult, error) {
	var res SearchResult
	if err := json.Unmarshal(buf, &res); err != nil {
		return nil, err
	}
	d := dim
	res.Meta.Dimension = &d
	return &res, nil
}

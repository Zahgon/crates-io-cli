package recents

import "encoding/json"

// parseVersionLine decodes one line of a crates.io index file.
//
// The index stores one JSON object per line with many more fields than are
// used here; unknown fields are ignored.
func parseVersionLine(line string) (Change, bool) {
	var c Change
	if err := json.Unmarshal([]byte(line), &c); err != nil {
		return Change{}, false
	}
	if c.Name == "" || c.Version == "" {
		return Change{}, false
	}
	return c, true
}

// Package structs is a port of src/structs.rs: the subset of the crates.io
// API payload this tool consumes.
package structs

// Crates is the top level of a crates.io `/api/v1/crates` response.
type Crates struct {
	Crates []Crate `json:"crates"`
	Meta   Meta    `json:"meta"`
}

// Crate is one crate entry.
//
// Description is a pointer because the API returns `null` for crates without
// one, and the human output renders that as an empty cell rather than the
// string "null". Field order matters: `serde_json::to_writer_pretty` emits
// keys in declaration order, and the JSON output is compared byte-for-byte.
type Crate struct {
	Description *string `json:"description"`
	Downloads   int64   `json:"downloads"`
	MaxVersion  string  `json:"max_version"`
	Name        string  `json:"name"`
}

// DescriptionOrEmpty returns the description, or "" when the API sent null.
// This mirrors `description.unwrap_or_default()`.
func (c Crate) DescriptionOrEmpty() string {
	if c.Description == nil {
		return ""
	}
	return *c.Description
}

// Meta carries the paging information crates.io reports alongside a page.
type Meta struct {
	Total uint32 `json:"total"`
}

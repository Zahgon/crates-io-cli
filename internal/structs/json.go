package structs

import (
	"bytes"
	"encoding/json"
	"io"
)

// WriteJSONPretty reproduces `serde_json::to_writer_pretty`.
//
// Go's encoding/json differs from serde in three ways that all show up in the
// bytes, so it cannot be used directly:
//
//   - serde indents with two spaces; Go's MarshalIndent needs to be told.
//   - serde emits no trailing newline; Encoder.Encode appends one.
//   - Go escapes <, > and & as \u003c etc. for HTML safety, while serde emits
//     them raw. Crate descriptions routinely contain "&" and "<", so this is
//     not a corner case.
func WriteJSONPretty(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encode appends a newline that serde does not.
	out := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	_, err := w.Write(out)
	return err
}

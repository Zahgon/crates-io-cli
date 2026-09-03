package structs

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestDescriptionOrEmpty pins the null handling: crates.io returns null for a
// crate without a description, which must render as an empty cell rather than
// the string "null".
func TestDescriptionOrEmpty(t *testing.T) {
	if got := (Crate{}).DescriptionOrEmpty(); got != "" {
		t.Errorf("nil description = %q", got)
	}
	d := "hi"
	if got := (Crate{Description: &d}).DescriptionOrEmpty(); got != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeCratesPayload(t *testing.T) {
	raw := `{"crates":[{"description":null,"downloads":7,"max_version":"1.0.0","name":"a"}],"meta":{"total":42}}`
	var c Crates
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Meta.Total != 42 || len(c.Crates) != 1 {
		t.Fatalf("got %+v", c)
	}
	if c.Crates[0].Description != nil {
		t.Error("null description should decode to nil")
	}
}

// TestWriteJSONPretty pins the three ways serde differs from encoding/json:
// two-space indent, no trailing newline, and no HTML escaping.
func TestWriteJSONPretty(t *testing.T) {
	d := "a & b <c>"
	crates := []Crate{{Description: &d, Downloads: 1, MaxVersion: "1.0.0", Name: "x"}}

	var buf bytes.Buffer
	if err := WriteJSONPretty(&buf, crates); err != nil {
		t.Fatal(err)
	}
	want := "[\n  {\n    \"description\": \"a & b <c>\",\n    \"downloads\": 1," +
		"\n    \"max_version\": \"1.0.0\",\n    \"name\": \"x\"\n  }\n]"
	if buf.String() != want {
		t.Errorf("got  %q\nwant %q", buf.String(), want)
	}

	buf.Reset()
	if err := WriteJSONPretty(&buf, []Crate{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[]" {
		t.Errorf("empty slice = %q, want []", buf.String())
	}
}

// TestFieldOrder pins that keys are emitted in declaration order, which is
// what serde does and what the recorded goldens expect.
func TestFieldOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSONPretty(&buf, Crate{Name: "n", MaxVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	iDesc := bytes.Index([]byte(got), []byte("description"))
	iDown := bytes.Index([]byte(got), []byte("downloads"))
	iMax := bytes.Index([]byte(got), []byte("max_version"))
	iName := bytes.Index([]byte(got), []byte("name"))
	if !(iDesc < iDown && iDown < iMax && iMax < iName) {
		t.Errorf("field order wrong:\n%s", got)
	}
}

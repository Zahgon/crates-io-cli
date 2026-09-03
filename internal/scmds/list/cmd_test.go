package list_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Byron/crates-io-cli/internal/scmds/list"
)

// TestCratesFromCallresult is migrated 1:1 from the crate's only Rust unit
// test, `scmds::list::cmd::tests::test_crates_from_callresult`. It decodes the
// committed crates.io fixture and checks the paging metadata and page size.
func TestCratesFromCallresult(t *testing.T) {
	buf, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "byrons-crates.json"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	crates, meta, err := list.CratesFromCallResultBuf(buf)
	if err != nil {
		t.Fatalf("decoding the fixture: %v", err)
	}
	if meta.Total != 244 {
		t.Errorf("meta.total = %d, want 244", meta.Total)
	}
	if len(crates) != 10 {
		t.Errorf("len(crates) = %d, want 10", len(crates))
	}
}

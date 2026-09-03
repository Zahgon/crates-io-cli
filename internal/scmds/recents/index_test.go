package recents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// buildIndexRepo creates a miniature crates.io index: a git repository whose
// files are JSON-lines crate records. Two commits let the diff logic be
// exercised without cloning the real multi-gigabyte index.
func buildIndexRepo(t *testing.T) (dir string, first, second plumbing.Hash) {
	t.Helper()
	dir = t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	write := func(name, content string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) plumbing.Hash {
		h, err := wt.Commit(msg, &git.CommitOptions{
			Author: &object.Signature{Name: "t", Email: "t@e"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}

	write("config.json", `{"dl":"x"}`)
	write("se/rd/serde", `{"name":"serde","vers":"1.0.0","yanked":false}`+"\n")
	first = commit("initial")

	write("se/rd/serde",
		`{"name":"serde","vers":"1.0.0","yanked":false}`+"\n"+
			`{"name":"serde","vers":"1.0.1","yanked":false}`+"\n")
	write("to/ki/tokio", `{"name":"tokio","vers":"1.2.3","yanked":false}`+"\n")
	second = commit("second")

	// A real index has an origin to fetch from; without one FetchChanges
	// cannot run, so the fixture points origin at itself.
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{dir},
	}); err != nil {
		t.Fatal(err)
	}

	return dir, first, second
}

// TestDiffVersionsFromScratch pins that a repository with no recorded position
// reports every crate in the index, and skips the index's own metadata files.
func TestDiffVersionsFromScratch(t *testing.T) {
	dir, _, second := buildIndexRepo(t)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}

	got, err := diffVersions(repo, nil, second)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, c := range got {
		names[c.Name]++
	}
	if names["serde"] != 2 || names["tokio"] != 1 {
		t.Errorf("got %+v", got)
	}
	if len(got) != 3 {
		t.Errorf("got %d changes, want 3 (config.json must be skipped)", len(got))
	}
}

// TestDiffVersionsBetweenCommits pins the incremental case: only the records
// added since the recorded position are reported.
func TestDiffVersionsBetweenCommits(t *testing.T) {
	dir, first, second := buildIndexRepo(t)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}

	got, err := diffVersions(repo, &first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(got), got)
	}
	seen := map[string]string{}
	for _, c := range got {
		seen[c.Name+"@"+c.Version] = c.Kind()
	}
	if seen["serde@1.0.1"] != "Changed" {
		t.Errorf("serde 1.0.1 kind = %q, want Changed", seen["serde@1.0.1"])
	}
	if seen["tokio@1.2.3"] != "Added" {
		t.Errorf("tokio kind = %q, want Added", seen["tokio@1.2.3"])
	}
	if _, ok := seen["serde@1.0.0"]; ok {
		t.Error("an unchanged record must not be reported")
	}
}

// TestDiffVersionsWithUnknownFromCommit pins the recovery path: if the
// recorded position no longer exists, everything is reported rather than
// failing.
func TestDiffVersionsWithUnknownFromCommit(t *testing.T) {
	dir, _, second := buildIndexRepo(t)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	missing := plumbing.NewHash("0123456789012345678901234567890123456789")
	got, err := diffVersions(repo, &missing, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d changes, want the full index", len(got))
	}
}

// TestRemoteHeadPrefersOrigin pins the branch resolution order.
func TestRemoteHead(t *testing.T) {
	dir, _, second := buildIndexRepo(t)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := remoteHead(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != second {
		t.Errorf("remoteHead = %s, want %s", got, second)
	}
}

// TestOpenOrCloneOpensExisting pins that an existing checkout is reused rather
// than re-cloned, and that a non-empty non-repository is refused instead of
// being clobbered.
func TestOpenOrClone(t *testing.T) {
	dir, _, _ := buildIndexRepo(t)
	if _, err := openOrClone(dir); err != nil {
		t.Errorf("existing repository should open: %v", err)
	}

	junk := t.TempDir()
	if err := os.WriteFile(filepath.Join(junk, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openOrClone(junk); err == nil {
		t.Error("a non-empty directory that is not a repository must be refused")
	}
}

// TestFetchChangesRecordsPosition pins the statefulness: the first call
// reports everything, and the second reports nothing because the position ref
// has been advanced.
func TestFetchChangesRecordsPosition(t *testing.T) {
	dir, _, _ := buildIndexRepo(t)

	first, err := FetchChanges(dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(first) != 3 {
		t.Errorf("first call reported %d changes, want the whole index", len(first))
	}

	second, err := FetchChanges(dir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second call reported %d changes, want none: %+v", len(second), second)
	}

	repo, _ := git.PlainOpen(dir)
	if _, err := repo.Reference(plumbing.ReferenceName(lastSeenRef), true); err != nil {
		t.Errorf("the last-seen ref was not written: %v", err)
	}
}

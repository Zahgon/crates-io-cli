package recents

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// FetchChanges opens or clones the crates.io index at path, fetches the latest
// state, and reports every crate version that appeared since the last run.
//
// The "since the last run" bookkeeping is a ref written into the repository,
// exactly as `crates-index-diff` does, so the command is stateful: the first
// invocation reports the whole index and later ones only the delta.
func FetchChanges(path string) ([]Change, error) {
	repo, err := openOrClone(path)
	if err != nil {
		return nil, &Error{Kind: ErrIndexInit, Err: err}
	}

	if err := fetch(repo); err != nil {
		return nil, &Error{Kind: ErrIndexDiff, Err: err}
	}

	head, err := remoteHead(repo)
	if err != nil {
		return nil, &Error{Kind: ErrIndexDiff, Err: err}
	}

	var lastSeen *plumbing.Hash
	if ref, err := repo.Reference(plumbing.ReferenceName(lastSeenRef), true); err == nil {
		h := ref.Hash()
		lastSeen = &h
	}

	changes, err := diffVersions(repo, lastSeen, head)
	if err != nil {
		return nil, &Error{Kind: ErrIndexDiff, Err: err}
	}

	// Record the new position only after a successful diff, so a failure does
	// not silently skip a range of changes on the next run.
	ref := plumbing.NewHashReference(plumbing.ReferenceName(lastSeenRef), head)
	if err := repo.Storer.SetReference(ref); err != nil {
		return nil, &Error{Kind: ErrIndexDiff, Err: err}
	}
	return changes, nil
}

// openOrClone is `Index::from_path_or_cloned`.
func openOrClone(path string) (*git.Repository, error) {
	repo, err := git.PlainOpen(path)
	if err == nil {
		return repo, nil
	}
	entries, rderr := os.ReadDir(path)
	if rderr == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s exists but is not a git repository: %w", path, err)
	}
	return git.PlainClone(path, true, &git.CloneOptions{
		URL:          IndexURL,
		SingleBranch: true,
	})
}

func fetch(repo *git.Repository) error {
	err := repo.Fetch(&git.FetchOptions{
		RefSpecs: []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
		Force:    true,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}
	return nil
}

// remoteHead resolves the tip of the index's default branch.
func remoteHead(repo *git.Repository) (plumbing.Hash, error) {
	for _, name := range []string{
		"refs/remotes/origin/master", "refs/remotes/origin/main",
		"refs/heads/master", "refs/heads/main",
	} {
		if ref, err := repo.Reference(plumbing.ReferenceName(name), true); err == nil {
			return ref.Hash(), nil
		}
	}
	head, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return head.Hash(), nil
}

// diffVersions returns the crate versions added between two commits.
//
// The crates.io index stores one JSON-lines file per crate, so a change is
// found by diffing the two trees and parsing the lines that are new in the
// file's later revision.
func diffVersions(repo *git.Repository, from *plumbing.Hash, to plumbing.Hash) ([]Change, error) {
	toCommit, err := repo.CommitObject(to)
	if err != nil {
		return nil, err
	}
	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, err
	}

	// Without a recorded position the whole index is new.
	if from == nil {
		return allVersions(toTree)
	}

	fromCommit, err := repo.CommitObject(*from)
	if err != nil {
		// The recorded commit is gone (a rewritten or pruned history), so fall
		// back to reporting everything rather than failing.
		return allVersions(toTree)
	}
	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return nil, err
	}

	var out []Change
	for _, ch := range changes {
		action, err := ch.Action()
		if err != nil {
			return nil, err
		}
		_, toFile, err := ch.Files()
		if err != nil || toFile == nil {
			continue
		}
		content, err := toFile.Contents()
		if err != nil {
			continue
		}
		isInsert := action == merkletrie.Insert
		var previous string
		if !isInsert {
			// Not an insertion, so diff against the file's previous revision
			// to report only the version lines that are actually new.
			if fromFile, _, err := ch.Files(); err == nil && fromFile != nil {
				previous, _ = fromFile.Contents()
			}
		}
		out = append(out, newLines(content, previous, isInsert)...)
	}
	return out, nil
}

func allVersions(tree *object.Tree) ([]Change, error) {
	var out []Change
	err := tree.Files().ForEach(func(f *object.File) error {
		if isMetadataPath(f.Name) {
			return nil
		}
		content, err := f.Contents()
		if err != nil {
			return nil
		}
		out = append(out, newLines(content, "", true)...)
		return nil
	})
	return out, err
}

// isMetadataPath skips the index's own bookkeeping files, which are not crates.
func isMetadataPath(name string) bool {
	return name == "config.json" || strings.HasPrefix(name, ".")
}

// newLines parses the JSON-lines records present in current but not previous.
func newLines(current, previous string, isInsert bool) []Change {
	seen := map[string]bool{}
	for _, line := range strings.Split(previous, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			seen[line] = true
		}
	}

	kind := "Added"
	if !isInsert && previous != "" {
		kind = "Changed"
	}

	var out []Change
	for _, line := range strings.Split(current, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		c, ok := parseVersionLine(line)
		if !ok {
			continue
		}
		c.kind = kind
		if c.Yanked {
			c.kind = "Yanked"
		}
		out = append(out, c)
	}
	return out
}

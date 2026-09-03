// Command crates lets you interact with crates.io from the command-line.
//
// This is a Go port of https://github.com/Byron/crates-io-cli. It is a
// behavioural migration: the CLI surface, the rendered tables and the JSON
// output are reproduced from the Rust original and pinned by tests that
// compare against output captured from the Rust binary.
//
// Scope: the `list`, `search` and `recent-changes` subcommands are ported. The
// Rust build additionally offers `mine`, which is a single line delegating to
// the separate `criner-cli` crate; that application is out of scope for this
// migration and the subcommand is not offered. See truth.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Byron/crates-io-cli/internal/args"
	"github.com/Byron/crates-io-cli/internal/cliexit"
	"github.com/Byron/crates-io-cli/internal/httputil"
	"github.com/Byron/crates-io-cli/internal/scmds/list"
	"github.com/Byron/crates-io-cli/internal/scmds/recents"
	"github.com/Byron/crates-io-cli/internal/scmds/search"
	"github.com/Byron/crates-io-cli/internal/termion"
)

func main() {
	parsed, err := args.Parse(os.Args[1:])
	if err != nil {
		var ex *args.Exit
		if errors.As(err, &ex) {
			w := os.Stderr
			if ex.Stdout {
				w = os.Stdout
			}
			fmt.Fprint(w, ex.Message)
			os.Exit(ex.Code)
		}
		cliexit.OkOrExit(os.Stdout, err)
		return
	}

	ctx := context.Background()
	boldTitles := termion.IsTerminal(os.Stdout)

	switch parsed.Sub {
	case args.SubRecentChanges:
		cliexit.OkOrExit(os.Stdout, recents.Handle(
			os.Stdout, parsed.Repository, outputKind(parsed.RecentOutput), boldTitles))
	case args.SubList:
		crates, err := list.ByUser(ctx, httputil.NewHTTPClient(), parsed.ByUserID)
		if err != nil {
			cliexit.OkOrExit(os.Stdout, err)
			return
		}
		cliexit.OkOrExit(os.Stdout, list.Handle(
			os.Stdout, outputKind(parsed.ListOutput), boldTitles, crates))
	case args.SubSearch:
		cliexit.OkOrExit(os.Stdout, search.HandleInteractive(ctx))
	default:
		// With no subcommand the Rust falls through to the interactive
		// search, which is the crate's default behaviour when the `search`
		// feature is enabled.
		cliexit.OkOrExit(os.Stdout, search.HandleInteractive(ctx))
	}
}

// outputKind bridges the CLI enum to the one the subcommands use, keeping the
// arg parser free of a dependency on them.
func outputKind(k args.OutputKind) list.OutputKind {
	if k == args.OutputJSON {
		return list.OutputJSON
	}
	return list.OutputHuman
}

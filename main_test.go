package main

import (
	"testing"

	"github.com/Byron/crates-io-cli/internal/args"
	"github.com/Byron/crates-io-cli/internal/scmds/list"
)

// TestOutputKindBridge pins the mapping between the CLI enum and the one the
// subcommands use, which keeps the parser free of a dependency on them.
func TestOutputKindBridge(t *testing.T) {
	if got := outputKind(args.OutputHuman); got != list.OutputHuman {
		t.Errorf("human mapped to %v", got)
	}
	if got := outputKind(args.OutputJSON); got != list.OutputJSON {
		t.Errorf("json mapped to %v", got)
	}
}

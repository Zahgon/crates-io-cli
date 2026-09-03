// Package args help texts.
//
// Every string below is a verbatim transcription of the bytes emitted by the
// Rust binary (clap 4.6). They are stored as literals rather than regenerated
// from a table because the exact wrapping, ordering and the long/short help
// split are externally observable, and reproducing clap's line-breaking would
// be a larger and more fragile surface than the text itself.
//
// Pinned byte-for-byte by TestCLIGolden against testdata/.
//
// Note the long/short distinction: an argument with a doc paragraph renders in
// full under --help and abbreviated under -h, and clap changes the trailing
// hint accordingly ("see more with '--help'" vs "see a summary with '-h'").
package args

// Usage lines, reused by the error paths.
const (
	usageRoot          = "crates [COMMAND]"
	usageList          = "crates list [OPTIONS] <COMMAND>"
	usageByUser        = "crates list by-user <ID>"
	usageSearch        = "crates search"
	usageRecentChanges = "crates recent-changes [OPTIONS]"
	usageHelp          = "crates help [COMMAND]..."
)

// RootHelp returns the top-level `--help` output.
func RootHelp() string {
	return `Interact with crates.io from the command-line

Usage: crates [COMMAND]

Commands:
  recent-changes  show all recently changed crates
  search          search crates interactively
  list            list crates by a particular criterion
  help            Print this message or the help of the given subcommand(s)

Options:
  -h, --help  Print help
`
}

// ListHelp returns `crates list --help`.
func ListHelp() string {
	return `list crates by a particular criterion

Usage: crates list [OPTIONS] <COMMAND>

Commands:
  by-user  crates for the given user id
  help     Print this message or the help of the given subcommand(s)

Options:
  -o, --output <OUTPUT_FORMAT>  The type of output to produce [default: human] [possible values: human, json]
  -h, --help                    Print help
`
}

// ByUserHelp returns `crates list by-user --help`.
func ByUserHelp() string {
	return `crates for the given user id

Usage: crates list by-user <ID>

Arguments:
  <ID>  The numerical id of your user, e.g. 980. Currently there is no way to easily obtain it though, so you will have to debug actual crates.io calls in your browser - the /me response contains all user data. Use any string to receive *all* crates!

Options:
  -h, --help  Print help
`
}

// SearchHelp returns `crates search --help`.
func SearchHelp() string {
	return `search crates interactively

Usage: crates search

Options:
  -h, --help  Print help
`
}

// RecentChangesLongHelp returns `crates recent-changes --help` (the long form).
func RecentChangesLongHelp() string {
	return `show all recently changed crates

The output of this command is based on the state of the current crates.io repository clone. It will remember the last result, so that the next invocation might yield different (or no) changed crates at all. Please note that the first query is likely to yield more than 40000 results! The first invocation may be slow as it might have to clone the crates.io index.

Usage: crates recent-changes [OPTIONS]

Options:
  -r, --repository <REPO>
          Path to the possibly existing crates.io repository clone. If unset, it will be cloned to a temporary spot

  -o, --output <OUTPUT_FORMAT>
          The type of output to produce
          
          [default: human]
          [possible values: human, json]

  -h, --help
          Print help (see a summary with '-h')
`
}

// RecentChangesShortHelp returns `crates recent-changes -h` (the short form).
func RecentChangesShortHelp() string {
	return `show all recently changed crates

Usage: crates recent-changes [OPTIONS]

Options:
  -r, --repository <REPO>       Path to the possibly existing crates.io repository clone. If unset, it will be cloned to a temporary spot
  -o, --output <OUTPUT_FORMAT>  The type of output to produce [default: human] [possible values: human, json]
  -h, --help                    Print help (see more with '--help')
`
}

// HelpHelp returns `crates help help`.
func HelpHelp() string {
	return `Print this message or the help of the given subcommand(s)

Usage: crates help [COMMAND]...

Arguments:
  [COMMAND]...  Print help for the subcommand(s)
`
}

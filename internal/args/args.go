// Package args is a port of src/args.rs together with the parts of clap v4
// that this tool's command line depends on.
//
// The help text, the error messages and the exit codes are externally
// observable, so this is a hand-written parser reproducing clap's output
// rather than an idiomatic `flag`-package CLI. Every expected string is pinned
// by golden files captured from the Rust binary in testdata/.
//
// The command tree is:
//
//	crates
//	├── recent-changes [-r REPO] [-o human|json]
//	├── search
//	├── list [-o human|json] <by-user ID>
//	└── help [SUBCOMMAND]
package args

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PackageBinName is the crate's declared binary name, from `[[bin]] name` in
// Cargo.toml.
const PackageBinName = "crates"

// binName returns the command name clap prints in usage lines.
//
// clap takes this from argv[0], not from the manifest: renaming or symlinking
// the binary changes every "Usage:" line. Hardcoding the package name would
// silently diverge for anyone who installs it under another name -- and note
// that the *prose* "crates.io" must not change with it, so the substitution is
// applied only where a command name appears.
func binName() string {
	if len(os.Args) == 0 {
		return PackageBinName
	}
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	if base == "" || base == "." || base == string(filepath.Separator) {
		return PackageBinName
	}
	return base
}

// withBinName rewrites the command name in rendered help and usage text.
//
// The help literals are transcribed from a binary invoked as "crates", so the
// command name is replaced only at the start of a usage line or usage string,
// leaving "crates.io" in the description untouched.
func withBinName(text string) string {
	name := binName()
	if name == PackageBinName {
		return text
	}
	text = strings.ReplaceAll(text, "Usage: "+PackageBinName+" ", "Usage: "+name+" ")
	text = strings.ReplaceAll(text, "Usage: "+PackageBinName+"\n", "Usage: "+name+"\n")
	return text
}

// About is the crate description, from `#[clap(about = ...)]`.
const About = "Interact with crates.io from the command-line"

// OutputKind mirrors the `OutputKind` value enum.
type OutputKind int

// The output formats, in clap declaration order.
const (
	OutputHuman OutputKind = iota
	OutputJSON
)

func (o OutputKind) String() string {
	if o == OutputJSON {
		return "json"
	}
	return "human"
}

// parseOutputKind accepts exactly the value-enum variants, lowercased by clap.
func parseOutputKind(s string) (OutputKind, bool) {
	switch s {
	case "human":
		return OutputHuman, true
	case "json":
		return OutputJSON, true
	}
	return 0, false
}

// SubCommand identifies which branch of the command tree was selected.
type SubCommand int

// The subcommands.
const (
	// SubNone means no subcommand was given.
	SubNone SubCommand = iota
	SubRecentChanges
	SubSearch
	SubList
)

// Parsed is the result of a successful parse, mirroring the `Parsed` struct.
type Parsed struct {
	Sub SubCommand

	// RecentChanges
	Repository   *string
	RecentOutput OutputKind

	// List
	ListOutput OutputKind
	ByUserID   uint32
}

// Exit is a request to terminate the process the way clap would.
type Exit struct {
	Code int
	// Stdout reports whether the message goes to stdout (help) or stderr.
	Stdout  bool
	Message string
}

func (e *Exit) Error() string { return e.Message }

// usageError renders clap's value-validation error block, which carries no
// usage line.
func usageError(msg string) *Exit {
	return &Exit{
		Code:    2,
		Message: fmt.Sprintf("error: %s\n\nFor more information, try '--help'.\n", msg),
	}
}

// usageErrorWithUsage renders clap's structural error block, which does.
func usageErrorWithUsage(msg, usage string) *Exit {
	return &Exit{
		Code: 2,
		Message: fmt.Sprintf("error: %s\n\nUsage: %s\n\nFor more information, try '--help'.\n",
			msg, withBinName("Usage: " + usage)[len("Usage: "):]),
	}
}

// helpExit prints help on stdout and exits 0.
func helpExit(text string) *Exit {
	return &Exit{Code: 0, Stdout: true, Message: withBinName(text)}
}

// Parse parses the arguments after the program name.
func Parse(argv []string) (*Parsed, error) {
	p := &Parsed{}

	if len(argv) == 0 {
		return p, nil
	}

	switch argv[0] {
	case "-h", "--help":
		return nil, helpExit(RootHelp())
	case "help":
		return nil, parseHelpSubcommand(argv[1:])
	case "recent-changes":
		return parseRecentChanges(p, argv[1:])
	case "search":
		return parseSearch(p, argv[1:])
	case "list":
		return parseList(p, argv[1:])
	}

	if strings.HasPrefix(argv[0], "-") {
		return nil, usageErrorWithUsage(
			fmt.Sprintf("unexpected argument '%s' found", argv[0]), usageRoot)
	}
	return nil, usageErrorWithUsage(
		fmt.Sprintf("unrecognized subcommand '%s'", argv[0]), usageRoot)
}

// parseHelpSubcommand implements `crates help [SUBCOMMAND]`, which prints the
// long help for the named subcommand.
func parseHelpSubcommand(rest []string) error {
	if len(rest) == 0 {
		return helpExit(RootHelp())
	}
	switch rest[0] {
	case "recent-changes":
		return helpExit(RecentChangesLongHelp())
	case "search":
		return helpExit(SearchHelp())
	case "list":
		return helpExit(ListHelp())
	case "help":
		return helpExit(HelpHelp())
	}
	return usageErrorWithUsage(
		fmt.Sprintf("unrecognized subcommand '%s'", rest[0]), usageHelp)
}

func parseSearch(p *Parsed, rest []string) (*Parsed, error) {
	for _, a := range rest {
		switch a {
		case "-h", "--help":
			return nil, helpExit(SearchHelp())
		default:
			return nil, usageErrorWithUsage(
				fmt.Sprintf("unexpected argument '%s' found", a), usageSearch)
		}
	}
	p.Sub = SubSearch
	return p, nil
}

func parseRecentChanges(p *Parsed, rest []string) (*Parsed, error) {
	p.Sub = SubRecentChanges
	p.RecentOutput = OutputHuman

	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, inline, hasInline := splitFlag(a)

		switch name {
		case "-h":
			return nil, helpExit(RecentChangesShortHelp())
		case "--help":
			return nil, helpExit(RecentChangesLongHelp())
		case "-r", "--repository":
			v, err := takeValue(rest, &i, inline, hasInline, "--repository <REPO>")
			if err != nil {
				return nil, err
			}
			p.Repository = &v
		case "-o", "--output":
			v, err := takeValue(rest, &i, inline, hasInline, "--output <OUTPUT_FORMAT>")
			if err != nil {
				return nil, err
			}
			k, ok := parseOutputKind(v)
			if !ok {
				return nil, invalidOutputValue(v)
			}
			p.RecentOutput = k
		default:
			return nil, usageErrorWithUsage(
				fmt.Sprintf("unexpected argument '%s' found", a), usageRecentChanges)
		}
	}
	return p, nil
}

func parseList(p *Parsed, rest []string) (*Parsed, error) {
	p.Sub = SubList
	p.ListOutput = OutputHuman

	i := 0
	for ; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "-") {
			break
		}
		name, inline, hasInline := splitFlag(a)
		switch name {
		case "-h", "--help":
			return nil, helpExit(ListHelp())
		case "-o", "--output":
			v, err := takeValue(rest, &i, inline, hasInline, "--output <OUTPUT_FORMAT>")
			if err != nil {
				return nil, err
			}
			k, ok := parseOutputKind(v)
			if !ok {
				return nil, invalidOutputValue(v)
			}
			p.ListOutput = k
		default:
			return nil, usageErrorWithUsage(
				fmt.Sprintf("unexpected argument '%s' found", a), usageList)
		}
	}

	// A missing subcommand prints the list help on *stderr* and exits 2. That
	// is clap's required-subcommand behaviour: the same text as --help, but
	// treated as a diagnostic rather than as requested output.
	if i >= len(rest) {
		return nil, &Exit{Code: 2, Message: withBinName(ListHelp())}
	}

	switch rest[i] {
	case "by-user":
		return parseByUser(p, rest[i+1:])
	case "help":
		if i+1 < len(rest) && rest[i+1] == "by-user" {
			return nil, helpExit(ByUserHelp())
		}
		return nil, helpExit(ListHelp())
	}
	return nil, usageErrorWithUsage(
		fmt.Sprintf("unrecognized subcommand '%s'", rest[i]), usageList)
}

func parseByUser(p *Parsed, rest []string) (*Parsed, error) {
	var idArg *string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			// Everything after -- is positional.
			for _, v := range rest[i+1:] {
				v := v
				if idArg == nil {
					idArg = &v
				} else {
					return nil, usageErrorWithUsage(
						fmt.Sprintf("unexpected argument '%s' found", v), usageByUser)
				}
			}
			break
		}
		if a == "-h" || a == "--help" {
			return nil, helpExit(ByUserHelp())
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			// by-user takes no options, so a flag here is an unexpected
			// argument -- this is why `list by-user 5 --output json` fails
			// while `list --output json by-user 5` works.
			return nil, unexpectedArgWithTip(a, usageByUser)
		}
		if idArg != nil {
			return nil, usageErrorWithUsage(
				fmt.Sprintf("unexpected argument '%s' found", a), usageByUser)
		}
		v := a
		idArg = &v
	}

	if idArg == nil {
		return nil, usageErrorWithUsage(
			"the following required arguments were not provided:\n  <ID>", usageByUser)
	}

	id, err := parseU32(*idArg)
	if err != nil {
		return nil, usageError(fmt.Sprintf("invalid value '%s' for '<ID>': %s", *idArg, err))
	}
	p.ByUserID = id
	return p, nil
}

// unexpectedArgWithTip renders the variant of clap's unexpected-argument error
// that suggests `-- value`, which it emits when a positional could have
// accepted the token.
func unexpectedArgWithTip(arg, usage string) *Exit {
	return usageErrorWithUsage(
		fmt.Sprintf("unexpected argument '%s' found\n\n  tip: to pass '%s' as a value, use '-- %s'",
			arg, arg, arg), usage)
}

func invalidOutputValue(v string) *Exit {
	return usageError(fmt.Sprintf(
		"invalid value '%s' for '--output <OUTPUT_FORMAT>'\n  [possible values: human, json]", v))
}

// splitFlag separates `--name=value` into its parts. A short flag never takes
// an attached value here.
func splitFlag(a string) (name, inline string, hasInline bool) {
	if strings.HasPrefix(a, "--") {
		n, v, ok := strings.Cut(a, "=")
		return n, v, ok
	}
	return a, "", false
}

// takeValue resolves an option's value from `--flag=value` or the next argv
// element.
func takeValue(rest []string, i *int, inline string, hasInline bool, display string) (string, error) {
	if hasInline {
		return inline, nil
	}
	if *i+1 >= len(rest) {
		return "", usageError(fmt.Sprintf(
			"a value is required for '%s' but none was supplied", display))
	}
	*i++
	return rest[*i], nil
}

// parseU32 reproduces clap's two-stage validation of a u32 argument, which
// yields three distinct messages a single ParseUint cannot express:
//
//	"abc"                  -> invalid digit found in string       (not a number)
//	"-1", "4294967296"     -> N is not in 0..=4294967295           (number, out of range)
//	"99999999999999999999" -> number too large to fit in target type (beyond i64)
//
// The middle case is clap's range check, so it must run *after* a successful
// integer parse rather than being folded into it.
func parseU32(s string) (uint32, error) {
	if s == "" {
		return 0, fmt.Errorf("cannot parse integer from empty string")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			if strings.HasPrefix(s, "-") {
				return 0, fmt.Errorf("number too small to fit in target type")
			}
			return 0, fmt.Errorf("number too large to fit in target type")
		}
		return 0, fmt.Errorf("invalid digit found in string")
	}
	if n < 0 || n > 4294967295 {
		return 0, fmt.Errorf("%s is not in 0..=4294967295", s)
	}
	return uint32(n), nil
}

package args

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// asBinary pins os.Args[0] for the duration of a test, so usage lines render
// with a known command name. clap takes that name from argv[0], which under
// `go test` would otherwise be the test binary.
func asBinary(t *testing.T, name string) {
	t.Helper()
	orig := os.Args
	os.Args = append([]string{name}, orig[1:]...)
	t.Cleanup(func() { os.Args = orig })
}

// runCase drives Parse the way main does and returns what would reach stdout
// and stderr plus the process exit code.
func runCase(argv []string) (stdout, stderr string, code int) {
	_, err := Parse(argv)
	if err == nil {
		return "", "", 0
	}
	var ex *Exit
	if !errors.As(err, &ex) {
		return "", err.Error(), 1
	}
	if ex.Stdout {
		return ex.Message, "", ex.Code
	}
	return "", ex.Message, ex.Code
}

// TestCLIGolden replays every clap-level invocation captured from the real
// Rust binary and requires byte-identical stdout, stderr and exit code.
//
// Invocations that parse successfully are not listed here; those are covered
// by the recorded-response differential in internal/scmds/list, which compares
// the actual rendered output.
func TestCLIGolden(t *testing.T) {
	// The golden files were captured from a binary invoked as "crates".
	asBinary(t, "/usr/local/bin/crates")

	cases := []struct {
		name string
		argv []string
	}{
		{"help", []string{"--help"}},
		{"help_short", []string{"-h"}},
		{"no_args_help", []string{"help"}},
		{"list_help", []string{"list", "--help"}},
		{"list_help_short", []string{"list", "-h"}},
		{"list_byuser_help", []string{"list", "by-user", "--help"}},
		{"list_byuser_help_short", []string{"list", "by-user", "-h"}},
		{"recent_help", []string{"recent-changes", "--help"}},
		{"recent_help_short", []string{"recent-changes", "-h"}},
		{"search_help", []string{"search", "--help"}},
		{"search_help_short", []string{"search", "-h"}},
		{"help_sub_list", []string{"help", "list"}},
		{"help_search", []string{"help", "search"}},
		{"help_recent", []string{"help", "recent-changes"}},
		{"help_help", []string{"help", "help"}},
		{"unknown_sub", []string{"bogus"}},
		{"unknown_flag", []string{"--bogus"}},
		{"list_no_sub", []string{"list"}},
		{"list_bad_id", []string{"list", "by-user", "abc"}},
		{"list_neg_id", []string{"list", "by-user", "--", "-1"}},
		{"list_bad_output", []string{"list", "by-user", "1", "--output", "bogus"}},
		{"list_bad_output_value", []string{"list", "--output", "bogus", "by-user", "1"}},
		{"list_byuser_missing", []string{"list", "by-user"}},
		{"list_byuser_extra", []string{"list", "by-user", "1", "2"}},
		{"list_output_after", []string{"list", "by-user", "5", "--output", "json"}},
		{"recent_bad_output", []string{"recent-changes", "--output", "bogus"}},
		{"recent_bad_repo_flag", []string{"recent-changes", "--repository"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotOut, gotErr, gotCode := runCase(c.argv)
			wantOut := readGolden(t, c.name+".stdout")
			wantErr := readGolden(t, c.name+".stderr")
			wantCode := readGoldenInt(t, c.name+".exit")

			if gotOut != wantOut {
				t.Errorf("stdout mismatch\n--- got ---\n%s\n--- want ---\n%s", gotOut, wantOut)
			}
			if gotErr != wantErr {
				t.Errorf("stderr mismatch\n--- got ---\n%s\n--- want ---\n%s", gotErr, wantErr)
			}
			if gotCode != wantCode {
				t.Errorf("exit code = %d, want %d", gotCode, wantCode)
			}
		})
	}
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading golden %s: %v", name, err)
	}
	return string(b)
}

func readGoldenInt(t *testing.T, name string) int {
	t.Helper()
	code := 0
	for _, c := range readGolden(t, name) {
		if c >= '0' && c <= '9' {
			code = code*10 + int(c-'0')
		}
	}
	return code
}

// TestParseSuccess covers the accepting paths, which produce no output and so
// cannot be checked by the golden files.
func TestParseSuccess(t *testing.T) {
	cases := []struct {
		name  string
		argv  []string
		check func(*testing.T, *Parsed)
	}{
		{"no args", nil, func(t *testing.T, p *Parsed) {
			if p.Sub != SubNone {
				t.Errorf("Sub = %v, want SubNone", p.Sub)
			}
		}},
		{"search", []string{"search"}, func(t *testing.T, p *Parsed) {
			if p.Sub != SubSearch {
				t.Errorf("Sub = %v, want SubSearch", p.Sub)
			}
		}},
		{"list by-user default output", []string{"list", "by-user", "980"},
			func(t *testing.T, p *Parsed) {
				if p.Sub != SubList || p.ByUserID != 980 || p.ListOutput != OutputHuman {
					t.Errorf("got %+v", p)
				}
			}},
		{"list long output before subcommand", []string{"list", "--output", "json", "by-user", "5"},
			func(t *testing.T, p *Parsed) {
				if p.ListOutput != OutputJSON || p.ByUserID != 5 {
					t.Errorf("got %+v", p)
				}
			}},
		{"list short output", []string{"list", "-o", "json", "by-user", "5"},
			func(t *testing.T, p *Parsed) {
				if p.ListOutput != OutputJSON {
					t.Errorf("got %+v", p)
				}
			}},
		{"list attached output", []string{"list", "--output=json", "by-user", "5"},
			func(t *testing.T, p *Parsed) {
				if p.ListOutput != OutputJSON {
					t.Errorf("got %+v", p)
				}
			}},
		{"by-user max id", []string{"list", "by-user", "4294967295"},
			func(t *testing.T, p *Parsed) {
				if p.ByUserID != 4294967295 {
					t.Errorf("ByUserID = %d", p.ByUserID)
				}
			}},
		{"recent-changes defaults", []string{"recent-changes"}, func(t *testing.T, p *Parsed) {
			if p.Sub != SubRecentChanges || p.Repository != nil || p.RecentOutput != OutputHuman {
				t.Errorf("got %+v", p)
			}
		}},
		{"recent-changes with repo", []string{"recent-changes", "-r", "/tmp/idx", "-o", "json"},
			func(t *testing.T, p *Parsed) {
				if p.Repository == nil || *p.Repository != "/tmp/idx" || p.RecentOutput != OutputJSON {
					t.Errorf("got %+v", p)
				}
			}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Parse(c.argv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			c.check(t, p)
		})
	}
}

// TestParseU32Messages pins the integer diagnostics clap embeds for <ID>.
func TestParseU32Messages(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "invalid digit found in string"},
		{"-1", "-1 is not in 0..=4294967295"},
		{"4294967296", "4294967296 is not in 0..=4294967295"},
		{"", "cannot parse integer from empty string"},
		{"99999999999999999999", "number too large to fit in target type"},
	}
	for _, c := range cases {
		if _, err := parseU32(c.in); err == nil || err.Error() != c.want {
			t.Errorf("parseU32(%q) = %v, want %q", c.in, err, c.want)
		}
	}
	for _, ok := range []string{"0", "1", "4294967295", "0980"} {
		if _, err := parseU32(ok); err != nil {
			t.Errorf("parseU32(%q) errored: %v", ok, err)
		}
	}
}

// TestOutputKindRoundTrip pins the value-enum spellings clap accepts and
// prints.
func TestOutputKindRoundTrip(t *testing.T) {
	for _, s := range []string{"human", "json"} {
		k, ok := parseOutputKind(s)
		if !ok {
			t.Fatalf("parseOutputKind(%q) failed", s)
		}
		if k.String() != s {
			t.Errorf("round trip: %q -> %v -> %q", s, k, k.String())
		}
	}
	for _, s := range []string{"Human", "JSON", "", "bogus"} {
		if _, ok := parseOutputKind(s); ok {
			t.Errorf("parseOutputKind(%q) unexpectedly succeeded", s)
		}
	}
}

// TestExitError pins that *Exit satisfies the error interface with the message
// main prints, so errors.As in main keeps working.
func TestExitError(t *testing.T) {
	_, err := Parse([]string{"bogus"})
	if err == nil {
		t.Fatal("expected an error")
	}
	var ex *Exit
	if !errors.As(err, &ex) {
		t.Fatalf("error is not *Exit: %T", err)
	}
	if err.Error() != ex.Message {
		t.Errorf("Error() = %q, want the Exit message", err.Error())
	}
	if ex.Code != 2 || ex.Stdout {
		t.Errorf("Code=%d Stdout=%v, want 2/false", ex.Code, ex.Stdout)
	}
}

// TestUsageFollowsArgv0 pins that the command name in usage lines tracks
// argv[0] while the prose does not.
//
// This is the difference the behaviour gate caught: the QC harness copies the
// binary to "qcapp", and a hardcoded name diverged by exactly one character on
// every help fixture. The "crates.io" in the description must stay put.
func TestUsageFollowsArgv0(t *testing.T) {
	for _, name := range []string{"qcapp", "crates", "my-crates"} {
		t.Run(name, func(t *testing.T) {
			asBinary(t, "/tmp/"+name)

			stdout, _, _ := runCase([]string{"--help"})
			if want := "Usage: " + name + " [COMMAND]"; !strings.Contains(stdout, want) {
				t.Errorf("missing %q in root help", want)
			}
			// The description names the website, not the binary.
			if !strings.Contains(stdout, "Interact with crates.io from the command-line") {
				t.Error("the description must not be rewritten")
			}

			stdout, _, _ = runCase([]string{"list", "--help"})
			if want := "Usage: " + name + " list [OPTIONS] <COMMAND>"; !strings.Contains(stdout, want) {
				t.Errorf("missing %q in list help", want)
			}

			_, stderr, code := runCase([]string{"bogus"})
			if want := "Usage: " + name + " [COMMAND]"; !strings.Contains(stderr, want) {
				t.Errorf("missing %q in the error usage line", want)
			}
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
		})
	}
}

// TestBinNameFallbacks covers the degenerate argv[0] values.
func TestBinNameFallbacks(t *testing.T) {
	cases := []struct{ argv0, want string }{
		{"/usr/local/bin/crates", "crates"},
		{"crates", "crates"},
		{"qcapp", "qcapp"},
		{"crates.exe", "crates"},
		{"", "crates"},
	}
	for _, c := range cases {
		asBinary(t, c.argv0)
		if got := binName(); got != c.want {
			t.Errorf("binName() with argv[0]=%q = %q, want %q", c.argv0, got, c.want)
		}
	}
}

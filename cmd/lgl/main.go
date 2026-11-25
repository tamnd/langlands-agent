// Command lgl is the toolchain of langlands. It checks the map of the content repository, prints the status of each node, copies the record of the Lean build, runs the experiments of the lab and builds the site langlands.tamnd.com.
//
// Exit codes: 0 for success, 1 for a failed check or a failed run, 2 for a usage error, and 3 for a command that is not written yet.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

// Version is the version of lgl. The release build sets it with -ldflags "-X main.Version=v0.1.0".
var Version = "dev"

// errUsage is a usage error. main exits with code 2.
var errUsage = errors.New("usage error")

// command is one subcommand of lgl.
type command struct {
	name      string
	summary   string
	milestone string // the milestone that adds the command, when it is not written yet
	run       func(args []string, stdout, stderr io.Writer) error
}

// commands returns the subcommands of lgl, in the order of the help text.
func commands() []command {
	return []command{
		{name: "version", summary: "print the version of lgl", run: runVersion},
		{name: "check", summary: "check the map: the tags, the links, the papers, the routes and the Lean names", run: runCheck},
		{name: "status", summary: "print the status of each node, from the record of the Lean build", run: runStatus},
		{name: "index", summary: "check the record of the Lean build and copy it to records/lean.json", run: runIndex},
		{name: "site", summary: "build, check or serve the site", run: runSite},
		{name: "trust", summary: "run the axiom check and the kernel replay, and write the trust report", milestone: "S1"},
		{name: "fidelity", summary: "list, show or set the fidelity checks F1 to F7 of a statement in records/fidelity.json", run: runFidelity},
		{name: "port", summary: "check the ported Lean files against manifests/ported.yaml (add and diff come in milestone S1)", run: runPort},
		{name: "paper", summary: "add, list or show a paper record", milestone: "S1"},
		{name: "claim", summary: "add, list, show or plan a fresh claim of route R1", milestone: "S2"},
		{name: "lab", summary: "run an experiment of the computation lab", milestone: "S1"},
		{name: "queue", summary: "print the work queue of a role, with the reason for each rank", milestone: "S2"},
		{name: "agent", summary: "run one role of the agent within the budget, or write the weekly report", milestone: "S2"},
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run runs lgl with the arguments args and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		usage(stdout)
		return 0
	}
	if args[0] == "-version" || args[0] == "--version" {
		args = []string{"version"}
	}
	for _, c := range commands() {
		if c.name != args[0] {
			continue
		}
		if c.run == nil {
			fmt.Fprintf(stderr, "lgl %s: this command is not written yet. Milestone %s adds it.\n", c.name, c.milestone)
			return 3
		}
		err := c.run(args[1:], stdout, stderr)
		switch {
		case err == nil, errors.Is(err, flag.ErrHelp):
			return 0
		case errors.Is(err, errUsage):
			fmt.Fprintf(stderr, "lgl %s: %v\n", c.name, err)
			return 2
		default:
			fmt.Fprintf(stderr, "lgl %s: %v\n", c.name, err)
			return 1
		}
	}
	fmt.Fprintf(stderr, "lgl: unknown command %q. Run lgl help to see the commands.\n", args[0])
	return 2
}

// usage writes the help text.
func usage(w io.Writer) {
	fmt.Fprintln(w, "lgl is the toolchain of langlands: check the map, read the Lean build and build langlands.tamnd.com.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: lgl <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	cs := commands()
	width := 0
	for _, c := range cs {
		width = max(width, len(c.name))
	}
	for _, c := range cs {
		note := ""
		if c.run == nil {
			note = " (not written yet, milestone " + c.milestone + ")"
		}
		fmt.Fprintf(w, "  %-*s  %s%s\n", width, c.name, c.summary, note)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run lgl <command> -h to see the flags of a command.")
}

func runVersion(args []string, stdout, _ io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: version takes no arguments", errUsage)
	}
	fmt.Fprintln(stdout, "lgl", Version)
	return nil
}

// names returns the names of the commands, sorted. The tests use it.
func names() []string {
	var out []string
	for _, c := range commands() {
		out = append(out, c.name)
	}
	sort.Strings(out)
	return out
}

// parse parses the flags of a subcommand and makes a usage error of a bad flag.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w: %s takes no arguments", errUsage, fs.Name())
	}
	return nil
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/langlands-agent/site"
)

const labSmallSpec = `name: e1-small
experiment: E1
question: Do the small cases agree?
tool: lgl
node: NODE
inputs: {primes: [2, 3], box: 10, max_conductor: 9, samples: 20, coefficients: 2, seed: 3}
outputs: [quadratic.json, cyclotomic.json, summary.md]
`

// labTree writes a content repository with the node LG00000001 and, when spec is true, a spec of E1 for that node.
func labTree(t *testing.T, spec bool, node string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "blueprint", "nodes", "LG00000001.yaml"), "tag: LG00000001\nkind: theorem\ntitle: t\nstatement: s\ntier: T3\n")
	if spec {
		writeFile(t, filepath.Join(root, "lab", "specs", "e1-small.yaml"), strings.Replace(labSmallSpec, "NODE", node, 1))
	}
	return root
}

func onlyRun(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "lab", "runs"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("lab/runs has %v (%v)", entries, err)
	}
	return entries[0].Name()
}

func TestLabCommands(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  bool   // write lab/specs/e1-small.yaml
		node  string // the node of the spec
		pre   []string
		args  []string
		code  int
		out   string
		check func(t *testing.T, root string)
	}{
		{name: "no subcommand", code: 2},
		{name: "unknown subcommand", args: []string{"frob"}, code: 2},
		{name: "list with nothing", args: []string{"list"}, code: 0, out: "runs in lab/runs: 0"},
		{name: "list a spec", spec: true, node: "LG00000001", args: []string{"list"}, code: 0, out: "e1-small.yaml  E1  LG00000001"},
		{name: "list a spec with no code", spec: true, node: "LG00000001", args: []string{"list"}, code: 0, out: "E3  no code yet"},
		{name: "spec E1", args: []string{"spec", "E1"}, code: 0, out: "experiment: E1"},
		{name: "spec E2", args: []string{"spec", "E2"}, code: 0, out: "experiment: E2"},
		{name: "spec E3", args: []string{"spec", "E3"}, code: 1},
		{name: "spec E10", args: []string{"spec", "E10"}, code: 2},
		{name: "run E1 with no spec", args: []string{"run", "E1"}, code: 1},
		{name: "run E2 with no spec", args: []string{"run", "E2"}, code: 1},
		{name: "run E3", args: []string{"run", "E3"}, code: 1},
		{name: "run E1", spec: true, node: "LG00000001", args: []string{"run", "-host", "test", "E1"}, code: 0, out: "agrees: in 14 fields", check: func(t *testing.T, root string) {
			id := onlyRun(t, root)
			data, err := os.ReadFile(filepath.Join(root, "lab", "runs", id, "record.json"))
			if err != nil {
				t.Fatal(err)
			}
			var rec site.LabRecord
			if err := json.Unmarshal(data, &rec); err != nil {
				t.Fatal(err)
			}
			if rec.ID != id || rec.Experiment != "E1" || rec.Node != "LG00000001" || rec.Tool != "lgl" || rec.Spec != "lab/specs/e1-small.yaml" || rec.Host != "test" {
				t.Errorf("the site reads the record as %+v", rec)
			}
			for name := range rec.Files {
				if _, err := os.Stat(filepath.Join(root, "lab", "runs", id, name)); err != nil {
					t.Errorf("the record names %s, which is not in the run", name)
				}
			}
		}},
		{name: "run a spec file", spec: true, node: "LG00000001", args: []string{"run", "SPEC"}, code: 0, out: "wrote"},
		{name: "run twice", spec: true, node: "LG00000001", pre: []string{"run", "E1"}, args: []string{"run", "E1"}, code: 1},
		{name: "run again with -force", spec: true, node: "LG00000001", pre: []string{"run", "E1"}, args: []string{"run", "-force", "E1"}, code: 0},
		{name: "run a spec of a node that is not in the map", spec: true, node: "LG99999999", args: []string{"run", "E1"}, code: 1},
		{name: "run a spec outside lab/specs", args: []string{"run", "OUTSIDE"}, code: 2},
		{name: "run with no spec", args: []string{"run"}, code: 2},
		{name: "show a run", spec: true, node: "LG00000001", pre: []string{"run", "E1"}, args: []string{"show", "RUN"}, code: 0, out: "## What the run does not do"},
		{name: "show an unknown run", args: []string{"show", "2026-11-25-nothing"}, code: 1},
		{name: "list a run", spec: true, node: "LG00000001", pre: []string{"run", "E1"}, args: []string{"list"}, code: 0, out: "runs in lab/runs: 1"},
	} {
		root := labTree(t, tc.spec, tc.node)
		outside := filepath.Join(t.TempDir(), "e1.yaml")
		writeFile(t, outside, strings.Replace(labSmallSpec, "NODE", "LG00000001", 1))
		subst := func(args []string) []string {
			out := append([]string(nil), args...)
			for i, a := range out {
				switch a {
				case "SPEC":
					out[i] = filepath.Join(root, "lab", "specs", "e1-small.yaml")
				case "OUTSIDE":
					out[i] = outside
				case "RUN":
					out[i] = onlyRun(t, root)
				}
			}
			switch {
			case len(out) == 0:
				return []string{"lab"}
			case out[0] == "spec":
				return append([]string{"lab"}, out...)
			}
			return append([]string{"lab", out[0], "-content", root}, out[1:]...)
		}
		var out, errb strings.Builder
		if tc.pre != nil {
			if got := run(subst(tc.pre), &out, &errb); got != 0 {
				t.Fatalf("%s: the first command exits %d (%s)", tc.name, got, errb.String())
			}
			out.Reset()
		}
		if got := run(subst(tc.args), &out, &errb); got != tc.code {
			t.Errorf("%s: exit %d, want %d (%s)", tc.name, got, tc.code, errb.String())
			continue
		}
		if !strings.Contains(out.String(), tc.out) {
			t.Errorf("%s: the output does not have %q:\n%s", tc.name, tc.out, out.String())
		}
		if tc.check != nil {
			tc.check(t, root)
		}
	}
}

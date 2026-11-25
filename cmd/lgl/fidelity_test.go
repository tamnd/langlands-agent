package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/langlands-agent/blueprint"
)

// contentTree writes a small content repository: a top statement, a plain statement, a statement with sorry, a conjecture and a definition.
func contentTree(t *testing.T, fidelity string) string {
	t.Helper()
	root := t.TempDir()
	node := func(tag, kind, stmt, extra string) {
		writeFile(t, filepath.Join(root, "blueprint", "nodes", tag+".yaml"), "tag: "+tag+"\nkind: "+kind+"\ntitle: t\nstatement: s\ntier: T3\nlean:\n  statement: "+stmt+"\n"+extra)
	}
	node("LG00000001", "theorem", "Langlands.LocalReciprocityGL1", "")
	node("LG00000002", "theorem", "Langlands.LocalExistence", "")
	node("LG00000003", "theorem", "Langlands.Broken", "")
	node("LG00000004", "conjecture", "Langlands.Open", "")
	node("LG00000005", "definition", "Langlands.WeilGroup", "")
	node("LG00000006", "theorem", "Langlands.Axiom", "")
	writeFile(t, filepath.Join(root, "records", "lean.json"), `{"repository":"https://github.com/tamnd/langlands-lean","commit":"`+goodCommit+`","toolchain":"4.35.0","decls":{
"Langlands.LocalReciprocityGL1":{"kind":"def","axioms":["propext","Classical.choice","Quot.sound"]},
"Langlands.LocalExistence":{"kind":"def","axioms":["propext"]},
"Langlands.Broken":{"kind":"def","axioms":["sorryAx"]},
"Langlands.Open":{"kind":"def","axioms":[]},
"Langlands.WeilGroup":{"kind":"def","axioms":[]},
"Langlands.Axiom":{"kind":"def","axioms":["Langlands.myAxiom"]}}}`)
	if fidelity != "" {
		writeFile(t, filepath.Join(root, "records", "fidelity.json"), fidelity)
	}
	return root
}

func readFidelity(t *testing.T, root string) map[string]blueprint.Fidelity {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "records", "fidelity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]blueprint.Fidelity
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFidelityMissing(t *testing.T) {
	all := map[string]bool{"F1": true, "F2": true, "F3": true, "F4": true, "F5": true, "F6": true, "F7": true}
	two := []blueprint.Reviewer{{Name: "A", Group: "local", Date: "2026-12-01"}, {Name: "B", Group: "local", Date: "2026-12-02"}}
	for _, tc := range []struct {
		name string
		stmt string
		f    blueprint.Fidelity
		want string // a part of the reasons; empty when the entry passes
	}{
		{"theorem F1 to F5", "Langlands.LocalExistence", blueprint.Fidelity{Class: "theorem", Checks: map[string]bool{"F1": true, "F2": true, "F3": true, "F4": true, "F5": true}}, ""},
		{"theorem without F4", "Langlands.LocalExistence", blueprint.Fidelity{Class: "theorem", Checks: map[string]bool{"F1": true, "F2": true, "F3": true, "F5": true}}, "F4 is not passed"},
		{"top theorem needs F6", "Langlands.LocalReciprocityGL1", blueprint.Fidelity{Class: "theorem", Checks: map[string]bool{"F1": true, "F2": true, "F3": true, "F4": true, "F5": true}}, "F6 is not passed"},
		{"F6 needs two reviewers", "Langlands.LocalReciprocityGL1", blueprint.Fidelity{Class: "theorem", Checks: all, Reviewers: two[:1]}, "two reviewers"},
		{"top theorem with F6", "Langlands.LocalReciprocityGL1", blueprint.Fidelity{Class: "theorem", Checks: all, Reviewers: two}, ""},
		{"open needs F7", "Langlands.Open", blueprint.Fidelity{Class: "open", Checks: map[string]bool{"F1": true, "F2": true, "F3": true, "F4": true, "F5": true, "F6": true}, Reviewers: two}, "F7 is not passed"},
		{"open needs two groups", "Langlands.Open", blueprint.Fidelity{Class: "open", Checks: all, Reviewers: two}, "two groups"},
		{"open with two groups", "Langlands.Open", blueprint.Fidelity{Class: "open", Checks: all, Reviewers: []blueprint.Reviewer{two[0], {Name: "B", Group: "lifting", Date: "2026-12-02"}}}, ""},
		{"unknown class", "Langlands.Open", blueprint.Fidelity{Class: "conjecture", Checks: all}, "is not one of"},
	} {
		got := strings.Join(tc.f.Missing(tc.stmt), "; ")
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: the reasons are %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFidelityF1(t *testing.T) {
	p, err := blueprint.Load(contentTree(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want bool
		why  string
	}{
		{"Langlands.LocalReciprocityGL1", true, "standard axioms only"},
		{"Langlands.LocalExistence", true, "standard axioms only"},
		{"Langlands.Broken", false, "sorry"},
		{"Langlands.Axiom", false, "Langlands.myAxiom"},
		{"Langlands.Nothing", false, "no declaration"},
	} {
		got, why := checkF1(p, tc.name)
		if got != tc.want || !strings.Contains(why, tc.why) {
			t.Errorf("%s: F1 %t (%s), want %t (%s)", tc.name, got, why, tc.want, tc.why)
		}
	}
}

func TestFidelityCommands(t *testing.T) {
	const old = `{"Langlands.LocalExistence":{"class":"theorem","checks":{"F1":false,"F2":true,"F3":true,"F4":true},"pass":false}}`
	for _, tc := range []struct {
		name  string
		args  []string
		code  int
		out   string
		check func(t *testing.T, m map[string]blueprint.Fidelity)
	}{
		{"no subcommand", nil, 2, "", nil},
		{"unknown subcommand", []string{"frob"}, 2, "", nil},
		{"list", []string{"list"}, 0, "Langlands.LocalExistence  theorem  not passed", nil},
		{"list has no definition", []string{"list"}, 0, "5 statements", nil},
		{"show by tag", []string{"show", "LG00000002"}, 0, "needs: F1, F2, F3, F4, F5", nil},
		{"show a top statement", []string{"show", "Langlands.LocalReciprocityGL1"}, 0, "also needs F6", nil},
		{"show a definition", []string{"show", "LG00000005"}, 1, "", nil},
		{"show an unknown name", []string{"show", "Langlands.Nothing"}, 1, "", nil},
		{"set F5 passes", []string{"set", "Langlands.LocalExistence", "F5=true"}, 0, "pass: yes", func(t *testing.T, m map[string]blueprint.Fidelity) {
			f := m["Langlands.LocalExistence"]
			if !f.Pass || !f.Checks["F1"] || !f.Checks["F5"] {
				t.Errorf("the entry is %+v", f)
			}
		}},
		{"set F1 is refused", []string{"set", "Langlands.LocalExistence", "F1=true"}, 2, "", nil},
		{"set a bad value", []string{"set", "Langlands.LocalExistence", "F5=maybe"}, 2, "", nil},
		{"set an unknown check", []string{"set", "Langlands.LocalExistence", "F9=true"}, 2, "", nil},
		{"set F6 without reviewers", []string{"set", "Langlands.LocalReciprocityGL1", "F6=true"}, 1, "", nil},
		{"set a bad reviewer", []string{"set", "-reviewer", "A", "Langlands.LocalReciprocityGL1", "F6=true"}, 2, "", nil},
		{"set a bad class", []string{"set", "-class", "guess", "Langlands.LocalExistence"}, 2, "", nil},
		{"set a new entry of a conjecture", []string{"set", "LG00000004", "F2=true"}, 0, "class: open", func(t *testing.T, m map[string]blueprint.Fidelity) {
			f := m["Langlands.Open"]
			if f.Pass || f.Class != "open" || len(f.Checks) != 7 {
				t.Errorf("the entry is %+v", f)
			}
		}},
		{"set F1 of a sorry is false", []string{"set", "Langlands.Broken", "F2=true", "F3=true", "F4=true", "F5=true"}, 0, "F1 is not passed", func(t *testing.T, m map[string]blueprint.Fidelity) {
			if f := m["Langlands.Broken"]; f.Pass || f.Checks["F1"] {
				t.Errorf("the entry is %+v", f)
			}
		}},
		{"set F6 with reviewers", []string{"set", "-reviewer", "A|local|2026-12-01", "-reviewer", "B|lifting|2026-12-02", "Langlands.LocalReciprocityGL1", "F2=true", "F3=true", "F4=true", "F5=true", "F6=true"}, 0, "pass: yes", func(t *testing.T, m map[string]blueprint.Fidelity) {
			if f := m["Langlands.LocalReciprocityGL1"]; !f.Pass || len(f.Reviewers) != 2 {
				t.Errorf("the entry is %+v", f)
			}
		}},
		{"run one node", []string{"run", "-node", "LG00000002"}, 0, "F1 yes", func(t *testing.T, m map[string]blueprint.Fidelity) {
			if f := m["Langlands.LocalExistence"]; !f.Checks["F1"] || f.Pass || len(m) != 1 {
				t.Errorf("the record is %+v", m)
			}
		}},
		{"run all", []string{"run", "-all"}, 0, "checked 5 statements", func(t *testing.T, m map[string]blueprint.Fidelity) {
			if len(m) != 5 || m["Langlands.Axiom"].Checks["F1"] || m["Langlands.Open"].Class != "open" {
				t.Errorf("the record is %+v", m)
			}
			for name, f := range m {
				if f.Pass {
					t.Errorf("%s passes after a run that computes only F1", name)
				}
			}
		}},
		{"run needs a node", []string{"run"}, 2, "", nil},
		{"run with both", []string{"run", "-all", "-node", "LG00000002"}, 2, "", nil},
	} {
		root := contentTree(t, old)
		args := append([]string{"fidelity"}, tc.args...)
		if len(tc.args) > 0 {
			args = append([]string{"fidelity", tc.args[0], "-content", root}, tc.args[1:]...)
		}
		var out, errb strings.Builder
		if got := run(args, &out, &errb); got != tc.code {
			t.Errorf("%s: exit %d, want %d (%s)", tc.name, got, tc.code, errb.String())
			continue
		}
		if !strings.Contains(out.String(), tc.out) {
			t.Errorf("%s: the output does not have %q:\n%s", tc.name, tc.out, out.String())
		}
		if tc.check != nil {
			tc.check(t, readFidelity(t, root))
		}
	}
}

func TestCheckFindsAFalsePass(t *testing.T) {
	root := contentTree(t, `{"Langlands.LocalExistence":{"class":"theorem","checks":{"F1":true,"F2":true},"pass":true},"Langlands.Open":{"class":"guess","checks":{},"pass":false}}`)
	p, err := blueprint.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	probs := strings.Join(p.Check(), "\n")
	for _, want := range []string{"Langlands.LocalExistence: pass is true, but F3 is not passed", "Langlands.Open: the class \"guess\""} {
		if !strings.Contains(probs, want) {
			t.Errorf("the check does not report %q:\n%s", want, probs)
		}
	}
}

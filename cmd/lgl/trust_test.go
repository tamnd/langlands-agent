package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const cleanAxioms = `["propext","Classical.choice","Quot.sound"]`

// trustTree writes a content repository with a proved top theorem, a stated node with sorry and a proved-mod node. The argument extra adds declarations to records/lean.json, and commit is the commit of the record.
func trustTree(t *testing.T, commit, extra string) string {
	t.Helper()
	root := t.TempDir()
	node := func(tag, body string) {
		writeFile(t, filepath.Join(root, "blueprint", "nodes", tag+".yaml"), "tag: "+tag+"\nkind: theorem\ntitle: t\nstatement: s\ntier: T3\n"+body)
	}
	node("LG00000001", "lean:\n  statement: Langlands.LocalReciprocityGL1\n  proof: Langlands.localReciprocityGL1\n")
	node("LG00000002", "lean:\n  statement: Langlands.LocalExistence\n  proof: Langlands.localExistence\n")
	node("LG00000003", "hypotheses: [LG00000002]\nlean:\n  statement: Langlands.Mod\n  proof: Langlands.mod\n")
	writeFile(t, filepath.Join(root, "records", "lean.json"), `{"repository":"https://github.com/tamnd/langlands-lean","commit":"`+commit+`","toolchain":"4.35.0","mathlib":"v4.35.0","built":"2026-11-25","decls":{
"Langlands.LocalReciprocityGL1":{"kind":"def","axioms":`+cleanAxioms+`},
"Langlands.localReciprocityGL1":{"kind":"theorem","axioms":`+cleanAxioms+`},
"Langlands.LocalExistence":{"kind":"def","axioms":[]},
"Langlands.localExistence":{"kind":"theorem","axioms":["propext","sorryAx"]},
"Langlands.Mod":{"kind":"def","axioms":[]},
"Langlands.mod":{"kind":"theorem","axioms":["propext"]}`+extra+`}}`)
	return root
}

// replayJSON is a replay record of commit with the checkers given as name=ok pairs. A name that starts with "kernel" is not independent.
func replayJSON(commit string, comparator string, checkers ...string) string {
	var cs []string
	for _, c := range checkers {
		name, ok, _ := strings.Cut(c, "=")
		ind := "true"
		if strings.HasPrefix(name, "kernel") {
			ind = "false"
		}
		cs = append(cs, `{"name":"`+name+`","independent":`+ind+`,"ok":`+ok+`}`)
	}
	s := `{"commit":"` + commit + `","date":"2026-11-25","checkers":[` + strings.Join(cs, ",") + `]`
	if comparator != "" {
		s += `,"comparator":{"ok":` + comparator + `}`
	}
	return s + "}"
}

type trustFile struct {
	Updated string `json:"updated"`
	Replay  string `json:"replay"`
	Checks  []struct {
		ID, State, Note string
	} `json:"checks"`
	Theorems []struct {
		Theorem, Node, Status string
		Checks                []struct{ ID, State string }
	} `json:"theorems"`
	Decls []struct {
		Name  string   `json:"name"`
		Other []string `json:"other_axioms"`
	} `json:"declarations"`
}

func readTrust(t *testing.T, root string) (trustFile, map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "records", "trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f trustFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, c := range f.Checks {
		states[c.ID] = c.State
	}
	return f, states
}

func TestTrust(t *testing.T) {
	for _, tc := range []struct {
		name   string
		extra  string // more declarations of records/lean.json
		replay string // records/replay.json, when not empty
		noLean bool   // remove records/lean.json
		args   []string
		code   int
		out    string
		want   map[string]string // the states of some rows
		note   string            // a part of the note of a row, as "Q3 text"
	}{
		{name: "no replay", code: 0, out: "Q3  not run", want: map[string]string{"Q0": "not run", "Q1": "yes", "Q2": "yes", "Q3": "not run", "Q4": "not run", "Q5": "open", "Q6": "open", "Q7": "not run", "Q8": "not run"}, note: "Q3 The kernel replay was not run"},
		{name: "no lean record", noLean: true, code: 0, want: map[string]string{"Q0": "not run", "Q1": "not run", "Q2": "not run", "Q3": "not run"}},
		{name: "an axiom that is not standard", extra: `,"Langlands.mod":{"kind":"theorem","axioms":["Langlands.cheat"]}`, code: 1, out: "Q2  no", want: map[string]string{"Q2": "no"}, note: "Q2 Langlands.mod uses Langlands.cheat"},
		{name: "a replay with lean4lean", replay: replayJSON(goodCommit, "", "kernel=true", "lean4lean=true"), code: 0, want: map[string]string{"Q3": "yes", "Q4": "not run"}, note: "Q3 kernel, lean4lean accepted"},
		{name: "only the kernel replayed", replay: replayJSON(goodCommit, "", "kernel=true"), code: 0, want: map[string]string{"Q3": "not run"}, note: "Q3 No independent checker ran"},
		{name: "a checker failed", replay: replayJSON(goodCommit, "", "kernel=true", "nanoda=false"), code: 1, want: map[string]string{"Q3": "no"}, note: "Q3 failed in nanoda"},
		{name: "a replay of another commit", replay: replayJSON("1111111111111111111111111111111111111111", "true", "lean4lean=true"), code: 0, want: map[string]string{"Q3": "not run", "Q4": "not run"}, note: "Q3 is of commit 1111111"},
		{name: "quick skips the replay", replay: replayJSON(goodCommit, "true", "lean4lean=true"), args: []string{"-quick"}, code: 0, want: map[string]string{"Q3": "not run", "Q4": "not run"}, note: "Q3 -quick"},
		{name: "the comparator passed", replay: replayJSON(goodCommit, "true", "lean4lean=true"), code: 0, want: map[string]string{"Q3": "yes", "Q4": "yes"}},
		{name: "the comparator failed", replay: replayJSON(goodCommit, "false", "lean4lean=true"), code: 1, want: map[string]string{"Q4": "no"}},
		{name: "a replay with an unknown field", replay: `{"commit":"x","checkers":[],"frob":1}`, code: 1},
		{name: "a missing replay file that the user names", args: []string{"-replay", "/does/not/exist.json"}, code: 1},
		{name: "a bad date", args: []string{"-date", "25 November"}, code: 2},
		{name: "an argument", args: []string{"extra"}, code: 2},
	} {
		root := trustTree(t, goodCommit, tc.extra)
		if tc.noLean {
			if err := os.Remove(filepath.Join(root, "records", "lean.json")); err != nil {
				t.Fatal(err)
			}
		}
		if tc.replay != "" {
			writeFile(t, filepath.Join(root, "records", "replay.json"), tc.replay)
		}
		args := append([]string{"trust", "-content", root, "-date", "2026-11-25"}, tc.args...)
		var out, errb strings.Builder
		if got := run(args, &out, &errb); got != tc.code {
			t.Errorf("%s: exit %d, want %d (%s)", tc.name, got, tc.code, errb.String())
			continue
		}
		if !strings.Contains(out.String(), tc.out) {
			t.Errorf("%s: the output does not have %q:\n%s", tc.name, tc.out, out.String())
		}
		if tc.want == nil {
			continue
		}
		f, states := readTrust(t, root)
		for id, want := range tc.want {
			if states[id] != want {
				t.Errorf("%s: %s is %q, want %q", tc.name, id, states[id], want)
			}
		}
		if tc.note != "" {
			id, text, _ := strings.Cut(tc.note, " ")
			found := false
			for _, c := range f.Checks {
				found = found || c.ID == id && strings.Contains(c.Note, text)
			}
			if !found {
				t.Errorf("%s: the note of %s does not have %q: %+v", tc.name, id, text, f.Checks)
			}
		}
		if f.Updated != "2026-11-25" || len(f.Checks) != 9 {
			t.Errorf("%s: the report has the date %q and %d rows", tc.name, f.Updated, len(f.Checks))
		}
	}
}

func TestTrustTopTheorems(t *testing.T) {
	root := trustTree(t, goodCommit, "")
	writeFile(t, filepath.Join(root, "records", "fidelity.json"), `{"Langlands.LocalReciprocityGL1":{"class":"theorem","checks":{"F1":true,"F2":true,"F3":true,"F4":true,"F5":true},"pass":false}}`)
	var out, errb strings.Builder
	if got := run([]string{"trust", "-content", root}, &out, &errb); got != 0 {
		t.Fatalf("exit %d (%s)", got, errb.String())
	}
	f, _ := readTrust(t, root)
	if f.Replay != "none" {
		t.Errorf("the replay is %q, want none", f.Replay)
	}
	found := false
	for _, th := range f.Theorems {
		if th.Theorem != "Langlands.localReciprocityGL1" {
			continue
		}
		found = true
		if th.Node != "LG00000001" || th.Status != "proved" || th.Checks[0].State != "yes" || th.Checks[1].State != "open" {
			t.Errorf("the row of localReciprocityGL1 is %+v", th)
		}
	}
	if !found || !strings.Contains(out.String(), "Langlands.localReciprocityGL1  S1  LG00000001 proved  Q2 yes, Q5 open") {
		t.Errorf("no row for localReciprocityGL1:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "LG00000003 takes LG00000002") {
		t.Errorf("Q6 does not list the hypothesis:\n%s", out.String())
	}
}

// leanRepo writes a git repository like langlands-lean with two statement files and their manifest. It returns the root and the commit.
func leanRepo(t *testing.T, manifest func(sums map[string]string) string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	sums := map[string]string{}
	for _, name := range []string{"Basic", "ClassField"} {
		text := "namespace Langlands\n-- " + name + "\nend Langlands\n"
		writeFile(t, filepath.Join(root, "Langlands", "Statements", name+".lean"), text)
		h := sha256.Sum256([]byte(text))
		sums["Langlands/Statements/"+name+".lean"] = hex.EncodeToString(h[:])
	}
	writeFile(t, filepath.Join(root, "manifests", "statements.sha256"), manifest(sums))
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := gitHead(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, head
}

func TestTrustLeanCheckout(t *testing.T) {
	good := func(sums map[string]string) string {
		return sums["Langlands/Statements/Basic.lean"] + "  Langlands/Statements/Basic.lean\n" + sums["Langlands/Statements/ClassField.lean"] + "  Langlands/Statements/ClassField.lean\n"
	}
	for _, tc := range []struct {
		name     string
		manifest func(map[string]string) string
		same     bool // the record is of the commit of the checkout
		code     int
		q0, q4   string
		note     string
	}{
		{"same commit and good hashes", good, true, 0, "yes", "not run", "match manifests/statements.sha256"},
		{"another commit", good, false, 1, "no", "not run", ""},
		{"a changed statement file", func(s map[string]string) string {
			return strings.Repeat("0", 64) + "  Langlands/Statements/Basic.lean\n" + s["Langlands/Statements/ClassField.lean"] + "  Langlands/Statements/ClassField.lean\n"
		}, true, 1, "yes", "no", "Basic.lean does not have the hash"},
		{"a file that is not in the manifest", func(s map[string]string) string {
			return s["Langlands/Statements/Basic.lean"] + "  Langlands/Statements/Basic.lean\n"
		}, true, 1, "yes", "no", "ClassField.lean is not in manifests/statements.sha256"},
		{"a missing file", func(s map[string]string) string {
			return good(s) + strings.Repeat("1", 64) + "  Langlands/Statements/Gone.lean\n"
		}, true, 1, "yes", "no", "Gone.lean is in manifests/statements.sha256 but cannot be read"},
	} {
		lean, head := leanRepo(t, tc.manifest)
		commit := head
		if !tc.same {
			commit = goodCommit
		}
		root := trustTree(t, commit, "")
		var out, errb strings.Builder
		if got := run([]string{"trust", "-content", root, "-lean", lean}, &out, &errb); got != tc.code {
			t.Errorf("%s: exit %d, want %d (%s)", tc.name, got, tc.code, errb.String())
			continue
		}
		f, states := readTrust(t, root)
		if states["Q0"] != tc.q0 || states["Q4"] != tc.q4 {
			t.Errorf("%s: Q0 %q and Q4 %q, want %q and %q", tc.name, states["Q0"], states["Q4"], tc.q0, tc.q4)
		}
		if tc.note != "" && !strings.Contains(f.Checks[4].Note, tc.note) {
			t.Errorf("%s: the note of Q4 is %q, want %q", tc.name, f.Checks[4].Note, tc.note)
		}
	}
	var out, errb strings.Builder
	if got := run([]string{"trust", "-content", trustTree(t, goodCommit, ""), "-lean", t.TempDir()}, &out, &errb); got != 1 {
		t.Errorf("a directory that is not a git checkout: exit %d, want 1", got)
	}
}

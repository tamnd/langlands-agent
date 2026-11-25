package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/langlands-agent/evalset"
)

// The eval sets of the repository.
const evalSets = "../../eval"

func TestEvalSetsOfTheRepository(t *testing.T) {
	out := lgl(t, 0, "eval", "check", "-dir", evalSets)
	if !strings.HasSuffix(out, "ok\n") || !strings.Contains(out, "tasks: 200 (the target); statements: 50 (the target); wrong statements: 50 (the target)") {
		t.Errorf("eval check:\n%s", out)
	}
	out = lgl(t, 0, "eval", "list", "-dir", evalSets)
	for _, want := range []string{"tasks: 200 (the target)", "statements: 50 (the target)", "wrong statements: 50 (the target)", "mathlib-numbertheory", "normalization"} {
		if !strings.Contains(out, want) {
			t.Errorf("eval list does not show %q:\n%s", want, out)
		}
	}
	out = lgl(t, 0, "eval", "run", "-dir", evalSets, "-dry-run")
	if !strings.Contains(out, "no file written") || !strings.Contains(out, "battery  100 units") {
		t.Errorf("eval run -dry-run:\n%s", out)
	}
}

func TestEvalRunAndCompare(t *testing.T) {
	tmp := t.TempDir()
	results := filepath.Join(tmp, "results")
	keyPath := filepath.Join(tmp, "key.json")
	if err := os.WriteFile(keyPath, []byte(lgl(t, 0, "eval", "key", "-dir", evalSets)), 0o600); err != nil {
		t.Fatal(err)
	}
	runArgs := []string{"eval", "run", "-dir", evalSets, "-answers", keyPath, "-out", results, "-date", "2026-01-02"}
	out := lgl(t, 0, runArgs...)
	keyResult := filepath.Join(results, "2026-01-02-key.json")
	if !strings.Contains(out, "tasks    200 of 200 passed, 100.00%") || !strings.Contains(out, "wrote "+keyResult) {
		t.Errorf("eval run:\n%s", out)
	}
	lgl(t, 1, runArgs...)
	lgl(t, 0, append(runArgs, "-force")...)
	if out := lgl(t, 0, append(runArgs, "-dry-run", "-label", "other")...); !strings.Contains(out, "dry run: no file written") {
		t.Errorf("eval run -dry-run with answers:\n%s", out)
	}

	// A model that fails 7 of the 100 battery units drops the battery by 7 points.
	var a evalset.Answers
	b, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	a.Model = "Model-B"
	for i := 0; i < 7; i++ {
		if a.Battery[i].Verdict == "accept" {
			a.Battery[i].Verdict = "reject"
		} else {
			a.Battery[i].Verdict = "accept"
		}
	}
	a.Tasks[0].Checks["build"] = false
	b, err = json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	worse := filepath.Join(tmp, "worse.json")
	if err := os.WriteFile(worse, b, 0o600); err != nil {
		t.Fatal(err)
	}
	lgl(t, 0, "eval", "run", "-dir", evalSets, "-answers", worse, "-out", results, "-date", "2026-01-03")
	worseResult := filepath.Join(results, "2026-01-03-model-b.json")

	out = lgl(t, 0, "eval", "compare", keyResult, keyResult)
	if !strings.Contains(out, "ok: no pass rate drops") {
		t.Errorf("eval compare of one result:\n%s", out)
	}
	out = lgl(t, 1, "eval", "compare", keyResult, worseResult)
	if !strings.Contains(out, "battery  100.00% ->  93.00% (-7.00 points)") || !strings.Contains(out, "blocks the merge") || !strings.Contains(out, "tasks    100.00% ->  99.50% (-0.50 points), cost $0.00 -> $0.00  ok") {
		t.Errorf("eval compare with a drop:\n%s", out)
	}
	if out := lgl(t, 0, "eval", "compare", worseResult, keyResult); !strings.Contains(out, "+7.00 points") {
		t.Errorf("eval compare with a rise:\n%s", out)
	}
}

func TestEvalDecl(t *testing.T) {
	lean := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lean, "Langlands"), 0o750); err != nil {
		t.Fatal(err)
	}
	src := "namespace Langlands\n\n/-- A doc. -/\ntheorem foo (K : Type) [Field K] : (1 : K) = 1 := by\n  rfl\n\nend Langlands\n"
	if err := os.WriteFile(filepath.Join(lean, "Langlands", "Foo.lean"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	out := lgl(t, 0, "eval", "decl", "-lean", lean, "-commit", commit, "Langlands/Foo.lean", "Langlands.foo")
	want := "# theorem, lines 4 to 5\nreviewed:\n  repo: langlands-lean\n  commit: " + commit + "\n  path: Langlands/Foo.lean\n  decl: Langlands.foo\n  lean: |\n    theorem foo (K : Type) [Field K] : (1 : K) = 1\n"
	if out != want {
		t.Errorf("eval decl:\n got %q\nwant %q", out, want)
	}
	lgl(t, 1, "eval", "decl", "-lean", lean, "-commit", commit, "Langlands/Foo.lean", "Langlands.bar")
}

func TestEvalUsage(t *testing.T) {
	for _, args := range [][]string{
		{"eval"},
		{"eval", "frob"},
		{"eval", "list", "extra"},
		{"eval", "run", "-dir", evalSets},
		{"eval", "run", "-dir", evalSets, "-answers", "x.json", "-date", "2026-13-40"},
		{"eval", "compare", "one.json"},
		{"eval", "decl", "Langlands/Foo.lean", "Langlands.foo"},
		{"eval", "gen"},
	} {
		lgl(t, 2, args...)
	}
	lgl(t, 1, "eval", "check", "-dir", "/does/not/exist")
	lgl(t, 1, "eval", "compare", "/does/not/exist.json", "/does/not/exist.json")
}

func TestIndent(t *testing.T) {
	if got := indent("a\n\n  b\n", "  "); got != "  a\n\n    b\n" {
		t.Errorf("indent: %q", got)
	}
}

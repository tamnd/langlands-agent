package evalset

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

const fooLean = `namespace Foo

/-- A sum. -/
theorem add_comm' (a b : ℕ) : a + b = b + a := by
  exact Nat.add_comm a b

def P : Prop := ∀ n : ℕ, n = n

end Foo
`

const goodTask = `id: nt-add-comm
kind: prove
origin: mathlib-numbertheory
title: Prove Foo.add_comm'.
source: {repo: mathlib, commit: ` + testCommit + `, path: Mathlib/Foo.lean, decl: Foo.add_comm'}
statement: |
  theorem add_comm' (a b : ℕ) : a + b = b + a := by
    sorry
answer: {repo: mathlib, commit: ` + testCommit + `, path: Mathlib/Foo.lean, decl: Foo.add_comm', lines: [4, 5]}
checks: [build, no-sorry]
`

const goodStatement = `id: foo-p
title: Each number is itself
informal: Each natural number is equal to itself.
reviewed:
  repo: mathlib
  commit: ` + testCommit + `
  path: Mathlib/Foo.lean
  decl: Foo.P
  lean: |
    def P : Prop := ∀ n : ℕ, n = n
checks: [build, equiv-reference]
wrong:
  - id: foo-p-succ
    trap: constant
    reason: Each number is not its successor.
    lean: |
      def P : Prop := ∀ n : ℕ, n = n + 1
`

// evalDir writes an eval directory with one task and one statement, and a Mathlib directory that is not a git checkout.
func evalDir(t *testing.T) (dir, mathlib string) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "eval")
	mathlib = filepath.Join(root, "mathlib")
	writeFile(t, filepath.Join(dir, "tasks", "nt-add-comm.yaml"), goodTask)
	writeFile(t, filepath.Join(dir, "statements", "foo-p.yaml"), goodStatement)
	writeFile(t, filepath.Join(mathlib, "Mathlib", "Foo.lean"), fooLean)
	return dir, mathlib
}

func TestLoadAndCheck(t *testing.T) {
	dir, mathlib := evalDir(t)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks) != 1 || len(s.Statements) != 1 || s.WrongCount() != 1 || len(s.Hash) != 64 {
		t.Fatalf("Load: %+v", s)
	}
	if p := s.Validate(); len(p) > 0 {
		t.Errorf("Validate: %v", p)
	}
	p, rep := CheckRefs(s, NewSource(map[string]string{"mathlib": mathlib}))
	if len(p) > 0 || rep.Checked != 3 {
		t.Errorf("CheckRefs: %v, %+v", p, rep)
	}
	p, rep = CheckRefs(s, NewSource(nil))
	if len(p) > 0 || rep.Checked != 0 || rep.Skipped["mathlib"] != 3 {
		t.Errorf("CheckRefs with no checkout: %v, %+v", p, rep)
	}

	// A change of the Lean file shows in the checks.
	writeFile(t, filepath.Join(mathlib, "Mathlib", "Foo.lean"), "\n"+strings.Replace(fooLean, "n = n", "n + 0 = n", 1))
	p, _ = CheckRefs(s, NewSource(map[string]string{"mathlib": mathlib}))
	got := strings.Join(problemStrings(p), "\n")
	for _, want := range []string{"is at lines 5 to 6", "the reviewed Lean text is not the text of Foo.P"} {
		if !strings.Contains(got, want) {
			t.Errorf("CheckRefs does not report %q:\n%s", want, got)
		}
	}
	if err := os.Remove(filepath.Join(mathlib, "Mathlib", "Foo.lean")); err != nil {
		t.Fatal(err)
	}
	if p, _ = CheckRefs(s, NewSource(map[string]string{"mathlib": mathlib})); len(p) != 3 {
		t.Errorf("CheckRefs with no file: %v", p)
	}
}

func TestLoadIsStrict(t *testing.T) {
	dir, _ := evalDir(t)
	writeFile(t, filepath.Join(dir, "tasks", "nt-add-comm.yaml"), goodTask+"extra: 1\n")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "extra") {
		t.Errorf("Load reads an unknown field: %v", err)
	}
}

func problemStrings(p []Problem) []string {
	var out []string
	for _, x := range p {
		out = append(out, x.String())
	}
	return out
}

func TestValidate(t *testing.T) {
	dir, _ := evalDir(t)
	writeFile(t, filepath.Join(dir, "tasks", "nt-two.yaml"), strings.NewReplacer(
		"id: nt-add-comm", "id: nt-add-comm",
		"commit: "+testCommit+", path: Mathlib/Foo.lean, decl: Foo.add_comm'}", "commit: abc, path: /Mathlib/Foo.lean, decl: Foo.add_comm'}",
		"    sorry", "    rfl",
		"checks: [build, no-sorry]", "checks: [build, frob]",
	).Replace(goodTask))
	writeFile(t, filepath.Join(dir, "statements", "bar.yaml"), strings.NewReplacer(
		"id: foo-p\n", "id: bar\n",
		"informal: Each natural number is equal to itself.", "informal: Each natural number — each one — is equal to itself.",
		"id: foo-p-succ", "id: foo-p",
		"trap: constant", "trap: frob",
		"n = n + 1", "n = n",
	).Replace(goodStatement))
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problemStrings(s.Validate()), "\n")
	for _, want := range []string{
		"nt-two.yaml: the file name must be the id nt-add-comm.yaml",
		"the id nt-add-comm is also in",
		"source: the commit \"abc\" is not a full commit hash",
		"source: the path \"/Mathlib/Foo.lean\" is not a relative path of a Lean file",
		"the check \"frob\" is not one of",
		"the statement of a prove task has sorry as its proof",
		"informal has an em dash or an en dash",
		"the id of the wrong statement foo-p must start with bar-",
		"the trap \"frob\" is not one of",
		"foo-p: the wrong statement is the reviewed statement",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Validate does not report %q:\n%s", want, got)
		}
	}
}

func TestScore(t *testing.T) {
	s := &Sets{
		Tasks: []Task{
			{ID: "a", Kind: "prove", Origin: "mathlib-numbertheory", Checks: []string{"build", "no-sorry"}},
			{ID: "b", Kind: "repair", Origin: "nyll-port", Checks: []string{"build"}},
		},
		Statements: []Statement{{ID: "s", Checks: []string{"build", "equiv-reference"}, Wrong: []Wrong{{ID: "s-w"}}}},
	}
	key := Key(s)
	res, err := Score(s, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range SetNames {
		if res[set].PassRate != 100 || res[set].Failed != nil {
			t.Errorf("the key does not pass %s: %+v", set, res[set])
		}
	}
	if res[SetBattery].Total != 2 || res[SetStater].Total != 1 {
		t.Errorf("units: %+v", res)
	}

	a := &Answers{
		Tasks:      []Answer{{ID: "a", Checks: map[string]bool{"build": true, "no-sorry": false}, CostUSD: 0.25, Tokens: 100}, {ID: "b", Checks: map[string]bool{"build": true}, CostUSD: 0.5, Tokens: 50}},
		Statements: []Answer{},
		Battery:    []Answer{{ID: "s", Verdict: "accept"}, {ID: "s-w", Verdict: "accept", CostUSD: 0.125}},
	}
	res, err = Score(s, a)
	if err != nil {
		t.Fatal(err)
	}
	tasks := res[SetTasks]
	if tasks.Passed != 1 || tasks.PassRate != 50 || tasks.CostUSD != 0.75 || tasks.Tokens != 150 || strings.Join(tasks.Failed, ",") != "a" {
		t.Errorf("tasks: %+v", tasks)
	}
	if tasks.ByKind["repair"] != (Count{1, 1}) || tasks.ByOrigin["mathlib-numbertheory"] != (Count{1, 0}) {
		t.Errorf("tasks by kind and origin: %+v", tasks)
	}
	if st := res[SetStater]; st.Answered != 0 || st.PassRate != 0 || st.Failed[0] != "s" {
		t.Errorf("a unit with no answer passes: %+v", st)
	}
	if b := res[SetBattery]; b.PassRate != 50 || b.ByKind["wrong"] != (Count{1, 0}) {
		t.Errorf("battery: %+v", b)
	}

	bad := &Answers{
		Tasks:   []Answer{{ID: "a"}, {ID: "a"}, {ID: "zzz"}, {ID: "b", CostUSD: -1}},
		Battery: []Answer{{ID: "s", Verdict: "maybe"}},
	}
	_, err = Score(s, bad)
	for _, want := range []string{"two answers for a", `"zzz" is not a unit`, "b has a negative cost", `the verdict of s is "maybe"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Score does not report %q: %v", want, err)
		}
	}
}

func TestCompare(t *testing.T) {
	result := func(rates map[string]float64) *Result {
		r := &Result{Version: 1, Sets: map[string]SetResult{}}
		for k, v := range rates {
			r.Sets[k] = SetResult{PassRate: v}
		}
		return r
	}
	blocks := func(old, cur *Result) map[string]bool {
		out := map[string]bool{}
		for _, c := range Compare(old, cur) {
			out[c.Set] = c.Blocks
		}
		return out
	}
	old := result(map[string]float64{SetTasks: 60, SetStater: 50, SetBattery: 90})
	got := blocks(old, result(map[string]float64{SetTasks: 55, SetStater: 44.5, SetBattery: 99}))
	if got[SetTasks] || !got[SetStater] || got[SetBattery] {
		t.Errorf("a drop of 5 points is allowed, and a drop of 5.5 points blocks: %v", got)
	}
	got = blocks(old, result(map[string]float64{SetTasks: 60, SetStater: 50}))
	if !got[SetBattery] {
		t.Errorf("a missing set blocks: %v", got)
	}
	got = blocks(result(map[string]float64{SetTasks: 60}), result(map[string]float64{SetTasks: 60, SetBattery: 0}))
	if got[SetBattery] {
		t.Errorf("a new set does not block: %v", got)
	}
}

func gitRepo(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for p, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(p)), text)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "test"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestGenerate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := t.TempDir()
	mathlib := filepath.Join(root, "mathlib")
	lean := filepath.Join(root, "langlands-lean")
	gitRepo(t, mathlib, map[string]string{
		"Mathlib/NumberTheory/A.lean":         "namespace Nat\n\ntheorem two_mul_example (a : ℕ) : 2 * a = a + a := by\n  omega\n\nprivate theorem hidden_example (a : ℕ) : a + 0 = a := by\n  simp\n\nend Nat\n",
		"Mathlib/RepresentationTheory/B.lean": "theorem rep_example (a b : ℕ) (h : a = b) : b = a := by\n  exact h.symm\n",
	})
	up := strings.Repeat("ab", 20)
	head := gitRepo(t, lean, map[string]string{
		"Langlands/Ported/NYLL/C.lean": "-- Ported from n-yamaguchi-0729/ClassFieldTheory at commit " + up + ", file `ClassFieldTheory/C.lean`\n\nnamespace CFT\n\n/-- For each finite group G of prime power order, the group G is a p-group for the prime p of its order. -/\ntheorem pgroup_of_card (G : Type) [Group G] (p : ℕ) : True := by\n  exact trivial\n\ntheorem uses_rename (G : Type) [Group G] (p : ℕ) : True := by\n  have := isPGroup' p\n  trivial\n\nend CFT\n",
		"Langlands/D.lean":             "namespace Langlands\n\ntheorem project_example (a b : ℕ) : a * b = b * a := by\n  exact Nat.mul_comm a b\n\nend Langlands\n",
	})
	src := NewSource(map[string]string{"mathlib": mathlib, "langlands-lean": lean})
	tasks, err := Generate(GenOptions{Source: src, Quotas: Quotas{Total: 5, RepresentationTheory: 1, NYLL: 1, Repair: 1, Project: 1}})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, task := range tasks {
		kinds[task.Origin] = task.Kind
		if task.Origin == "nyll-port" {
			if !strings.Contains(task.Broken, "isPGroup p") || !strings.Contains(task.Error, "isPGroup is not a name") || task.Source.Commit != up || task.Answer.Commit != head {
				t.Errorf("the repair task: %+v", task)
			}
		}
	}
	want := map[string]string{"mathlib-numbertheory": "prove", "mathlib-representationtheory": "prove", "nyll": "state", "nyll-port": "repair", "project": "prove"}
	for k, v := range want {
		if kinds[k] != v {
			t.Errorf("no %s task of origin %s: %v", v, k, kinds)
		}
	}
	dir := filepath.Join(root, "eval")
	writeFile(t, filepath.Join(dir, "tasks", "old-task.yaml"), "stale")
	if err := WriteTasks(filepath.Join(dir, "tasks"), tasks); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks) != 5 {
		t.Errorf("WriteTasks does not remove the old file: %d tasks", len(s.Tasks))
	}
	if p := s.Validate(); len(p) > 0 {
		t.Errorf("Validate: %v", p)
	}
	if p, rep := CheckRefs(s, src); len(p) > 0 || rep.Skipped["nyll"] != 2 {
		t.Errorf("CheckRefs: %v, %+v", p, rep)
	}
	if _, err := Generate(GenOptions{Source: src, Quotas: Quotas{Total: 9, RepresentationTheory: 1}}); err == nil {
		t.Error("Generate makes more tasks than the checkouts have")
	}
}

package evalset

import (
	"strings"
	"testing"
)

const sampleLean = `/-! # A file

theorem inComment : True := trivial
-/
import Mathlib

namespace Foo

open Bar

variable (n : ℕ)

/-- The doc of two. -/
@[simp]
theorem two_eq : (2 : ℕ) = 1 + 1 := by
  rfl

private lemma hidden : True := trivial

@[deprecated (since := "2024-01-01")]
theorem old : True := trivial

variable {m : ℕ} in
/-- A doc after a prefix. -/
theorem withPrefix : m = m := rfl

def byMatch : ℕ → ℕ
  | 0 => 0
  | n + 1 => n

structure S where
  x : ℕ

def T : Type := ℕ deriving Inhabited

theorem _root_.Top.thing : True := trivial

namespace Inner
lemma deep (h : 1 = 1) :
    1 = 1 := h
end Inner

end Foo
`

func TestParseLean(t *testing.T) {
	decls := ParseLean(sampleLean)
	var names []string
	for _, d := range decls {
		names = append(names, d.Name)
	}
	want := "Foo.two_eq Foo.hidden Foo.old Foo.withPrefix Foo.byMatch Foo.S Foo.T Top.thing Foo.Inner.deep"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("names:\n got %s\nwant %s", got, want)
	}
	lines := strings.Split(sampleLean, "\n")
	get := func(name string) Decl {
		d, ok := FindDecl(decls, name)
		if !ok {
			t.Fatalf("no declaration %s", name)
		}
		return d
	}
	two := get("Foo.two_eq")
	if two.Keyword != "theorem" || two.Doc != "The doc of two." || !two.HasProof || two.ProofLines != 2 {
		t.Errorf("two_eq: %+v", two)
	}
	if two.Header != "@[simp]\ntheorem two_eq : (2 : ℕ) = 1 + 1" {
		t.Errorf("two_eq header: %q", two.Header)
	}
	if got := two.Text(lines); got != "@[simp]\ntheorem two_eq : (2 : ℕ) = 1 + 1 := by\n  rfl" {
		t.Errorf("two_eq text: %q", got)
	}
	if d := get("Foo.hidden"); !d.Private {
		t.Errorf("hidden is not private: %+v", d)
	}
	if d := get("Foo.old"); !d.Deprecated {
		t.Errorf("old is not deprecated: %+v", d)
	}
	if d := get("Foo.withPrefix"); d.Doc != "A doc after a prefix." || !strings.HasPrefix(d.Header, "variable {m : ℕ} in\n") {
		t.Errorf("withPrefix: %+v", d)
	}
	if d := get("Foo.byMatch"); d.HasProof || d.End-d.Start != 2 {
		t.Errorf("byMatch: %+v", d)
	}
	if d := get("Foo.S"); d.HasProof || d.Header != "structure S" {
		t.Errorf("S: %+v", d)
	}
	if d := get("Foo.Inner.deep"); d.Header != "lemma deep (h : 1 = 1) :\n    1 = 1" || d.Proof != "h" {
		t.Errorf("deep: %+v", d)
	}
	if got := StatementOf(get("Foo.Inner.deep")); got != "lemma deep (h : 1 = 1) :\n    1 = 1 := by\n  sorry\n" {
		t.Errorf("the statement of deep: %q", got)
	}
}

func TestSameText(t *testing.T) {
	if !SameText("theorem a :\n    b = c := by\n  sorry\n", "theorem a : b = c := by sorry") {
		t.Error("the texts differ only in whitespace")
	}
	if SameText("theorem a : b = c", "theorem a : b = d") {
		t.Error("the texts differ")
	}
}

func TestReplaceName(t *testing.T) {
	for _, tc := range []struct {
		text, pin, up, want string
		n                   int
	}{
		{"exact isPGroup' h", "isPGroup'", "isPGroup", "exact isPGroup h", 1},
		{"exact P.isPGroup'.of_equiv h", "isPGroup'", "isPGroup", "exact P.isPGroup.of_equiv h", 1},
		{"exact isPGroup'_foo h", "isPGroup'", "isPGroup", "exact isPGroup'_foo h", 0},
		{"-- isPGroup'\nexact h", "isPGroup'", "isPGroup", "-- isPGroup'\nexact h", 0},
		{"rw [PowerSeries.X_prime, PowerSeries.X_prime]", "PowerSeries.X_prime", "PowerSeries.prime_X", "rw [PowerSeries.prime_X, PowerSeries.prime_X]", 2},
		{"exact _root_.abelianizationCongr_symm e", "_root_.abelianizationCongr_symm", "Group.abelianizationCongr_symm", "exact Group.abelianizationCongr_symm e", 1},
		{"exact abelianizationCongr_symm e", "_root_.abelianizationCongr_symm", "Group.abelianizationCongr_symm", "exact abelianizationCongr_symm e", 0},
	} {
		got, n := replaceName(tc.text, tc.pin, tc.up)
		if got != tc.want || n != tc.n {
			t.Errorf("replaceName(%q, %q): got %q, %d; want %q, %d", tc.text, tc.pin, got, n, tc.want, tc.n)
		}
	}
}

func TestSlugAndTaskID(t *testing.T) {
	for in, want := range map[string]string{
		"isPGroup'":            "is-pgroup-prime",
		"Nat.Prime.sq_add_sq":  "nat-prime-sq-add-sq",
		"padicValNat.mul":      "padic-val-nat-mul",
		"Pell.Solution₁.x_mul": "pell-solution1-x-mul",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	id := taskID("nt", "Nat.Prime.sq_add_sq")
	if !strings.HasPrefix(id, "nt-prime-sq-add-sq-") || len(id) != len("nt-prime-sq-add-sq-")+6 || !idPattern.MatchString(id) {
		t.Errorf("taskID: %q", id)
	}
	if taskID("nt", "A.b") == taskID("nt", "C.A.b") {
		t.Error("two names with the same last parts have the same id")
	}
	long := taskID("nt", strings.Repeat("abcdefgh", 10))
	if len(long) > 80 || !idPattern.MatchString(long) {
		t.Errorf("a long id: %q", long)
	}
}

package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCharacter(t *testing.T) {
	// The values follow from the Hilbert symbol (x, d)_p, which is the Artin map of x on Q_p(sqrt d).
	for _, tc := range []struct {
		p, d int
		x    int64
		want int
	}{
		{3, -3, 2, -1}, // (2/3) = -1
		{3, -3, 4, 1},
		{3, -3, 3, 1}, // p acts trivially on Q_3(zeta_3)
		{3, 2, 3, -1}, // the unramified extension: (-1)^v
		{3, 2, 2, 1},
		{5, 5, 2, -1},
		{5, 5, 5, 1},
		{7, -7, 3, -1}, // 3^3 = 27 = -1 mod 7
		{7, -7, 2, 1},
		{7, 3, 7, -1},
		{2, 5, 2, -1}, // Q_2(sqrt 5) is unramified
		{2, 5, 3, 1},
		{2, -1, 3, -1}, // Q_2(zeta_4): u = 1 mod 4
		{2, -1, 5, 1},
		{2, -1, 2, 1},
		{2, 2, 5, -1}, // Q_2(sqrt 2) in Q_2(zeta_8): u = 1 or 7 mod 8
		{2, 2, 7, 1},
		{2, -2, 3, 1}, // Q_2(sqrt -2): u = 1 or 3 mod 8
		{2, -2, 5, -1},
		{2, 10, 2, -1}, // Q_2(sqrt 10) = Q_2(sqrt 5 sqrt 2)
	} {
		chi, _ := character(tc.p, tc.d)
		if got := chi(big.NewInt(tc.x)); got != tc.want {
			t.Errorf("p = %d, d = %d: the character of %d is %d, want %d", tc.p, tc.d, tc.x, got, tc.want)
		}
	}
}

func TestSquareClass(t *testing.T) {
	for _, tc := range []struct {
		x    int64
		p    int
		want int64
	}{
		{1, 3, 1}, {2, 3, 2}, {-1, 3, 2}, {12, 3, 3}, {-3, 3, 6}, {9, 3, 1},
		{3, 7, 3}, {-7, 7, 21}, {49, 7, 1},
		{3, 2, 3}, {-1, 2, 7}, {12, 2, 3}, {-2, 2, 14}, {17, 2, 1}, {40, 2, 10},
	} {
		if got := squareClass(big.NewInt(tc.x), tc.p); got != tc.want {
			t.Errorf("the class of %d in Q_%d is %d, want %d", tc.x, tc.p, got, tc.want)
		}
	}
}

func TestNorm(t *testing.T) {
	for _, tc := range []struct {
		name string
		p, n int
		a    func(m int) []int64
		want int64
	}{
		{"1 - zeta_3", 3, 1, func(_ int) []int64 { return []int64{1, -1} }, 3},
		{"1 - zeta_9", 3, 2, func(m int) []int64 { a := make([]int64, m); a[0], a[1] = 1, -1; return a }, 3},
		{"1 - zeta_8", 2, 3, func(m int) []int64 { a := make([]int64, m); a[0], a[1] = 1, -1; return a }, 2},
		{"1 - zeta_49", 7, 2, func(m int) []int64 { a := make([]int64, m); a[0], a[1] = 1, -1; return a }, 7},
		{"zeta_5", 5, 1, func(m int) []int64 { a := make([]int64, m); a[1] = 1; return a }, 1},
		{"1 + zeta_5", 5, 1, func(m int) []int64 { a := make([]int64, m); a[0], a[1] = 1, 1; return a }, 1},
		{"2 in Q(zeta_9)", 3, 2, func(m int) []int64 { a := make([]int64, m); a[0] = 2; return a }, 64},
		{"1 + i", 2, 2, func(_ int) []int64 { return []int64{1, 1} }, 2},
		{"3 + 2i", 2, 2, func(_ int) []int64 { return []int64{3, 2} }, 13},
		{"0", 3, 1, func(m int) []int64 { return make([]int64, m) }, 0},
	} {
		step := 1
		for range tc.n - 1 {
			step *= tc.p
		}
		got := norm(tc.a((tc.p-1)*step), tc.p, step)
		if got.Cmp(big.NewInt(tc.want)) != 0 {
			t.Errorf("%s: the norm is %s, want %d", tc.name, got, tc.want)
		}
	}
}

func TestRunE1(t *testing.T) {
	r, err := RunE1(E1Inputs{Primes: []int{2, 3, 5, 7}, Box: 20, MaxConductor: 27, Samples: 60, Coefficients: 2, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Quadratic) != 16 || len(r.Cyclotomic) != 9 {
		t.Errorf("the run has %d quadratic and %d cyclotomic fields, want 16 and 9", len(r.Quadratic), len(r.Cyclotomic))
	}
	if r.Mismatches() != 0 || r.Gaps() != 0 {
		t.Errorf("the run has %d mismatches and %d gaps: %+v", r.Mismatches(), r.Gaps(), r)
	}
	for _, q := range r.Quadratic {
		if got, want := len(q.Kernel), len(classes(q.P))/2; got != want {
			t.Errorf("Q_%d(sqrt %d): the kernel has %d classes, want %d (index 2)", q.P, q.D, got, want)
		}
	}
}

// TestWrongPredictionIsCaught shows that the check can fail: the norms of one quadratic field are not all in the kernel of the character of another.
func TestWrongPredictionIsCaught(t *testing.T) {
	for _, p := range []int{2, 3, 7} {
		for _, d := range quadraticDs(p) {
			for _, e := range quadraticDs(p) {
				if d == e {
					continue
				}
				chi, _ := character(p, e)
				caught := false
				for a := -10; a <= 10 && !caught; a++ {
					for b := 0; b <= 10 && !caught; b++ {
						if n := int64(a*a - d*b*b); n != 0 {
							caught = chi(big.NewInt(n)) == -1
						}
					}
				}
				if !caught {
					t.Errorf("p = %d: every norm of Q_p(sqrt %d) is in the kernel of the character of Q_p(sqrt %d)", p, d, e)
				}
			}
		}
	}
	// The kernel of a larger conductor is wrong for Q_3(zeta_3): its norms are not all 1 mod 9.
	r, err := RunE1(E1Inputs{Primes: []int{3}, Box: 1, MaxConductor: 3, Samples: 40, Coefficients: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if c := r.Cyclotomic[0]; len(c.Classes) < 2 {
		t.Errorf("the norms of Q_3(zeta_3) reach only the classes %v mod 9", c.Classes)
	}
}

func TestE1Inputs(t *testing.T) {
	good := E1Inputs{Primes: []int{3}, Box: 5, MaxConductor: 9, Samples: 5, Coefficients: 1}
	for _, tc := range []struct {
		name string
		edit func(in *E1Inputs)
		want string
	}{
		{"good", func(_ *E1Inputs) {}, ""},
		{"no primes", func(in *E1Inputs) { in.Primes = nil }, "at least one prime"},
		{"not a prime", func(in *E1Inputs) { in.Primes = []int{9} }, "not a prime"},
		{"a large box", func(in *E1Inputs) { in.Box = 5000 }, "box"},
		{"a large conductor", func(in *E1Inputs) { in.MaxConductor = 343 }, "max_conductor"},
		{"no coefficients", func(in *E1Inputs) { in.Coefficients = 0 }, "coefficients"},
	} {
		in := good
		tc.edit(&in)
		err := in.check()
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: the error is %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseSpec(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(s string) string
		want string
	}{
		{"the spec of E1", func(s string) string { return s }, ""},
		{"a bad name", func(s string) string { return strings.Replace(s, "name: e1-local", "name: E1 local", 1) }, "the name"},
		{"an unknown experiment", func(s string) string { return strings.Replace(s, "experiment: E1", "experiment: E10", 1) }, "E1 to E9"},
		{"an experiment with no code", func(s string) string { return strings.Replace(s, "experiment: E1", "experiment: E3", 1) }, "no code for experiment E3"},
		{"another tool", func(s string) string { return strings.Replace(s, "tool: lgl", "tool: lgl-lab", 1) }, "the tool"},
		{"no node", func(s string) string {
			return strings.Replace(s, "node: LGAV7YV6K6 # LocalReciprocityGL1", "node: \"\"", 1)
		}, "a node"},
		{"an unknown field", func(s string) string { return s + "image: pari\n" }, "image"},
		{"bad inputs", func(s string) string { return strings.Replace(s, "box: 40", "box: 0", 1) }, "box"},
	} {
		s, err := ParseSpec("lab/specs/e1.yaml", []byte(tc.edit(SpecE1)))
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
			} else if s.E1.MaxConductor != 49 || len(s.Hash) != 64 {
				t.Errorf("%s: the spec is %+v", tc.name, s)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: the error is %v, want %q", tc.name, err, tc.want)
		}
	}
}

const smallSpec = `name: e1-small
experiment: E1
question: Do the small cases agree?
tool: lgl
node: LG00000001
inputs: {primes: [2, 3], box: 10, max_conductor: 9, samples: 20, coefficients: 2, seed: 3}
outputs: [quadratic.json, cyclotomic.json, summary.md]
`

func TestRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lab", "specs", "e1-small.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(smallSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	runs := RunsDir(path)
	if runs != filepath.Join(root, "lab", "runs") {
		t.Fatalf("the runs directory is %q", runs)
	}
	now := func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) }
	opt := Options{Runs: runs, Host: "test", ToolVersion: "dev", Now: now}
	rec, err := Run(s, opt)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-11-25-e1-small-" + s.Hash[:8]; rec.ID != want {
		t.Errorf("the id is %q, want %q", rec.ID, want)
	}
	if rec.Spec != "lab/specs/e1-small.yaml" || rec.Container != "none" || rec.Network || rec.Mismatches != 0 || !strings.HasPrefix(rec.Result, "agrees") {
		t.Errorf("the record is %+v", rec)
	}
	dir := filepath.Join(runs, rec.ID)
	for name, sum := range rec.Files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != sum {
			t.Errorf("%s does not have the hash of the record", name)
		}
	}
	if len(rec.Files) != 3 {
		t.Errorf("the run has the files %v", rec.Files)
	}
	summary, _ := os.ReadFile(filepath.Join(dir, "summary.md"))
	if !strings.Contains(string(summary), "## What the run does not do") {
		t.Errorf("the summary does not say what the run does not do:\n%s", summary)
	}
	if _, err := Run(s, opt); err == nil || !strings.Contains(err.Error(), "-force") {
		t.Errorf("a second run gives %v, want an error about -force", err)
	}
	opt.Force = true
	again, err := Run(s, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"quadratic.json", "cyclotomic.json", "summary.md"} {
		if again.Files[name] != rec.Files[name] {
			t.Errorf("%s changed between two runs of the same spec", name)
		}
	}
	got, err := LoadRuns(runs)
	if err != nil || len(got) != 1 || got[0].ID != rec.ID {
		t.Errorf("LoadRuns gives %v, %v", got, err)
	}
	if err := os.Rename(dir, filepath.Join(runs, "other")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuns(runs); err == nil {
		t.Error("LoadRuns reads a run whose id is not the name of its directory")
	}
	if runs, err := LoadRuns(filepath.Join(root, "nothing")); err != nil || runs != nil {
		t.Errorf("a missing directory gives %v, %v", runs, err)
	}
}

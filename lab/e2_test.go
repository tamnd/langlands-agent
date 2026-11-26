package lab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var e2Primes = []int{3, 5, 7}

// conj is the complex conjugate of a: zeta_N^e goes to zeta_N^-e.
func (f *e2Field) conj(a cyc) cyc {
	out := f.zero()
	out.den = a.den
	for e, c := range a.v {
		out.v[mod(-e, f.n)] += c
	}
	return out
}

func totient(n int) int {
	r := 0
	for k := 1; k <= n; k++ {
		if gcd64(int64(k), int64(n)) == 1 {
			r++
		}
	}
	return r
}

func TestE2Field(t *testing.T) {
	for _, p := range e2Primes {
		f := newE2Field(p)
		if f.deg != totient(f.n) {
			t.Errorf("p = %d: Phi_%d has degree %d, want %d", p, f.n, f.deg, totient(f.n))
		}
		if q := f.scale(f.root(0, 1), int64(p), 1); !f.equal(f.mul(f.sqrtq, f.sqrtq), q) {
			t.Errorf("p = %d: the square of sqrt(q) is not q", p)
		}
		if z := f.complex(f.sqrtq); real(z) < 0 || imag(z) > 1e-9 || imag(z) < -1e-9 {
			t.Errorf("p = %d: sqrt(q) is %v, not the positive root", p, z)
		}
		if f.gamma == [2]int{} || f.g == 0 {
			t.Errorf("p = %d: no generator", p)
		}
	}
}

// TestGaussSums checks G(chi) G(chi^-1) = chi(-1) q^n for each character chi of conductor exponent n in {1, 2}.
func TestGaussSums(t *testing.T) {
	for _, p := range e2Primes {
		f := newE2Field(p)
		for k := 1; k < f.m; k++ {
			n := f.cond(k)
			sign := int64(1)
			if k%2 == 1 {
				sign = -1 // chi_k(-1) = (-1)^k, as -1 = g^(m/2)
			}
			got := f.mul(f.gaussF(k, n), f.gaussF(mod(-k, f.m), n))
			if want := f.root(0, sign*ipow(p, n)); !f.equal(got, want) {
				t.Errorf("p = %d, chi_%d: G(chi) G(chi^-1) is not chi(-1) q^%d", p, k, n)
			}
		}
	}
}

// sigmaChar is the character of sigma_theta_j at the class key, as an element of Q(zeta_N).
func (f *e2Field) sigmaChar(j, key int) cyc {
	typ, l := key/f.e, key%f.e
	r := f.n / f.e
	switch typ {
	case 0:
		return f.root(j*l*r, int64(f.q-1))
	case 1:
		return f.root(j*l*r, -1)
	}
	return f.add(f.root(j*l*r, -1), f.root(j*l*f.q*r, -1))
}

// TestCuspidalCharacter checks that the character of sigma_theta is irreducible for each regular theta: the sum of |chi(h)|^2 over GL2(F_q) is the order of the group.
func TestCuspidalCharacter(t *testing.T) {
	for _, p := range e2Primes {
		f := newE2Field(p)
		order := int64((p*p - 1) * (p*p - p))
		for _, j := range f.regularThetas() {
			sum := f.zero()
			var n int64
			for _, key := range f.keyOfBar {
				if key < 0 {
					continue
				}
				n++
				c := f.sigmaChar(j, key)
				sum = f.add(sum, f.mul(c, f.conj(c)))
			}
			if !f.equal(sum, f.root(0, order)) {
				t.Errorf("p = %d, theta_%d: the norm of the character is not |GL2(F_q)| = %d", p, j, order)
			}
			if j == 1 && n > order {
				t.Errorf("p = %d: %d elements have a class key, more than %d", p, n, order)
			}
		}
		if got := len(f.regularThetas()); got != (p*p-p)/2 {
			t.Errorf("p = %d: %d cuspidal representations, want %d", p, got, (p*p-p)/2)
		}
	}
}

// TestKondo checks the sum over GL2(F_q) of the character of sigma_theta^v times psi(tr h / p): it is -q(q - 1) times the Gauss sum of theta^-1 over k_E with psi(Tr x / p) (Kondo 1963). The sum with psi trivial is 0.
func TestKondo(t *testing.T) {
	for _, p := range e2Primes {
		f := newE2Field(p)
		for _, j := range f.regularThetas() {
			ge, _ := f.tateE(gl1E{j: j, s: 1})
			want := f.scale(ge.c, -int64(p*(p-1)), 1)
			if got := f.gl2Sum(j, 0, 1, -1); !f.equal(got, want) {
				t.Errorf("p = %d, theta_%d: T_-1 is not -q(q - 1) G_E", p, j)
			}
			if got := f.gl2Sum(j, 0, 1, 0); !f.isZero(got) {
				t.Errorf("p = %d, theta_%d: T_0 is not 0", p, j)
			}
		}
	}
}

func TestSteinbergUnramified(t *testing.T) {
	for _, p := range e2Primes {
		f := newE2Field(p)
		alphas, eps, err := f.steinbergGL2(gl1{s: 1, a: [3]int{1}})
		if err != nil {
			t.Fatal(err)
		}
		if want := (mono{c: f.root(0, -1), h: 1, a: [3]int{1}, x: 1}); !f.mequal(eps, want) {
			t.Errorf("p = %d: epsilon(s, St) = %s, want -q^(1/2) a X", p, f.format(eps, namesSt))
		}
		if want := (mono{c: f.root(0, 1), h: -1, a: [3]int{1}}); len(alphas) != 1 || !f.mequal(alphas[0], want) {
			t.Errorf("p = %d: L(s, St) = %s, want 1/(1 - q^(-1/2) a X)", p, f.formatL(alphas, namesSt))
		}
		if l := f.lambda(); !f.mequal(l, f.one()) {
			t.Errorf("p = %d: lambda(E/F, psi) = %s, want 1", p, f.format(l, nil))
		}
	}
}

var e2All = []string{FamilyPrincipal, FamilySteinberg, FamilySupercuspidal}

func TestRunE2(t *testing.T) {
	r, err := RunE2(E2Inputs{Primes: []int{3}, ConductorExponent: 4, TwistConductor: 2, AdditiveCharacter: "conductor-O", Families: e2All})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mismatches() != 0 || r.FirstMismatch != nil {
		t.Fatalf("%d mismatches, the first is %+v", r.Mismatches(), r.FirstMismatch)
	}
	want := map[string][3]int{FamilyPrincipal: {21, 6, 126}, FamilySteinberg: {6, 6, 36}, FamilySupercuspidal: {3, 6, 18}}
	for _, c := range r.Counts {
		w := want[c.Family]
		if c.Pis != w[0] || c.Twists != w[1] || c.Pairs != w[2] || c.Matches != c.Pairs {
			t.Errorf("%s: the count is %+v, want %v", c.Family, c, w)
		}
	}
	if r.Pairs() != 180 {
		t.Errorf("%d pairs, want 180", r.Pairs())
	}
	for _, x := range r.Supercuspidal {
		if x.Twist == "chi_0" && x.GL2Epsilon != x.GaloisEpsilon {
			t.Errorf("%s: the formatted factors differ: %s and %s", x.Pi, x.GL2Epsilon, x.GaloisEpsilon)
		}
		if x.PairConductor != 2 && x.PairConductor != 4 {
			t.Errorf("%s x %s: the conductor exponent is %d", x.Pi, x.Twist, x.PairConductor)
		}
	}
}

// TestNaiveIsCaught checks that the run catches the correspondence with no rectifier: the central character fails for each pair, and the epsilon-factor fails for each tame twist.
func TestNaiveIsCaught(t *testing.T) {
	for _, p := range e2Primes {
		r, err := RunE2(E2Inputs{Primes: []int{p}, ConductorExponent: 2, TwistConductor: 2, AdditiveCharacter: "conductor-O", Families: []string{FamilySupercuspidal}})
		if err != nil {
			t.Fatal(err)
		}
		c := r.Counts[0]
		if c.Matches != c.Pairs || c.NaiveCentralMatches != 0 {
			t.Errorf("p = %d: the count is %+v", p, c)
		}
		for _, x := range r.Supercuspidal {
			tame := x.PairConductor == 2
			if *x.NaiveEpsilonMatch == tame {
				t.Errorf("p = %d, %s x %s: the naive epsilon-factor match is %v", p, x.Pi, x.Twist, *x.NaiveEpsilonMatch)
			}
		}
	}
}

func TestE2Inputs(t *testing.T) {
	good := E2Inputs{Primes: []int{3}, ConductorExponent: 4, TwistConductor: 2, AdditiveCharacter: "conductor-O", Families: e2All}
	for _, tc := range []struct {
		name string
		edit func(in *E2Inputs)
		want string
	}{
		{"good", func(_ *E2Inputs) {}, ""},
		{"no primes", func(in *E2Inputs) { in.Primes = nil }, "at least one prime"},
		{"p = 2", func(in *E2Inputs) { in.Primes = []int{2} }, "not 3, 5 or 7"},
		{"a large conductor", func(in *E2Inputs) { in.ConductorExponent = 5 }, "conductor_exponent"},
		{"a large twist", func(in *E2Inputs) { in.TwistConductor = 3 }, "twist_conductor"},
		{"another psi", func(in *E2Inputs) { in.AdditiveCharacter = "conductor-p" }, "conductor-O"},
		{"no families", func(in *E2Inputs) { in.Families = nil }, "at least one family"},
		{"an unknown family", func(in *E2Inputs) { in.Families = []string{"positive-depth"} }, "positive-depth"},
	} {
		in := good
		tc.edit(&in)
		err := in.check()
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: the error is %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseSpecE2(t *testing.T) {
	s, err := ParseSpec("lab/specs/e2-llc-gl2-small.yaml", []byte(SpecE2))
	if err != nil {
		t.Fatal(err)
	}
	if s.Experiment != "E2" || len(s.E2.Primes) != 3 || s.E2.ConductorExponent != 4 || s.E2.TwistConductor != 2 || len(s.E2.Families) != 3 {
		t.Errorf("the spec is %+v", s.E2)
	}
	bad := strings.Replace(SpecE2, "  twist_conductor: 2", "  twist_conductor: 2\n  seed: 1", 1)
	if _, err := ParseSpec("lab/specs/e2.yaml", []byte(bad)); err == nil || !strings.Contains(err.Error(), "seed") {
		t.Errorf("an unknown input gives %v", err)
	}
}

const smallSpecE2 = `name: e2-small
experiment: E2
question: Do the small cases agree?
tool: lgl
node: LG00000002
inputs: {primes: [3], conductor_exponent: 2, twist_conductor: 1, additive_character: conductor-O, families: [principal-series, steinberg, depth-zero-supercuspidal]}
outputs: [counts.json, principal.json, steinberg.json, supercuspidal.json, summary.md]
`

func TestRunE2Spec(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lab", "specs", "e2-small.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(smallSpecE2), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) }
	rec, err := Run(s, Options{Runs: RunsDir(path), Host: "test", ToolVersion: "dev", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Mismatches != 0 || !strings.HasPrefix(rec.Result, "agrees") || len(rec.Files) != 5 {
		t.Errorf("the record is %+v", rec)
	}
	summary, err := os.ReadFile(filepath.Join(RunsDir(path), rec.ID, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## What the run does not do", "## Counts", "lambda(E/F, psi) = 1", "Bushnell-Henniart"} {
		if !strings.Contains(string(summary), want) {
			t.Errorf("the summary does not have %q", want)
		}
	}
	if strings.ContainsAny(string(summary), "\u2013\u2014") {
		t.Error("the summary has a dash character")
	}
}

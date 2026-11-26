package lab

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"sort"
	"strings"
)

// E2Inputs are the inputs of experiment E2, the explicit local Langlands correspondence for GL2(Q_p) with the factors of pairs.
type E2Inputs struct {
	Primes            []int    `yaml:"primes" json:"primes"`
	ConductorExponent int      `yaml:"conductor_exponent" json:"conductor_exponent"` // the largest conductor exponent a(pi)
	TwistConductor    int      `yaml:"twist_conductor" json:"twist_conductor"`       // the largest conductor exponent of a twist chi
	AdditiveCharacter string   `yaml:"additive_character" json:"additive_character"` // only conductor-O
	Families          []string `yaml:"families" json:"families"`
}

// The families of representations of GL2(Q_p) that E2 covers.
const (
	FamilyPrincipal     = "principal-series"
	FamilySteinberg     = "steinberg"
	FamilySupercuspidal = "depth-zero-supercuspidal"
)

func (in E2Inputs) check() error {
	if len(in.Primes) == 0 {
		return errors.New("E2 needs at least one prime")
	}
	for _, p := range in.Primes {
		if p != 3 && p != 5 && p != 7 {
			return fmt.Errorf("E2: the prime %d is not 3, 5 or 7", p)
		}
	}
	switch {
	case in.ConductorExponent < 0 || in.ConductorExponent > 4:
		return errors.New("E2: conductor_exponent must be from 0 to 4")
	case in.TwistConductor < 0 || in.TwistConductor > 2:
		return errors.New("E2: twist_conductor must be from 0 to 2")
	case in.AdditiveCharacter != "conductor-O":
		return fmt.Errorf("E2: the additive character %q is not conductor-O", in.AdditiveCharacter)
	case len(in.Families) == 0:
		return errors.New("E2 needs at least one family")
	}
	for _, f := range in.Families {
		if f != FamilyPrincipal && f != FamilySteinberg && f != FamilySupercuspidal {
			return fmt.Errorf("E2: the family %q is not %s, %s or %s", f, FamilyPrincipal, FamilySteinberg, FamilySupercuspidal)
		}
	}
	return nil
}

func (in E2Inputs) has(family string) bool {
	for _, f := range in.Families {
		if f == family {
			return true
		}
	}
	return false
}

// E2Pair is the result for one pair (pi, chi): the L-factor and the epsilon-factor of pi x chi on the two sides.
type E2Pair struct {
	P             int    `json:"p"`
	Pi            string `json:"pi"`
	Twist         string `json:"twist"`
	PiConductor   int    `json:"pi_conductor"`   // a(pi)
	PairConductor int    `json:"pair_conductor"` // a(pi x chi), the power of X in the epsilon-factor
	GL2L          string `json:"gl2_L"`
	GL2Epsilon    string `json:"gl2_epsilon"`
	GaloisL       string `json:"galois_L"`
	GaloisEpsilon string `json:"galois_epsilon"`
	Match         bool   `json:"match"`
	Note          string `json:"note,omitempty"` // why the pair does not match
	// For the supercuspidal pairs only: the central character against the determinant, and the same checks with no rectifier.
	CentralMatch      *bool `json:"central_match,omitempty"`
	NaiveEpsilonMatch *bool `json:"naive_epsilon_match,omitempty"`
	NaiveCentralMatch *bool `json:"naive_central_match,omitempty"`
}

// E2Group counts the principal series pairs of one prime with one conductor of pi and one conductor of the pair.
type E2Group struct {
	P             int     `json:"p"`
	PiConductor   int     `json:"pi_conductor"`
	PairConductor int     `json:"pair_conductor"`
	Pairs         int     `json:"pairs"`
	Matches       int     `json:"matches"`
	Example       E2Pair  `json:"example"`
	FirstMismatch *E2Pair `json:"first_mismatch,omitempty"`
}

// E2Count counts the pairs of one prime and one family.
type E2Count struct {
	P       int    `json:"p"`
	Family  string `json:"family"`
	Field   int    `json:"field"` // N of the field Q(zeta_N) of the run
	Pis     int    `json:"representations"`
	Twists  int    `json:"twists"`
	Pairs   int    `json:"pairs"`
	Matches int    `json:"matches"`
	// For the supercuspidal pairs: the pairs whose epsilon-factor, and whose central character, match with no rectifier.
	NaiveEpsilonMatches int `json:"naive_epsilon_matches,omitempty"`
	NaiveCentralMatches int `json:"naive_central_matches,omitempty"`
}

// E2Result is the result of a run of E2.
type E2Result struct {
	Counts        []E2Count `json:"counts"`
	Principal     []E2Group `json:"principal_series"`
	Steinberg     []E2Pair  `json:"steinberg"`
	Supercuspidal []E2Pair  `json:"supercuspidal"`
	Lambda        []string  `json:"lambda"` // lambda(E/F, psi) for each prime
	FirstMismatch *E2Pair   `json:"first_mismatch,omitempty"`
}

// Mismatches counts the pairs whose factors do not agree.
func (r E2Result) Mismatches() int {
	n := 0
	for _, c := range r.Counts {
		n += c.Pairs - c.Matches
	}
	return n
}

// Pairs counts all the pairs of the run.
func (r E2Result) Pairs() int {
	n := 0
	for _, c := range r.Counts {
		n += c.Pairs
	}
	return n
}

var errOverflow = errors.New("E2: an integer overflow in the cyclotomic arithmetic")

// RunE2 runs experiment E2. It uses no network and no tool outside this binary. All the arithmetic is exact, in Z[zeta_N] with rational denominators, and checked for overflow.
func RunE2(in E2Inputs) (res E2Result, err error) {
	if err := in.check(); err != nil {
		return E2Result{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && errors.Is(e, errOverflow) {
				err = errOverflow
				return
			}
			panic(r)
		}
	}()
	for _, p := range in.Primes {
		f := newE2Field(p)
		res.Lambda = append(res.Lambda, fmt.Sprintf("p = %d: lambda(E/F, psi) = %s", p, f.format(f.lambda(), nil)))
		if in.has(FamilyPrincipal) {
			groups, count := f.principal(in)
			res.Principal = append(res.Principal, groups...)
			res.Counts = append(res.Counts, count)
		}
		if in.has(FamilySteinberg) {
			pairs, count := f.steinberg(in)
			res.Steinberg = append(res.Steinberg, pairs...)
			res.Counts = append(res.Counts, count)
		}
		if in.has(FamilySupercuspidal) {
			pairs, count := f.supercuspidal(in)
			res.Supercuspidal = append(res.Supercuspidal, pairs...)
			res.Counts = append(res.Counts, count)
		}
	}
	for _, g := range res.Principal {
		if g.FirstMismatch != nil {
			res.FirstMismatch = g.FirstMismatch
			break
		}
	}
	for _, list := range [][]E2Pair{res.Steinberg, res.Supercuspidal} {
		for i := range list {
			if res.FirstMismatch == nil && !list[i].Match {
				res.FirstMismatch = &list[i]
			}
		}
	}
	return res, nil
}

// Checked integer arithmetic. An overflow panics with errOverflow, and RunE2 returns it as an error.

func cadd(a, b int64) int64 {
	c := a + b
	if (a > 0 && b > 0 && c < 0) || (a < 0 && b < 0 && c >= 0) {
		panic(errOverflow)
	}
	return c
}

func cmul(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	c := a * b
	if c/b != a || (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) {
		panic(errOverflow)
	}
	return c
}

func gcd64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func ipow(b, e int) int64 {
	r := int64(1)
	for range e {
		r = cmul(r, int64(b))
	}
	return r
}

func mod(a, m int) int {
	a %= m
	if a < 0 {
		a += m
	}
	return a
}

// cyc is an element of Q(zeta_N): the sum of v[i] zeta_N^i, divided by den. The vector v has length N and is not reduced; two elements are equal when their difference is 0 modulo the cyclotomic polynomial Phi_N.
type cyc struct {
	v   []int64
	den int64
}

// e2Field holds the arithmetic of one prime p: the field Q(zeta_N) with N = lcm(4, p^2, p^2 - 1), the characters of (Z/p^2)^x and of the residue field of the unramified quadratic extension E of Q_p.
type e2Field struct {
	p, q, n  int // q = p, n = N
	m        int // p(p-1), the order of (Z/p^2)^x
	e        int // q^2 - 1, the order of k_E^x
	phi      []int64
	deg      int
	sqrtq    cyc
	g        int   // the smallest primitive root mod p^2
	logF     []int // log_g of u mod p^2, -1 when p divides u
	eps      int   // the smallest non-residue mod p: E = Q_p(sqrt eps)
	logE     []int // log_gamma of a + b sqrt(eps) in k_E^x at the index a + p b, -1 at 0
	gamma    [2]int
	gauss    map[[2]int]cyc // G_n(chi_k^-1) at {k, n}
	tables   map[int][]int64
	nkeys    int
	keyOfBar []int // the class key of an element of M_2(F_p) at the index a + p b + p^2 c + p^3 d, -1 when split or singular
}

func newE2Field(p int) *e2Field {
	f := &e2Field{p: p, q: p, m: p * (p - 1), e: p*p - 1, gauss: map[[2]int]cyc{}, tables: map[int][]int64{}}
	f.n = lcm(lcm(4, p*p), p*p-1)
	f.phi = cyclotomicPoly(f.n)
	f.deg = len(f.phi) - 1
	// The primitive root mod p^2 and the table of discrete logarithms.
	p2 := p * p
	for g := 2; ; g++ {
		if g%p != 0 && orderMod(g, p2) == f.m {
			f.g = g
			break
		}
	}
	f.logF = make([]int, p2)
	for i := range f.logF {
		f.logF[i] = -1
	}
	for k, x := 0, 1; k < f.m; k, x = k+1, x*f.g%p2 {
		f.logF[x] = k
	}
	f.eps = int(nonResidue(p))
	// The generator gamma of k_E^x = F_p(sqrt eps)^x: the first element, in the order of a + p b, of order q^2 - 1.
	for idx := 1; idx < p2 && f.gamma == [2]int{}; idx++ {
		x := [2]int{idx % p, idx / p}
		k := 1
		for y := x; y != [2]int{1, 0}; y = f.mulE(y, x, p) {
			k++
		}
		if k == f.e {
			f.gamma = x
		}
	}
	f.logE = make([]int, p2)
	for i := range f.logE {
		f.logE[i] = -1
	}
	for k, y := 0, [2]int{1, 0}; k < f.e; k, y = k+1, f.mulE(y, f.gamma, p) {
		f.logE[y[0]+p*y[1]] = k
	}
	// The positive square root of q: Gauss's quadratic sum is sqrt(p) for p = 1 mod 4 and i sqrt(p) for p = 3 mod 4, with zeta_N = exp(2 pi i / N).
	gs := f.zero()
	for a := 1; a < p; a++ {
		gs.v[mod(a*(f.n/p), f.n)] += int64(legendreInt(a, p))
	}
	if p%4 == 3 {
		gs = f.mul(f.root(3*f.n/4, 1), gs) // -i = zeta_4^3
	}
	f.sqrtq = gs
	f.classKeys()
	return f
}

func lcm(a, b int) int {
	x, y := a, b
	for y != 0 {
		x, y = y, x%y
	}
	return a / x * b
}

func orderMod(g, m int) int {
	k, x := 1, g%m
	for x != 1 {
		x = x * g % m
		k++
	}
	return k
}

func legendreInt(a, p int) int {
	r := 1
	for range (p - 1) / 2 {
		r = r * a % p
	}
	if mod(r, p) == 1 {
		return 1
	}
	return -1
}

// mulE multiplies a + b sqrt(eps) in (Z/r)[sqrt eps].
func (f *e2Field) mulE(x, y [2]int, r int) [2]int {
	return [2]int{mod(x[0]*y[0]+f.eps*x[1]*y[1], r), mod(x[0]*y[1]+x[1]*y[0], r)}
}

func moebius(n int) int {
	r := 1
	for d := 2; d*d <= n; d++ {
		if n%d == 0 {
			n /= d
			if n%d == 0 {
				return 0
			}
			r = -r
		}
	}
	if n > 1 {
		r = -r
	}
	return r
}

// cyclotomicPoly returns Phi_N, the product of (x^d - 1)^mu(N/d) over the divisors d of N, as coefficients from the constant term up.
func cyclotomicPoly(n int) []int64 {
	poly := []int64{1}
	var down []int
	for d := 1; d <= n; d++ {
		if n%d != 0 {
			continue
		}
		switch moebius(n / d) {
		case 1:
			out := make([]int64, len(poly)+d)
			for i, c := range poly {
				out[i+d] = cadd(out[i+d], c)
				out[i] = cadd(out[i], -c)
			}
			poly = out
		case -1:
			down = append(down, d)
		}
	}
	for _, d := range down {
		// P = Q (x^d - 1), so q_(j-d) = p_j + q_j from the top.
		qd := make([]int64, len(poly)-d)
		for j := len(poly) - 1; j >= d; j-- {
			var qj int64
			if j < len(qd) {
				qj = qd[j]
			}
			qd[j-d] = cadd(poly[j], qj)
		}
		poly = qd
	}
	return poly
}

func (f *e2Field) zero() cyc { return cyc{v: make([]int64, f.n), den: 1} }

// root is c zeta_N^e.
func (f *e2Field) root(e int, c int64) cyc {
	x := f.zero()
	x.v[mod(e, f.n)] = c
	return x
}

func (f *e2Field) mul(a, b cyc) cyc {
	out := f.zero()
	out.den = cmul(a.den, b.den)
	var ia []int
	for i, c := range a.v {
		if c != 0 {
			ia = append(ia, i)
		}
	}
	for j, cb := range b.v {
		if cb == 0 {
			continue
		}
		for _, i := range ia {
			k := i + j
			if k >= f.n {
				k -= f.n
			}
			out.v[k] = cadd(out.v[k], cmul(a.v[i], cb))
		}
	}
	return f.normal(out)
}

func (f *e2Field) add(a, b cyc) cyc {
	out := f.zero()
	out.den = cmul(a.den, b.den)
	for i := range out.v {
		out.v[i] = cadd(cmul(a.v[i], b.den), cmul(b.v[i], a.den))
	}
	return f.normal(out)
}

// scale is a num / den.
func (f *e2Field) scale(a cyc, num, den int64) cyc {
	out := f.zero()
	out.den = cmul(a.den, den)
	for i, c := range a.v {
		out.v[i] = cmul(c, num)
	}
	if out.den < 0 {
		out.den = -out.den
		for i := range out.v {
			out.v[i] = -out.v[i]
		}
	}
	return f.normal(out)
}

// normal divides the coefficients and the denominator by their gcd.
func (f *e2Field) normal(a cyc) cyc {
	g := a.den
	for _, c := range a.v {
		if c != 0 {
			g = gcd64(g, c)
			if g == 1 {
				return a
			}
		}
	}
	if g > 1 {
		a.den /= g
		for i := range a.v {
			a.v[i] /= g
		}
	}
	return a
}

// reduce returns the numerator of a as a polynomial of degree less than phi(N), the remainder modulo Phi_N.
func (f *e2Field) reduce(a cyc) []int64 {
	w := append([]int64(nil), a.v...)
	for i := len(w) - 1; i >= f.deg; i-- {
		c := w[i]
		if c == 0 {
			continue
		}
		for j, pc := range f.phi {
			if pc != 0 {
				w[i-f.deg+j] = cadd(w[i-f.deg+j], -cmul(c, pc))
			}
		}
	}
	return w[:f.deg]
}

func (f *e2Field) isZero(a cyc) bool {
	for _, c := range f.reduce(a) {
		if c != 0 {
			return false
		}
	}
	return true
}

func (f *e2Field) equal(a, b cyc) bool {
	return f.isZero(f.add(a, f.scale(b, -1, 1)))
}

// unit returns s and e when a = s zeta_N^e with s = 1 or -1 as written.
func (f *e2Field) unit(a cyc) (e int, s int64, ok bool) {
	if a.den != 1 {
		return 0, 0, false
	}
	n := 0
	for i, c := range a.v {
		if c != 0 {
			n++
			e, s = i, c
		}
	}
	return e, s, n == 1 && (s == 1 || s == -1)
}

func (f *e2Field) complex(a cyc) complex128 {
	var z complex128
	for i, c := range a.v {
		if c != 0 {
			z += complex(float64(c), 0) * cmplx.Exp(complex(0, 2*math.Pi*float64(i)/float64(f.n)))
		}
	}
	return z / complex(float64(a.den), 0)
}

// mono is c q^(h/2) a1^a[0] a2^a[1] a3^a[2] X^x, with X = q^-s. The a_i are formal variables: the values at p of the unramified parts of the characters, so one row covers all the unramified twists.
type mono struct {
	c cyc
	h int
	a [3]int
	x int
}

func (f *e2Field) one() mono { return mono{c: f.root(0, 1)} }

func (f *e2Field) mmul(u, v mono) mono {
	return mono{c: f.mul(u.c, v.c), h: u.h + v.h, a: [3]int{u.a[0] + v.a[0], u.a[1] + v.a[1], u.a[2] + v.a[2]}, x: u.x + v.x}
}

// minv inverts a monomial whose coefficient is a root of unity up to sign.
func (f *e2Field) minv(u mono) (mono, bool) {
	e, s, ok := f.unit(u.c)
	if !ok {
		return mono{}, false
	}
	return mono{c: f.root(-e, s), h: -u.h, a: [3]int{-u.a[0], -u.a[1], -u.a[2]}, x: -u.x}, true
}

// sqrtPow is q^(k/2) for k >= 0.
func (f *e2Field) sqrtPow(k int) cyc {
	c := f.scale(f.root(0, 1), ipow(f.q, k/2), 1)
	if k%2 == 1 {
		c = f.mul(c, f.sqrtq)
	}
	return c
}

func (f *e2Field) mequal(u, v mono) bool {
	if u.x != v.x || u.a != v.a {
		return false
	}
	m := min(u.h, v.h)
	return f.equal(f.mul(u.c, f.sqrtPow(u.h-m)), f.mul(v.c, f.sqrtPow(v.h-m)))
}

// format writes a monomial: an exact coefficient when it is rational, else its complex value to 6 places.
func (f *e2Field) format(u mono, names []string) string {
	var coef string
	r := f.reduce(u.c)
	rational := true
	for _, c := range r[1:] {
		if c != 0 {
			rational = false
		}
	}
	if rational {
		num, den := r[0], u.c.den
		if g := gcd64(num, den); g > 1 {
			num, den = num/g, den/g
		}
		coef = fmt.Sprint(num)
		if den != 1 {
			coef += fmt.Sprintf("/%d", den)
		}
		if u.h != 0 {
			q := fmt.Sprintf("q^(%s)", half(u.h))
			switch coef {
			case "1":
				coef = q
			case "-1":
				coef = "-" + q
			default:
				coef += " " + q
			}
		}
	} else {
		z := f.complex(u.c) * complex(math.Pow(float64(f.q), float64(u.h)/2), 0)
		coef = fmt.Sprintf("(%.6f%+.6fi)", real(z), imag(z))
	}
	var vars []string
	for i, k := range u.a {
		if k != 0 && i < len(names) {
			vars = append(vars, power(names[i], k))
		}
	}
	if u.x != 0 {
		vars = append(vars, power("X", u.x))
	}
	v := strings.Join(vars, " ")
	switch {
	case v == "":
		return coef
	case coef == "1":
		return v
	case coef == "-1":
		return "-" + v
	}
	return coef + " " + v
}

func half(h int) string {
	if h%2 == 0 {
		return fmt.Sprint(h / 2)
	}
	return fmt.Sprintf("%d/2", h)
}

func power(name string, k int) string {
	if k == 1 {
		return name
	}
	return fmt.Sprintf("%s^%d", name, k)
}

// formatL writes an L-factor given by the alphas of 1/L = prod (1 - alpha X).
func (f *e2Field) formatL(alphas []mono, names []string) string {
	if len(alphas) == 0 {
		return "1"
	}
	var parts []string
	for _, a := range alphas {
		a.x = 1
		parts = append(parts, "(1 - "+f.format(a, names)+")")
	}
	return "1/" + strings.Join(parts, "")
}

// sameL compares two L-factors as multisets of alphas.
func (f *e2Field) sameL(a, b []mono) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, x := range a {
		found := false
		for j, y := range b {
			if !used[j] && f.mequal(x, y) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// gl1 is a character of Q_p^x of conductor at most 2: chi_k on Z_p^x, with chi_k(g) = zeta_(p(p-1))^k for the primitive root g mod p^2, and chi(p) = s q^(h/2) a^a.
type gl1 struct {
	k int
	s int64
	h int
	a [3]int
}

func (c gl1) mulc(d gl1, m int) gl1 {
	return gl1{k: mod(c.k+d.k, m), s: c.s * d.s, h: c.h + d.h, a: [3]int{c.a[0] + d.a[0], c.a[1] + d.a[1], c.a[2] + d.a[2]}}
}

// shift is chi |.|^(-h/2): its value at p is multiplied by q^(h/2).
func (c gl1) shift(h int) gl1 { c.h += h; return c }

func (c gl1) inverse(m int) gl1 {
	return gl1{k: mod(-c.k, m), s: c.s, h: -c.h, a: [3]int{-c.a[0], -c.a[1], -c.a[2]}}
}

// cond is the conductor exponent of chi_k: 0 when k = 0, 1 when chi_k is trivial on 1 + p Z_p, that is p divides k, else 2.
func (f *e2Field) cond(k int) int {
	switch {
	case k == 0:
		return 0
	case k%f.p == 0:
		return 1
	}
	return 2
}

func (f *e2Field) atP(c gl1) mono {
	return mono{c: f.root(0, c.s), h: c.h, a: c.a}
}

// gaussF is G_n(chi_k^-1) = sum over u in (Z/p^n)^x of chi_k^-1(u) psi(u / p^n), with psi(x) = exp(2 pi i {x}_p) of conductor O.
func (f *e2Field) gaussF(k, n int) cyc {
	if g, ok := f.gauss[[2]int{k, n}]; ok {
		return g
	}
	pn := int(ipow(f.p, n))
	g := f.zero()
	for u := 1; u < pn; u++ {
		if u%f.p == 0 {
			continue
		}
		e := -k*f.logF[u]*(f.n/f.m) + u*(f.n/pn)
		g.v[mod(e, f.n)]++
	}
	f.gauss[[2]int{k, n}] = g
	return g
}

// tate gives Tate's factors of a character chi of Q_p^x with psi of conductor O: L(s, chi) = 1/(1 - chi(p) X) when chi is unramified and 1 otherwise, and epsilon(s, chi, psi) = chi(p)^n X^n G_n(chi^-1) with n = a(chi) (Tate's thesis; Tate, Number theoretic background, 1979).
func (f *e2Field) tate(c gl1) (eps mono, alphas []mono) {
	n := f.cond(c.k)
	if n == 0 {
		return f.one(), []mono{f.atP(c)}
	}
	s := int64(1)
	if n%2 == 1 {
		s = c.s
	}
	return mono{c: f.scale(f.gaussF(c.k, n), s, 1), h: n * c.h, a: [3]int{n * c.a[0], n * c.a[1], n * c.a[2]}, x: n}, nil
}

// gl1E is a character xi of E^x, E = Q_p(sqrt eps) unramified: xi(u) = theta_j(u mod p) chi_k(N u) on O_E^x, with theta_j(gamma) = zeta_(q^2-1)^j, and xi(p) = s q^(h/2) a^a.
type gl1E struct {
	j, k int
	s    int64
	h    int
	a    [3]int
}

// evalE is the exponent e of xi(u) = zeta_N^e for u = (a, b) = a + b sqrt(eps) in (O_E / p^2)^x.
func (f *e2Field) evalE(xi gl1E, u [2]int) int {
	p2 := f.p * f.p
	bar := u[0]%f.p + f.p*(u[1]%f.p)
	nm := mod(u[0]*u[0]-f.eps*u[1]*u[1], p2)
	return mod(xi.j*f.logE[bar]*(f.n/f.e)+xi.k*f.logF[nm]*(f.n/f.m), f.n)
}

// condE is the conductor exponent of xi on E.
func (f *e2Field) condE(xi gl1E) int {
	if f.cond(xi.k) == 2 {
		return 2 // chi o N is not trivial on 1 + p O_E, as N maps it onto 1 + p Z_p
	}
	for b := range f.p {
		for a := range f.p {
			if (a != 0 || b != 0) && f.evalE(xi, [2]int{a, b}) != 0 {
				return 1
			}
		}
	}
	return 0
}

// tateE gives Tate's factors of xi over E with psi_E = psi o Tr, of conductor O_E: epsilon(s, xi, psi_E) = xi(p)^n q_E^(-ns) G_n(xi^-1) with q_E^-s = X^2.
func (f *e2Field) tateE(xi gl1E) (eps mono, ramified bool) {
	n := f.condE(xi)
	if n == 0 {
		return f.one(), false
	}
	pn := int(ipow(f.p, n))
	g := f.zero()
	for b := range pn {
		for a := range pn {
			if a%f.p == 0 && b%f.p == 0 {
				continue
			}
			// The trace of a + b sqrt(eps) is 2a.
			e := -f.evalE(xi, [2]int{a, b}) + mod(2*a, pn)*(f.n/pn)
			g.v[mod(e, f.n)]++
		}
	}
	s := int64(1)
	if n%2 == 1 {
		s = xi.s
	}
	return mono{c: f.scale(g, s, 1), h: n * xi.h, a: [3]int{n * xi.a[0], n * xi.a[1], n * xi.a[2]}, x: 2 * n}, true
}

// lambda is the Langlands constant lambda(E/F, psi) = epsilon(Ind 1_E, psi) / epsilon(1_E, psi_E), with Ind 1_E = 1_F + eta, eta the unramified quadratic character of F^x.
func (f *e2Field) lambda() mono {
	e1, _ := f.tate(gl1{s: 1})
	eEta, _ := f.tate(gl1{s: -1})
	eE, _ := f.tateE(gl1E{s: 1})
	inv, ok := f.minv(eE)
	if !ok {
		panic("E2: epsilon(1_E) is not a unit")
	}
	return f.mmul(f.mmul(e1, eEta), inv)
}

// The Galois side.

// wd is a Weil-Deligne representation: a sum of characters, and N with N e_i = e_j for each pair {i, j}.
type wd struct {
	chars []gl1
	n     [][2]int
}

// galoisWD gives L(s, rho) = det(1 - Phi X | (V^I)^(N=0))^-1 and epsilon(s, rho, psi) = prod epsilon(s, chi_i, psi) det(-Phi X | V^I / (V^I)^(N=0)), with Phi the geometric Frobenius, the image of p (Deligne 1973; Tate 1979, section 4).
func (f *e2Field) galoisWD(r wd) (alphas []mono, eps mono, err error) {
	source := map[int]bool{}
	for _, e := range r.n {
		ci, cj := r.chars[e[0]], r.chars[e[1]]
		// Phi N Phi^-1 = q^-1 N: the Frobenius eigenvalue on e_j is q^-1 times the one on e_i.
		if ci.k != cj.k || ci.a != cj.a || ci.s != cj.s || cj.h != ci.h-2 {
			return nil, mono{}, errors.New("N does not satisfy Phi N Phi^-1 = q^-1 N")
		}
		source[e[0]] = true
	}
	eps = f.one()
	for i, c := range r.chars {
		e, a := f.tate(c)
		eps = f.mmul(eps, e)
		if len(a) == 0 {
			continue
		}
		if source[i] {
			// e_i is in V^I but not in the kernel of N.
			t := f.atP(c)
			t.c = f.scale(t.c, -1, 1)
			t.x = 1
			eps = f.mmul(eps, t)
			continue
		}
		alphas = append(alphas, a...)
	}
	return alphas, eps, nil
}

// galoisInduced gives the factors of Ind_(W_E)^(W_F) xi by inductivity in degree 0: L(s, Ind xi) = L(s, xi) and epsilon(s, Ind xi, psi) = lambda(E/F, psi) epsilon(s, xi, psi_E) (Deligne 1973; Tate 1979).
func (f *e2Field) galoisInduced(xi gl1E) (alphas []mono, eps mono, err error) {
	e, ramified := f.tateE(xi)
	if !ramified {
		return nil, mono{}, errors.New("xi is unramified, so Ind xi is not irreducible")
	}
	return nil, f.mmul(f.lambda(), e), nil
}

// The GL2 side.

// steinbergGL2 gives the factors of eta St, the unique irreducible subrepresentation of the normalized induction from eta |.|^(1/2) x eta |.|^(-1/2): L(s, eta St) = L(s, mu1) and epsilon(s, eta St, psi) = epsilon(s, mu1, psi) epsilon(s, mu2, psi) L(1 - s, mu1^-1) / L(s, mu2), with mu1 = eta |.|^(1/2) and mu2 = eta |.|^(-1/2) (Jacquet-Langlands 1970). The ratio of L-factors must cancel to a monomial.
func (f *e2Field) steinbergGL2(eta gl1) (alphas []mono, eps mono, err error) {
	mu1, mu2 := eta.shift(-1), eta.shift(1)
	e1, l1 := f.tate(mu1)
	e2, l2 := f.tate(mu2)
	_, linv := f.tate(mu1.inverse(f.m))
	eps = f.mmul(e1, e2)
	num := append([]mono(nil), l2...)
	for _, b := range linv {
		// L(1 - s, mu) = 1/(1 - mu(p) q^-1 X^-1), and 1 - beta X^-1 = -beta X^-1 (1 - beta^-1 X).
		b.h -= 2
		binv, ok := f.minv(b)
		if !ok {
			return nil, mono{}, errors.New("an L-factor of the ratio has no inverse")
		}
		t := binv
		t.c = f.scale(t.c, -1, 1)
		t.x = 1
		eps = f.mmul(eps, t)
		found := false
		for i, a := range num {
			if f.mequal(a, binv) {
				num = append(num[:i], num[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return nil, mono{}, errors.New("the ratio of L-factors is not a monomial")
		}
	}
	if len(num) > 0 {
		return nil, mono{}, errors.New("the ratio of L-factors is not a monomial")
	}
	return l1, eps, nil
}

// classKeys finds the class of each element of GL2(F_p) for the character of a cuspidal representation sigma_theta of GL2(F_p) (Fulton-Harris, Representation theory, section 5.2): (q - 1) theta(a) on the scalar a, -theta(a) on a times a nontrivial unipotent, 0 on the split semisimple classes, and -(theta(x) + theta(x^q)) on the elliptic class with eigenvalue x in k_E \ k. The key is the type times (q^2 - 1) plus log_gamma of the eigenvalue.
func (f *e2Field) classKeys() {
	p := f.p
	f.nkeys = 3 * f.e
	f.keyOfBar = make([]int, p*p*p*p)
	inv2 := (p + 1) / 2
	for idx := range f.keyOfBar {
		a, b, c, d := idx%p, idx/p%p, idx/(p*p)%p, idx/(p*p*p)
		f.keyOfBar[idx] = -1
		det := mod(a*d-b*c, p)
		if det == 0 {
			continue
		}
		t := mod(a+d, p)
		disc := mod(t*t-4*det, p)
		switch {
		case disc == 0:
			l := t * inv2 % p
			typ := 1
			if b == 0 && c == 0 && a == d {
				typ = 0
			}
			f.keyOfBar[idx] = typ*f.e + f.logE[l]
		case legendreInt(disc, p) == -1:
			r := disc * modInverse(f.eps, p) % p
			s := 1
			for s*s%p != r {
				s++
			}
			x := [2]int{t * inv2 % p, s * inv2 % p}
			f.keyOfBar[idx] = 2*f.e + f.logE[x[0]+p*x[1]]
		}
	}
}

func modInverse(a, p int) int {
	for x := 1; x < p; x++ {
		if a*x%p == 1 {
			return x
		}
	}
	panic("no inverse")
}

// table counts the h in GL2(Z/p^n) by the class key of h mod p, det h mod p^n and tr h mod p^n.
func (f *e2Field) table(n int) []int64 {
	if t, ok := f.tables[n]; ok {
		return t
	}
	pn := int(ipow(f.p, n))
	p := f.p
	t := make([]int64, f.nkeys*pn*pn)
	for d := range pn {
		for c := range pn {
			for b := range pn {
				for a := range pn {
					key := f.keyOfBar[a%p+p*(b%p)+p*p*(c%p)+p*p*p*(d%p)]
					if key < 0 {
						continue
					}
					det := mod(a*d-b*c, pn)
					tr := (a + d) % pn
					t[(key*pn+det)*pn+tr]++
				}
			}
		}
	}
	f.tables[n] = t
	return t
}

// gl2Sum is T_m = sum over h in GL2(Z/p^n) of chi_(Lambda')^v(h) psi(p^m tr h), where Lambda' = sigma_theta_j tensor chi_k o det and chi^v is the character of the contragredient: the character of sigma_theta_(-j) at h mod p times chi_k^-1(det h).
func (f *e2Field) gl2Sum(j, k, n, m int) cyc {
	pn := int(ipow(f.p, n))
	t := f.table(n)
	out := f.zero()
	pm := int(ipow(f.p, -m)) // psi(p^m t) = zeta_(p^-m)^t
	for key := range f.nkeys {
		typ, l := key/f.e, key%f.e
		var exps []int
		var coef int64
		switch typ {
		case 0:
			exps, coef = []int{-j * l}, int64(f.q-1)
		case 1:
			exps, coef = []int{-j * l}, -1
		case 2:
			exps, coef = []int{-j * l, -j * l * f.q}, -1
		}
		for det := range pn {
			if f.logF[det%(f.p*f.p)] < 0 {
				continue
			}
			ce := -k * f.logF[det] * (f.n / f.m)
			for tr := range pn {
				cnt := t[(key*pn+det)*pn+tr]
				if cnt == 0 {
					continue
				}
				pe := (tr % pm) * (f.n / pm)
				for _, e := range exps {
					x := mod(e*(f.n/f.e)+ce+pe, f.n)
					out.v[x] = cadd(out.v[x], cmul(cnt, coef))
				}
			}
		}
	}
	return out
}

// supercuspidalGL2 gives epsilon(s, pi_theta tensor chi, psi) from the functional equation of Godement and Jacquet (Godement-Jacquet, Zeta functions of simple algebras, 1972), with an explicit test function and matrix coefficient.
//
// pi_theta tensor chi is c-Ind from ZK of Lambda', K = GL2(Z_p), Lambda' = sigma_theta tensor chi o det on K, Lambda'(p) = theta(p) chi(p)^2 = c. Lambda' factors through GL2(Z/p^n), n = max(1, a(chi)), and has dimension q - 1. Take f = tr Lambda' on ZK and 0 elsewhere, a matrix coefficient, and Phi = the conjugate of tr Lambda' on K and 0 elsewhere, with dx self-dual for psi(tr xy) and vol(K) = 1 for dg. Then Z(Phi, s, f) = 1, Phi^ is q^-4n sum over g of Phi(g) psi(tr gy) on p^-n M_2(Z_p), and Z(Phi^, 1 - s, f^v) = sum over m >= -n of c^-m q^(-4n-3m) X^(-2m) T_m / (q - 1), by the convolution of characters. The terms with m >= 0 are T_0 times the same factors, and T_0 = 0 when Lambda' is not trivial. L(s, pi tensor chi) = 1, so epsilon = gamma = Z(Phi^, 1 - s, f^v). For n = 1 this is epsilon(s, pi_theta, psi) = -theta(p) X^2 sum over x in k_E^x of theta^-1(x) psi(Tr x / p), the Gauss sum of Kondo for GL2(F_q).
func (f *e2Field) supercuspidalGL2(j, k int) (eps mono, err error) {
	n := max(1, f.cond(k))
	terms := map[int]cyc{}
	for m := -n; m <= 0; m++ {
		tm := f.gl2Sum(j, k, n, m)
		// q^(-4n-3m) / (q - 1)
		e := -4*n - 3*m
		var c cyc
		if e >= 0 {
			c = f.scale(tm, ipow(f.q, e), int64(f.q-1))
		} else {
			c = f.scale(tm, 1, cmul(ipow(f.q, -e), int64(f.q-1)))
		}
		terms[-m] = c // the power of c is -m and the power of X is -2m
	}
	var keep []int
	for i, c := range terms {
		if !f.isZero(c) {
			keep = append(keep, i)
		}
	}
	if len(keep) != 1 {
		sort.Ints(keep)
		return mono{}, fmt.Errorf("the zeta integral is not a monomial in X: the powers of X are %v", keep)
	}
	i := keep[0]
	return mono{c: terms[i], a: [3]int{i}, x: 2 * i}, nil
}

// centralGL2 is the central character theta|F^x chi^2 of pi_theta tensor chi, as the exponent of its value at g and its value at p.
func (f *e2Field) centralGL2(j, k int) (int, mono) {
	l := f.logE[f.g%f.p]
	return mod(j*l*(f.n/f.e)+2*k*(f.n/f.m), f.n), mono{c: f.root(0, 1), a: [3]int{1}}
}

// detGalois is det Ind xi = eta_(E/F) xi|F^x (the determinant of an induced representation), as the exponent of its value at g and its value at p. eta is unramified with eta(p) = -1.
func (f *e2Field) detGalois(xi gl1E) (int, mono) {
	return f.evalE(xi, [2]int{f.g, 0}), mono{c: f.root(0, -xi.s), h: xi.h, a: xi.a}
}

// The families.

func (f *e2Field) twists(in E2Inputs) []int {
	var ks []int
	for k := range f.m {
		if f.cond(k) <= in.TwistConductor {
			ks = append(ks, k)
		}
	}
	return ks
}

func chiName(k int) string { return fmt.Sprintf("chi_%d", k) }

var (
	namesPS = []string{"a1", "a2"}
	namesSt = []string{"a"}
	namesSC = []string{"c"}
)

func (f *e2Field) principal(in E2Inputs) ([]E2Group, E2Count) {
	count := E2Count{P: f.p, Family: FamilyPrincipal, Field: f.n}
	ks := f.twists(in)
	count.Twists = len(ks)
	groups := map[[2]int]*E2Group{}
	for k1 := range f.m {
		for k2 := k1; k2 < f.m; k2++ {
			api := f.cond(k1) + f.cond(k2)
			if api > in.ConductorExponent {
				continue
			}
			count.Pis++
			c1, c2 := gl1{k: k1, s: 1, a: [3]int{1}}, gl1{k: k2, s: 1, a: [3]int{0, 1}}
			for _, k := range ks {
				chi := gl1{k: k, s: 1}
				d1, d2 := c1.mulc(chi, f.m), c2.mulc(chi, f.m)
				// GL2 side: L(s, pi(chi1, chi2) chi) = L(s, chi1 chi) L(s, chi2 chi) and epsilon is the product of the two epsilons (Jacquet-Langlands 1970).
				g1, l1 := f.tate(d1)
				g2, l2 := f.tate(d2)
				gl2L, gl2E := append(l1, l2...), f.mmul(g1, g2)
				// Galois side: chi1 chi + chi2 chi with N = 0.
				galL, galE, err := f.galoisWD(wd{chars: []gl1{d1, d2}})
				pair := E2Pair{P: f.p, Pi: fmt.Sprintf("pi(%s a1, %s a2)", chiName(k1), chiName(k2)), Twist: chiName(k), PiConductor: api, PairConductor: gl2E.x,
					GL2L: f.formatL(gl2L, namesPS), GL2Epsilon: f.format(gl2E, namesPS)}
				pair.Match, pair.Note = f.compare(gl2L, gl2E, galL, galE, err)
				if err == nil {
					pair.GaloisL, pair.GaloisEpsilon = f.formatL(galL, namesPS), f.format(galE, namesPS)
				}
				key := [2]int{api, pair.PairConductor}
				gr := groups[key]
				if gr == nil {
					gr = &E2Group{P: f.p, PiConductor: api, PairConductor: pair.PairConductor, Example: pair}
					groups[key] = gr
				}
				gr.Pairs++
				count.Pairs++
				if pair.Match {
					gr.Matches++
					count.Matches++
				} else if gr.FirstMismatch == nil {
					pp := pair
					gr.FirstMismatch = &pp
				}
			}
		}
	}
	var out []E2Group
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PiConductor != out[j].PiConductor {
			return out[i].PiConductor < out[j].PiConductor
		}
		return out[i].PairConductor < out[j].PairConductor
	})
	return out, count
}

func (f *e2Field) compare(gl2L []mono, gl2E mono, galL []mono, galE mono, err error) (bool, string) {
	switch {
	case err != nil:
		return false, "the Galois side: " + err.Error()
	case !f.sameL(gl2L, galL):
		return false, "the L-factors differ"
	case !f.mequal(gl2E, galE):
		return false, "the epsilon-factors differ"
	}
	return true, ""
}

func (f *e2Field) steinberg(in E2Inputs) ([]E2Pair, E2Count) {
	count := E2Count{P: f.p, Family: FamilySteinberg, Field: f.n}
	ks := f.twists(in)
	count.Twists = len(ks)
	var out []E2Pair
	for k0 := range f.m {
		api := 1
		if f.cond(k0) > 0 {
			api = 2 * f.cond(k0)
		}
		if api > in.ConductorExponent {
			continue
		}
		count.Pis++
		for _, k := range ks {
			eta := gl1{k: mod(k0+k, f.m), s: 1, a: [3]int{1}} // (chi St) tensor chi' = (chi chi') St
			pair := E2Pair{P: f.p, Pi: fmt.Sprintf("%s St, chi(p) = a", chiName(k0)), Twist: chiName(k), PiConductor: api}
			gl2L, gl2E, err := f.steinbergGL2(eta)
			if err != nil {
				pair.Note = "the GL2 side: " + err.Error()
				out = append(out, pair)
				count.Pairs++
				continue
			}
			pair.PairConductor = gl2E.x
			pair.GL2L, pair.GL2Epsilon = f.formatL(gl2L, namesSt), f.format(gl2E, namesSt)
			// Galois side: sp(2) tensor eta = eta |.|^(-1/2) e1 + eta |.|^(1/2) e2 with N e1 = e2.
			galL, galE, gerr := f.galoisWD(wd{chars: []gl1{eta.shift(1), eta.shift(-1)}, n: [][2]int{{0, 1}}})
			pair.Match, pair.Note = f.compare(gl2L, gl2E, galL, galE, gerr)
			if gerr == nil {
				pair.GaloisL, pair.GaloisEpsilon = f.formatL(galL, namesSt), f.format(galE, namesSt)
			}
			out = append(out, pair)
			count.Pairs++
			if pair.Match {
				count.Matches++
			}
		}
	}
	return out, count
}

// regularThetas returns one j for each pair {theta_j, theta_j^q} of regular characters of k_E^x: theta_j^q = theta_qj, and theta_j is regular when q + 1 does not divide j.
func (f *e2Field) regularThetas() []int {
	var js []int
	for j := range f.e {
		if j%(f.q+1) != 0 && j < j*f.q%f.e {
			js = append(js, j)
		}
	}
	return js
}

func (f *e2Field) supercuspidal(in E2Inputs) ([]E2Pair, E2Count) {
	count := E2Count{P: f.p, Family: FamilySupercuspidal, Field: f.n}
	ks := f.twists(in)
	count.Twists = len(ks)
	var out []E2Pair
	if in.ConductorExponent < 2 {
		return nil, count
	}
	for _, j := range f.regularThetas() {
		count.Pis++
		for _, k := range ks {
			pair := E2Pair{P: f.p, Pi: fmt.Sprintf("pi(theta_%d), theta(p) = c", j), Twist: chiName(k), PiConductor: 2}
			count.Pairs++
			gl2E, err := f.supercuspidalGL2(j, k)
			if err != nil {
				pair.Note = "the GL2 side: " + err.Error()
				out = append(out, pair)
				continue
			}
			pair.PairConductor = gl2E.x
			pair.GL2L, pair.GL2Epsilon = "1", f.format(gl2E, namesSC)
			wg, wp := f.centralGL2(j, k)
			// Galois side: Ind of theta Delta (chi o N), with Delta_theta the unramified quadratic character of E^x, Delta(p) = -1 (Bushnell-Henniart, chapter 8, section 34).
			xi := gl1E{j: j, k: k, s: -1, a: [3]int{1}}
			galL, galE, gerr := f.galoisInduced(xi)
			pair.Match, pair.Note = f.compare(nil, gl2E, galL, galE, gerr)
			if gerr == nil {
				pair.GaloisL, pair.GaloisEpsilon = f.formatL(galL, namesSC), f.format(galE, namesSC)
			}
			dg, dp := f.detGalois(xi)
			cm := wg == dg && f.mequal(wp, dp)
			pair.CentralMatch = &cm
			if !cm {
				pair.Match = false
				if pair.Note == "" {
					pair.Note = "the central character is not the determinant"
				}
			}
			// The control: the naive correspondence, Ind theta (chi o N) with no rectifier.
			naive := xi
			naive.s = 1
			_, nE, nerr := f.galoisInduced(naive)
			ne := nerr == nil && f.mequal(gl2E, nE)
			ng, np := f.detGalois(naive)
			nc := ng == wg && f.mequal(wp, np)
			pair.NaiveEpsilonMatch, pair.NaiveCentralMatch = &ne, &nc
			if ne {
				count.NaiveEpsilonMatches++
			}
			if nc {
				count.NaiveCentralMatches++
			}
			out = append(out, pair)
			if pair.Match {
				count.Matches++
			}
		}
	}
	return out, count
}

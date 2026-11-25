package lab

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"sort"
	"strings"
)

// E1Inputs are the inputs of experiment E1, the small cases of local reciprocity.
type E1Inputs struct {
	Primes       []int  `yaml:"primes" json:"primes"`
	Box          int    `yaml:"box" json:"box"`                     // the norms a^2 - d b^2 use |a|, |b| <= box
	MaxConductor int    `yaml:"max_conductor" json:"max_conductor"` // the largest p^n of the fields Q_p(zeta_{p^n})
	Samples      int    `yaml:"samples" json:"samples"`             // the random elements of Z[zeta] for each cyclotomic field
	Coefficients int    `yaml:"coefficients" json:"coefficients"`   // their coefficients are in [-coefficients, coefficients]
	Seed         uint64 `yaml:"seed" json:"seed"`
}

func (in E1Inputs) check() error {
	if len(in.Primes) == 0 {
		return errors.New("E1 needs at least one prime")
	}
	for _, p := range in.Primes {
		if p < 2 || p > 97 || !isPrime(p) {
			return fmt.Errorf("E1: %d is not a prime from 2 to 97", p)
		}
	}
	switch {
	case in.Box < 1 || in.Box > 1000:
		return errors.New("E1: box must be from 1 to 1000")
	case in.MaxConductor < 0 || in.MaxConductor > 128:
		return errors.New("E1: max_conductor must be from 0 to 128")
	case in.Samples < 0 || in.Samples > 10000:
		return errors.New("E1: samples must be from 0 to 10000")
	case in.Samples > 0 && (in.Coefficients < 1 || in.Coefficients > 100):
		return errors.New("E1: coefficients must be from 1 to 100")
	}
	return nil
}

// QuadraticRow is the result for one quadratic extension Q_p(sqrt d).
type QuadraticRow struct {
	P         int     `json:"p"`
	D         int     `json:"d"`
	Kind      string  `json:"kind"`      // unramified or ramified
	Character string  `json:"character"` // the character of Q_p^x that Lubin-Tate predicts for the Artin map
	Kernel    []int64 `json:"kernel"`    // the square classes in the predicted kernel, as integers
	Found     []int64 `json:"found"`     // the square classes of the norms that the run found
	Norms     int     `json:"norms"`     // the nonzero norms that the run computed
	Outside   int     `json:"outside"`   // the norms outside the predicted kernel; 0 when the prediction holds
	Example   string  `json:"example,omitempty"`
	Missing   []int64 `json:"missing,omitempty"` // the classes of the kernel with no norm
}

// CyclotomicRow is the result for one field Q_p(zeta_{p^n}).
type CyclotomicRow struct {
	P       int    `json:"p"`
	N       int    `json:"n"`
	Degree  int    `json:"degree"`
	Kernel  string `json:"kernel"` // the norm group that Lubin-Tate predicts
	Norms   int    `json:"norms"`
	Outside int    `json:"outside"` // the norms whose unit part is not 1 mod p^n
	Example string `json:"example,omitempty"`
	// Classes are the classes of the unit parts of the norms in (1 + p^n Z_p) / (1 + p^{n+1} Z_p), as k in 1 + k p^n. Missing are the classes with no norm.
	Classes []int `json:"classes"`
	Missing []int `json:"missing,omitempty"`
	PIsNorm bool  `json:"p_is_norm"` // the norm of 1 - zeta is p
}

// E1Result is the result of a run of E1.
type E1Result struct {
	Quadratic  []QuadraticRow  `json:"quadratic"`
	Cyclotomic []CyclotomicRow `json:"cyclotomic"`
}

// Mismatches counts the norms outside the predicted kernels.
func (r E1Result) Mismatches() int {
	n := 0
	for _, q := range r.Quadratic {
		n += q.Outside
	}
	for _, c := range r.Cyclotomic {
		n += c.Outside
		if !c.PIsNorm {
			n++
		}
	}
	return n
}

// Gaps counts the classes of the predicted kernels that have no norm in the run.
func (r E1Result) Gaps() int {
	n := 0
	for _, q := range r.Quadratic {
		n += len(q.Missing)
	}
	for _, c := range r.Cyclotomic {
		n += len(c.Missing)
	}
	return n
}

// RunE1 runs experiment E1. It uses no network and no tool outside this binary.
//
// Local class field theory says that the Artin map of a finite abelian extension L/Q_p has the norm group N(L^x) as its kernel. Lubin-Tate theory with the uniformizer p gives the Artin map explicitly: u p^v acts as the Frobenius to the power v on the unramified extensions and as zeta -> zeta^(1/u) on the roots of unity of p-power order. The run computes norms and checks that each one is in the kernel that this formula predicts, and that each class of the kernel has a norm.
func RunE1(in E1Inputs) (E1Result, error) {
	if err := in.check(); err != nil {
		return E1Result{}, err
	}
	var r E1Result
	for _, p := range in.Primes {
		for _, d := range quadraticDs(p) {
			r.Quadratic = append(r.Quadratic, quadraticRow(p, d, in.Box))
		}
	}
	rng := rand.New(rand.NewPCG(in.Seed, 0x6c676c)) // #nosec G404 -- a fixed seed, so that a run is reproducible
	for _, p := range in.Primes {
		for n, q := 1, p; q <= in.MaxConductor; n, q = n+1, q*p {
			if q == 2 {
				continue // Q_2(zeta_2) is Q_2
			}
			r.Cyclotomic = append(r.Cyclotomic, cyclotomicRow(p, n, in.Samples, in.Coefficients, rng))
		}
	}
	return r, nil
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	for d := 2; d*d <= n; d++ {
		if n%d == 0 {
			return false
		}
	}
	return true
}

// legendre is the Legendre symbol (u/p) for an odd prime p and u prime to p.
func legendre(u *big.Int, p int) int {
	pp := big.NewInt(int64(p))
	e := big.NewInt(int64((p - 1) / 2))
	r := new(big.Int).Exp(new(big.Int).Mod(u, pp), e, pp)
	if r.Cmp(big.NewInt(1)) == 0 {
		return 1
	}
	return -1
}

// split writes a nonzero integer x as u p^v with u prime to p.
func split(x *big.Int, p int) (*big.Int, int) {
	u := new(big.Int).Set(x)
	pp := big.NewInt(int64(p))
	q, m := new(big.Int), new(big.Int)
	v := 0
	for {
		q.QuoRem(u, pp, m)
		if m.Sign() != 0 {
			return u, v
		}
		u.Set(q)
		v++
	}
}

// nonResidue is the smallest positive integer that is not a square mod the odd prime p.
func nonResidue(p int) int64 {
	for e := int64(2); ; e++ {
		if legendre(big.NewInt(e), p) == -1 {
			return e
		}
	}
}

// squareClass returns the class of a nonzero integer x in Q_p^x / (Q_p^x)^2 as a small integer. For p odd the classes are 1, e, p and e p, with e the smallest non-residue. For p = 2 they are u 2^c with u in {1, 3, 5, 7} and c in {0, 1}.
func squareClass(x *big.Int, p int) int64 {
	u, v := split(x, p)
	c := int64(1)
	if v%2 == 1 {
		c = int64(p)
	}
	if p == 2 {
		return c * new(big.Int).Mod(u, big.NewInt(8)).Int64()
	}
	if legendre(u, p) == -1 {
		return c * nonResidue(p)
	}
	return c
}

// classes returns all the square classes of Q_p^x.
func classes(p int) []int64 {
	if p == 2 {
		return []int64{1, 3, 5, 7, 2, 6, 10, 14}
	}
	e := nonResidue(p)
	return []int64{1, e, int64(p), e * int64(p)}
}

// quadraticDs returns one d for each quadratic extension Q_p(sqrt d). For p odd they are e, p* and e p*, with p* = (-1)^((p-1)/2) p, so that Q_p(sqrt p*) is the quadratic subfield of Q_p(zeta_p).
func quadraticDs(p int) []int {
	if p == 2 {
		return []int{5, -1, -5, 2, -2, 10, -10}
	}
	e := int(nonResidue(p))
	ps := p
	if p%4 == 3 {
		ps = -p
	}
	return []int{e, ps, e * ps}
}

// character is the quadratic character of Q_p^x that Lubin-Tate predicts for Q_p(sqrt d): the Artin map of x = u p^v, restricted to Q_p(sqrt d). It returns the character as a function of the square class, and a text.
func character(p, d int) (func(x *big.Int) int, string) {
	ud, vd := split(big.NewInt(int64(d)), p)
	c := vd % 2 // the part of the cyclotomic character of p-power roots of unity
	var a, b int
	if p == 2 {
		switch new(big.Int).Mod(ud, big.NewInt(8)).Int64() {
		case 5:
			a = 1
		case 7:
			b = 1
		case 3:
			a, b = 1, 1
		}
		var parts []string
		if a == 1 {
			parts = append(parts, "(-1)^v from Q_2(sqrt 5) = Q_2(zeta_3)")
		}
		if b == 1 {
			parts = append(parts, "the sign of u mod 4 from Q_2(zeta_4)")
		}
		if c == 1 {
			parts = append(parts, "the sign of u mod 8 up to -1 from Q_2(zeta_8)")
		}
		return func(x *big.Int) int {
			u, v := split(x, 2)
			m := new(big.Int).Mod(u, big.NewInt(8)).Int64()
			s := 1
			if a == 1 && v%2 == 1 {
				s = -s
			}
			if b == 1 && m%4 != 1 {
				s = -s
			}
			if c == 1 && m != 1 && m != 7 {
				s = -s
			}
			return s
		}, strings.Join(parts, " times ")
	}
	// For p odd, d = e^a (p*)^c up to squares.
	w := new(big.Int).Set(ud)
	if c == 1 && p%4 == 3 {
		w.Neg(w)
	}
	if legendre(w, p) == -1 {
		a = 1
	}
	var parts []string
	if a == 1 {
		parts = append(parts, "(-1)^v from the unramified extension")
	}
	if c == 1 {
		parts = append(parts, fmt.Sprintf("the Legendre symbol (u/%d) from Q_%d(zeta_%d)", p, p, p))
	}
	return func(x *big.Int) int {
		u, v := split(x, p)
		s := 1
		if a == 1 && v%2 == 1 {
			s = -s
		}
		if c == 1 {
			s *= legendre(u, p)
		}
		return s
	}, strings.Join(parts, " times ")
}

func quadraticRow(p, d, box int) QuadraticRow {
	chi, text := character(p, d)
	row := QuadraticRow{P: p, D: d, Kind: "ramified", Character: "x = u p^v goes to " + text}
	if _, v := split(big.NewInt(int64(d)), p); v%2 == 0 && (p != 2 || d == 5) {
		row.Kind = "unramified"
	}
	kernel := map[int64]bool{}
	for _, c := range classes(p) {
		if chi(big.NewInt(c)) == 1 {
			kernel[c] = true
			row.Kernel = append(row.Kernel, c)
		}
	}
	found := map[int64]bool{}
	x := new(big.Int)
	for a := -box; a <= box; a++ {
		for b := 0; b <= box; b++ {
			n := int64(a)*int64(a) - int64(d)*int64(b)*int64(b)
			if n == 0 {
				continue
			}
			x.SetInt64(n)
			row.Norms++
			c := squareClass(x, p)
			found[c] = true
			if !kernel[c] {
				row.Outside++
				if row.Example == "" {
					row.Example = fmt.Sprintf("N(%d + %d sqrt %d) = %d is outside the predicted kernel", a, b, d, n)
				}
			}
		}
	}
	row.Found = sortedKeys(found)
	for _, c := range row.Kernel {
		if !found[c] {
			row.Missing = append(row.Missing, c)
		}
	}
	return row
}

func sortedKeys(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func cyclotomicRow(p, n, samples, coef int, rng *rand.Rand) CyclotomicRow {
	q := 1
	for range n {
		q *= p
	}
	step := q / p
	m := (p - 1) * step
	row := CyclotomicRow{P: p, N: n, Degree: m, Kernel: fmt.Sprintf("<%d> x (1 + %d Z_%d)", p, q, p)}
	pn := big.NewInt(int64(q))
	pn1 := new(big.Int).Mul(pn, big.NewInt(int64(p)))
	seen := map[int]bool{}
	elements := make([][]int64, 0, samples+1)
	// The first element is 1 - zeta, whose norm is Phi(1) = p.
	first := make([]int64, m)
	first[0], first[1] = 1, -1
	elements = append(elements, first)
	for range samples {
		a := make([]int64, m)
		for i := range a {
			a[i] = int64(rng.IntN(2*coef+1) - coef)
		}
		elements = append(elements, a)
	}
	for i, a := range elements {
		nm := norm(a, p, step)
		if nm.Sign() == 0 {
			continue
		}
		if i == 0 {
			row.PIsNorm = nm.Cmp(big.NewInt(int64(p))) == 0
		}
		row.Norms++
		u, _ := split(nm, p)
		r := new(big.Int).Mod(u, pn1)
		if new(big.Int).Mod(r, pn).Cmp(big.NewInt(1)) != 0 {
			row.Outside++
			if row.Example == "" {
				row.Example = fmt.Sprintf("the norm %s of element %d has the unit part %s mod %d", nm, i, new(big.Int).Mod(u, pn), q)
			}
			continue
		}
		k := new(big.Int).Div(new(big.Int).Sub(r, big.NewInt(1)), pn).Int64()
		seen[int(k)] = true
	}
	for k := range p {
		if seen[k] {
			row.Classes = append(row.Classes, k)
		} else {
			row.Missing = append(row.Missing, k)
		}
	}
	return row
}

// norm returns the norm from Q(zeta_{p^n}) to Q of sum a_i zeta^i, the determinant of the multiplication by the element on the basis 1, zeta, ..., zeta^(m-1). The cyclotomic polynomial is irreducible over Q_p, so this is also the norm from Q_p(zeta_{p^n}) to Q_p. The argument step is p^(n-1).
func norm(a []int64, p, step int) *big.Int {
	m := len(a)
	cols := make([][]int64, m)
	cur := append([]int64(nil), a...)
	for j := range m {
		cols[j] = append([]int64(nil), cur...)
		// Multiply by zeta: zeta^m = -(1 + zeta^step + ... + zeta^((p-2) step)).
		top := cur[m-1]
		copy(cur[1:], cur[:m-1])
		cur[0] = 0
		for k := 0; k <= p-2; k++ {
			cur[k*step] -= top
		}
	}
	mat := make([][]*big.Int, m)
	for i := range m {
		mat[i] = make([]*big.Int, m)
		for j := range m {
			mat[i][j] = big.NewInt(cols[j][i])
		}
	}
	return det(mat)
}

// det is the determinant of an integer matrix by the fraction-free elimination of Bareiss. It changes the matrix.
func det(a [][]*big.Int) *big.Int {
	n := len(a)
	sign := 1
	prev := big.NewInt(1)
	t := new(big.Int)
	for k := 0; k < n-1; k++ {
		if a[k][k].Sign() == 0 {
			r := k + 1
			for r < n && a[r][k].Sign() == 0 {
				r++
			}
			if r == n {
				return new(big.Int)
			}
			a[k], a[r] = a[r], a[k]
			sign = -sign
		}
		for i := k + 1; i < n; i++ {
			for j := k + 1; j < n; j++ {
				// a[i][j] = (a[i][j] a[k][k] - a[i][k] a[k][j]) / prev, an exact division.
				v := new(big.Int).Mul(a[i][j], a[k][k])
				v.Sub(v, t.Mul(a[i][k], a[k][j]))
				a[i][j] = v.Quo(v, prev)
			}
		}
		prev = a[k][k]
	}
	d := new(big.Int).Set(a[n-1][n-1])
	if sign < 0 {
		d.Neg(d)
	}
	return d
}

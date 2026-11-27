package lab

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// E5Inputs are the inputs of experiment E5, the modularity of the elliptic curves of small conductor.
//
// The run reads files of the Cremona database (the repository ecdata) from a directory that the caller names. Each file has its sha256 in the spec, and the run stops when a file does not have it, so the run is reproducible from the spec and the files.
type E5Inputs struct {
	MaxConductor    int     `yaml:"max_conductor" json:"max_conductor"`         // the classes of conductor at most this
	LivnePrimeBound int     `yaml:"livne_prime_bound" json:"livne_prime_bound"` // the search for the primes of the Livne set stops here
	Source          string  `yaml:"source" json:"source"`                       // where the data comes from
	Commit          string  `yaml:"commit" json:"commit"`                       // the commit of the repository that the files come from
	Licence         string  `yaml:"licence" json:"licence"`                     // the licence of the data
	Files           E5Files `yaml:"files" json:"files"`
}

// E5Files are the files of the Cremona database that the run reads.
type E5Files struct {
	Curves  E5File `yaml:"curves" json:"curves"`   // allcurves: N class number [a1,a2,a3,a4,a6] r t
	Aplist  E5File `yaml:"aplist" json:"aplist"`   // aplist: N class, a_p of the newform for p < 100, and W_q for the q > 100 of N
	Labels  E5File `yaml:"labels" json:"labels"`   // alllabels: the Cremona labels and the LMFDB labels
	Readme  E5File `yaml:"readme" json:"readme"`   // the README of the repository, for the record
	Format  E5File `yaml:"format" json:"format"`   // docs/file-format.txt, which defines the fields, for the record
	Licence E5File `yaml:"licence" json:"licence"` // the LICENSE of the repository, for the record
}

// E5File is one downloaded file.
type E5File struct {
	Path   string `yaml:"path" json:"path"`     // the path in the repository
	Name   string `yaml:"name" json:"name"`     // the name in the data directory
	Sha256 string `yaml:"sha256" json:"sha256"` // the sha256 of the bytes of the file
}

func (f E5Files) all() []E5File {
	return []E5File{f.Curves, f.Aplist, f.Labels, f.Readme, f.Format, f.Licence}
}

var (
	e5FileRE   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.txt$`)
	e5CommitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func (in E5Inputs) check() error {
	switch {
	case in.MaxConductor < 1 || in.MaxConductor > 9999:
		return errors.New("E5: max_conductor must be from 1 to 9999")
	case in.LivnePrimeBound < 3 || in.LivnePrimeBound > 1000000:
		return errors.New("E5: livne_prime_bound must be from 3 to 1000000")
	case strings.TrimSpace(in.Source) == "":
		return errors.New("E5: the spec needs the source of the data")
	case strings.TrimSpace(in.Licence) == "":
		return errors.New("E5: the spec needs the licence of the data")
	case !e5CommitRE.MatchString(in.Commit):
		return errors.New("E5: commit must be 40 hex digits")
	}
	seen := map[string]bool{}
	for _, f := range in.Files.all() {
		if !e5FileRE.MatchString(f.Name) {
			return fmt.Errorf("E5: the data file %q must be a plain name that ends in .txt", f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("E5: the data file %s is in the spec twice", f.Name)
		}
		seen[f.Name] = true
		if strings.TrimSpace(f.Path) == "" {
			return fmt.Errorf("E5: the data file %s needs its path in the repository", f.Name)
		}
		if b, err := hex.DecodeString(f.Sha256); err != nil || len(b) != sha256.Size {
			return fmt.Errorf("E5: the sha256 of %s is not 64 hex digits", f.Name)
		}
	}
	return nil
}

// E5Bad is a prime p of bad reduction of a class.
type E5Bad struct {
	P          int    `json:"p"`
	Exponent   int    `json:"exponent"`  // the exponent of p in N
	Reduction  string `json:"reduction"` // split, nonsplit or additive, from the point count of the minimal model
	CurveAP    int64  `json:"curve_ap"`
	NewformW   string `json:"newform_w,omitempty"`  // + or -, the eigenvalue of W_p in the aplist
	NewformAP  *int64 `json:"newform_ap,omitempty"` // -w_p when p || N, and 0 when p^2 | N
	Minimal    bool   `json:"minimal"`              // v_p(Delta) < 12, v_p(c4) < 4 or v_p(c6) < 6, so the model is minimal at p
	TypeAgrees bool   `json:"type_agrees"`          // a_p = 0 exactly when p^2 divides N
}

// E5Livne is the criterion of Livne for one class. The set T depends only on S, and livne.json gives it for each S.
type E5Livne struct {
	Status     string   `json:"status"`             // computed, not found or not applicable
	S          []int    `json:"S,omitempty"`        // 2 and the primes of N
	Characters []int    `json:"-"`                  // d of the quadratic characters (d/.) that give the coordinates of Frobenius
	Monomials  int      `json:"-"`                  // the dimension of the space of cubic forms on F_2^t
	Primes     []int    `json:"-"`                  // the set T, non-cubic in F_2^t
	Vectors    []string `json:"-"`                  // the Frobenius of each prime of T in F_2^t
	CurveAP    []int64  `json:"curve_ap,omitempty"` // a_p of the curve at the primes of T
	Checked    int      `json:"checked"`            // the primes of T with a_p in the aplist
	// EvenPrimes are the primes p outside S in the aplist, and NewformEven says that a_p of the newform is even for each of them. This is evidence for the hypothesis on the traces of rho_f mod 2, not a proof.
	EvenPrimes  int    `json:"newform_even_primes,omitempty"`
	NewformEven bool   `json:"newform_even"`
	Agrees      bool   `json:"agrees"` // a_p of the curve = a_p of the newform on all of T
	Note        string `json:"note,omitempty"`
}

// E5LivneSet is the set T of Livne for one S, in livne.json.
type E5LivneSet struct {
	S          []int    `json:"S"`
	Characters []int    `json:"characters"` // d of the quadratic characters (d/.) that give the coordinates of Frobenius
	Monomials  int      `json:"monomials"`  // the dimension of the space of cubic forms on F_2^t
	Primes     []int    `json:"T"`          // non-cubic in F_2^t
	Vectors    []string `json:"vectors"`    // the Frobenius of each prime of T in F_2^t
	Classes    []string `json:"classes"`    // the classes with a rational point of order 2 and this S
}

// LivneSets returns the set T for each S of the classes with a rational point of order 2.
func (r E5Result) LivneSets() []E5LivneSet {
	var out []E5LivneSet
	at := map[string]int{}
	for _, c := range r.Classes {
		if c.Livne.Status != "computed" {
			continue
		}
		k := fmt.Sprint(c.Livne.S)
		i, ok := at[k]
		if !ok {
			i = len(out)
			at[k] = i
			l := c.Livne
			out = append(out, E5LivneSet{S: l.S, Characters: l.Characters, Monomials: l.Monomials, Primes: l.Primes, Vectors: l.Vectors})
		}
		out[i].Classes = append(out[i].Classes, c.Class)
	}
	return out
}

// E5Class is the result for one isogeny class.
type E5Class struct {
	Class         string   `json:"class"` // the Cremona label of the class, Nx
	LMFDB         string   `json:"lmfdb"` // the LMFDB label of the class, N.x, from alllabels
	Curve         string   `json:"curve"` // the curve of the class that the run uses: number 1 in the Cremona numbering
	Ainvs         []string `json:"ainvs"`
	Conductor     int      `json:"conductor"`
	Newform       bool     `json:"newform"`     // the aplist has the class
	Sturm         int      `json:"sturm_bound"` // floor(N prod (1 + 1/p) / 6)
	Primes        int      `json:"primes_checked"`
	LargestPrime  int      `json:"largest_prime_checked"`
	SturmComplete bool     `json:"sturm_complete"`    // the aplist has a_p for each prime p <= the Sturm bound
	Missing       []int    `json:"missing,omitempty"` // the primes p <= the Sturm bound that are not in the aplist
	BadPrimes     bool     `json:"bad_primes_agree"`  // the primes of Delta are the primes of N
	Bad           []E5Bad  `json:"bad"`
	TwoTorsion    bool     `json:"two_torsion"` // the curve has a rational point of order 2
	Livne         E5Livne  `json:"livne"`
	Mismatches    []string `json:"mismatches,omitempty"`
	Status        string   `json:"status"`         // agrees, mismatch, or no newform
	Candidate     bool     `json:"candidate"`      // agrees, and the check is complete to the Sturm bound
	LivneComplete bool     `json:"livne_complete"` // agrees on all of the Livne set, and a_p of the newform is even in the aplist
}

// E5Newform is a class of the aplist with no curve in the data.
type E5Newform struct {
	Label string `json:"label"`
	Level int    `json:"level"`
	Note  string `json:"note"`
}

// E5Count sums the classes of a range of conductors.
type E5Count struct {
	From          int `json:"from"`
	To            int `json:"to"`
	Classes       int `json:"classes"`
	Newforms      int `json:"newforms"` // the classes of these levels in the aplist
	Paired        int `json:"paired"`
	Primes        int `json:"primes_checked"`
	Mismatches    int `json:"mismatches"`
	SturmComplete int `json:"sturm_complete"`
	TwoTorsion    int `json:"two_torsion"`
	LivneComplete int `json:"livne_complete"` // the classes whose Livne set is computed and checked on all of its primes
	Candidates    int `json:"candidates"`
}

// E5Result is the result of a run of E5.
type E5Result struct {
	Classes  []E5Class         `json:"classes"`
	Unpaired []E5Newform       `json:"unpaired_newforms"`
	Counts   []E5Count         `json:"counts"`
	Data     map[string]string `json:"-"` // the sha256 of each data file that the run read
	MaxLevel int               `json:"-"` // the largest conductor in the curve file, also above max_conductor
}

// Mismatches counts the classes whose a_p do not agree with the newform.
func (r E5Result) Mismatches() int {
	n := 0
	for _, c := range r.Classes {
		if c.Status == "mismatch" {
			n++
		}
	}
	return n
}

// Gaps counts the classes with no newform and the newforms with no class.
func (r E5Result) Gaps() int {
	n := len(r.Unpaired)
	for _, c := range r.Classes {
		if c.Status == "no newform" {
			n++
		}
	}
	return n
}

// Total sums the counts.
func (r E5Result) Total() E5Count {
	var t E5Count
	for _, c := range r.Counts {
		t.Classes += c.Classes
		t.Newforms += c.Newforms
		t.Paired += c.Paired
		t.Primes += c.Primes
		t.Mismatches += c.Mismatches
		t.SturmComplete += c.SturmComplete
		t.TwoTorsion += c.TwoTorsion
		t.LivneComplete += c.LivneComplete
		t.Candidates += c.Candidates
	}
	return t
}

// e5Curve is curve number 1 of a class of allcurves.
type e5Curve struct {
	class     string // Nx
	conductor int
	a         [5]*big.Int // a1, a2, a3, a4, a6
}

// e5Ap is one entry of the aplist: a_p for p not dividing N, or the sign of W_p for p dividing N.
type e5Ap struct {
	p    int
	ap   int64
	sign int // +1 or -1 for p | N, 0 for p not dividing N
}

// e5Form is a class of the aplist.
type e5Form struct {
	class string
	level int
	aps   []e5Ap // the 25 primes below 100, then the primes above 100 of N
}

func (f *e5Form) lookup(p int) (e5Ap, bool) {
	for _, x := range f.aps {
		if x.p == p {
			return x, true
		}
	}
	return e5Ap{}, false
}

const (
	e5MaxFile = 64 << 20
	e5MaxLine = 4096
)

// readE5File reads a data file and checks its sha256. The data are not trusted: the run checks the sha256 first, and then the form of each field.
func readE5File(dir string, f E5File) ([]byte, error) {
	h, err := os.Open(filepath.Join(dir, f.Name)) // #nosec G304 -- a name of the spec, checked by e5FileRE
	if err != nil {
		return nil, err
	}
	defer h.Close()
	data, err := io.ReadAll(io.LimitReader(h, e5MaxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > e5MaxFile {
		return nil, fmt.Errorf("E5: %s is larger than %d bytes", f.Name, e5MaxFile)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != f.Sha256 {
		return nil, fmt.Errorf("E5: %s has the sha256 %s, but the spec gives %s", f.Name, got, f.Sha256)
	}
	return data, nil
}

// e5Lines calls fn with the fields of each line whose conductor is at most maxN. The files are sorted by conductor, and the run checks this.
func e5Lines(name string, data []byte, maxN int, fn func(n int, fields []string) error) error {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, e5MaxLine), e5MaxLine)
	line, last := 0, 0
	for sc.Scan() {
		line++
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		n, err := e5Conductor(fields[0])
		if err != nil {
			return fmt.Errorf("E5: %s, line %d: %w", name, line, err)
		}
		if n < last {
			return fmt.Errorf("E5: %s, line %d: the conductor %d comes after %d", name, line, n, last)
		}
		last = n
		if n > maxN {
			continue
		}
		if err := fn(n, fields); err != nil {
			return fmt.Errorf("E5: %s, line %d: %w", name, line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("E5: %s, line %d: %w", name, line+1, err)
	}
	return nil
}

var (
	lettersRE = regexp.MustCompile(`^[a-z]{1,4}$`)
	ainvsRE   = regexp.MustCompile(`^\[(-?[0-9]{1,80}),(-?[0-9]{1,80}),(-?[0-9]{1,80}),(-?[0-9]{1,80}),(-?[0-9]{1,80})\]$`)
	natRE     = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
	apRE      = regexp.MustCompile(`^(0|-?[1-9][0-9]{0,3})$`)
	bqRE      = regexp.MustCompile(`^([+-])\(([1-9][0-9]{0,8})\)$`)
)

func e5Conductor(s string) (int, error) {
	if !natRE.MatchString(s) {
		return 0, fmt.Errorf("%q is not a conductor", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// parseE5Curves returns curve number 1 of each class of allcurves.
func parseE5Curves(name string, data []byte, maxN int) (curves []*e5Curve, maxLevel int, err error) {
	seen := map[string]bool{}
	err = e5Lines(name, data, 1<<30, func(n int, f []string) error {
		maxLevel = n
		if n > maxN {
			return nil
		}
		if len(f) != 6 || !lettersRE.MatchString(f[1]) || !natRE.MatchString(f[2]) || !ainvsRE.MatchString(f[3]) {
			return errors.New("the line is not N class number [a1,a2,a3,a4,a6] r t")
		}
		if f[2] != "1" {
			return nil
		}
		c := &e5Curve{class: f[0] + f[1], conductor: n}
		if seen[c.class] {
			return fmt.Errorf("the curve %s1 is in the file twice", c.class)
		}
		seen[c.class] = true
		for i, s := range ainvsRE.FindStringSubmatch(f[3])[1:] {
			c.a[i], _ = new(big.Int).SetString(s, 10)
		}
		curves = append(curves, c)
		return nil
	})
	return curves, maxLevel, err
}

// parseE5Aplist returns the classes of the aplist.
func parseE5Aplist(name string, data []byte, maxN int) ([]*e5Form, error) {
	var out []*e5Form
	seen := map[string]bool{}
	small := primesUpTo(99)
	err := e5Lines(name, data, maxN, func(n int, f []string) error {
		if len(f) < 2+len(small) || !lettersRE.MatchString(f[1]) {
			return fmt.Errorf("the line is not N class and %d entries", len(small))
		}
		form := &e5Form{class: f[0] + f[1], level: n}
		if seen[form.class] {
			return fmt.Errorf("the class %s is in the file twice", form.class)
		}
		seen[form.class] = true
		for i, p := range small {
			s := f[2+i]
			x := e5Ap{p: p}
			switch {
			case n%p == 0 && s == "+":
				x.sign = 1
			case n%p == 0 && s == "-":
				x.sign = -1
			case n%p != 0 && apRE.MatchString(s):
				x.ap, _ = strconv.ParseInt(s, 10, 64)
			default:
				return fmt.Errorf("%s: the entry %q for p = %d does not fit N = %d", form.class, s, p, n)
			}
			form.aps = append(form.aps, x)
		}
		var large []int
		ps, _ := factor(n)
		for _, p := range ps {
			if p > 100 {
				large = append(large, p)
			}
		}
		rest := f[2+len(small):]
		if len(rest) != len(large) {
			return fmt.Errorf("%s: the line has %d entries for primes q > 100, but N has %d such primes", form.class, len(rest), len(large))
		}
		for i, s := range rest {
			m := bqRE.FindStringSubmatch(s)
			if m == nil || m[2] != strconv.Itoa(large[i]) {
				return fmt.Errorf("%s: the entry %q is not +(q) or -(q) with q = %d", form.class, s, large[i])
			}
			x := e5Ap{p: large[i], sign: 1}
			if m[1] == "-" {
				x.sign = -1
			}
			form.aps = append(form.aps, x)
		}
		out = append(out, form)
		return nil
	})
	return out, err
}

// parseE5Labels returns the LMFDB label N.x of each Cremona class Nx.
func parseE5Labels(name string, data []byte, maxN int) (map[string]string, error) {
	out := map[string]string{}
	err := e5Lines(name, data, maxN, func(n int, f []string) error {
		if len(f) != 6 || !lettersRE.MatchString(f[1]) || !natRE.MatchString(f[2]) || f[3] != f[0] || !lettersRE.MatchString(f[4]) || !natRE.MatchString(f[5]) {
			return errors.New("the line is not N class number N class number")
		}
		c, l := f[0]+f[1], f[0]+"."+f[4]
		if old, ok := out[c]; ok && old != l {
			return fmt.Errorf("the class %s has the LMFDB labels %s and %s", c, old, l)
		}
		out[c] = l
		return nil
	})
	return out, err
}

// factor returns the primes of n and their exponents.
func factor(n int) (ps, es []int) {
	for p := 2; p*p <= n; p++ {
		if n%p == 0 {
			e := 0
			for n%p == 0 {
				n /= p
				e++
			}
			ps, es = append(ps, p), append(es, e)
		}
	}
	if n > 1 {
		ps, es = append(ps, n), append(es, 1)
	}
	return ps, es
}

// SturmBound is floor(k/12 [SL2(Z) : Gamma0(N)]) for k = 2, that is floor(N prod_{p | N} (1 + 1/p) / 6).
func SturmBound(n int) int {
	psi := n
	ps, _ := factor(n)
	for _, p := range ps {
		psi = psi / p * (p + 1)
	}
	return psi / 6
}

func primesUpTo(n int) []int {
	if n < 2 {
		return nil
	}
	comp := make([]bool, n+1)
	var out []int
	for i := 2; i <= n; i++ {
		if !comp[i] {
			out = append(out, i)
			for j := i * i; j <= n; j += i {
				comp[j] = true
			}
		}
	}
	return out
}

// weierstrass holds the invariants of a Weierstrass equation.
type weierstrass struct {
	b2, b4, b6, b8 *big.Int
	c4, c6, disc   *big.Int
}

func newWeierstrass(a [5]*big.Int) *weierstrass {
	a1, a2, a3, a4, a6 := a[0], a[1], a[2], a[3], a[4]
	mul := func(x ...*big.Int) *big.Int {
		z := big.NewInt(1)
		for _, y := range x {
			z.Mul(z, y)
		}
		return z
	}
	k := func(c int64) *big.Int { return big.NewInt(c) }
	add := func(x ...*big.Int) *big.Int {
		z := new(big.Int)
		for _, y := range x {
			z.Add(z, y)
		}
		return z
	}
	neg := func(x *big.Int) *big.Int { return new(big.Int).Neg(x) }
	b2 := add(mul(a1, a1), mul(k(4), a2))
	b4 := add(mul(k(2), a4), mul(a1, a3))
	b6 := add(mul(a3, a3), mul(k(4), a6))
	b8 := add(mul(a1, a1, a6), mul(k(4), a2, a6), neg(mul(a1, a3, a4)), mul(a2, a3, a3), neg(mul(a4, a4)))
	c4 := add(mul(b2, b2), mul(k(-24), b4))
	c6 := add(neg(mul(b2, b2, b2)), mul(k(36), b2, b4), mul(k(-216), b6))
	disc := add(neg(mul(b2, b2, b8)), mul(k(-8), b4, b4, b4), mul(k(-27), b6, b6), mul(k(9), b2, b4, b6))
	return &weierstrass{b2: b2, b4: b4, b6: b6, b8: b8, c4: c4, c6: c6, disc: disc}
}

// valuation is v_p(x), and -1 for x = 0.
func valuation(x *big.Int, p int) int {
	if x.Sign() == 0 {
		return -1
	}
	z := new(big.Int).Abs(x)
	bp := big.NewInt(int64(p))
	r := new(big.Int)
	v := 0
	for {
		q, m := new(big.Int).QuoRem(z, bp, r)
		if m.Sign() != 0 {
			return v
		}
		z = q
		v++
	}
}

// affinePoints counts the solutions of y^2 + a1 x y + a3 y = x^3 + a2 x^2 + a4 x + a6 in F_p^2. Then a_p = p - the count for each p, of good or bad reduction, when the model is minimal at p.
func affinePoints(a [5]*big.Int, p int) int {
	bp := big.NewInt(int64(p))
	var r [5]int
	for i := range a {
		r[i] = int(new(big.Int).Mod(a[i], bp).Int64())
	}
	a1, a2, a3, a4, a6 := r[0], r[1], r[2], r[3], r[4]
	if p == 2 {
		n := 0
		for x := range 2 {
			for y := range 2 {
				if (y*y+a1*x*y+a3*y-x*x*x-a2*x*x-a4*x-a6)%2 == 0 {
					n++
				}
			}
		}
		return n
	}
	// (2y + a1 x + a3)^2 = 4(x^3 + a2 x^2 + a4 x + a6) + (a1 x + a3)^2, and y -> 2y + a1 x + a3 is a bijection of F_p.
	roots := make([]int, p) // the number of square roots of each residue
	for y := range p {
		roots[y*y%p]++
	}
	n := 0
	for x := range p {
		u := (a1*x + a3) % p
		rhs := (4*((((x+a2)*x+a4)%p*x+a6)%p) + u*u) % p
		n += roots[rhs]
	}
	return n
}

// curveAP is a_p of the curve, p minus the number of affine points of the model.
func curveAP(a [5]*big.Int, p int) int64 {
	return int64(p - affinePoints(a, p))
}

// powMod is b^e mod m.
func powMod(b, e, m int) int {
	r, x := 1, mod(b, m)
	for e > 0 {
		if e&1 == 1 {
			r = r * x % m
		}
		x = x * x % m
		e >>= 1
	}
	return r
}

// kronecker is the Legendre symbol (d/p) for an odd prime p that does not divide d.
func kronecker(d, p int) int {
	if powMod(d, (p-1)/2, p) == 1 {
		return 1
	}
	return -1
}

// evalCubic is X^3 + b X^2 + c X + d at x.
func evalCubic(b, c, d, x *big.Int) *big.Int {
	z := new(big.Int).Add(x, b)
	z.Mul(z, x)
	z.Add(z, c)
	z.Mul(z, x)
	return z.Add(z, d)
}

// hasIntegerRoot says whether X^3 + b X^2 + c X + d has an integer root. The search is exact: it splits the line at the critical points, where the derivative 3X^2 + 2bX + c is 0, and bisects each monotone piece.
func hasIntegerRoot(b, c, d *big.Int) bool {
	if d.Sign() == 0 {
		return true
	}
	// Cauchy: each root has |X| <= 1 + max(|b|, |c|, |d|).
	m := new(big.Int).Abs(b)
	for _, x := range []*big.Int{c, d} {
		if ax := new(big.Int).Abs(x); ax.Cmp(m) > 0 {
			m = ax
		}
	}
	m.Add(m, big.NewInt(1))
	cuts := []*big.Int{new(big.Int).Neg(m), m}
	// The critical points are (-b +- sqrt(b^2 - 3c)) / 3.
	disc := new(big.Int).Mul(b, b)
	disc.Sub(disc, new(big.Int).Mul(big.NewInt(3), c))
	if disc.Sign() >= 0 {
		s := new(big.Int).Sqrt(disc)
		for _, sg := range []int64{-1, 1} {
			x := new(big.Int).Mul(big.NewInt(sg), s)
			x.Sub(x, b)
			x.Quo(x, big.NewInt(3))
			for k := int64(-2); k <= 2; k++ {
				y := new(big.Int).Add(x, big.NewInt(k))
				if y.CmpAbs(m) <= 0 {
					cuts = append(cuts, y)
				}
			}
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].Cmp(cuts[j]) < 0 })
	for _, x := range cuts {
		if evalCubic(b, c, d, x).Sign() == 0 {
			return true
		}
	}
	// Between two consecutive cuts the polynomial is monotone: the cuts hold an integer on each side of each critical point.
	for i := 0; i+1 < len(cuts); i++ {
		lo, hi := new(big.Int).Set(cuts[i]), new(big.Int).Set(cuts[i+1])
		flo, fhi := evalCubic(b, c, d, lo).Sign(), evalCubic(b, c, d, hi).Sign()
		if flo == fhi || flo == 0 || fhi == 0 {
			continue
		}
		one := big.NewInt(1)
		for new(big.Int).Sub(hi, lo).Cmp(one) > 0 {
			mid := new(big.Int).Add(lo, hi)
			mid.Rsh(mid, 1)
			fm := evalCubic(b, c, d, mid).Sign()
			if fm == 0 {
				return true
			}
			if fm == flo {
				lo = mid
			} else {
				hi = mid
			}
		}
	}
	return false
}

// twoTorsion says whether the curve has a rational point of order 2: X^3 + b2 X^2 + 8 b4 X + 16 b6, with X = 4x, has an integer root.
func (w *weierstrass) twoTorsion() bool {
	c := new(big.Int).Mul(big.NewInt(8), w.b4)
	d := new(big.Int).Mul(big.NewInt(16), w.b6)
	return hasIntegerRoot(w.b2, c, d)
}

// livneSet finds the set T of Livne for S = {2} and the primes of N: the coordinates of Frobenius at p are the values of the quadratic characters (d/p) for d = -1, 2 and the odd primes of N, as a vector of F_2^t. T is non-cubic when the cubic forms on F_2^t that vanish on T vanish on F_2^t. Over F_2 the cubic forms are the sums of the monomials x_I with 1 <= |I| <= 3, so T is non-cubic when the matrix of these monomials at the points of T has full rank. The run takes the primes in order and keeps each one that raises the rank; this gives the T whose largest prime is the least possible.
func livneSet(n, bound int) (s, chars []int, monomials int, primes []int, vectors []string, ok bool) {
	ps, _ := factor(n)
	s = []int{2}
	chars = []int{-1, 2}
	for _, p := range ps {
		if p != 2 {
			s = append(s, p)
			chars = append(chars, p)
		}
	}
	t := len(chars)
	var monos []uint
	for i := range t {
		monos = append(monos, 1<<i)
		for j := i + 1; j < t; j++ {
			monos = append(monos, 1<<i|1<<j)
			for k := j + 1; k < t; k++ {
				monos = append(monos, 1<<i|1<<j|1<<k)
			}
		}
	}
	monomials = len(monos)
	basis := make([]uint64, monomials) // basis[i] has its leading bit at i, or is 0
	rank := 0
	inS := func(p int) bool {
		for _, q := range s {
			if p == q {
				return true
			}
		}
		return false
	}
	for _, p := range primesUpTo(bound) {
		if inS(p) {
			continue
		}
		var v uint
		for i, d := range chars {
			if kronecker(d, p) == -1 {
				v |= 1 << i
			}
		}
		var row uint64
		for j, m := range monos {
			if v&m == m {
				row |= 1 << j
			}
		}
		for row != 0 {
			lead := bits.Len64(row) - 1
			if basis[lead] == 0 {
				basis[lead] = row
				rank++
				primes = append(primes, p)
				var sb strings.Builder
				for i := range t {
					sb.WriteByte('0' + byte(v>>i&1))
				}
				vectors = append(vectors, sb.String())
				break
			}
			row ^= basis[lead]
		}
		if rank == monomials {
			return s, chars, monomials, primes, vectors, true
		}
	}
	return s, chars, monomials, primes, vectors, false
}

// RunE5 compares the curves of allcurves with the newforms of the aplist in dir.
func RunE5(in E5Inputs, dir string) (res E5Result, err error) {
	if err := in.check(); err != nil {
		return res, err
	}
	if dir == "" {
		return res, errors.New("E5: the directory of the data is not set; use -data")
	}
	res.Data = map[string]string{}
	data := map[string][]byte{}
	for _, f := range in.Files.all() {
		b, err := readE5File(dir, f)
		if err != nil {
			return res, err
		}
		data[f.Name] = b
		res.Data[f.Name] = f.Sha256
	}
	fs := in.Files
	curves, maxLevel, err := parseE5Curves(fs.Curves.Name, data[fs.Curves.Name], in.MaxConductor)
	if err != nil {
		return res, err
	}
	res.MaxLevel = maxLevel
	forms, err := parseE5Aplist(fs.Aplist.Name, data[fs.Aplist.Name], in.MaxConductor)
	if err != nil {
		return res, err
	}
	labels, err := parseE5Labels(fs.Labels.Name, data[fs.Labels.Name], in.MaxConductor)
	if err != nil {
		return res, err
	}
	byClass := map[string]*e5Form{}
	for _, f := range forms {
		byClass[f.class] = f
	}
	sort.SliceStable(curves, func(i, j int) bool {
		if curves[i].conductor != curves[j].conductor {
			return curves[i].conductor < curves[j].conductor
		}
		return labelLess(curves[i].class, curves[j].class)
	})
	used := map[string]bool{}
	for _, c := range curves {
		f := byClass[c.class]
		k := e5Class(c, f, in.LivnePrimeBound)
		k.LMFDB = labels[c.class]
		if k.LMFDB == "" {
			k.Mismatches = append(k.Mismatches, "the class has no LMFDB label in alllabels")
			if k.Status == "agrees" {
				k.Status = "mismatch"
			}
			k.Candidate, k.LivneComplete = false, false
		}
		res.Classes = append(res.Classes, k)
		used[c.class] = true
	}
	for _, f := range forms {
		if !used[f.class] {
			res.Unpaired = append(res.Unpaired, E5Newform{Label: f.class, Level: f.level, Note: "the aplist has the class, but allcurves has no curve number 1 of it"})
		}
	}
	res.Counts = e5Counts(res, forms, in.MaxConductor)
	return res, nil
}

// labelLess orders the letters of the labels: a, ..., z, ba, bb, ...
func labelLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func e5Class(c *e5Curve, f *e5Form, bound int) E5Class {
	w := newWeierstrass(c.a)
	out := E5Class{Class: c.class, Curve: c.class + "1", Conductor: c.conductor, Sturm: SturmBound(c.conductor), Status: "no newform", Newform: f != nil}
	for _, a := range c.a {
		out.Ainvs = append(out.Ainvs, a.String())
	}
	ps, es := factor(c.conductor)
	bad := map[int]int{}
	for i, p := range ps {
		bad[p] = es[i]
	}
	// The discriminant has no prime outside N, and each prime of N divides it.
	rest := new(big.Int).Abs(w.disc)
	all := rest.Sign() != 0
	for _, p := range ps {
		bp := big.NewInt(int64(p))
		if new(big.Int).Mod(rest, bp).Sign() != 0 {
			all = false
		}
		for rest.Sign() != 0 && new(big.Int).Mod(rest, bp).Sign() == 0 {
			rest.Quo(rest, bp)
		}
	}
	out.BadPrimes = all && rest.Cmp(big.NewInt(1)) == 0
	if !out.BadPrimes {
		out.Mismatches = append(out.Mismatches, fmt.Sprintf("the primes of the discriminant %s are not the primes of N", w.disc))
	}
	if f != nil {
		out.Status = "agrees"
	}
	// newformAP is a_p of the newform: the aplist entry for p not dividing N, -w_p for p || N, and 0 for p^2 | N.
	newformAP := func(p int) (int64, bool) {
		if f == nil {
			return 0, false
		}
		x, ok := f.lookup(p)
		switch {
		case !ok:
			return 0, false
		case bad[p] == 0:
			return x.ap, true
		case bad[p] == 1:
			return int64(-x.sign), true
		}
		return 0, true
	}
	for _, p := range ps {
		e := bad[p]
		a := curveAP(c.a, p)
		b := E5Bad{P: p, Exponent: e, CurveAP: a}
		switch a {
		case 1:
			b.Reduction = "split"
		case -1:
			b.Reduction = "nonsplit"
		case 0:
			b.Reduction = "additive"
		default:
			b.Reduction = "not singular"
		}
		vd, v4, v6 := valuation(w.disc, p), valuation(w.c4, p), valuation(w.c6, p)
		b.Minimal = (vd >= 0 && vd < 12) || (v4 >= 0 && v4 < 4) || (v6 >= 0 && v6 < 6)
		b.TypeAgrees = (a == 0) == (e >= 2) && (a == 0 || a == 1 || a == -1)
		if !b.TypeAgrees {
			out.Mismatches = append(out.Mismatches, fmt.Sprintf("a_%d = %d of the curve does not fit the exponent %d of %d in N", p, a, e, p))
		}
		if f != nil {
			if x, ok := f.lookup(p); ok {
				b.NewformW = "+"
				if x.sign < 0 {
					b.NewformW = "-"
				}
				v, _ := newformAP(p)
				b.NewformAP = &v
			}
		}
		out.Bad = append(out.Bad, b)
	}
	// Compare a_p at each prime of the aplist.
	if f != nil {
		for _, x := range f.aps {
			v, _ := newformAP(x.p)
			a := curveAP(c.a, x.p)
			out.Primes++
			out.LargestPrime = max(out.LargestPrime, x.p)
			if a != v {
				out.Mismatches = append(out.Mismatches, fmt.Sprintf("a_%d: curve %d, newform %d", x.p, a, v))
			}
		}
		for _, p := range primesUpTo(out.Sturm) {
			if _, ok := f.lookup(p); !ok {
				out.Missing = append(out.Missing, p)
			}
		}
		out.SturmComplete = len(out.Missing) == 0
	}
	// The criterion of Livne for the 2-adic representations.
	out.TwoTorsion = w.twoTorsion()
	if !out.TwoTorsion {
		out.Livne = E5Livne{Status: "not applicable", Note: "no rational point of order 2"}
	} else {
		s, chars, monos, tp, vecs, ok := livneSet(c.conductor, bound)
		l := E5Livne{Status: "computed", S: s, Characters: chars, Monomials: monos, Primes: tp, Vectors: vecs}
		if !ok {
			l.Status = "not found"
			l.Note = fmt.Sprintf("the primes up to %d do not give a non-cubic set", bound)
		} else {
			agrees := f != nil
			for _, p := range tp {
				a := curveAP(c.a, p)
				l.CurveAP = append(l.CurveAP, a)
				if v, ok := newformAP(p); ok {
					l.Checked++
					if v != a {
						agrees = false
					}
				}
			}
			l.Agrees = agrees && l.Checked == len(tp)
			if f != nil {
				l.NewformEven = true
				for _, x := range f.aps {
					if x.p == 2 || bad[x.p] > 0 {
						continue
					}
					l.EvenPrimes++
					if x.ap%2 != 0 {
						l.NewformEven = false
					}
				}
			}
			switch {
			case f == nil:
				l.Note = "no newform in the data"
			case l.Checked < len(tp):
				l.Note = fmt.Sprintf("%d of the %d primes of T are above 100, outside the aplist", len(tp)-l.Checked, len(tp))
			}
		}
		out.Livne = l
	}
	if f != nil && len(out.Mismatches) > 0 {
		out.Status = "mismatch"
	}
	out.Candidate = out.Status == "agrees" && out.SturmComplete
	out.LivneComplete = out.Status == "agrees" && out.Livne.Agrees && out.Livne.NewformEven
	return out
}

func e5Counts(r E5Result, forms []*e5Form, maxN int) []E5Count {
	var out []E5Count
	for from := 1; from <= maxN; from += 100 {
		c := E5Count{From: from, To: min(from+99, maxN)}
		for _, f := range forms {
			if f.level >= c.From && f.level <= c.To {
				c.Newforms++
			}
		}
		for _, k := range r.Classes {
			if k.Conductor < c.From || k.Conductor > c.To {
				continue
			}
			c.Classes++
			if k.Newform {
				c.Paired++
			}
			c.Primes += k.Primes
			if k.Status == "mismatch" {
				c.Mismatches++
			}
			if k.SturmComplete {
				c.SturmComplete++
			}
			if k.TwoTorsion {
				c.TwoTorsion++
			}
			if k.LivneComplete {
				c.LivneComplete++
			}
			if k.Candidate {
				c.Candidates++
			}
		}
		if c.Classes > 0 || c.Newforms > 0 {
			out = append(out, c)
		}
	}
	return out
}

// e5CountsFile is counts.json of a run of E5.
type e5CountsFile struct {
	Counts   []E5Count   `json:"counts"`
	Total    E5Count     `json:"total"`
	Unpaired []E5Newform `json:"unpaired_newforms"`
}

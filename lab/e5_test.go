package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ainvs(xs ...int64) [5]*big.Int {
	var a [5]*big.Int
	for i, x := range xs {
		a[i] = big.NewInt(x)
	}
	return a
}

// The coefficients a_1, ..., a_43 of the newform 11.2.a.a, from the LMFDB (traces of mf_newforms).
var traces11a = []int64{1, -2, -1, 2, 1, 2, -2, 0, -2, -2, 1, -2, 4, 4, -1, -4, -2, 4, 0, 2, 2, -2, -1, 0, -4, -8, 5, -4, 0, 2, 7, 8, -1, 4, -2, -4, 3, 0, -4, 0, -8, -4, -6}

func TestSturmBound(t *testing.T) {
	for n, want := range map[int]int{1: 0, 11: 2, 14: 4, 37: 6, 27: 6, 990: 432, 997: 166, 1000: 300} {
		if got := SturmBound(n); got != want {
			t.Errorf("B(%d) = %d, want %d", n, got, want)
		}
	}
}

// naiveAP counts the points of the equation mod p by trying each (x, y).
func naiveAP(a [5]*big.Int, p int) int64 {
	bp := big.NewInt(int64(p))
	var r [5]int64
	for i := range a {
		r[i] = new(big.Int).Mod(a[i], bp).Int64()
	}
	n := 0
	q := int64(p)
	for x := range q {
		for y := range q {
			l := (y*y + r[0]*x*y + r[2]*y) % q
			rr := (x*x%q*x + r[1]*x%q*x + r[3]*x + r[4]) % q
			if l == rr {
				n++
			}
		}
	}
	return int64(p - n)
}

func TestCurveAP(t *testing.T) {
	e11 := ainvs(0, -1, 1, -10, -20) // 11.a2
	for _, p := range primesUpTo(43) {
		if got := curveAP(e11, p); got != traces11a[p-1] {
			t.Errorf("11.a2: a_%d = %d, want %d", p, got, traces11a[p-1])
		}
	}
	// 27.a: y^2 + y = x^3 - 7 has additive reduction at 3.
	if got := curveAP(ainvs(0, 0, 1, 0, -7), 3); got != 0 {
		t.Errorf("27.a: a_3 = %d, want 0", got)
	}
	for _, a := range [][5]*big.Int{ainvs(1, 0, 1, 4, -6), ainvs(0, 0, 1, -1, 0), ainvs(1, -1, 1, -1000003, 98765), ainvs(-7, 12, 5, -3, 2)} {
		for _, p := range primesUpTo(61) {
			if got, want := curveAP(a, p), naiveAP(a, p); got != want {
				t.Errorf("%v: a_%d = %d, the naive count gives %d", a, p, got, want)
			}
		}
	}
}

func TestHasIntegerRoot(t *testing.T) {
	// The polynomials X^3 + b X^2 + c X + d.
	poly := func(roots ...int64) (b, c, d *big.Int) {
		// (X - r1)(X - r2)(X - r3)
		r1, r2, r3 := big.NewInt(roots[0]), big.NewInt(roots[1]), big.NewInt(roots[2])
		b = new(big.Int).Neg(new(big.Int).Add(new(big.Int).Add(r1, r2), r3))
		c = new(big.Int).Add(new(big.Int).Add(new(big.Int).Mul(r1, r2), new(big.Int).Mul(r1, r3)), new(big.Int).Mul(r2, r3))
		d = new(big.Int).Neg(new(big.Int).Mul(new(big.Int).Mul(r1, r2), r3))
		return b, c, d
	}
	for _, roots := range [][]int64{{5, 5, -3}, {7, 7, 7}, {-1000000007, 2, 3}, {0, 1, 2}, {-4, 9, 1 << 40}} {
		b, c, d := poly(roots...)
		if !hasIntegerRoot(b, c, d) {
			t.Errorf("the polynomial with the roots %v has no integer root", roots)
		}
	}
	for _, x := range [][3]int64{{0, 0, -2}, {0, 0, 2}, {1, 1, 2}, {-3, 0, 1}, {0, 1, 1}, {-1000000000, 1, -999999999}} {
		if hasIntegerRoot(big.NewInt(x[0]), big.NewInt(x[1]), big.NewInt(x[2])) {
			t.Errorf("X^3 + %d X^2 + %d X + %d has an integer root", x[0], x[1], x[2])
		}
	}
}

func TestTwoTorsion(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    [5]*big.Int
		want bool
	}{
		{"11.a2, torsion Z/5", ainvs(0, -1, 1, -10, -20), false},
		{"14.a, torsion Z/6", ainvs(1, 0, 1, 4, -6), true},
		{"37.a, rank 1 and no torsion", ainvs(0, 0, 1, -1, 0), false},
		{"y^2 = x^3 - x", ainvs(0, 0, 0, -1, 0), true},
		{"y^2 = x^3 + 1", ainvs(0, 0, 0, 0, 1), true},
		{"y^2 = x^3 + 2", ainvs(0, 0, 0, 0, 2), false},
		{"y^2 = (x - 10^9)(x^2 + 1)", ainvs(0, -1000000000, 0, 1, -1000000000), true},
		{"y^2 + xy = x^3 - x^2 + 1 is not x^3 + ... with a root", ainvs(1, -1, 0, 0, 1), false},
	} {
		if got := newWeierstrass(tc.a).twoTorsion(); got != tc.want {
			t.Errorf("%s: a point of order 2 is %v, want %v", tc.name, got, tc.want)
		}
	}
	// 15.a has the torsion Z/4 x Z/2.
	if !newWeierstrass(ainvs(1, 1, 1, -10, -10)).twoTorsion() {
		t.Error("15.a has no point of order 2")
	}
}

func TestDiscriminant(t *testing.T) {
	if d := newWeierstrass(ainvs(0, -1, 1, -10, -20)).disc; d.Cmp(big.NewInt(-161051)) != 0 {
		t.Errorf("the discriminant of 11.a2 is %s, want -11^5", d)
	}
	if d := newWeierstrass(ainvs(0, 0, 1, -1, 0)).disc; d.Cmp(big.NewInt(37)) != 0 {
		t.Errorf("the discriminant of 37.a1 is %s, want 37", d)
	}
}

// nonCubic checks by brute force that each nonzero cubic form on F_2^t is 1 at some vector of T.
func nonCubic(t int, vecs []string) bool {
	var monos []uint
	for i := range t {
		for j := i; j < t; j++ {
			for k := j; k < t; k++ {
				monos = append(monos, 1<<i|1<<j|1<<k)
			}
		}
	}
	// distinct monomials as functions
	seen := map[uint]bool{}
	var ms []uint
	for _, m := range monos {
		if !seen[m] {
			seen[m] = true
			ms = append(ms, m)
		}
	}
	var pts []uint
	for _, v := range vecs {
		var x uint
		for i, c := range v {
			if c == '1' {
				x |= 1 << i
			}
		}
		pts = append(pts, x)
	}
	for f := uint64(1); f < 1<<len(ms); f++ {
		zero := true
		for _, x := range pts {
			val := 0
			for j, m := range ms {
				if f>>j&1 == 1 && x&m == m {
					val ^= 1
				}
			}
			if val == 1 {
				zero = false
				break
			}
		}
		if zero {
			return false
		}
	}
	return true
}

func TestLivneSet(t *testing.T) {
	s, chars, monos, primes, vecs, ok := livneSet(1, 1000)
	if !ok || fmt.Sprint(s) != "[2]" || fmt.Sprint(chars) != "[-1 2]" || monos != 3 || fmt.Sprint(primes) != "[3 5 7]" {
		t.Errorf("S = {2}: %v %v %d %v %v %v", s, chars, monos, primes, vecs, ok)
	}
	for _, n := range []int{11, 14, 15, 30, 210} {
		s, chars, monos, primes, vecs, ok := livneSet(n, 100000)
		if !ok || len(primes) != monos {
			t.Errorf("N = %d: no Livne set (%v, %v)", n, primes, ok)
			continue
		}
		for _, p := range primes {
			for _, q := range s {
				if p == q {
					t.Errorf("N = %d: the prime %d of T is in S", n, p)
				}
			}
		}
		if len(chars) <= 4 && !nonCubic(len(chars), vecs) {
			t.Errorf("N = %d: the set %v with the vectors %v is not non-cubic", n, primes, vecs)
		}
		// One prime fewer is not enough: the rank is the number of monomials.
		if len(chars) <= 4 && nonCubic(len(chars), vecs[:len(vecs)-1]) {
			t.Errorf("N = %d: T without its last prime is still non-cubic", n)
		}
	}
	if _, _, _, _, _, ok := livneSet(210, 20); ok {
		t.Error("the primes up to 20 give a Livne set for S = {2, 3, 5, 7}")
	}
}

// writePage writes a page of the API and returns its sha256.
// The lines of the Cremona files for the fixtures. 27a is not in the aplist, and 17a is not in allcurves.
const (
	fixCurves = `11 a 1 [0,-1,1,-10,-20] 0 5
11 a 2 [0,-1,1,-7820,-263580] 0 1
11 a 3 [0,-1,1,0,0] 0 5
14 a 1 [1,0,1,4,-6] 0 6
27 a 1 [0,0,1,0,-7] 0 3
64 a 1 [0,0,0,-4,0] 0 4
202 a 1 [1,-1,0,4,-176] 0 1
990 a 1 [1,-1,0,-15,25] 1 2
`
	fixAplist = ` 11 a -2 -1  1 -2  -  4 -2  0 -1   0   7   3  -8  -6   8  -6   5  12  -7  -3   4 -10  -6  15  -7
 14 a  + -2  0  -  0 -4  6  2  0  -6  -4   2   6   8 -12   6  -6   8  -4   0   2   8  -6  -6 -10
 17 a -1  0 -2  4  0 -2  - -4  4   6   4  -2  -6   4   0   6 -12 -10   4  -4  -6  12  -4  10   2
 64 a  -  0  2  0  0 -6  2  0  0  10   0   2  10   0   0 -14   0  10   0   0  -6   0   0  10  18
202 a  +  0  2  1  4  0  5  1  6  -5   0  -8  -4  -5   6   3 -12  -1   2 -10 -16  -2  16   0  13 -(101)
990 a  +  +  +  0  +  0 -2  2 -6  -2   0  -6  -2  -2   2  -2  -8   4  -4   2 -10   4 -12   0 -18
`
	fixLabels = `11 a 2 11 a 1
11 a 1 11 a 2
11 a 3 11 a 3
14 a 1 14 a 6
27 a 1 27 a 3
64 a 1 64 a 3
202 a 1 202 a 1
990 a 1 990 a 1
`
)

func writeFile(t *testing.T, dir, name, path, text string) E5File {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(text))
	return E5File{Path: path, Name: name, Sha256: hex.EncodeToString(h[:])}
}

// e5Fixture writes the fixture files to a new directory. edit changes the aplist before it is written.
func e5Fixture(t *testing.T, edit func(aplist string) string) (E5Inputs, string) {
	t.Helper()
	dir := t.TempDir()
	aplist := fixAplist
	if edit != nil {
		aplist = edit(aplist)
	}
	in := E5Inputs{MaxConductor: 1000, LivnePrimeBound: 10000, Source: "test files", Commit: strings.Repeat("0", 40), Licence: "test licence",
		Files: E5Files{
			Curves:  writeFile(t, dir, "curves.txt", "allcurves/x", fixCurves),
			Aplist:  writeFile(t, dir, "aplist.txt", "aplist/x", aplist),
			Labels:  writeFile(t, dir, "labels.txt", "alllabels/x", fixLabels),
			Readme:  writeFile(t, dir, "readme.txt", "README.md", "readme\n"),
			Format:  writeFile(t, dir, "format.txt", "docs/file-format.txt", "format\n"),
			Licence: writeFile(t, dir, "licence.txt", "LICENSE", "licence\n"),
		},
	}
	return in, dir
}

func findClass(t *testing.T, r E5Result, class string) E5Class {
	t.Helper()
	for _, c := range r.Classes {
		if c.Class == class {
			return c
		}
	}
	t.Fatalf("the run has no class %s", class)
	return E5Class{}
}

func TestRunE5(t *testing.T) {
	in, dir := e5Fixture(t, nil)
	r, err := RunE5(in, dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Mismatches() != 0 || len(r.Classes) != 6 || r.MaxLevel != 990 {
		t.Fatalf("the result is %+v", r)
	}
	c11 := findClass(t, r, "11a")
	if c11.Status != "agrees" || c11.LMFDB != "11.a" || c11.Curve != "11a1" || c11.Sturm != 2 || c11.Primes != 25 || c11.LargestPrime != 97 || !c11.SturmComplete || !c11.BadPrimes || !c11.Candidate {
		t.Errorf("11a: %+v", c11)
	}
	if b := c11.Bad[0]; b.Reduction != "split" || b.CurveAP != 1 || b.NewformW != "-" || b.NewformAP == nil || *b.NewformAP != 1 || !b.Minimal || !b.TypeAgrees {
		t.Errorf("11a at 11: %+v", b)
	}
	if c11.TwoTorsion || c11.Livne.Status != "not applicable" || c11.LivneComplete {
		t.Errorf("11a: the Livne set is %+v", c11.Livne)
	}
	c14 := findClass(t, r, "14a")
	if c14.Status != "agrees" || !c14.TwoTorsion || c14.Livne.Status != "computed" || c14.Livne.Checked != 7 || !c14.Livne.Agrees || !c14.Livne.NewformEven || !c14.LivneComplete || !c14.Candidate {
		t.Errorf("14a: %+v", c14)
	}
	// 64 = 2^6: the aplist gives only the sign of W_2, and a_2 = 0.
	c64 := findClass(t, r, "64a")
	if b := c64.Bad[0]; c64.Status != "agrees" || b.Reduction != "additive" || b.NewformW != "-" || *b.NewformAP != 0 || !c64.LivneComplete || c64.Livne.S[0] != 2 || len(c64.Livne.S) != 1 {
		t.Errorf("64a: %+v", c64)
	}
	// 202 = 2 * 101: the aplist has W_101 after the primes below 100.
	c202 := findClass(t, r, "202a")
	if c202.Primes != 26 || c202.LargestPrime != 101 || c202.Status != "agrees" || c202.Sturm != 51 || !c202.SturmComplete {
		t.Errorf("202a: %+v", c202)
	}
	// B(990) = 432, far above the primes of the aplist.
	c990 := findClass(t, r, "990a")
	if c990.Status != "agrees" || c990.SturmComplete || c990.Candidate || len(c990.Missing) == 0 || c990.Missing[0] != 101 || c990.Livne.Agrees || c990.Livne.Checked >= len(c990.Livne.Primes) {
		t.Errorf("990a: %+v", c990)
	}
	c27 := findClass(t, r, "27a")
	if c27.Status != "no newform" || c27.Bad[0].Reduction != "additive" || !c27.Bad[0].TypeAgrees || c27.Candidate {
		t.Errorf("27a: %+v", c27)
	}
	if len(r.Unpaired) != 1 || r.Unpaired[0].Label != "17a" || r.Gaps() != 2 {
		t.Errorf("the unpaired newforms are %+v", r.Unpaired)
	}
	if tot := r.Total(); tot.Classes != 6 || tot.Paired != 5 || tot.Candidates != 4 || tot.SturmComplete != 4 || tot.Newforms != 6 || tot.Primes != 25*4+26 {
		t.Errorf("the total is %+v", tot)
	}
	sets := r.LivneSets()
	if len(sets) != 3 || fmt.Sprint(sets[1].S) != "[2]" || fmt.Sprint(sets[1].Primes) != "[3 5 7]" || fmt.Sprint(sets[1].Classes) != "[64a]" {
		t.Errorf("the Livne sets are %+v", sets)
	}
}

func TestWrongNewformIsCaught(t *testing.T) {
	for _, tc := range []struct {
		name, old, new, want string
	}{
		{"a_2", " 11 a -2", " 11 a  0", "a_2: curve -2, newform 0"},
		{"W_11", " 11 a -2 -1  1 -2  -", " 11 a -2 -1  1 -2  +", "a_11: curve 1, newform -1"},
		{"a_11, a prime of the Livne set", " 14 a  + -2  0  -  0", " 14 a  + -2  0  -  2", "a_11: curve 0, newform 2"},
		{"W_101", "-(101)", "+(101)", "a_101: curve 1, newform -1"},
		{"a_97", "-12   0 -18\n", "-12   0 -16\n", "a_97: curve -18, newform -16"},
	} {
		in, dir := e5Fixture(t, func(s string) string {
			if !strings.Contains(s, tc.old) {
				t.Fatalf("%s: the fixture has no %q", tc.name, tc.old)
			}
			return strings.Replace(s, tc.old, tc.new, 1)
		})
		r, err := RunE5(in, dir)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range r.Classes {
			for _, m := range c.Mismatches {
				if strings.Contains(m, tc.want) {
					found = c.Status == "mismatch" && !c.Candidate && !c.LivneComplete
				}
			}
		}
		if !found || r.Mismatches() != 1 {
			t.Errorf("%s: the run does not report %q: %+v", tc.name, tc.want, r.Classes)
		}
	}
}

func TestE5Data(t *testing.T) {
	in, dir := e5Fixture(t, nil)
	if err := os.WriteFile(filepath.Join(dir, "aplist.txt"), []byte(" 11 a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunE5(in, dir); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Errorf("a changed file gives %v", err)
	}
	if _, err := RunE5(in, ""); err == nil || !strings.Contains(err.Error(), "-data") {
		t.Errorf("no directory gives %v", err)
	}
	for _, tc := range []struct {
		name, old, new, want string
	}{
		{"a sign at a good prime", " 11 a -2", " 11 a  +", "does not fit"},
		{"a number at a bad prime", " 14 a  +", " 14 a  1", "does not fit"},
		{"a short line", "  15  -7\n", "\n", "entries"},
		{"no W_101", " -(101)", "", "primes q > 100"},
		{"a wrong q", "-(101)", "-(103)", "q = 101"},
		{"a large a_p", " 11 a -2", " 11 a -20000", "does not fit"},
		{"not sorted", "\n990 a", "\n99 a", "comes after"},
		{"a class twice", " 17 a", " 14 a", "twice"},
	} {
		in, dir := e5Fixture(t, func(s string) string {
			if !strings.Contains(s, tc.old) {
				t.Fatalf("%s: the fixture has no %q", tc.name, tc.old)
			}
			return strings.Replace(s, tc.old, tc.new, 1)
		})
		if _, err := RunE5(in, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: the error is %v, want %q", tc.name, err, tc.want)
		}
	}
	// A bad line of allcurves.
	in, dir = e5Fixture(t, nil)
	in.Files.Curves = writeFile(t, dir, "curves.txt", "allcurves/x", "11 a 1 [0,-1,1,-10] 0 5\n")
	if _, err := RunE5(in, dir); err == nil || !strings.Contains(err.Error(), "[a1,a2,a3,a4,a6]") {
		t.Errorf("a bad curve gives %v", err)
	}
	// Two LMFDB labels for one class.
	in, dir = e5Fixture(t, nil)
	in.Files.Labels = writeFile(t, dir, "labels.txt", "alllabels/x", "11 a 4 11 b 1\n"+fixLabels)
	if _, err := RunE5(in, dir); err == nil || !strings.Contains(err.Error(), "LMFDB labels") {
		t.Errorf("two labels give %v", err)
	}
	// A class with no LMFDB label is not a candidate.
	in, dir = e5Fixture(t, nil)
	in.Files.Labels = writeFile(t, dir, "labels.txt", "alllabels/x", strings.Replace(fixLabels, "14 a 1 14 a 6\n", "", 1))
	r, err := RunE5(in, dir)
	if err != nil {
		t.Fatal(err)
	}
	if c := findClass(t, r, "14a"); c.Candidate || c.Status != "mismatch" {
		t.Errorf("14a with no LMFDB label: %+v", c)
	}
	// max_conductor drops the larger conductors.
	in, dir = e5Fixture(t, nil)
	in.MaxConductor = 100
	r, err = RunE5(in, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Classes) != 4 || r.MaxLevel != 990 {
		t.Errorf("max_conductor 100 gives %d classes", len(r.Classes))
	}
}

func TestE5Inputs(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	file := func(name string) E5File { return E5File{Path: "p/" + name, Name: name + ".txt", Sha256: sum} }
	good := E5Inputs{MaxConductor: 10, LivnePrimeBound: 100, Source: "s", Commit: strings.Repeat("a", 40), Licence: "l",
		Files: E5Files{Curves: file("c"), Aplist: file("a"), Labels: file("l"), Readme: file("r"), Format: file("f"), Licence: file("x")}}
	for _, tc := range []struct {
		name string
		edit func(in *E5Inputs)
		want string
	}{
		{"good", func(_ *E5Inputs) {}, ""},
		{"no curves", func(in *E5Inputs) { in.Files.Curves = E5File{} }, "plain name"},
		{"a path", func(in *E5Inputs) { in.Files.Aplist.Name = "../x.txt" }, "plain name"},
		{"a json file", func(in *E5Inputs) { in.Files.Aplist.Name = "x.json" }, "plain name"},
		{"no path", func(in *E5Inputs) { in.Files.Aplist.Path = "" }, "path in the repository"},
		{"a bad hash", func(in *E5Inputs) { in.Files.Labels.Sha256 = "abc" }, "64 hex"},
		{"no source", func(in *E5Inputs) { in.Source = " " }, "source"},
		{"no licence", func(in *E5Inputs) { in.Licence = "" }, "licence"},
		{"a bad commit", func(in *E5Inputs) { in.Commit = "master" }, "commit"},
		{"a small conductor", func(in *E5Inputs) { in.MaxConductor = 0 }, "max_conductor"},
		{"a large conductor", func(in *E5Inputs) { in.MaxConductor = 10000 }, "max_conductor"},
		{"a file twice", func(in *E5Inputs) { in.Files.Readme = in.Files.Curves }, "twice"},
	} {
		in := good
		tc.edit(&in)
		err := in.check()
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: the error is %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseSpecE5(t *testing.T) {
	s, err := ParseSpec("lab/specs/e5-modularity-elliptic.yaml", []byte(SpecE5))
	if err != nil {
		t.Fatal(err)
	}
	if s.Experiment != "E5" || s.E5.MaxConductor != 1000 || s.E5.Files.Aplist.Path != "aplist/aplist.00000-09999" || !strings.Contains(s.E5.Licence, "Artistic License 2.0") || strings.Contains(SpecE5, "ec-000.json") {
		t.Errorf("the spec is %+v", s.E5)
	}
}

func TestRunE5Spec(t *testing.T) {
	in, dir := e5Fixture(t, nil)
	root := t.TempDir()
	path := filepath.Join(root, "lab", "specs", "e5-small.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	var files strings.Builder
	for _, kv := range []struct {
		key string
		f   E5File
	}{{"curves", in.Files.Curves}, {"aplist", in.Files.Aplist}, {"labels", in.Files.Labels}, {"readme", in.Files.Readme}, {"format", in.Files.Format}, {"licence", in.Files.Licence}} {
		fmt.Fprintf(&files, "    %s: {path: %s, name: %s, sha256: %s}\n", kv.key, kv.f.Path, kv.f.Name, kv.f.Sha256)
	}
	spec := fmt.Sprintf("name: e5-small\nexperiment: E5\nquestion: Do the small cases agree?\ntool: lgl\nnode: LG00000005\ninputs:\n  max_conductor: 1000\n  livne_prime_bound: 10000\n  source: test files\n  commit: %s\n  licence: test licence\n  files:\n%soutputs: [classes.json, counts.json, livne.json, summary.md]\n", in.Commit, files.String())
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) }
	if _, err := Run(s, Options{Runs: RunsDir(path), Host: "test", ToolVersion: "dev", Now: now}); err == nil {
		t.Error("a run of E5 with no data directory does not fail")
	}
	rec, err := Run(s, Options{Runs: RunsDir(path), Data: dir, Host: "test", ToolVersion: "dev", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Mismatches != 0 || rec.Gaps != 2 || !strings.HasPrefix(rec.Result, "incomplete") || len(rec.Files) != 4 || len(rec.Data) != 6 || !strings.Contains(rec.DataSource, "test licence") {
		t.Errorf("the record is %+v", rec)
	}
	summary, err := os.ReadFile(filepath.Join(RunsDir(path), rec.ID, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## What the run does not do", "## Counts", "## The criterion of Livne", "## Which N reach the Sturm bound", "Sturm-complete candidates (4), with their LMFDB labels: 11.a, 14.a, 64.a, 202.a.", "The aplist has 1 class with no curve in allcurves: 17a.", "the check is partial"} {
		if !strings.Contains(string(summary), want) {
			t.Errorf("the summary does not have %q:\n%s", want, summary)
		}
	}
	if strings.ContainsAny(string(summary), "\u2013\u2014") {
		t.Error("the summary has a dash character")
	}
	classes, err := os.ReadFile(filepath.Join(RunsDir(path), rec.ID, "classes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []E5Class
	if err := json.Unmarshal(classes, &got); err != nil || len(got) != 6 {
		t.Errorf("classes.json has %d classes: %v", len(got), err)
	}
}

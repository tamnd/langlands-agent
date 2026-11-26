// Package lab runs the experiments of the computation lab, spec 13. A spec is lab/specs/<name>.yaml of the content repository, and a run is lab/runs/<id>/ with record.json, summary.md and the outputs.
package lab

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Experiment is a row of the table of spec 13.3.
type Experiment struct {
	ID       string
	Title    string
	Route    string
	Runnable bool // lgl lab run has code for it
}

// Experiments are the first experiments of spec 13.3. E1 and E2 have code.
var Experiments = []Experiment{
	{"E1", "small cases of local reciprocity for Q_p, p <= 7, against Lubin-Tate", "library, S1", true},
	{"E2", "the explicit LLC for GL2(Q_p), p in {3, 5, 7}, with the factors of pairs", "R2", true},
	{"E3", "depth-zero and regular supercuspidals of SL2, GSp4 and G2 by Deligne-Lusztig", "R1, R2", false},
	{"E4", "orbital integrals and transfer factors for SL2, U(2) and U(3)", "R1, R5", false},
	{"E5", "modularity of the elliptic curves of LMFDB of conductor at most 1000", "R3, F2", false},
	{"E6", "genus 2 curves and paramodular forms", "R3", false},
	{"E7", "Satake parameter statistics for Sym^n and for Maass forms of level 1", "R4, R6", false},
	{"E8", "Serre weights for small p and f", "R9", false},
	{"E9", "the explicit identities of each new claim of route R1", "R1", false},
}

// FindExperiment returns the experiment with the id, or nil.
func FindExperiment(id string) *Experiment {
	for i := range Experiments {
		if Experiments[i].ID == id {
			return &Experiments[i]
		}
	}
	return nil
}

// Tool is the value of tool in a spec that lgl runs itself.
const Tool = "lgl"

// Spec is a lab job, spec 13.2.
type Spec struct {
	Name       string    `yaml:"name"`
	Experiment string    `yaml:"experiment"`
	Question   string    `yaml:"question"`
	Tool       string    `yaml:"tool"`
	Node       string    `yaml:"node"`
	Nodes      []string  `yaml:"nodes,omitempty"`
	Inputs     yaml.Node `yaml:"inputs"` // the inputs, in the form of the experiment
	Outputs    []string  `yaml:"outputs"`
	Notes      string    `yaml:"notes,omitempty"`

	E1 E1Inputs `yaml:"-"` // the inputs of a spec of E1
	E2 E2Inputs `yaml:"-"` // the inputs of a spec of E2

	Path string `yaml:"-"` // the file of the spec
	Hash string `yaml:"-"` // the sha256 of the bytes of the file
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseSpec reads a spec from its bytes. The inputs have the form of the experiment, so a spec of an experiment with no code is an error.
func ParseSpec(path string, data []byte) (*Spec, error) {
	var s Spec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	s.Path, s.Hash = path, hex.EncodeToString(sum[:])
	e := FindExperiment(s.Experiment)
	switch {
	case !nameRE.MatchString(s.Name):
		return nil, fmt.Errorf("%s: the name %q must have only lower case letters, digits and hyphens", path, s.Name)
	case e == nil:
		return nil, fmt.Errorf("%s: the experiment %q is not one of E1 to E9 of spec 13.3", path, s.Experiment)
	case !e.Runnable:
		return nil, fmt.Errorf("%s: lgl has no code for experiment %s yet", path, s.Experiment)
	case s.Tool != Tool:
		return nil, fmt.Errorf("%s: the tool is %q, but lgl lab runs only the specs with the tool %s", path, s.Tool, Tool)
	case strings.TrimSpace(s.Question) == "" || s.Node == "":
		return nil, fmt.Errorf("%s: the spec needs a question and a node", path)
	}
	if err := s.decodeInputs(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

// decodeInputs reads the inputs into the form of the experiment, with no unknown fields, and checks them.
func (s *Spec) decodeInputs() error {
	if s.Inputs.Kind == 0 {
		return errors.New("the spec needs inputs")
	}
	data, err := yaml.Marshal(&s.Inputs)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	switch s.Experiment {
	case "E1":
		if err := dec.Decode(&s.E1); err != nil {
			return fmt.Errorf("the inputs of E1: %w", err)
		}
		return s.E1.check()
	case "E2":
		if err := dec.Decode(&s.E2); err != nil {
			return fmt.Errorf("the inputs of E2: %w", err)
		}
		return s.E2.check()
	}
	return fmt.Errorf("lgl has no code for experiment %s yet", s.Experiment)
}

// LoadSpec reads a spec file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- a spec that the user names
	if err != nil {
		return nil, err
	}
	return ParseSpec(path, data)
}

// SpecText returns the spec that lgl lab spec prints for an experiment, or "" when the experiment has no code.
func SpecText(id string) string {
	switch id {
	case "E1":
		return SpecE1
	case "E2":
		return SpecE2
	}
	return ""
}

// SpecE1 is the spec of E1 that lgl lab spec E1 prints. A person copies it to lab/specs/ of the content repository.
const SpecE1 = `name: e1-local-reciprocity-small
experiment: E1
question: For p in {2, 3, 5, 7}, is the norm group of each quadratic extension of Q_p, and of each cyclotomic extension Q_p(zeta_{p^n}) with p^n <= 49, the kernel of the Artin map that Lubin-Tate theory with the uniformizer p predicts?
tool: lgl
node: LGAV7YV6K6 # LocalReciprocityGL1
inputs:
  primes: [2, 3, 5, 7]
  box: 40           # the norms a^2 - d b^2 of the quadratic fields use |a|, |b| <= 40
  max_conductor: 49 # the fields Q_p(zeta_{p^n}) with p^n <= 49
  samples: 200      # random elements of Z[zeta] for each cyclotomic field
  coefficients: 2   # their coefficients are in [-2, 2]
  seed: 1
outputs: [quadratic.json, cyclotomic.json, summary.md]
notes: A small version of E1 in Go, with no container and no network. It checks norm groups, which are the kernels of the Artin maps. It does not check the Artin map of a cyclotomic field of degree more than 2 element by element, and it has no Lean certificate (13.4).
`

// SpecE2 is the spec of E2 that lgl lab spec E2 prints.
const SpecE2 = `name: e2-llc-gl2-small
experiment: E2
question: For p in {3, 5, 7}, does the explicit local Langlands correspondence give the same L-factor and epsilon-factor of pi x chi on the GL2 side and on the Galois side, for the principal series, the twisted Steinberg and the depth-zero supercuspidal representations pi of GL2(Q_p) of conductor exponent at most 4 and the characters chi of conductor exponent at most 2?
tool: lgl
node: LG453577KN # LocalLanglandsGL2
inputs:
  primes: [3, 5, 7]
  conductor_exponent: 4           # a(pi) <= 4
  twist_conductor: 2              # a(chi) <= 2
  additive_character: conductor-O # psi(x) = exp(2 pi i {x}_p), trivial on Z_p and not on p^-1 Z_p
  families: [principal-series, steinberg, depth-zero-supercuspidal]
outputs: [counts.json, principal.json, steinberg.json, supercuspidal.json, summary.md]
notes: A small version of E2 in Go, with no container and no network. The arithmetic is exact in Q(zeta_N) with N = lcm(4, p^2, p^2 - 1). The values at p of the unramified parts of the characters are formal variables. The principal series use inducing characters of conductor exponent at most 2 each, so the principal series with an inducing character of conductor exponent 3 or 4 are not in the run. The supercuspidal representations of positive depth are not in the run, except as the twists of a depth-zero one by a character of conductor exponent 2. The case p = 2 is not in the run. The run has no Lean certificate (13.4).
`

// Record is record.json of a run. The site reads the fields of site.LabRecord, and the other fields make the run reproducible.
type Record struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Experiment  string            `json:"experiment"`
	Question    string            `json:"question"`
	Spec        string            `json:"spec"`
	SpecSHA256  string            `json:"spec_sha256"`
	Node        string            `json:"node"`
	Nodes       []string          `json:"nodes,omitempty"`
	Tool        string            `json:"tool"`
	ToolVersion string            `json:"tool_version"`
	Commit      string            `json:"commit,omitempty"`
	Modified    bool              `json:"modified,omitempty"` // the binary was built from a checkout with changes
	Go          string            `json:"go"`
	Container   string            `json:"container"` // the image digest of spec 13.2, or none
	Network     bool              `json:"network"`
	Seed        uint64            `json:"seed"`
	Host        string            `json:"host"`
	Started     string            `json:"started"`
	WallSeconds float64           `json:"wall_seconds"`
	Result      string            `json:"result"`
	Mismatches  int               `json:"mismatches"` // the outputs that contradict the prediction
	Gaps        int               `json:"gaps"`       // the predicted classes with no example in the run
	Files       map[string]string `json:"files"`      // the sha256 of each file of the run, record.json excluded
}

// Options are the settings of a run that are not in the spec.
type Options struct {
	Runs        string // the directory of the runs
	Force       bool   // replace a run with the same id
	Host        string
	ToolVersion string
	Commit      string
	Modified    bool
	Go          string
	Now         func() time.Time
}

// RunsDir returns lab/runs next to the lab/specs of a spec path, or "" when the spec is not in lab/specs.
func RunsDir(spec string) string {
	abs, err := filepath.Abs(spec)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) == "specs" && filepath.Base(filepath.Dir(dir)) == "lab" {
		return filepath.Join(filepath.Dir(dir), "runs")
	}
	return ""
}

// specRel returns the path of a spec from the root of the content repository, when the path has lab/specs/ in it.
func specRel(path string) string {
	p := filepath.ToSlash(path)
	if i := strings.LastIndex(p, "lab/specs/"); i >= 0 {
		return p[i:]
	}
	return filepath.Base(p)
}

// Run runs a spec and writes the run directory. It returns the record. A run whose outputs contradict the prediction is written too; the caller reads Mismatches.
func Run(s *Spec, opt Options) (*Record, error) {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Runs == "" {
		return nil, errors.New("lab: the directory of the runs is not set")
	}
	start := opt.Now().UTC()
	rec := &Record{
		Name: s.Name, Experiment: s.Experiment, Question: s.Question, Spec: specRel(s.Path), SpecSHA256: s.Hash,
		Node: s.Node, Nodes: s.Nodes, Tool: Tool, ToolVersion: opt.ToolVersion, Commit: opt.Commit, Modified: opt.Modified,
		Go: opt.Go, Container: "none", Seed: s.E1.Seed, Host: opt.Host, Started: start.Format(time.RFC3339), Files: map[string]string{},
	}
	rec.ID = start.Format("2006-01-02") + "-" + s.Name + "-" + s.Hash[:8]
	dir := filepath.Join(opt.Runs, rec.ID)
	if _, err := os.Stat(dir); err == nil && !opt.Force {
		return nil, fmt.Errorf("lab: the run %s exists; use -force to replace it", dir)
	}
	var files map[string][]byte
	var summary func() string
	switch s.Experiment {
	case "E1":
		res, err := RunE1(s.E1)
		if err != nil {
			return nil, err
		}
		rec.Mismatches, rec.Gaps = res.Mismatches(), res.Gaps()
		rec.Result = e1Result(res)
		files = map[string][]byte{
			"quadratic.json":  marshal(res.Quadratic),
			"cyclotomic.json": marshal(res.Cyclotomic),
		}
		summary = func() string { return e1Summary(s, rec, res) }
	case "E2":
		res, err := RunE2(s.E2)
		if err != nil {
			return nil, err
		}
		rec.Mismatches = res.Mismatches()
		rec.Result = e2Result(res)
		files = map[string][]byte{
			"counts.json":        marshal(res.Counts),
			"principal.json":     marshal(res.Principal),
			"steinberg.json":     marshal(res.Steinberg),
			"supercuspidal.json": marshal(res.Supercuspidal),
		}
		summary = func() string { return e2Summary(s, rec, res) }
	default:
		return nil, fmt.Errorf("lab: lgl has no code for experiment %s", s.Experiment)
	}
	rec.WallSeconds = float64(opt.Now().Sub(start).Milliseconds()) / 1000
	files["summary.md"] = []byte(summary())
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	for n, data := range files {
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o644); err != nil { // #nosec G306 -- public data
			return nil, err
		}
		sum := sha256.Sum256(data)
		rec.Files[n] = hex.EncodeToString(sum[:])
	}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), marshal(rec), 0o644); err != nil { // #nosec G306 -- public data
		return nil, err
	}
	return rec, nil
}

func marshal(v any) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err) // the types of this package always marshal
	}
	return append(data, '\n')
}

// e1Result is the one-line result of a run of E1.
func e1Result(r E1Result) string {
	fields := len(r.Quadratic) + len(r.Cyclotomic)
	switch {
	case r.Mismatches() > 0:
		return fmt.Sprintf("mismatch: %d norms are outside the kernel that Lubin-Tate predicts", r.Mismatches())
	case r.Gaps() > 0:
		return fmt.Sprintf("incomplete: no computed norm contradicts the prediction in %d fields, but %d classes of the predicted kernels have no norm in the run", fields, r.Gaps())
	}
	return fmt.Sprintf("agrees: in %d fields each norm that the run computed is in the kernel that Lubin-Tate predicts, and each class of the kernel has a norm", fields)
}

func e1Summary(s *Spec, rec *Record, r E1Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", rec.ID)
	fmt.Fprintf(&b, "%s\n\n", s.Question)
	fmt.Fprintf(&b, "Result: %s.\n\n", rec.Result)
	b.WriteString("## What the run does\n\n")
	b.WriteString("Local class field theory says that the Artin map of a finite abelian extension L of Q_p has the norm group N(L^x) as its kernel. Lubin-Tate theory with the uniformizer p gives the map: u p^v acts as the Frobenius to the power v on the unramified extensions, and as zeta to zeta^(1/u) on the roots of unity of p-power order. The run computes norms and compares them with the kernel that this formula predicts.\n\n")
	fmt.Fprintf(&b, "- Quadratic fields: for each Q_p(sqrt d), the run computes the norms a^2 - d b^2 with |a|, |b| <= %d and finds their classes in Q_p^x modulo squares.\n", s.E1.Box)
	fmt.Fprintf(&b, "- Cyclotomic fields: for each Q_p(zeta_{p^n}) with p^n <= %d, the run computes the norm of 1 - zeta and of %d random elements of Z[zeta] with coefficients in [-%d, %d] (seed %d). Each norm is an exact determinant. The prediction is that the unit part of each norm is 1 mod p^n. The run also records which classes of (1 + p^n Z_p) / (1 + p^(n+1) Z_p) the norms reach.\n\n", s.E1.MaxConductor, s.E1.Samples, s.E1.Coefficients, s.E1.Coefficients, s.E1.Seed)
	b.WriteString("## Quadratic fields\n\n| p | d | kind | predicted kernel | classes of the norms | norms | outside |\n|---|---|---|---|---|---|---|\n")
	for _, q := range r.Quadratic {
		fmt.Fprintf(&b, "| %d | %d | %s | %s | %s | %d | %d |\n", q.P, q.D, q.Kind, ints(q.Kernel), ints(q.Found), q.Norms, q.Outside)
	}
	b.WriteString("\n## Cyclotomic fields\n\n| p | n | degree | predicted norm group | norms | outside | classes k of 1 + k p^n | N(1 - zeta) = p |\n|---|---|---|---|---|---|---|---|\n")
	for _, c := range r.Cyclotomic {
		ks := make([]int64, len(c.Classes))
		for i, k := range c.Classes {
			ks[i] = int64(k)
		}
		fmt.Fprintf(&b, "| %d | %d | %d | %s | %d | %d | %s | %s |\n", c.P, c.N, c.Degree, c.Kernel, c.Norms, c.Outside, ints(ks), yes(c.PIsNorm))
	}
	b.WriteString("\n## What the run does not do\n\n")
	b.WriteString("- It is a search, not a proof. A norm in the predicted kernel is evidence, and only a certificate that Lean checks makes a theorem (spec 13.4).\n")
	b.WriteString("- It checks the kernels of the Artin maps, the norm groups. For a quadratic field the kernel fixes the map. For a cyclotomic field of degree more than 2 the run does not check the map element by element.\n")
	b.WriteString("- It shows that the norm groups are inside the predicted kernels only for the norms that it computed. It does not show that no other norm is outside.\n")
	b.WriteString("- It covers only the quadratic and the cyclotomic fields of the spec, not every abelian extension.\n")
	b.WriteString("- It runs in the lgl binary, not in a container with pinned images, and it uses no network and no other tool. The record gives the commit of lgl and the seed, so the outputs are reproducible.\n")
	return b.String()
}

// e2Result is the one-line result of a run of E2.
func e2Result(r E2Result) string {
	if m := r.Mismatches(); m > 0 {
		p := r.FirstMismatch
		return fmt.Sprintf("mismatch: %d of %d pairs do not agree; the first is p = %d, %s, twist %s: %s", m, r.Pairs(), p.P, p.Pi, p.Twist, p.Note)
	}
	return fmt.Sprintf("agrees: for each of the %d pairs (pi, chi) the L-factor and the epsilon-factor of pi x chi on the GL2 side are equal to those on the Galois side, exactly in Q(zeta_N)", r.Pairs())
}

func e2Summary(s *Spec, rec *Record, r E2Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", rec.ID)
	fmt.Fprintf(&b, "%s\n\n", s.Question)
	fmt.Fprintf(&b, "Result: %s.\n\n", rec.Result)
	if p := r.FirstMismatch; p != nil {
		fmt.Fprintf(&b, "First mismatch: p = %d, %s, twist %s. GL2 side: L = %s, epsilon = %s. Galois side: L = %s, epsilon = %s. %s.\n\n", p.P, p.Pi, p.Twist, p.GL2L, p.GL2Epsilon, p.GaloisL, p.GaloisEpsilon, p.Note)
	}
	b.WriteString("## What the run does\n\n")
	b.WriteString("F is Q_p and q = p. The additive character is psi(x) = exp(2 pi i {x}_p). It is trivial on Z_p and not on p^-1 Z_p. X is q^-s. The primitive root g is the smallest primitive root mod p^2, and chi_k is the character of Z_p^x with chi_k(g) = exp(2 pi i k / (p(p - 1))). E is the unramified quadratic extension Q_p(sqrt e), with e the smallest non-residue mod p. gamma is the first generator of k_E^x in the order of a + b sqrt e by a + p b, and theta_j is the character of k_E^x with theta_j(gamma) = exp(2 pi i j / (q^2 - 1)).\n\n")
	b.WriteString("The values at p of the unramified parts are formal variables: a1 and a2 for the inducing characters of a principal series, a for the character of a twisted Steinberg representation, and c = theta(p) for a supercuspidal representation. Thus each row holds for all the unramified twists of pi. The twists chi are the chi_k of conductor exponent at most 2 with chi(p) = 1. An unramified twist of chi is a shift of s, so these twists cover all the characters of conductor exponent at most 2.\n\n")
	b.WriteString("The arithmetic is exact. Each number is an element of Z[zeta_N] with a rational denominator, with N = lcm(4, p^2, p^2 - 1), and the run checks each integer operation for overflow. Two numbers are equal when their difference is 0 modulo the cyclotomic polynomial Phi_N. The square root of q is the quadratic Gauss sum, times -i when p = 3 mod 4. The complex numbers in the tables are for the reader only, and the run does not use them in a comparison.\n\n")
	b.WriteString("For each pair (pi, chi), the run computes L(s, pi x chi) and epsilon(s, pi x chi, psi) on the GL2 side from the representation pi tensor chi, and L(s, rec(pi) tensor chi) and epsilon(s, rec(pi) tensor chi, psi) on the Galois side from the Weil-Deligne representation. Then it compares the two L-factors as multisets of roots, and the two epsilon-factors as monomials in X and in the formal variables.\n\n")
	b.WriteString("## The families and the formulas\n\n")
	b.WriteString("- Principal series pi(chi1, chi2), with chi1 and chi2 of conductor exponent at most 2 each and a(chi1) + a(chi2) at most the bound. The family with formal a1 and a2 is irreducible outside the locus chi1 / chi2 = |.|^(1 or -1). GL2 side: L(s, pi tensor chi) = L(s, chi1 chi) L(s, chi2 chi) and epsilon(s, pi tensor chi, psi) = epsilon(s, chi1 chi, psi) epsilon(s, chi2 chi, psi) (Jacquet-Langlands 1970). Galois side: rec(pi) = chi1 + chi2 with N = 0, and the factors are additive. Both sides come to the same Tate factors, so these rows check the bookkeeping of the run, not an independent identity.\n")
	b.WriteString("- Twisted Steinberg representations eta St, the irreducible subrepresentation of the normalized induction from eta |.|^(1/2) x eta |.|^(-1/2). (chi St) tensor chi' = (chi chi') St. GL2 side: L(s, eta St) = L(s, mu1) and epsilon(s, eta St, psi) = epsilon(s, mu1, psi) epsilon(s, mu2, psi) L(1 - s, mu1^-1) / L(s, mu2), with mu1 = eta |.|^(1/2) and mu2 = eta |.|^(-1/2) (Jacquet-Langlands 1970). The run cancels the ratio of L-factors and checks that the result is a monomial. Galois side: sp(2) tensor eta, the sum of eta |.|^(-1/2) e1 and eta |.|^(1/2) e2 with N e1 = e2. The run checks Phi N Phi^-1 = q^-1 N for the geometric Frobenius Phi. Then L(s, rho) = det(1 - Phi X | (V^I)^(N = 0))^-1 and epsilon(s, rho, psi) = epsilon(s, rho_ss, psi) det(-Phi X | V^I / (V^I)^(N = 0)) (Deligne 1973; Tate 1979). For eta unramified the two sides are different computations, and both give L = 1/(1 - q^(-1/2) a X) and epsilon = -q^(1/2) a X. For eta ramified both sides come to the same Tate factors.\n")
	b.WriteString("- Depth-zero supercuspidal representations pi_theta = c-Ind from ZK of Lambda, K = GL2(Z_p), with Lambda on K the inflation of the cuspidal representation sigma_theta of GL2(F_p) of a regular character theta of k_E^x, and Lambda(p) = c. There is one pi_theta for each pair {theta, theta^q}. The character of sigma_theta is (q - 1) theta(a) on a scalar a, -theta(a) on a times a nontrivial unipotent, 0 on a split semisimple class and -(theta(x) + theta(x^q)) on an elliptic class with eigenvalue x (Fulton-Harris, Representation theory, section 5.2).\n")
	b.WriteString("- GL2 side of the supercuspidal pairs. pi_theta tensor chi is c-Ind of Lambda' = Lambda tensor chi o det, which factors through GL2(Z/p^n) on K with n = max(1, a(chi)). The run uses the functional equation of Godement-Jacquet (Godement-Jacquet 1972) with the matrix coefficient f = tr Lambda' on ZK and the test function Phi = the conjugate of tr Lambda' on K. Then Z(Phi, s, f) = 1 and epsilon(s, pi tensor chi, psi) = Z(Phi^, 1 - s, f^v) = sum over m from -n to 0 of c^-m q^(-4n-3m) X^(-2m) T_m / (q - 1), with T_m the sum over h in GL2(Z/p^n) of the character of the contragredient of Lambda' at h times psi(p^m tr h). The run computes each T_m by a sum over all of GL2(Z/p^n), with the counts of the elements by class mod p, det and trace, and it checks that only m = -n gives a term that is not 0. For a tame chi this is the formula epsilon(s, pi_theta', psi) = -theta'(p) X^2 times the Gauss sum of theta'^-1 over k_E^x with the character psi(Tr x / p), with theta' = theta (chi o N). The tests check this reduction to the Gauss sum of k_E (Kondo 1963). L(s, pi tensor chi) = 1 for a supercuspidal representation.\n")
	b.WriteString("- Galois side of the supercuspidal pairs. rec(pi_theta) = Ind from W_E to W_F of theta Delta, with Delta the unramified quadratic character of E^x, the rectifier of a level-zero admissible pair (Bushnell-Henniart, The local Langlands conjecture for GL(2), chapter 8, section 34). rec(pi_theta) tensor chi = Ind of xi = theta Delta (chi o N). Then L(s, Ind xi) = L(s, xi) = 1, and epsilon(s, Ind xi, psi) = lambda(E/F, psi) epsilon(s, xi, psi o Tr) by inductivity in degree 0 (Deligne 1973; Tate 1979). The run computes lambda(E/F, psi) = epsilon(1_F) epsilon(eta_E/F) / epsilon(1_E) from Tate's factors, and epsilon(s, xi, psi o Tr) from Tate's Gauss sum over (O_E / p^n)^x. The run also compares the central character theta|F^x chi^2 with det Ind xi = eta_E/F xi|F^x.\n")
	b.WriteString("- Control. For each supercuspidal pair the run also tests the naive correspondence pi_theta to Ind theta, with no rectifier. A check that cannot fail is not a test, so the control must fail.\n")
	for _, l := range r.Lambda {
		fmt.Fprintf(&b, "\nThe run computes %s.\n", l)
	}
	b.WriteString("\n## Counts\n\n| p | family | N | representations | twists | pairs | matches |\n|---|---|---|---|---|---|---|\n")
	for _, c := range r.Counts {
		fmt.Fprintf(&b, "| %d | %s | %d | %d | %d | %d | %d |\n", c.P, c.Family, c.Field, c.Pis, c.Twists, c.Pairs, c.Matches)
	}
	if len(r.Principal) > 0 {
		b.WriteString("\n## Principal series by conductor\n\n| p | a(pi) | a(pi x chi) | pairs | matches | example | epsilon |\n|---|---|---|---|---|---|---|\n")
		for _, g := range r.Principal {
			fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %s x %s | %s |\n", g.P, g.PiConductor, g.PairConductor, g.Pairs, g.Matches, g.Example.Pi, g.Example.Twist, g.Example.GL2Epsilon)
		}
	}
	if len(r.Steinberg) > 0 {
		b.WriteString("\n## Twisted Steinberg representations\n\n| p | eta | pairs | matches | L on both sides | epsilon on both sides |\n|---|---|---|---|---|---|\n")
		type row struct {
			p, pairs, matches int
			unram             bool
			l, e              string
		}
		rows := map[[2]int]*row{}
		var order [][2]int
		for _, x := range r.Steinberg {
			k := [2]int{x.P, x.PairConductor}
			if x.PairConductor != 1 {
				k[1] = 0
			}
			if rows[k] == nil {
				rows[k] = &row{p: x.P, unram: x.PairConductor == 1, l: x.GL2L, e: x.GL2Epsilon}
				order = append(order, k)
			}
			rows[k].pairs++
			if x.Match {
				rows[k].matches++
			}
		}
		for _, k := range order {
			w := rows[k]
			if w.unram {
				fmt.Fprintf(&b, "| %d | unramified | %d | %d | %s | %s |\n", w.p, w.pairs, w.matches, w.l, w.e)
			} else {
				fmt.Fprintf(&b, "| %d | ramified | %d | %d | 1 | eps(eta norm^(1/2)) eps(eta norm^(-1/2)), with norm = the absolute value; for example %s |\n", w.p, w.pairs, w.matches, w.e)
			}
		}
	}
	if len(r.Supercuspidal) > 0 {
		b.WriteString("\n## Depth-zero supercuspidal representations\n\n| p | a(chi) | pairs | matches | naive epsilon matches | naive central character matches |\n|---|---|---|---|---|---|\n")
		type agg struct{ pairs, matches, ne, nc int }
		rows := map[[2]int]*agg{}
		var order [][2]int
		for _, x := range r.Supercuspidal {
			tame := 1
			if x.PairConductor == 4 {
				tame = 2
			}
			k := [2]int{x.P, tame}
			if rows[k] == nil {
				rows[k] = &agg{}
				order = append(order, k)
			}
			a := rows[k]
			a.pairs++
			if x.Match {
				a.matches++
			}
			if x.NaiveEpsilonMatch != nil && *x.NaiveEpsilonMatch {
				a.ne++
			}
			if x.NaiveCentralMatch != nil && *x.NaiveCentralMatch {
				a.nc++
			}
		}
		for _, k := range order {
			a := rows[k]
			ac := "at most 1"
			if k[1] == 2 {
				ac = "2"
			}
			fmt.Fprintf(&b, "| %d | %s | %d | %d | %d | %d |\n", k[0], ac, a.pairs, a.matches, a.ne, a.nc)
		}
		b.WriteString("\nWith no rectifier, the epsilon-factor of a pair with a tame twist has the wrong sign, because Delta(p) = -1 enters to the first power. For a twist of conductor exponent 2, Delta(p) enters to the power 2, so the naive epsilon-factor agrees, and only the central character shows the error: det Ind theta is eta_E/F theta|F^x, not theta|F^x.\n")
		b.WriteString("\nExamples:\n\n")
		for _, x := range r.Supercuspidal {
			if x.P == r.Supercuspidal[0].P && (x.Twist == "chi_0" || x.Twist == "chi_1") && strings.Contains(x.Pi, "theta_1)") {
				fmt.Fprintf(&b, "- p = %d, %s, twist %s: GL2 side epsilon = %s; Galois side epsilon = %s.\n", x.P, x.Pi, x.Twist, x.GL2Epsilon, x.GaloisEpsilon)
			}
		}
	}
	b.WriteString("\n## Sources\n\n")
	b.WriteString("- Tate's thesis, and J. Tate, Number theoretic background, Corvallis 1979: the factors of a character, and of a Weil-Deligne representation.\n")
	b.WriteString("- P. Deligne, Les constantes des equations fonctionnelles des fonctions L, 1973: the local constants, and their inductivity in degree 0.\n")
	b.WriteString("- H. Jacquet and R. P. Langlands, Automorphic forms on GL(2), 1970: the factors of the principal series and of the special representations.\n")
	b.WriteString("- R. Godement and H. Jacquet, Zeta functions of simple algebras, 1972: the zeta integrals and their functional equation. The run applies it to the test function above; the derivation is in the comments of lab/e2.go.\n")
	b.WriteString("- W. Fulton and J. Harris, Representation theory, section 5.2: the characters of GL2(F_q).\n")
	b.WriteString("- T. Kondo, On Gaussian sums attached to the general linear groups over finite fields, 1963: the sum over GL2(F_q) is -q(q - 1) times a Gauss sum over k_E. The tests of lab/e2_test.go check this identity for p = 3, 5 and 7.\n")
	b.WriteString("- C. J. Bushnell and G. Henniart, The local Langlands conjecture for GL(2), 2006, chapter 8, section 34: the tame correspondence and the rectifier Delta of a level-zero admissible pair, the unramified quadratic character of E^x.\n")
	b.WriteString("\n## What the run does not do\n\n")
	b.WriteString("- It is a computation, not a proof. An agreement is evidence, and only a certificate that Lean checks makes a theorem (spec 13.4).\n")
	b.WriteString("- The principal series have inducing characters of conductor exponent at most 2 each. The principal series with an inducing character of conductor exponent 3 or 4 are not in the run.\n")
	b.WriteString("- The supercuspidal representations of positive depth are not in the run, except as the twists of a depth-zero one by a character of conductor exponent 2. The ramified admissible pairs (conductor exponent 3) and the minimal unramified pairs of level 1 (conductor exponent 4) need the full tame types and a rectifier of positive level, and the run does not have them.\n")
	b.WriteString("- The case p = 2 of spec 13.3 is not in the run.\n")
	b.WriteString("- For the principal series and for the Steinberg representations with a ramified eta, both sides come to the same Tate factors. Those rows check the bookkeeping, not an independent identity.\n")
	b.WriteString("- It does not check the factors of pairs GL2 x GL2 or the converse theorem, so it does not by itself fix rec on the supercuspidal representations.\n")
	b.WriteString("- It runs in the lgl binary, not in a container with pinned images, and it uses no network and no other tool. It uses no random numbers. The record gives the commit of lgl, so the outputs are reproducible.\n")
	return b.String()
}

func ints(xs []int64) string {
	if len(xs) == 0 {
		return "none"
	}
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// LoadRun reads record.json of a run directory. The id must be the name of the directory.
func LoadRun(dir string) (*Record, error) {
	data, err := os.ReadFile(filepath.Join(dir, "record.json")) // #nosec G304 -- a file of the content repository
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	if rec.ID == "" || rec.ID != filepath.Base(dir) {
		return nil, fmt.Errorf("%s: the id %q of record.json is not the name of the directory", dir, rec.ID)
	}
	return &rec, nil
}

// LoadRuns reads each run of the directory, sorted by id. A missing directory has no runs.
func LoadRuns(dir string) ([]*Record, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Record
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rec, err := LoadRun(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// SpecInfo is a spec file of lab/specs, for lgl lab list. Err is set when the file does not parse.
type SpecInfo struct {
	Path string
	Spec *Spec
	Err  error
}

// LoadSpecs reads each YAML file of the directory. A missing directory has no specs.
func LoadSpecs(dir string) ([]SpecInfo, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SpecInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		s, err := LoadSpec(path)
		out = append(out, SpecInfo{Path: path, Spec: s, Err: err})
	}
	return out, nil
}

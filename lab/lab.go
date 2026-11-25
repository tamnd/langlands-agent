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

// Experiments are the first experiments of spec 13.3. Only E1 has code yet.
var Experiments = []Experiment{
	{"E1", "small cases of local reciprocity for Q_p, p <= 7, against Lubin-Tate", "library, S1", true},
	{"E2", "the explicit LLC for GL2(Q_p) of depth at most 2, with the factors of pairs", "R2", false},
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
	Name       string   `yaml:"name"`
	Experiment string   `yaml:"experiment"`
	Question   string   `yaml:"question"`
	Tool       string   `yaml:"tool"`
	Node       string   `yaml:"node"`
	Nodes      []string `yaml:"nodes,omitempty"`
	Inputs     E1Inputs `yaml:"inputs"`
	Outputs    []string `yaml:"outputs"`
	Notes      string   `yaml:"notes,omitempty"`

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
	if err := s.Inputs.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

// LoadSpec reads a spec file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- a spec that the user names
	if err != nil {
		return nil, err
	}
	return ParseSpec(path, data)
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
		Go: opt.Go, Container: "none", Seed: s.Inputs.Seed, Host: opt.Host, Started: start.Format(time.RFC3339), Files: map[string]string{},
	}
	rec.ID = start.Format("2006-01-02") + "-" + s.Name + "-" + s.Hash[:8]
	dir := filepath.Join(opt.Runs, rec.ID)
	if _, err := os.Stat(dir); err == nil && !opt.Force {
		return nil, fmt.Errorf("lab: the run %s exists; use -force to replace it", dir)
	}
	res, err := RunE1(s.Inputs)
	if err != nil {
		return nil, err
	}
	rec.Mismatches, rec.Gaps = res.Mismatches(), res.Gaps()
	rec.Result = e1Result(res)
	files := map[string][]byte{
		"quadratic.json":  marshal(res.Quadratic),
		"cyclotomic.json": marshal(res.Cyclotomic),
	}
	rec.WallSeconds = float64(opt.Now().Sub(start).Milliseconds()) / 1000
	files["summary.md"] = []byte(e1Summary(s, rec, res))
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
	fmt.Fprintf(&b, "- Quadratic fields: for each Q_p(sqrt d), the run computes the norms a^2 - d b^2 with |a|, |b| <= %d and finds their classes in Q_p^x modulo squares.\n", s.Inputs.Box)
	fmt.Fprintf(&b, "- Cyclotomic fields: for each Q_p(zeta_{p^n}) with p^n <= %d, the run computes the norm of 1 - zeta and of %d random elements of Z[zeta] with coefficients in [-%d, %d] (seed %d). Each norm is an exact determinant. The prediction is that the unit part of each norm is 1 mod p^n. The run also records which classes of (1 + p^n Z_p) / (1 + p^(n+1) Z_p) the norms reach.\n\n", s.Inputs.MaxConductor, s.Inputs.Samples, s.Inputs.Coefficients, s.Inputs.Coefficients, s.Inputs.Seed)
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

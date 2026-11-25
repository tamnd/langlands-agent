// Package blueprint reads the map of the langlands project: the nodes, the routes, the paper records and the Lean records.
//
// The map is a directed graph with no cycle. A node is a statement or a definition. An edge goes from a node to a node that it uses. The status of a node is computed from the Lean records of langlands-lean and is never written by hand.
package blueprint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Kinds are the kinds of a node.
var Kinds = []string{"definition", "theorem", "lemma", "conjecture", "example", "counterexample", "hypothesis", "check", "claim"}

// Tiers are the tiers of the corpus, spec 07.
var Tiers = []string{"T0", "T1", "T2", "T3", "T4", "T5", "T6", "T7", "T8", "T9", "T10", "S"}

// TierNames are the titles of the tiers. S is the tier of the statements of the program.
var TierNames = map[string]string{
	"S":   "The statements of the program",
	"T0":  "Foundations and references",
	"T1":  "Number fields, local fields and adeles",
	"T2":  "Algebraic and reductive groups, root data and L-groups",
	"T3":  "Class field theory and GL(1)",
	"T4":  "Modular and automorphic forms of GL(2)",
	"T5":  "Smooth representations and local Langlands",
	"T6":  "L-functions, converse theorems and functoriality",
	"T7":  "Trace formula, endoscopy and Arthur's classification",
	"T8":  "Shimura varieties, Galois representations and modularity",
	"T9":  "p-adic Hodge theory, p-adic and mod p Langlands, Fargues-Scholze",
	"T10": "Function fields and geometric Langlands",
}

// Status values, spec 08.3.
const (
	Planned   = "unstated"   // no Lean statement yet
	Stated    = "stated"     // the Lean statement builds
	Faithful  = "faithful"   // the statement passed the fidelity checks of its class, spec 06.3
	Defined   = "defined"    // a definition that builds with no sorry
	Proved    = "proved"     // a proof with the standard axioms only, and each node it uses is proved
	ProvedMod = "proved-mod" // a proof that takes named nodes as hypotheses
	Open      = "open"       // an open problem; no proof is claimed
	Refuted   = "refuted"    // the negation is proved
	Claimed   = "claimed"    // a fresh claim of route R1; it is never a hypothesis of a node
)

// LeanUses are the ways in which the project uses a formalization in another project, spec 07.13. A "port" copies the files, a "bridge" proves that two statements agree, and a "read" only reads the code.
var LeanUses = []string{"port", "bridge", "read"}

// ExpertStatuses are the values of expert_status in a paper record, spec 07.13. A person sets the value.
var ExpertStatuses = []string{"accepted", "claimed", "announced", "withdrawn", "disputed"}

// LeanStatuses are the values of lean_status in a paper record, spec 07.13.
var LeanStatuses = []string{"none", "stated", "port-available", "proved"}

// ResultKinds are the kinds of a numbered result of a paper, spec 07.13.
var ResultKinds = []string{"theorem", "definition", "conjecture", "claim", "example"}

// Statuses lists the status values in the order of the site.
var Statuses = []string{Proved, ProvedMod, Defined, Faithful, Stated, Open, Claimed, Refuted, Planned}

// StandardAxioms are the axioms that a proof may use.
var StandardAxioms = map[string]bool{"propext": true, "Classical.choice": true, "Quot.sound": true}

var tagRE = regexp.MustCompile(`^LG[0-9A-HJKMNP-TV-Z]{8}$`)
var routeRE = regexp.MustCompile(`^R[1-9]$`)
var keyRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)
var idRE = regexp.MustCompile(`^T([0-9]|10)-[0-9]{2}$`)

// Source is a place in a paper.
type Source struct {
	Paper string `yaml:"paper" json:"paper"`
	Loc   string `yaml:"loc,omitempty" json:"loc,omitempty"`
}

// Lean names the declarations of a node in langlands-lean.
type Lean struct {
	Statement string `yaml:"statement,omitempty" json:"statement,omitempty"`
	Proof     string `yaml:"proof,omitempty" json:"proof,omitempty"`
	Negation  string `yaml:"negation,omitempty" json:"negation,omitempty"`
}

// Node is one file of blueprint/nodes/.
type Node struct {
	Tag        string   `yaml:"tag" json:"tag"`
	Slug       string   `yaml:"slug,omitempty" json:"slug,omitempty"` // the working name of spec 04, for example llc-gl2
	Kind       string   `yaml:"kind" json:"kind"`
	Title      string   `yaml:"title" json:"title"`
	Statement  string   `yaml:"statement" json:"statement"`
	Sources    []Source `yaml:"sources,omitempty" json:"sources,omitempty"`
	Uses       []string `yaml:"uses,omitempty" json:"uses,omitempty"`
	Hypotheses []string `yaml:"hypotheses,omitempty" json:"hypotheses,omitempty"` // the nodes that a proved-mod proof takes as hypotheses
	Lean       Lean     `yaml:"lean,omitempty" json:"lean,omitempty"`
	Tier       string   `yaml:"tier" json:"tier"`
	Route      []string `yaml:"route,omitempty" json:"route,omitempty"`
	Math2      string   `yaml:"math2,omitempty" json:"math2,omitempty"`
	Notes      string   `yaml:"notes,omitempty" json:"notes,omitempty"`

	File   string   `yaml:"-" json:"-"`
	Status string   `yaml:"-" json:"status"`
	Why    string   `yaml:"-" json:"why,omitempty"` // the reason for the status
	UsedBy []string `yaml:"-" json:"used_by,omitempty"`
}

// Route is one entry of blueprint/routes.yaml.
type Route struct {
	ID          string  `yaml:"id" json:"id"`
	Title       string  `yaml:"title" json:"title"`
	Weight      float64 `yaml:"weight" json:"weight"`
	Idea        string  `yaml:"idea" json:"idea"`
	Obstruction string  `yaml:"obstruction" json:"obstruction"`
	Known       string  `yaml:"known" json:"known"`
	First       string  `yaml:"first" json:"first"`
}

// Read records who read a paper, which version, and on which date. Tag is P (proved in the paper), S (stated with a reference) or U (unchecked), spec 01.
type Read struct {
	Version string `yaml:"version" json:"version"`
	By      string `yaml:"by" json:"by"`
	Date    string `yaml:"date" json:"date"`
	Tag     string `yaml:"tag" json:"tag"`
}

// Result is one numbered result of a paper that the project needs.
type Result struct {
	ID        string `yaml:"id" json:"id"`               // the number in the paper, for example "Chapter VI, Theorem 2"
	Statement string `yaml:"statement" json:"statement"` // the name in Langlands/Statements, spec 06.2
	Node      string `yaml:"node" json:"node,omitempty"` // the node of the map, when there is one
	Kind      string `yaml:"kind" json:"kind"`
	Status    string `yaml:"status" json:"status"` // copied from the node by lgl; it is not a source of truth
}

// LeanElsewhere is a formalization of a result of the paper in another project.
type LeanElsewhere struct {
	Repo    string `yaml:"repo" json:"repo"`
	Commit  string `yaml:"commit" json:"commit,omitempty"`
	Licence string `yaml:"licence" json:"licence"`
	What    string `yaml:"what" json:"what"`
	Use     string `yaml:"use" json:"use"`
}

// HandbookRef is the place of a paper in the handbook, spec 11.
type HandbookRef struct {
	Volume  int `yaml:"volume" json:"volume"`
	Chapter int `yaml:"chapter" json:"chapter,omitempty"` // 0 when the chapter is not fixed yet
}

// Paper is one file of papers/, in the schema of spec 07.13.
type Paper struct {
	ID            string          `yaml:"id" json:"id"` // the corpus id of spec 07, for example T3-04
	Key           string          `yaml:"key" json:"key"`
	Title         string          `yaml:"title" json:"title"`
	Authors       []string        `yaml:"authors,omitempty" json:"authors,omitempty"`
	Where         string          `yaml:"where,omitempty" json:"where,omitempty"`
	Year          int             `yaml:"year,omitempty" json:"year,omitempty"`
	DOI           string          `yaml:"doi" json:"doi,omitempty"`
	Arxiv         string          `yaml:"arxiv" json:"arxiv,omitempty"`
	Also          []string        `yaml:"also,omitempty" json:"also,omitempty"` // the corpus ids of related records
	Read          Read            `yaml:"read" json:"read"`
	Tier          string          `yaml:"tier" json:"tier"`
	Why           string          `yaml:"why" json:"why"`
	ExpertStatus  string          `yaml:"expert_status" json:"expert_status"`
	Uses          []string        `yaml:"uses,omitempty" json:"uses,omitempty"` // the corpus ids of the papers that this paper uses
	Results       []Result        `yaml:"results,omitempty" json:"results,omitempty"`
	LeanElsewhere []LeanElsewhere `yaml:"lean_elsewhere,omitempty" json:"lean_elsewhere,omitempty"`
	LeanStatus    string          `yaml:"lean_status" json:"lean_status"`
	CodeLicence   string          `yaml:"code_licence" json:"code_licence,omitempty"`
	Handbook      *HandbookRef    `yaml:"handbook,omitempty" json:"handbook,omitempty"`
	Route         []string        `yaml:"route,omitempty" json:"route,omitempty"`
	Notes         string          `yaml:"notes,omitempty" json:"notes,omitempty"`

	File  string   `yaml:"-" json:"-"`
	Nodes []string `yaml:"-" json:"nodes,omitempty"` // the nodes that cite the paper
}

// Decl is one declaration in the Lean records.
type Decl struct {
	Kind   string   `json:"kind"`
	File   string   `json:"file,omitempty"`
	Line   int      `json:"line,omitempty"`
	Axioms []string `json:"axioms"`
	Type   string   `json:"type,omitempty"`
}

// Sorry reports whether the declaration depends on sorryAx.
func (d Decl) Sorry() bool {
	for _, a := range d.Axioms {
		if a == "sorryAx" {
			return true
		}
	}
	return false
}

// Clean reports whether the declaration uses the standard axioms only.
func (d Decl) Clean() bool {
	for _, a := range d.Axioms {
		if !StandardAxioms[a] {
			return false
		}
	}
	return true
}

// Fidelity is one entry of records/fidelity.json: the result of the fidelity checks F1 to F5 of spec 06.3 for one Lean statement.
type Fidelity struct {
	Class  string          `json:"class"`  // the class of the statement, which sets the checks it needs
	Checks map[string]bool `json:"checks"` // the result of each check, for example "F1": true
	Pass   bool            `json:"pass"`   // all the checks of the class passed
}

// LeanRecord is records/lean.json: what the build of langlands-lean found.
type LeanRecord struct {
	Repository string          `json:"repository"`
	Commit     string          `json:"commit"`
	Toolchain  string          `json:"toolchain"`
	Mathlib    string          `json:"mathlib"`
	Built      string          `json:"built"`
	Decls      map[string]Decl `json:"decls"`
}

// Project is the whole map.
type Project struct {
	Root   string
	Nodes  []*Node
	Routes []*Route
	Papers []*Paper
	Lean   *LeanRecord
	// Fidelity is records/fidelity.json, keyed by the Lean name of a statement.
	Fidelity map[string]Fidelity

	byTag   map[string]*Node
	byPaper map[string]*Paper
}

// Node returns the node with the tag, or nil.
func (p *Project) Node(tag string) *Node { return p.byTag[tag] }

// Paper returns the paper with the key, or nil.
func (p *Project) Paper(key string) *Paper { return p.byPaper[key] }

// Load reads the map from the root of the content repository. It does not check it; call Check for that.
func Load(root string) (*Project, error) {
	p := &Project{Root: root, byTag: map[string]*Node{}, byPaper: map[string]*Paper{}}
	files, err := yamlFiles(filepath.Join(root, "blueprint", "nodes"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		var n Node
		if err := readYAML(f, &n); err != nil {
			return nil, err
		}
		n.File = rel(root, f)
		p.Nodes = append(p.Nodes, &n)
	}
	if err := readYAML(filepath.Join(root, "blueprint", "routes.yaml"), &p.Routes); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files, err = yamlFiles(filepath.Join(root, "papers"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		var pp Paper
		if err := readYAML(f, &pp); err != nil {
			return nil, err
		}
		pp.File = rel(root, f)
		p.Papers = append(p.Papers, &pp)
	}
	data, err := os.ReadFile(filepath.Join(root, "records", "lean.json")) // #nosec G304 -- a file of the content repository
	switch {
	case err == nil:
		var l LeanRecord
		if err := json.Unmarshal(data, &l); err != nil {
			return nil, fmt.Errorf("records/lean.json: %w", err)
		}
		p.Lean = &l
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	data, err = os.ReadFile(filepath.Join(root, "records", "fidelity.json")) // #nosec G304 -- a file of the content repository
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &p.Fidelity); err != nil {
			return nil, fmt.Errorf("records/fidelity.json: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	sort.Slice(p.Nodes, func(i, j int) bool { return p.Nodes[i].Tag < p.Nodes[j].Tag })
	sort.Slice(p.Papers, func(i, j int) bool { return p.Papers[i].Key < p.Papers[j].Key })
	for _, n := range p.Nodes {
		if _, dup := p.byTag[n.Tag]; !dup {
			p.byTag[n.Tag] = n
		}
	}
	for _, pp := range p.Papers {
		if _, dup := p.byPaper[pp.Key]; !dup {
			p.byPaper[pp.Key] = pp
		}
	}
	p.link()
	p.status()
	return p, nil
}

// link fills the used-by lists and the nodes of each paper.
func (p *Project) link() {
	for _, n := range p.Nodes {
		for _, u := range n.Uses {
			if m := p.byTag[u]; m != nil {
				m.UsedBy = append(m.UsedBy, n.Tag)
			}
		}
		for _, s := range n.Sources {
			if pp := p.byPaper[s.Paper]; pp != nil && !contains(pp.Nodes, n.Tag) {
				pp.Nodes = append(pp.Nodes, n.Tag)
			}
		}
	}
}

func (p *Project) decl(name string) (Decl, bool) {
	if p.Lean == nil || name == "" {
		return Decl{}, false
	}
	d, ok := p.Lean.Decls[name]
	return d, ok
}

// status computes the status of each node. A proof counts only when each node it uses is proved too; otherwise the node is proved-mod.
func (p *Project) status() {
	state := map[string]int{} // 0 not seen, 1 in progress, 2 done
	var visit func(n *Node)
	visit = func(n *Node) {
		if state[n.Tag] != 0 {
			return
		}
		state[n.Tag] = 1
		for _, u := range n.Uses {
			if m := p.byTag[u]; m != nil {
				visit(m)
			}
		}
		n.Status, n.Why = p.own(n)
		if n.Status == Proved {
			for _, u := range n.Uses {
				m := p.byTag[u]
				if m == nil {
					continue
				}
				if m.Status != Proved && m.Status != Defined {
					n.Status, n.Why = ProvedMod, "the proof is complete, but the node "+m.Tag+" that it uses is "+m.Status
					break
				}
			}
		}
		state[n.Tag] = 2
	}
	for _, n := range p.Nodes {
		visit(n)
	}
}

// own is the status of a node from its own Lean declarations, before the nodes it uses.
func (p *Project) own(n *Node) (string, string) {
	if n.Lean.Negation != "" {
		if d, ok := p.decl(n.Lean.Negation); ok && !d.Sorry() && d.Clean() {
			return Refuted, "the negation " + n.Lean.Negation + " is proved"
		}
	}
	if n.Kind == "claim" {
		return Claimed, "a fresh claim of route R1; it is not a hypothesis of any node"
	}
	st, ok := p.decl(n.Lean.Statement)
	if !ok {
		if n.Lean.Statement == "" {
			return Planned, "no Lean statement yet"
		}
		return Planned, "the Lean records have no declaration " + n.Lean.Statement
	}
	if n.Lean.Proof != "" {
		if d, ok := p.decl(n.Lean.Proof); ok {
			switch {
			case d.Sorry():
				return Stated, "the proof " + n.Lean.Proof + " has a sorry"
			case !d.Clean():
				return Stated, "the proof " + n.Lean.Proof + " uses an axiom that is not standard"
			case len(n.Hypotheses) > 0:
				return ProvedMod, "the proof takes " + strings.Join(n.Hypotheses, ", ") + " as hypotheses"
			}
			return Proved, "the proof " + n.Lean.Proof + " uses the standard axioms only"
		}
		return Stated, "the Lean records have no proof " + n.Lean.Proof
	}
	if n.Kind == "definition" {
		if !st.Sorry() && st.Clean() {
			return Defined, "the definition builds with the standard axioms only"
		}
		return Stated, "the definition has a sorry"
	}
	if n.Kind == "conjecture" {
		return Open, "an open problem; no proof is claimed"
	}
	if f, ok := p.Fidelity[n.Lean.Statement]; ok && f.Pass {
		return Faithful, "the statement builds and passed the fidelity checks of the class " + f.Class + "; there is no proof yet"
	}
	if n.Kind == "hypothesis" {
		return Stated, "a published result, stated as a named Prop and not proved yet"
	}
	return Stated, "the statement builds; there is no proof yet"
}

// Counts returns the number of nodes for each status.
func (p *Project) Counts() map[string]int {
	c := map[string]int{}
	for _, n := range p.Nodes {
		c[n.Status]++
	}
	return c
}

// Check returns the problems of the map: bad tags, unknown references, cycles, duplicates and missing fields.
func (p *Project) Check() []string {
	var out []string
	bad := func(file, format string, args ...any) {
		out = append(out, file+": "+fmt.Sprintf(format, args...))
	}
	seen := map[string]string{}
	for _, n := range p.Nodes {
		if !tagRE.MatchString(n.Tag) {
			bad(n.File, "the tag %q is not LG and 8 characters of base 32", n.Tag)
		}
		if f, dup := seen[n.Tag]; dup {
			bad(n.File, "the tag %s is also in %s", n.Tag, f)
		}
		seen[n.Tag] = n.File
		if want := n.Tag + ".yaml"; filepath.Base(n.File) != want {
			bad(n.File, "the file name must be %s", want)
		}
		if !contains(Kinds, n.Kind) {
			bad(n.File, "the kind %q is not one of %s", n.Kind, strings.Join(Kinds, ", "))
		}
		if !contains(Tiers, n.Tier) {
			bad(n.File, "the tier %q is not one of %s", n.Tier, strings.Join(Tiers, ", "))
		}
		if strings.TrimSpace(n.Title) == "" || strings.TrimSpace(n.Statement) == "" {
			bad(n.File, "the node needs a title and a statement")
		}
		for _, u := range n.Uses {
			switch m := p.byTag[u]; {
			case m == nil:
				bad(n.File, "uses %s, which is not a node", u)
			case m.Kind == "claim" && n.Kind != "check":
				bad(n.File, "uses the fresh claim %s; only a node of kind check may use a claim (spec 06.6)", u)
			}
		}
		for _, h := range n.Hypotheses {
			switch m := p.byTag[h]; {
			case m == nil:
				bad(n.File, "the hypothesis %s is not a node", h)
			case m.Kind == "claim":
				bad(n.File, "the hypothesis %s is a fresh claim; a claim is never a hypothesis (spec 06.6)", h)
			}
		}
		for _, s := range n.Sources {
			if p.byPaper[s.Paper] == nil {
				bad(n.File, "cites %s, which is not a paper record", s.Paper)
			}
		}
		for _, r := range n.Route {
			if !routeRE.MatchString(r) || p.route(r) == nil {
				bad(n.File, "the route %q is not in blueprint/routes.yaml", r)
			}
		}
	}
	if c := p.cycle(); c != nil {
		out = append(out, "blueprint/nodes: the uses have a cycle: "+strings.Join(c, " -> "))
	}
	keys := map[string]string{}
	corpus := map[string]string{}
	for _, pp := range p.Papers {
		if !keyRE.MatchString(pp.Key) {
			bad(pp.File, "the key %q has a character that is not a letter, a digit or a hyphen", pp.Key)
		}
		if f, dup := keys[pp.Key]; dup {
			bad(pp.File, "the key %s is also in %s", pp.Key, f)
		}
		keys[pp.Key] = pp.File
		if want := pp.Key + ".yaml"; filepath.Base(pp.File) != want {
			bad(pp.File, "the file name must be %s", want)
		}
		if !idRE.MatchString(pp.ID) {
			bad(pp.File, "the id %q is not a corpus id of spec 07, for example T3-04", pp.ID)
		}
		if f, dup := corpus[pp.ID]; dup && pp.ID != "" {
			bad(pp.File, "the id %s is also in %s", pp.ID, f)
		}
		corpus[pp.ID] = pp.File
		if !contains([]string{"P", "S", "U"}, pp.Read.Tag) {
			bad(pp.File, "the read tag %q is not P, S or U", pp.Read.Tag)
		}
		if !contains(Tiers, pp.Tier) {
			bad(pp.File, "the tier %q is not one of %s", pp.Tier, strings.Join(Tiers, ", "))
		}
		if strings.TrimSpace(pp.Title) == "" || strings.TrimSpace(pp.Why) == "" {
			bad(pp.File, "the record needs a title and a reason")
		}
		if !contains(ExpertStatuses, pp.ExpertStatus) {
			bad(pp.File, "the expert status %q is not one of %s", pp.ExpertStatus, strings.Join(ExpertStatuses, ", "))
		}
		if !contains(LeanStatuses, pp.LeanStatus) {
			bad(pp.File, "the lean status %q is not one of %s", pp.LeanStatus, strings.Join(LeanStatuses, ", "))
		}
		for _, r := range pp.Results {
			if !contains(ResultKinds, r.Kind) {
				bad(pp.File, "the result %q has the kind %q, which is not one of %s", r.ID, r.Kind, strings.Join(ResultKinds, ", "))
			}
			if r.Node != "" && p.byTag[r.Node] == nil {
				bad(pp.File, "the result %q names the node %s, which is not a node", r.ID, r.Node)
			}
		}
		for _, l := range pp.LeanElsewhere {
			if !contains(LeanUses, l.Use) {
				bad(pp.File, "the use %q of %s is not one of %s", l.Use, l.Repo, strings.Join(LeanUses, ", "))
			}
		}
	}
	for _, pp := range p.Papers {
		for _, list := range [][]string{pp.Also, pp.Uses} {
			for _, id := range list {
				if !idRE.MatchString(id) {
					bad(pp.File, "names the paper %q, which is not a corpus id of spec 07", id)
				}
			}
		}
	}
	ids := map[string]bool{}
	for _, r := range p.Routes {
		if !routeRE.MatchString(r.ID) || ids[r.ID] {
			out = append(out, fmt.Sprintf("blueprint/routes.yaml: the route id %q is bad or used twice", r.ID))
		}
		ids[r.ID] = true
	}
	return out
}

func (p *Project) route(id string) *Route {
	for _, r := range p.Routes {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// cycle returns a cycle of the uses, or nil.
func (p *Project) cycle() []string {
	state := map[string]int{}
	var stack []string
	var found []string
	var visit func(t string) bool
	visit = func(t string) bool {
		state[t] = 1
		stack = append(stack, t)
		if n := p.byTag[t]; n != nil {
			for _, u := range n.Uses {
				if state[u] == 1 {
					for i, s := range stack {
						if s == u {
							found = append(append([]string(nil), stack[i:]...), u)
						}
					}
					return true
				}
				if state[u] == 0 && p.byTag[u] != nil && visit(u) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[t] = 2
		return false
	}
	for _, n := range p.Nodes {
		if state[n.Tag] == 0 && visit(n.Tag) {
			return found
		}
	}
	return nil
}

func yamlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- a file of the content repository
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func rel(root, f string) string {
	r, err := filepath.Rel(root, f)
	if err != nil {
		return f
	}
	return filepath.ToSlash(r)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

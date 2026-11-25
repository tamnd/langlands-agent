// Package evalset reads, checks, makes and scores the evaluation sets of spec 10.9: the tasks of eval/tasks and the statements of eval/statements.
//
// A task has a known answer: a declaration of Mathlib, of the NYLL port or of the project, named by its repository, commit, path and full name. A statement has a reviewed Lean statement and a list of wrong Lean statements that the fidelity battery must reject. lgl does not call a model here. A runner writes an answers file, and Score compares it with the known answers.
package evalset

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The targets of spec 10.9.
const (
	TargetTasks      = 200
	TargetStatements = 50
	TargetWrong      = 50
)

// Repos are the repositories that a reference can name. The key is the value of the repo field.
var Repos = map[string]string{
	"mathlib":        "https://github.com/leanprover-community/mathlib4",
	"langlands-lean": "https://github.com/tamnd/langlands-lean",
	"nyll":           "https://github.com/n-yamaguchi-0729/ClassFieldTheory",
}

// Kinds are the kinds of a task.
var Kinds = []string{"prove", "state", "repair"}

// Origins are the places a task comes from.
var Origins = map[string]string{
	"mathlib-numbertheory":         "a lemma of Mathlib/NumberTheory with its proof removed",
	"mathlib-representationtheory": "a lemma of Mathlib/RepresentationTheory with its proof removed",
	"nyll":                         "a statement of the NYLL project, from its docstring",
	"nyll-port":                    "a failed build of the NYLL port at the project pin",
	"project":                      "a theorem of the project history, with its proof removed",
}

// Checks are the checks that a task or a statement can name.
var Checks = map[string]string{
	"build":           "the file with the answer builds at the commit of the answer",
	"no-sorry":        "the answer has no sorry",
	"standard-axioms": "the answer uses only propext, Classical.choice and Quot.sound",
	"same-statement":  "the statement of the declaration does not change",
	"equiv-reference": "the answer is equivalent to the reference statement, by a Lean proof in both directions",
}

// Traps are the kinds of mistake of a wrong statement.
var Traps = map[string]string{
	"normalization":     "the wrong normalization, such as the arithmetic Frobenius in place of the geometric one",
	"missing-condition": "a hypothesis or a condition of the statement is missing",
	"quantifier":        "a wrong quantifier, such as exists in place of exists unique",
	"universe":          "a wrong universe, or a statement for fewer types",
	"field-class":       "a wrong class of fields, rings or groups",
	"constant":          "a wrong constant, exponent or bound",
	"coercion":          "the arithmetic is done in the wrong type, such as a division in the integers in place of the reals",
	"sign":              "a wrong sign or a wrong inverse",
	"vacuous":           "the statement is true for a trivial reason, or the condition can never hold",
	"wrong-object":      "the statement is about a different object, such as a different measure or a different map",
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Ref names a declaration in a repository at a commit. Lines and SHA256 are optional. When they are given, they pin the text of the declaration.
type Ref struct {
	Repo   string `yaml:"repo" json:"repo"`
	Commit string `yaml:"commit" json:"commit"`
	Path   string `yaml:"path" json:"path"`
	Decl   string `yaml:"decl" json:"decl"`
	Lines  []int  `yaml:"lines,omitempty,flow" json:"lines,omitempty"`
	SHA256 string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// Task is one task of eval/tasks.
type Task struct {
	ID         string   `yaml:"id"`
	Kind       string   `yaml:"kind"`
	Origin     string   `yaml:"origin"`
	Title      string   `yaml:"title"`
	Source     Ref      `yaml:"source"`
	Statement  string   `yaml:"statement,omitempty"` // prove: the Lean statement, with sorry as its proof
	Informal   string   `yaml:"informal,omitempty"`  // state: the informal statement
	Broken     string   `yaml:"broken,omitempty"`    // repair: the declaration that does not build
	Error      string   `yaml:"error,omitempty"`     // repair: why the build fails
	Answer     Ref      `yaml:"answer"`
	Checks     []string `yaml:"checks,flow"`
	ProofLines int      `yaml:"proof_lines,omitempty"`
	Difficulty string   `yaml:"difficulty,omitempty"`

	Path string `yaml:"-"`
}

// Wrong is a subtly wrong Lean statement that the fidelity battery must reject.
type Wrong struct {
	ID     string `yaml:"id"`
	Trap   string `yaml:"trap"`
	Reason string `yaml:"reason"`
	Lean   string `yaml:"lean"`
	Ref    *Ref   `yaml:"ref,omitempty"` // a declaration that states or refutes the wrong statement, such as one of Langlands/Fidelity
}

// Reviewed is the reviewed Lean statement of an informal statement.
type Reviewed struct {
	Ref  `yaml:",inline"`
	Lean string `yaml:"lean"`
}

// Statement is one entry of eval/statements.
type Statement struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Informal string   `yaml:"informal"`
	Reviewed Reviewed `yaml:"reviewed"`
	Checks   []string `yaml:"checks,flow"`
	Wrong    []Wrong  `yaml:"wrong,omitempty"`

	Path string `yaml:"-"`
}

// Sets is the content of an eval directory.
type Sets struct {
	Dir        string
	Tasks      []Task
	Statements []Statement
	Hash       string // the sha256 of the files, in the order of their paths
}

// Load reads dir/tasks/*.yaml and dir/statements/*.yaml. A missing folder is an empty set. A file that does not parse is an error.
func Load(dir string) (*Sets, error) {
	s := &Sets{Dir: dir}
	h := sha256.New()
	taskFiles, err := yamlFiles(filepath.Join(dir, "tasks"))
	if err != nil {
		return nil, err
	}
	for _, p := range taskFiles {
		b, err := os.ReadFile(p) // #nosec G304 -- a file of the eval directory
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(rel(dir, p)))
		h.Write(b)
		var t Task
		if err := strictYAML(b, &t); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		t.Path = p
		s.Tasks = append(s.Tasks, t)
	}
	stFiles, err := yamlFiles(filepath.Join(dir, "statements"))
	if err != nil {
		return nil, err
	}
	for _, p := range stFiles {
		b, err := os.ReadFile(p) // #nosec G304 -- a file of the eval directory
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(h, "%s\n", filepath.ToSlash(rel(dir, p)))
		h.Write(b)
		var st Statement
		if err := strictYAML(b, &st); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		st.Path = p
		s.Statements = append(s.Statements, st)
	}
	s.Hash = hex.EncodeToString(h.Sum(nil))
	return s, nil
}

func rel(dir, p string) string {
	if r, err := filepath.Rel(dir, p); err == nil {
		return r
	}
	return p
}

func strictYAML(b []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func yamlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
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

// WrongCount returns the number of wrong statements.
func (s *Sets) WrongCount() int {
	n := 0
	for _, st := range s.Statements {
		n += len(st.Wrong)
	}
	return n
}

// KindCounts returns the number of tasks of each kind and of each origin.
func (s *Sets) KindCounts() (kinds, origins map[string]int) {
	kinds, origins = map[string]int{}, map[string]int{}
	for _, t := range s.Tasks {
		kinds[t.Kind]++
		origins[t.Origin]++
	}
	return kinds, origins
}

// Problem is one failed check of a file.
type Problem struct {
	Path string
	Msg  string
}

// String gives the problem as "path: message".
func (p Problem) String() string { return p.Path + ": " + p.Msg }

// Validate checks the fields of every task and statement and the uniqueness of the ids. It does not read the repositories; see CheckRefs.
func (s *Sets) Validate() []Problem {
	var out []Problem
	seen := map[string]string{}
	add := func(path, format string, a ...any) { out = append(out, Problem{path, fmt.Sprintf(format, a...)}) }
	useID := func(path, id string) {
		if !idPattern.MatchString(id) || len(id) > 80 {
			add(path, "the id %q is not lowercase words with dashes of at most 80 characters", id)
		}
		if other, ok := seen[id]; ok {
			add(path, "the id %s is also in %s", id, other)
		}
		seen[id] = path
	}
	for _, t := range s.Tasks {
		p := t.Path
		useID(p, t.ID)
		if base := strings.TrimSuffix(filepath.Base(p), ".yaml"); base != t.ID {
			add(p, "the file name must be the id %s.yaml", t.ID)
		}
		if _, ok := Origins[t.Origin]; !ok {
			add(p, "the origin %q is not one of %s", t.Origin, keys(Origins))
		}
		checkText(add, p, "title", t.Title, true)
		checkRef(add, p, "source", t.Source)
		checkRef(add, p, "answer", t.Answer)
		checkChecks(add, p, t.Checks)
		switch t.Kind {
		case "prove":
			if strings.TrimSpace(t.Statement) == "" {
				add(p, "a prove task needs a statement")
			} else if !strings.Contains(t.Statement, "sorry") {
				add(p, "the statement of a prove task has sorry as its proof")
			}
		case "state":
			if strings.TrimSpace(t.Informal) == "" {
				add(p, "a state task needs an informal statement")
			}
			checkText(add, p, "informal", t.Informal, true)
		case "repair":
			if strings.TrimSpace(t.Broken) == "" {
				add(p, "a repair task needs the broken declaration")
			}
			checkText(add, p, "error", t.Error, true)
		default:
			add(p, "the kind %q is not one of %s", t.Kind, strings.Join(Kinds, ", "))
		}
		if t.Answer.Decl == "" {
			add(p, "the answer needs a declaration name")
		}
	}
	for _, st := range s.Statements {
		p := st.Path
		useID(p, st.ID)
		if base := strings.TrimSuffix(filepath.Base(p), ".yaml"); base != st.ID {
			add(p, "the file name must be the id %s.yaml", st.ID)
		}
		checkText(add, p, "title", st.Title, true)
		checkText(add, p, "informal", st.Informal, true)
		checkRef(add, p, "reviewed", st.Reviewed.Ref)
		if strings.TrimSpace(st.Reviewed.Lean) == "" {
			add(p, "the reviewed statement needs its Lean text")
		}
		checkChecks(add, p, st.Checks)
		for _, w := range st.Wrong {
			useID(p, w.ID)
			if !strings.HasPrefix(w.ID, st.ID+"-") {
				add(p, "the id of the wrong statement %s must start with %s-", w.ID, st.ID)
			}
			if _, ok := Traps[w.Trap]; !ok {
				add(p, "%s: the trap %q is not one of %s", w.ID, w.Trap, keys(Traps))
			}
			checkText(add, p, w.ID+" reason", w.Reason, true)
			if strings.TrimSpace(w.Lean) == "" {
				add(p, "%s: the wrong statement needs its Lean text", w.ID)
			} else if SameText(w.Lean, st.Reviewed.Lean) {
				add(p, "%s: the wrong statement is the reviewed statement", w.ID)
			}
			if w.Ref != nil {
				checkRef(add, p, w.ID+" ref", *w.Ref)
			}
		}
	}
	return out
}

func checkText(add func(string, string, ...any), path, field, text string, oneLine bool) {
	switch {
	case strings.TrimSpace(text) == "":
		add(path, "%s is empty", field)
	case oneLine && strings.Contains(strings.TrimSpace(text), "\n"):
		add(path, "%s must be one line", field)
	case strings.ContainsAny(text, "—–"):
		add(path, "%s has an em dash or an en dash", field)
	}
}

func checkRef(add func(string, string, ...any), path, field string, r Ref) {
	if _, ok := Repos[r.Repo]; !ok {
		add(path, "%s: the repo %q is not one of %s", field, r.Repo, keys(Repos))
	}
	if !commitPattern.MatchString(r.Commit) {
		add(path, "%s: the commit %q is not a full commit hash", field, r.Commit)
	}
	if r.Path == "" || filepath.IsAbs(r.Path) || strings.Contains(r.Path, "..") || !strings.HasSuffix(r.Path, ".lean") {
		add(path, "%s: the path %q is not a relative path of a Lean file", field, r.Path)
	}
	if r.Decl == "" {
		add(path, "%s: the declaration name is empty", field)
	}
	if len(r.Lines) != 0 && (len(r.Lines) != 2 || r.Lines[0] < 1 || r.Lines[1] < r.Lines[0]) {
		add(path, "%s: lines must be [first, last]", field)
	}
	if r.SHA256 != "" && len(r.Lines) == 0 {
		add(path, "%s: a sha256 needs the lines", field)
	}
}

func checkChecks(add func(string, string, ...any), path string, checks []string) {
	if len(checks) == 0 {
		add(path, "the checks are empty")
	}
	for _, c := range checks {
		if _, ok := Checks[c]; !ok {
			add(path, "the check %q is not one of %s", c, keys(Checks))
		}
	}
}

func keys[V any](m map[string]V) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

// HashLines returns the sha256 of a declaration text, as a Ref stores it.
func HashLines(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

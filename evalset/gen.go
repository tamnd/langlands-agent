package evalset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Quotas are the numbers of tasks of each origin that Generate makes. The number of tasks from mathlib-numbertheory fills the rest up to Total.
type Quotas struct {
	Total, RepresentationTheory, NYLL, Repair, Project int
}

// DefaultQuotas are the quotas of the first set.
var DefaultQuotas = Quotas{Total: TargetTasks, RepresentationTheory: 40, NYLL: 20, Repair: 15, Project: 15}

// Rename is one change of a Mathlib name between the NYLL source pin and the project pin, from the changes of manifests/ported.yaml. Pin is the name at the project pin, and Upstream is the name in the source.
type Rename struct{ Pin, Upstream string }

// Renames are the renames of the NYLL port that make a build fail when the port uses the upstream name. Each upstream name is not in the Mathlib of the project pin. The list leaves out the renames whose upstream name is also a name at the pin, such as Countable.of_moduleFinite.
var Renames = []Rename{
	{"Abelianization.commutator_subset_ker", "Group.Abelianization.commutator_le_ker"},
	{"CharP.subsemiring", "SubsemiringClass.charP_iff"},
	{"FiniteField.X_pow_card_sub_X_ne_zero", "Polynomial.X_pow_sub_X_ne_zero"},
	{"IsLocalRing.local_hom_TFAE", "IsLocalRing.local_hom_tfae"},
	{"isPGroup'", "isPGroup"},
	{"IntermediateField.le_iff_le", "IntermediateField.le_fixedField_iff_le_fixingSubgroup"},
	{"PowerSeries.X_irreducible", "PowerSeries.irreducible_X"},
	{"PowerSeries.X_prime", "PowerSeries.prime_X"},
	{"Sylow.exists_subgroup_card_pow_prime_le", "Sylow.exists_subgroup_card_pow_prime_ge"},
	{"WittVector.RecursionBase.solution_nonzero", "WittVector.RecursionBase.solution_ne_zero"},
	{"WittVector.p_nonzero", "WittVector.p_ne_zero"},
	{"nnnormHom", "nnNormHom"},
	{"coe_nnnorm", "coe_nnNorm"},
	{"continuous_nnnorm", "continuous_nnNorm"},
	{"div_left_inj'", "div_left_inj₀"},
	{"Submodule.closed_of_finiteDimensional", "Submodule.isClosed_of_finiteDimensional"},
	{"FractionalIdeal.count_coe", "FractionalIdeal.count_coeIdeal"},
	{"padicNorm.eq_zpow_of_nonzero", "padicNorm.eq_zpow_of_ne_zero"},
	{"_root_.abelianizationCongr_symm", "Group.abelianizationCongr_symm"},
}

// The NYLL port: the folder in langlands-lean and the upstream commit.
const (
	nyllFolder = "Langlands/Ported/NYLL"
	nyllCommit = "fc644f42dade5bdc7157fc3ef2eef4dbf2d6a565"
)

var provenance = regexp.MustCompile("-- Ported from n-yamaguchi-0729/ClassFieldTheory at commit ([0-9a-f]{40}), file `([^`]+)`")

// GenOptions are the inputs of Generate.
type GenOptions struct {
	Source     *Source // with the directories of mathlib and langlands-lean
	Quotas     Quotas
	LeanCommit string // the commit of langlands-lean; empty for HEAD
}

type candidate struct {
	task Task
	file string
	rank string // the sha256 of the full name; it orders the candidates
	pick string // a group, such as a rename; the selection takes one of each group first
}

func rank(name string) string {
	s := sha256.Sum256([]byte(name))
	return hex.EncodeToString(s[:])
}

// Generate makes the tasks of eval/tasks from the checkouts. The same checkouts at the same commits give the same tasks.
func Generate(o GenOptions) ([]Task, error) {
	src := o.Source
	if !src.Has("mathlib") || !src.Has("langlands-lean") {
		return nil, errors.New("gen needs the directories of mathlib and langlands-lean")
	}
	mathlib, err := commitOf(src, "mathlib", "")
	if err != nil {
		return nil, err
	}
	lean, err := commitOf(src, "langlands-lean", o.LeanCommit)
	if err != nil {
		return nil, err
	}
	q := o.Quotas
	nt, err := mathlibCandidates(src, mathlib, "Mathlib/NumberTheory", "nt", "mathlib-numbertheory")
	if err != nil {
		return nil, err
	}
	rt, err := mathlibCandidates(src, mathlib, "Mathlib/RepresentationTheory", "rt", "mathlib-representationtheory")
	if err != nil {
		return nil, err
	}
	nyll, fix, err := nyllCandidates(src, lean)
	if err != nil {
		return nil, err
	}
	proj, err := projectCandidates(src, lean)
	if err != nil {
		return nil, err
	}
	var out []Task
	used := map[string]bool{}
	take := func(cs []candidate, n int) {
		for _, c := range spread(cs, n, used) {
			out = append(out, c.task)
		}
	}
	take(rt, q.RepresentationTheory)
	take(nyll, q.NYLL)
	take(fix, q.Repair)
	take(proj, q.Project)
	take(nt, q.Total-len(out))
	if len(out) < q.Total {
		return out, fmt.Errorf("only %d tasks; the target is %d", len(out), q.Total)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func commitOf(src *Source, repo, commit string) (string, error) {
	if commit != "" && commitPattern.MatchString(commit) {
		return commit, nil
	}
	dir := src.Dirs[repo]
	if !isGit(dir) {
		return "", fmt.Errorf("%s is not a git checkout, so gen cannot pin its commit", dir)
	}
	ref := "HEAD"
	if commit != "" {
		ref = commit
	}
	out, err := git(dir, "rev-parse", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// spread takes n candidates: one of each group first, then one of each file, then the rest, in the order of their rank. It skips the ids in used.
func spread(cs []candidate, n int, used map[string]bool) []candidate {
	sort.Slice(cs, func(i, j int) bool { return cs[i].rank < cs[j].rank })
	var out []candidate
	taken := map[int]bool{}
	pass := func(key func(candidate) string) {
		seen := map[string]bool{}
		for _, c := range out {
			seen[key(c)] = true
		}
		for i, c := range cs {
			if len(out) >= n {
				return
			}
			k := key(c)
			if taken[i] || used[c.task.ID] || seen[k] {
				continue
			}
			seen[k] = true
			taken[i] = true
			used[c.task.ID] = true
			out = append(out, c)
		}
	}
	pass(func(c candidate) string { return c.pick })
	pass(func(c candidate) string { return c.file })
	pass(func(c candidate) string { return c.file + "\x00" + c.rank[:1] })
	pass(func(c candidate) string { return c.rank })
	return out
}

// proveCandidate is true for a theorem with a short statement and a short proof.
func proveCandidate(d Decl, maxProofLines int) bool {
	if d.Keyword != "theorem" && d.Keyword != "lemma" || !d.HasProof || d.Private || d.Deprecated {
		return false
	}
	if strings.Contains(d.Proof, "sorry") || strings.Contains(d.Header, "«") {
		return false
	}
	h := utf8.RuneCountInString(d.Header)
	p := utf8.RuneCountInString(d.Proof)
	return plain(d.Header) && plain(d.Proof) && h >= 30 && h <= 400 && p >= 10 && p <= 700 && d.ProofLines >= 1 && d.ProofLines <= maxProofLines
}

// plain is false for a text with a character outside the basic plane, such as 𝟙 or 𝒪. The YAML encoder writes such a text as an escaped string, which is hard to read, so the generator leaves these declarations out.
func plain(text string) bool {
	for _, r := range text {
		if r > 0xFFFF {
			return false
		}
	}
	return true
}

func difficulty(lines int) string {
	switch {
	case lines <= 1:
		return "easy"
	case lines <= 4:
		return "medium"
	}
	return "hard"
}

var slugDrop = regexp.MustCompile(`[^a-z0-9]+`)

// slug makes an id part of a Lean name.
func slug(name string) string {
	var b strings.Builder
	prev := rune(0)
	for _, r := range name {
		// A capital after a lowercase letter starts a new word, so isPGroup is is-pgroup.
		if r >= 'A' && r <= 'Z' && prev >= 'a' && prev <= 'z' {
			b.WriteByte('-')
		}
		switch r {
		case '₀':
			b.WriteString("0")
		case '₁':
			b.WriteString("1")
		case '₂':
			b.WriteString("2")
		case '\'':
			b.WriteString("-prime")
		default:
			b.WriteRune(r)
		}
		prev = r
	}
	s := strings.Trim(slugDrop.ReplaceAllString(strings.ToLower(b.String()), "-"), "-")
	if len(s) > 64 {
		s = strings.TrimRight(s[:64], "-")
	}
	return s
}

// taskID makes the id of a task from the last two parts of the name and a short hash of the full name.
func taskID(prefix, name string) string {
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	s := slug(strings.Join(parts, "."))
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	return prefix + "-" + s + "-" + rank(name)[:6]
}

func mathlibCandidates(src *Source, commit, folder, prefix, origin string) ([]candidate, error) {
	files, err := src.List("mathlib", commit, folder)
	if err != nil {
		return nil, err
	}
	var out []candidate
	for _, f := range files {
		lines, decls, err := src.File("mathlib", commit, f)
		if err != nil {
			return nil, err
		}
		for _, d := range decls {
			if !proveCandidate(d, 12) {
				continue
			}
			ref := Ref{Repo: "mathlib", Commit: commit, Path: f, Decl: d.Name}
			ans := ref
			ans.Lines = []int{d.Start, d.End}
			ans.SHA256 = HashLines(d.Text(lines))
			t := Task{ID: taskID(prefix, d.Name), Kind: "prove", Origin: origin, Title: "Prove " + d.Name + ".", Source: ref, Statement: StatementOf(d), Answer: ans, Checks: []string{"build", "no-sorry", "standard-axioms", "same-statement"}, ProofLines: d.ProofLines, Difficulty: difficulty(d.ProofLines)}
			out = append(out, candidate{task: t, file: f, rank: rank(d.Name), pick: f})
		}
	}
	return out, nil
}

// upstreamPath returns the path of the source file of a ported file, from its provenance line.
func upstreamPath(lines []string) (commit, path string) {
	for _, l := range lines {
		if m := provenance.FindStringSubmatch(l); m != nil {
			return m[1], m[2]
		}
	}
	return "", ""
}

func nyllCandidates(src *Source, commit string) (state, repair []candidate, err error) {
	files, err := src.List("langlands-lean", commit, nyllFolder)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range files {
		lines, decls, err := src.File("langlands-lean", commit, f)
		if err != nil {
			return nil, nil, err
		}
		upCommit, upPath := upstreamPath(lines)
		if upCommit == "" {
			continue // a file of the port that is not in the source, such as Compat/
		}
		for _, d := range decls {
			ans := Ref{Repo: "langlands-lean", Commit: commit, Path: f, Decl: d.Name, Lines: []int{d.Start, d.End}, SHA256: HashLines(d.Text(lines))}
			up := Ref{Repo: "nyll", Commit: upCommit, Path: upPath, Decl: d.Name}
			if c, ok := stateCandidate(d, f, up, ans); ok {
				state = append(state, c)
			}
			if c, ok := repairCandidate(d, lines, f, up, ans); ok {
				repair = append(repair, c)
			}
		}
	}
	return state, repair, nil
}

func stateCandidate(d Decl, file string, up, ans Ref) (candidate, bool) {
	if d.Keyword != "theorem" && d.Keyword != "lemma" || d.Private || d.Deprecated || d.Doc == "" {
		return candidate{}, false
	}
	n := utf8.RuneCountInString(d.Doc)
	h := utf8.RuneCountInString(d.Header)
	low := strings.ToLower(d.Doc)
	for _, w := range []string{"establishes", "rewriting", "used in", "this lemma", "later versions", "pointwise formula", "special case", "consequence"} {
		if strings.Contains(low, w) {
			return candidate{}, false
		}
	}
	for _, w := range []string{"unfolds", "canonical", "evaluation of", "the chosen"} {
		if strings.HasPrefix(low, w) {
			return candidate{}, false
		}
	}
	if !plain(d.Doc) || !plain(d.Header) {
		return candidate{}, false
	}
	if n < 80 || n > 400 || h < 40 || h > 400 || strings.ContainsAny(d.Doc, "—–") || strings.Contains(d.Doc, "TODO") || strings.Contains(low, "proof") || strings.Contains(low, "auxiliary") || strings.Contains(low, "helper") {
		return candidate{}, false
	}
	t := Task{ID: taskID("nyll", d.Name), Kind: "state", Origin: "nyll", Title: "State " + d.Name + " in Lean from its docstring.", Source: up, Informal: d.Doc, Answer: ans, Checks: []string{"build", "equiv-reference"}}
	return candidate{task: t, file: file, rank: rank(d.Name), pick: file}, true
}

// replaceName replaces the name pin with up in the code of text. It changes a run of the dot parts of a name, so that P.isPGroup'.of_equiv is P.isPGroup.of_equiv, but it does not change a longer part such as isPGroup'_foo. It returns the number of changes.
func replaceName(text, pin, up string) (string, int) {
	masked := maskComments(text)
	pinParts := strings.Split(strings.TrimPrefix(pin, "_root_."), ".")
	root := strings.HasPrefix(pin, "_root_.")
	var b strings.Builder
	n := 0
	i := 0
	for i < len(masked) {
		r, size := utf8.DecodeRuneInString(masked[i:])
		if !identRune(r) {
			b.WriteString(text[i : i+size])
			i += size
			continue
		}
		j := i
		for j < len(masked) {
			r2, s2 := utf8.DecodeRuneInString(masked[j:])
			if !identRune(r2) {
				break
			}
			j += s2
		}
		token := text[i:j]
		parts := strings.Split(token, ".")
		start := -1
		for k := 0; k+len(pinParts) <= len(parts); k++ {
			if root && (k != 1 || parts[0] != "_root_") {
				continue
			}
			match := true
			for m := range pinParts {
				if parts[k+m] != pinParts[m] {
					match = false
					break
				}
			}
			if match {
				start = k
				break
			}
		}
		if start < 0 {
			b.WriteString(token)
		} else {
			if root {
				start = 0
			}
			end := start + len(pinParts)
			if root {
				end++
			}
			newParts := append(append(append([]string{}, parts[:start]...), up), parts[end:]...)
			b.WriteString(strings.Join(newParts, "."))
			n++
		}
		i = j
	}
	return b.String(), n
}

func repairCandidate(d Decl, lines []string, file string, up, ans Ref) (candidate, bool) {
	if d.Private || d.ProofLines > 30 || d.ProofLines == 0 {
		return candidate{}, false
	}
	text := d.Text(lines)
	if !plain(text) {
		return candidate{}, false
	}
	for _, rn := range Renames {
		broken, n := replaceName(text, rn.Pin, rn.Upstream)
		if n == 0 {
			continue
		}
		t := Task{ID: taskID("fix", d.Name), Kind: "repair", Origin: "nyll-port", Title: "Repair " + d.Name + " for the Mathlib of the project pin.", Source: up, Broken: broken + "\n", Error: fmt.Sprintf("The build fails at the project pin: %s is not a name in its Mathlib.", strings.TrimPrefix(rn.Upstream, "_root_.")), Answer: ans, Checks: []string{"build", "no-sorry", "standard-axioms", "same-statement"}, ProofLines: d.ProofLines, Difficulty: difficulty(d.ProofLines)}
		return candidate{task: t, file: file, rank: rank(d.Name), pick: rn.Pin}, true
	}
	return candidate{}, false
}

// projectCandidates are the theorems of langlands-lean outside the ports. The source commit is the first commit with the declaration, and the answer is at the commit of the set.
func projectCandidates(src *Source, commit string) ([]candidate, error) {
	files, err := src.List("langlands-lean", commit, "Langlands")
	if err != nil {
		return nil, err
	}
	var out []candidate
	for _, f := range files {
		if strings.HasPrefix(f, "Langlands/Ported/") {
			continue
		}
		lines, decls, err := src.File("langlands-lean", commit, f)
		if err != nil {
			return nil, err
		}
		for _, d := range decls {
			if !proveCandidate(d, 30) {
				continue
			}
			ans := Ref{Repo: "langlands-lean", Commit: commit, Path: f, Decl: d.Name, Lines: []int{d.Start, d.End}, SHA256: HashLines(d.Text(lines))}
			first := firstCommit(src, commit, f, d)
			source := Ref{Repo: "langlands-lean", Commit: first, Path: f, Decl: d.Name}
			t := Task{ID: taskID("lgl", d.Name), Kind: "prove", Origin: "project", Title: "Prove " + d.Name + ".", Source: source, Statement: StatementOf(d), Answer: ans, Checks: []string{"build", "no-sorry", "standard-axioms", "same-statement"}, ProofLines: d.ProofLines, Difficulty: difficulty(d.ProofLines)}
			out = append(out, candidate{task: t, file: f, rank: rank(d.Name), pick: f})
		}
	}
	return out, nil
}

// firstCommit returns the first commit that has the declaration in the file, or commit when git cannot tell.
func firstCommit(src *Source, commit, file string, d Decl) string {
	dir := src.Dirs["langlands-lean"]
	if !isGit(dir) {
		return commit
	}
	short := d.Name[strings.LastIndex(d.Name, ".")+1:]
	out, err := git(dir, "log", "--reverse", "--format=%H", "-S", d.Keyword+" "+short, commit, "--", file)
	if err != nil {
		return commit
	}
	for _, c := range strings.Fields(string(out)) {
		if _, decls, err := src.File("langlands-lean", c, file); err == nil {
			if _, ok := FindDecl(decls, d.Name); ok {
				return c
			}
		}
	}
	return commit
}

// WriteTasks writes one file for each task to dir, and removes the other task files of dir.
func WriteTasks(dir string, tasks []Task) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	old, err := yamlFiles(dir)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, t := range tasks {
		p := filepath.Join(dir, t.ID+".yaml")
		keep[p] = true
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "# Made by lgl eval gen from %s. Do not edit by hand.\n", Repos[t.Source.Repo])
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(t); err != nil {
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}
		if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
			return err
		}
	}
	for _, p := range old {
		if !keep[p] {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}

package evalset

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Source reads the Lean files of the repositories of the references. A directory with a .git folder is read at the commit of the reference with git show. Another directory is read as it is, and the commit is not checked. The tests use the second kind.
type Source struct {
	Dirs  map[string]string // the repo key of Repos, to a local directory
	cache map[string]*leanFile
}

type leanFile struct {
	lines []string
	decls []Decl
	err   error
}

// NewSource returns a Source for the directories. An empty directory is left out.
func NewSource(dirs map[string]string) *Source {
	s := &Source{Dirs: map[string]string{}, cache: map[string]*leanFile{}}
	for k, v := range dirs {
		if v != "" {
			s.Dirs[k] = v
		}
	}
	return s
}

// Has reports whether the source can read the repository.
func (s *Source) Has(repo string) bool { return s.Dirs[repo] != "" }

func isGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// git runs git in dir and returns its output.
func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) // #nosec G204 -- git with the arguments of lgl, not of a file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Head returns the commit of HEAD of the repository, or "" when the directory is not a git checkout.
func (s *Source) Head(repo string) (string, error) {
	dir := s.Dirs[repo]
	if !isGit(dir) {
		return "", nil
	}
	out, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Read returns the text of a file of the repository at the commit.
func (s *Source) Read(repo, commit, path string) (string, error) {
	dir := s.Dirs[repo]
	if dir == "" {
		return "", fmt.Errorf("no directory for the repo %s", repo)
	}
	if isGit(dir) {
		out, err := git(dir, "show", commit+":"+path)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path))) // #nosec G304 -- a file of a checkout that the user names
	return string(b), err
}

// List returns the Lean files under the folder of the repository at the commit, sorted.
func (s *Source) List(repo, commit, folder string) ([]string, error) {
	dir := s.Dirs[repo]
	var out []string
	if isGit(dir) {
		b, err := git(dir, "ls-tree", "-r", "--name-only", commit, "--", folder)
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasSuffix(l, ".lean") {
				out = append(out, l)
			}
		}
	} else {
		root := filepath.Join(dir, filepath.FromSlash(folder))
		err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !e.IsDir() && strings.HasSuffix(p, ".lean") {
				r, err := filepath.Rel(dir, p)
				if err != nil {
					return err
				}
				out = append(out, filepath.ToSlash(r))
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// File returns the lines and the declarations of a file, from a cache.
func (s *Source) File(repo, commit, path string) ([]string, []Decl, error) {
	key := repo + "\x00" + commit + "\x00" + path
	if f, ok := s.cache[key]; ok {
		return f.lines, f.decls, f.err
	}
	text, err := s.Read(repo, commit, path)
	f := &leanFile{err: err}
	if err == nil {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		f.lines = strings.Split(text, "\n")
		f.decls = ParseLean(text)
	}
	s.cache[key] = f
	return f.lines, f.decls, f.err
}

// Resolve finds the declaration of a reference and checks its lines and its sha256, when the reference has them.
func (s *Source) Resolve(r Ref) (Decl, []string, error) {
	lines, decls, err := s.File(r.Repo, r.Commit, r.Path)
	if err != nil {
		return Decl{}, nil, err
	}
	d, ok := FindDecl(decls, r.Decl)
	if !ok {
		return Decl{}, nil, fmt.Errorf("%s has no declaration %s at %.12s", r.Path, r.Decl, r.Commit)
	}
	if len(r.Lines) == 2 && (r.Lines[0] != d.Start || r.Lines[1] != d.End) {
		return d, lines, fmt.Errorf("%s is at lines %d to %d of %s, not %d to %d", r.Decl, d.Start, d.End, r.Path, r.Lines[0], r.Lines[1])
	}
	if r.SHA256 != "" && HashLines(d.Text(lines)) != r.SHA256 {
		return d, lines, fmt.Errorf("the text of %s in %s does not match its sha256", r.Decl, r.Path)
	}
	return d, lines, nil
}

// RefReport counts the references that CheckRefs read and those it skipped, because their repository has no directory.
type RefReport struct {
	Checked int
	Skipped map[string]int
}

// CheckRefs checks that each reference names a declaration that exists, with the lines and the sha256 of the reference. It also checks that the Lean text of a prove task and of a reviewed statement is the text of the declaration.
func CheckRefs(sets *Sets, src *Source) ([]Problem, RefReport) {
	var out []Problem
	rep := RefReport{Skipped: map[string]int{}}
	add := func(path, format string, a ...any) { out = append(out, Problem{path, fmt.Sprintf(format, a...)}) }
	resolve := func(path, field string, r Ref) (Decl, []string, bool) {
		if !src.Has(r.Repo) {
			rep.Skipped[r.Repo]++
			return Decl{}, nil, false
		}
		rep.Checked++
		d, lines, err := src.Resolve(r)
		if err != nil {
			add(path, "%s: %v", field, err)
			return d, lines, false
		}
		return d, lines, true
	}
	for _, t := range sets.Tasks {
		resolve(t.Path, "source", t.Source)
		d, lines, ok := resolve(t.Path, "answer", t.Answer)
		if !ok {
			continue
		}
		switch t.Kind {
		case "prove":
			if !SameText(StatementOf(d), t.Statement) {
				add(t.Path, "the statement is not the statement of %s with sorry as its proof", t.Answer.Decl)
			}
		case "repair":
			if SameText(t.Broken, d.Text(lines)) {
				add(t.Path, "the broken declaration is the same as the answer")
			}
		}
	}
	for _, st := range sets.Statements {
		d, lines, ok := resolve(st.Path, "reviewed", st.Reviewed.Ref)
		if ok && !SameText(st.Reviewed.Lean, d.Text(lines)) && !SameText(st.Reviewed.Lean, d.Header) {
			add(st.Path, "the reviewed Lean text is not the text of %s", st.Reviewed.Decl)
		}
		for _, w := range st.Wrong {
			if w.Ref != nil {
				resolve(st.Path, w.ID+" ref", *w.Ref)
			}
		}
	}
	return out, rep
}

// StatementOf returns the statement of a prove task for a declaration: its header, with sorry as the proof.
func StatementOf(d Decl) string {
	return d.Header + " := by\n  sorry\n"
}

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tamnd/langlands-agent/evalset"
)

func runEval(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: eval needs a subcommand: list, check, gen, run, compare, key or decl", errUsage)
	}
	switch args[0] {
	case "list":
		return evalList(args[1:], stdout, stderr)
	case "check":
		return evalCheck(args[1:], stdout, stderr)
	case "gen":
		return evalGen(args[1:], stdout, stderr)
	case "run":
		return evalRun(args[1:], stdout, stderr)
	case "compare":
		return evalCompare(args[1:], stdout, stderr)
	case "key":
		return evalKey(args[1:], stdout, stderr)
	case "decl":
		return evalDecl(args[1:], stdout, stderr)
	}
	return fmt.Errorf("%w: unknown eval subcommand %q; use list, check, gen, run, compare, key or decl", errUsage, args[0])
}

// mathlibDir returns the Mathlib checkout: the flag, or the Mathlib package of the Lean checkout.
func mathlibDir(flagValue, lean string) string {
	if flagValue != "" || lean == "" {
		return flagValue
	}
	p := filepath.Join(lean, ".lake", "packages", "mathlib")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return ""
}

func loadSets(dir string) (*evalset.Sets, error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory; give the eval directory with -dir", dir)
	}
	return evalset.Load(dir)
}

func sortedCounts(m map[string]int) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func target(n, want int) string {
	switch {
	case n == want:
		return "the target"
	case n < want:
		return fmt.Sprintf("%d short of the target %d", want-n, want)
	}
	return fmt.Sprintf("%d over the target %d", n-want, want)
}

func evalList(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "eval", "the eval directory, with tasks/ and statements/")
	if err := parse(fs, args); err != nil {
		return err
	}
	s, err := loadSets(*dir)
	if err != nil {
		return err
	}
	kinds, origins := s.KindCounts()
	fmt.Fprintf(stdout, "tasks: %d (%s)\n", len(s.Tasks), target(len(s.Tasks), evalset.TargetTasks))
	for _, k := range evalset.Kinds {
		fmt.Fprintf(stdout, "  %-8s %d\n", k, kinds[k])
	}
	fmt.Fprintln(stdout, "tasks by origin:")
	for _, o := range sortedCounts(origins) {
		fmt.Fprintf(stdout, "  %-30s %d\n", o, origins[o])
	}
	traps := map[string]int{}
	for _, st := range s.Statements {
		for _, w := range st.Wrong {
			traps[w.Trap]++
		}
	}
	fmt.Fprintf(stdout, "statements: %d (%s)\n", len(s.Statements), target(len(s.Statements), evalset.TargetStatements))
	fmt.Fprintf(stdout, "wrong statements: %d (%s)\n", s.WrongCount(), target(s.WrongCount(), evalset.TargetWrong))
	for _, t := range sortedCounts(traps) {
		fmt.Fprintf(stdout, "  %-18s %d\n", t, traps[t])
	}
	fmt.Fprintf(stdout, "scored units: tasks %d, stater %d, battery %d\n", s.Units(evalset.SetTasks), s.Units(evalset.SetStater), s.Units(evalset.SetBattery))
	return nil
}

func evalCheck(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "eval", "the eval directory, with tasks/ and statements/")
	lean := fs.String("lean", "", "a checkout of langlands-lean, to check the references to it")
	mathlib := fs.String("mathlib", "", "a checkout of Mathlib, to check the references to it (default: the Mathlib package of -lean)")
	if err := parse(fs, args); err != nil {
		return err
	}
	s, err := loadSets(*dir)
	if err != nil {
		return err
	}
	problems := s.Validate()
	src := evalset.NewSource(map[string]string{"langlands-lean": *lean, "mathlib": mathlibDir(*mathlib, *lean)})
	refProblems, rep := evalset.CheckRefs(s, src)
	problems = append(problems, refProblems...)
	for _, p := range problems {
		fmt.Fprintln(stdout, p)
	}
	fmt.Fprintf(stdout, "tasks: %d (%s); statements: %d (%s); wrong statements: %d (%s)\n", len(s.Tasks), target(len(s.Tasks), evalset.TargetTasks), len(s.Statements), target(len(s.Statements), evalset.TargetStatements), s.WrongCount(), target(s.WrongCount(), evalset.TargetWrong))
	var skipped []string
	for _, r := range sortedCounts(rep.Skipped) {
		skipped = append(skipped, fmt.Sprintf("%s %d", r, rep.Skipped[r]))
	}
	if len(skipped) > 0 {
		fmt.Fprintf(stdout, "references read: %d; not read, with no checkout: %s\n", rep.Checked, strings.Join(skipped, ", "))
	} else {
		fmt.Fprintf(stdout, "references read: %d\n", rep.Checked)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problems in %s", len(problems), *dir)
	}
	fmt.Fprintln(stdout, "ok")
	return nil
}

func evalGen(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "eval", "the eval directory; gen writes its tasks/ folder")
	lean := fs.String("lean", "", "a git checkout of langlands-lean")
	mathlib := fs.String("mathlib", "", "a git checkout of Mathlib (default: the Mathlib package of -lean)")
	commit := fs.String("commit", "", "the commit of langlands-lean (default: HEAD)")
	n := fs.Int("n", evalset.TargetTasks, "the number of tasks")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *lean == "" {
		return fmt.Errorf("%w: eval gen needs -lean", errUsage)
	}
	q := evalset.DefaultQuotas
	q.Total = *n
	src := evalset.NewSource(map[string]string{"langlands-lean": *lean, "mathlib": mathlibDir(*mathlib, *lean)})
	tasks, err := evalset.Generate(evalset.GenOptions{Source: src, Quotas: q, LeanCommit: *commit})
	if err != nil {
		return err
	}
	out := filepath.Join(*dir, "tasks")
	if err := evalset.WriteTasks(out, tasks); err != nil {
		return err
	}
	origins := map[string]int{}
	for _, t := range tasks {
		origins[t.Origin]++
	}
	fmt.Fprintf(stdout, "wrote %d tasks to %s\n", len(tasks), out)
	for _, o := range sortedCounts(origins) {
		fmt.Fprintf(stdout, "  %-30s %d\n", o, origins[o])
	}
	return nil
}

var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

func evalRun(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "eval", "the eval directory, with tasks/ and statements/")
	answers := fs.String("answers", "", "the answers file of a runner, to score")
	dry := fs.Bool("dry-run", false, "print the units and the score, and write no file")
	label := fs.String("label", "", "the label of the result file (default: the model of the answers)")
	date := fs.String("date", "", "the date of the result, YYYY-MM-DD (default: today, UTC)")
	out := fs.String("out", "", "the directory of the result file (default: results/ of -dir)")
	force := fs.Bool("force", false, "replace a result file with the same name")
	if err := parse(fs, args); err != nil {
		return err
	}
	s, err := loadSets(*dir)
	if err != nil {
		return err
	}
	if *answers == "" {
		if !*dry {
			return fmt.Errorf("%w: eval run needs -answers, or -dry-run", errUsage)
		}
		fmt.Fprintf(stdout, "dry run of %s (sets sha256 %.12s); no file written\n", *dir, s.Hash)
		for _, set := range evalset.SetNames {
			fmt.Fprintf(stdout, "  %-8s %d units\n", set, s.Units(set))
		}
		fmt.Fprintln(stdout, "a runner writes an answers file for these units; lgl eval run -answers FILE scores it")
		return nil
	}
	if *date == "" {
		*date = time.Now().UTC().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", *date); err != nil {
		return fmt.Errorf("%w: -date %q is not YYYY-MM-DD", errUsage, *date)
	}
	a, sum, err := evalset.LoadAnswers(*answers)
	if err != nil {
		return err
	}
	sets, err := evalset.Score(s, a)
	if err != nil {
		return err
	}
	if *label == "" {
		*label = strings.ToLower(a.Model)
	}
	if !labelPattern.MatchString(*label) {
		return fmt.Errorf("%w: the label %q must be lowercase letters, digits, dots and dashes; set it with -label", errUsage, *label)
	}
	r := evalset.Result{Version: 1, Date: *date, Label: *label, Model: a.Model, PromptVersion: a.PromptVersion, Answers: filepath.ToSlash(*answers), AnswersSHA256: sum, SetsSHA256: s.Hash, Sets: sets}
	for _, set := range evalset.SetNames {
		x := sets[set]
		r.CostUSD += x.CostUSD
		fmt.Fprintf(stdout, "%-8s %3d of %3d passed, %6.2f%%, cost $%.2f\n", set, x.Passed, x.Total, x.PassRate, x.CostUSD)
	}
	if *dry {
		fmt.Fprintln(stdout, "dry run: no file written")
		return nil
	}
	if *out == "" {
		*out = filepath.Join(*dir, "results")
	}
	path := filepath.Join(*out, *date+"-"+*label+".json")
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s exists; use -force to replace it, or another -label", path)
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	return nil
}

func evalCompare(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return usageErr(err)
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("%w: eval compare needs two result files: the old one and the new one", errUsage)
	}
	old, err := evalset.LoadResult(fs.Arg(0))
	if err != nil {
		return err
	}
	cur, err := evalset.LoadResult(fs.Arg(1))
	if err != nil {
		return err
	}
	if old.SetsSHA256 != cur.SetsSHA256 {
		fmt.Fprintln(stdout, "note: the two results are for different versions of the sets")
	}
	blocked := 0
	for _, c := range evalset.Compare(old, cur) {
		switch {
		case c.Missing:
			fmt.Fprintf(stdout, "%-8s %6.2f%% -> no result  blocks the merge\n", c.Set, c.Old)
			blocked++
			continue
		}
		verdict := "ok"
		if c.Blocks {
			verdict = fmt.Sprintf("blocks the merge: the drop is more than %.0f points", evalset.MaxDrop)
			blocked++
		}
		fmt.Fprintf(stdout, "%-8s %6.2f%% -> %6.2f%% (%+.2f points), cost $%.2f -> $%.2f  %s\n", c.Set, c.Old, c.New, c.New-c.Old, c.OldCost, c.NewCost, verdict)
	}
	fmt.Fprintf(stdout, "total cost $%.2f -> $%.2f\n", old.CostUSD, cur.CostUSD)
	if blocked > 0 {
		return errors.New("the change lowers a pass rate by more than 5 points, so it is not merged (spec 10.9)")
	}
	fmt.Fprintln(stdout, "ok: no pass rate drops by more than 5 points")
	return nil
}

func evalKey(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval key", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "eval", "the eval directory, with tasks/ and statements/")
	if err := parse(fs, args); err != nil {
		return err
	}
	s, err := loadSets(*dir)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(evalset.Key(s), "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s\n", b)
	return err
}

func evalDecl(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lgl eval decl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lean := fs.String("lean", "", "a checkout of langlands-lean")
	mathlib := fs.String("mathlib", "", "a checkout of Mathlib (default: the Mathlib package of -lean)")
	repo := fs.String("repo", "langlands-lean", "the repository of the file: langlands-lean or mathlib")
	commit := fs.String("commit", "HEAD", "the commit")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: lgl eval decl [flags] <path> <full name>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return usageErr(err)
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("%w: eval decl needs a path and a full name, after the flags", errUsage)
	}
	src := evalset.NewSource(map[string]string{"langlands-lean": *lean, "mathlib": mathlibDir(*mathlib, *lean)})
	if !src.Has(*repo) {
		return fmt.Errorf("%w: give a checkout of %s", errUsage, *repo)
	}
	c := *commit
	if c == "HEAD" {
		h, err := src.Head(*repo)
		if err != nil {
			return err
		}
		c = h
	}
	r := evalset.Ref{Repo: *repo, Commit: c, Path: fs.Arg(0), Decl: fs.Arg(1)}
	d, lines, err := src.Resolve(r)
	if err != nil {
		return err
	}
	text := d.Text(lines)
	if d.Keyword == "theorem" || d.Keyword == "lemma" {
		text = d.Header
	}
	// The snippet is written by hand and not with yaml.Marshal: the YAML library writes a string with a character outside the basic plane, such as 𝓞, as an escaped string, and a literal block is easier to review.
	_, err = fmt.Fprintf(stdout, "# %s, lines %d to %d\nreviewed:\n  repo: %s\n  commit: %s\n  path: %s\n  decl: %s\n  lean: |\n%s", d.Keyword, d.Start, d.End, r.Repo, r.Commit, r.Path, r.Decl, indent(text+"\n", "    "))
	return err
}

// indent puts the prefix before each line of s that is not empty.
func indent(s, prefix string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(s, "\n") {
		if strings.TrimSpace(l) != "" {
			b.WriteString(prefix)
		}
		b.WriteString(l)
	}
	return b.String()
}

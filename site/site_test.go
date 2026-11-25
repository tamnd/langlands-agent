package site

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func content(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, "site/home.md", "---\ntitle: The Langlands program\nlead: Reciprocity and functoriality for $GL_n$, in Lean.\ndoors:\n  - title: The program\n    path: program/\n    text: What is open.\n---\nThe text of the main page.\n")
	put(t, root, "site/program.md", "---\ntitle: The problem\nlead: What is open.\n---\nSee [the statement](node:LG00000001) and [Serre](paper:Ser67). The formula $$a_p = p + 1 - \\#E(\\mathbb{F}_p)$$ shows.\n")
	put(t, root, "site/map.md", "---\ntitle: The map\n---\nThe nodes.\n")
	put(t, root, "site/papers.md", "---\ntitle: The papers\n---\nThe corpus.\n")
	put(t, root, "site/routes.md", "---\ntitle: The routes\n---\nThe routes.\n")
	put(t, root, "site/trust.md", "---\ntitle: Trust\n---\nThe checks.\n")
	for _, name := range []string{"learn", "handbook", "code", "about"} {
		put(t, root, "site/"+name+".md", "---\ntitle: "+name+"\n---\nText.\n")
	}
	put(t, root, "blueprint/routes.yaml", "- id: R1\n  title: Polyhedra\n  weight: 0.25\n  idea: i\n  obstruction: o\n  known: k\n  first: f\n")
	put(t, root, "blueprint/nodes/LG00000001.yaml", "tag: LG00000001\nkind: conjecture\ntitle: Local Langlands for GL(n)\nstatement: The local Langlands correspondence for $GL_n$ over a $p$-adic field.\ntier: S\nroute: [R1]\nsources:\n  - paper: Ser67\n    loc: Main theorem\nlean:\n  statement: Langlands.LocalLanglandsGL\n")
	put(t, root, "papers/Ser67.yaml", "key: Ser67\ntitle: Local class field theory\nauthors: [J.-P. Serre]\nyear: 1967\ntag: P\ntier: T3\nwhy: Local reciprocity.\nuse: [read]\n")
	put(t, root, "records/lean.json", `{"commit":"0123456789abcdef","toolchain":"leanprover/lean4:v4.35.0","decls":{"Langlands.LocalLanglandsGL":{"kind":"def","file":"Langlands/Statements/Main.lean","line":12,"axioms":[]}}}`)
	return root
}

func TestBuild(t *testing.T) {
	out := filepath.Join(t.TempDir(), "public")
	res, err := Build(context.Background(), Options{Content: content(t), Out: out, Commit: "abc", Date: "2026-11-23"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages < 9 {
		t.Errorf("built %d pages", res.Pages)
	}
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for name, want := range map[string]string{
		"index.html":                 "<math",
		"program/index.html":         `href="/node/LG00000001/"`,
		"node/LG00000001/index.html": "Langlands/Statements/Main.lean#L12",
		"paper/ser67/index.html":     "Local Langlands for GL(n)",
		"routes/r1/index.html":       "Local Langlands for GL(n)",
		"map/index.html":             `class="mark open"`,
		"api/search.json":            "LG00000001",
		"llms.txt":                   "No open case of the program is proved here",
		"program/index.md":           "# The problem",
		"404.html":                   "Page not found",
	} {
		if got := read(name); !strings.Contains(got, want) {
			t.Errorf("%s does not have %q", name, want)
		}
	}
	r, err := Check(out)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK() {
		t.Errorf("the check finds problems: %v", r.Problems)
	}
}

func TestBuildStopsOnABadMap(t *testing.T) {
	root := content(t)
	put(t, root, "blueprint/nodes/LG00000002.yaml", "tag: LG00000002\nkind: lemma\ntitle: t\nstatement: s\ntier: T1\nuses: [LG0000000Z]\n")
	if _, err := Build(context.Background(), Options{Content: root, Out: filepath.Join(t.TempDir(), "p")}); err == nil {
		t.Fatal("a map with a bad link builds")
	}
}

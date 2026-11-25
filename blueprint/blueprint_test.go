package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "blueprint/routes.yaml", "- id: R1\n  title: Polyhedra\n  weight: 0.25\n  idea: i\n  obstruction: o\n  known: k\n  first: f\n")
	write(t, root, "blueprint/nodes/LG00000001.yaml", "tag: LG00000001\nkind: definition\ntitle: Regular\nstatement: A regular scheme.\ntier: S\nlean:\n  statement: Langlands.IsRegularScheme\n")
	write(t, root, "blueprint/nodes/LG00000002.yaml", "tag: LG00000002\nkind: theorem\ntitle: Unramified GL(1)\nstatement: Unramified characters.\ntier: S\nuses: [LG00000001]\nlean:\n  statement: Langlands.LocalReciprocityGL1\n  proof: Langlands.localReciprocityGL1_unramified\nsources:\n  - paper: Ser67\n")
	write(t, root, "blueprint/nodes/LG00000003.yaml", "tag: LG00000003\nkind: theorem\ntitle: Local GL(1)\nstatement: Local class field theory.\ntier: T5\nuses: [LG00000004]\nroute: [R1]\nlean:\n  statement: Langlands.LocalReciprocityGL1\n  proof: Langlands.localReciprocityGL1\n")
	write(t, root, "blueprint/nodes/LG00000004.yaml", "tag: LG00000004\nkind: hypothesis\ntitle: Normalization\nstatement: Finite.\ntier: T1\nlean:\n  statement: Langlands.Normalization\n")
	write(t, root, "blueprint/nodes/LG00000005.yaml", "tag: LG00000005\nkind: conjecture\ntitle: Dimension 4\nstatement: Open.\ntier: S\nlean:\n  statement: Langlands.ResolutionDimFourCharP\n")
	write(t, root, "papers/Ser67.yaml", "key: Ser67\ntitle: Local class field theory\ntag: P\ntier: T3\nwhy: local CFT\nuse: [read]\n")
	write(t, root, "records/lean.json", `{"commit":"abc","decls":{
"Langlands.IsRegularScheme":{"kind":"def","axioms":["propext"]},
"Langlands.LocalReciprocityGL1":{"kind":"def","axioms":[]},
"Langlands.localReciprocityGL1_unramified":{"kind":"theorem","axioms":["propext","Classical.choice","Quot.sound"]},
"Langlands.localReciprocityGL1":{"kind":"theorem","axioms":["propext"]},
"Langlands.Normalization":{"kind":"def","axioms":[]},
"Langlands.ResolutionDimFourCharP":{"kind":"def","axioms":[]}}}`)
	return root
}

func TestLoadAndStatus(t *testing.T) {
	p, err := Load(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if probs := p.Check(); len(probs) != 0 {
		t.Fatalf("problems: %v", probs)
	}
	want := map[string]string{
		"LG00000001": Defined,
		"LG00000002": Proved,
		"LG00000003": ProvedMod,
		"LG00000004": Stated,
		"LG00000005": Open,
	}
	for tag, st := range want {
		if got := p.Node(tag).Status; got != st {
			t.Errorf("%s: status %s, want %s (%s)", tag, got, st, p.Node(tag).Why)
		}
	}
	if got := p.Paper("Ser67").Nodes; len(got) != 1 || got[0] != "LG00000002" {
		t.Errorf("Ser67 nodes = %v", got)
	}
	if got := p.Node("LG00000001").UsedBy; len(got) != 1 {
		t.Errorf("used by = %v", got)
	}
}

func TestSorryIsNotAProof(t *testing.T) {
	root := fixture(t)
	write(t, root, "records/lean.json", `{"decls":{
"Langlands.IsRegularScheme":{"kind":"def","axioms":[]},
"Langlands.LocalReciprocityGL1":{"kind":"def","axioms":[]},
"Langlands.localReciprocityGL1_unramified":{"kind":"theorem","axioms":["sorryAx"]},
"Langlands.localReciprocityGL1":{"kind":"theorem","axioms":["propext","Langlands.myAxiom"]},
"Langlands.Normalization":{"kind":"def","axioms":[]},
"Langlands.ResolutionDimFourCharP":{"kind":"def","axioms":[]}}}`)
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Node("LG00000002").Status; got != Stated {
		t.Errorf("a proof with sorry has status %s", got)
	}
	if got := p.Node("LG00000003").Status; got != Stated {
		t.Errorf("a proof with a new axiom has status %s", got)
	}
}

func TestCheckFindsFaults(t *testing.T) {
	root := fixture(t)
	write(t, root, "blueprint/nodes/LG00000006.yaml", "tag: LG00000006\nkind: guess\ntitle: Bad\nstatement: x\ntier: T11\nuses: [LG00000007, LG00000006]\nsources:\n  - paper: Nobody\nroute: [R7]\n")
	write(t, root, "blueprint/nodes/SGbad.yaml", "tag: SGbad\nkind: lemma\ntitle: t\nstatement: s\ntier: T1\n")
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	probs := strings.Join(p.Check(), "\n")
	for _, want := range []string{"kind \"guess\"", "tier \"T11\"", "LG00000007, which is not a node", "Nobody", "route \"R7\"", "cycle", "SGbad"} {
		if !strings.Contains(probs, want) {
			t.Errorf("the check does not report %q:\n%s", want, probs)
		}
	}
}

func TestUnknownFieldIsAnError(t *testing.T) {
	root := fixture(t)
	write(t, root, "papers/Bad.yaml", "key: Bad\ntitle: t\ntag: P\ntier: T1\nwhy: w\nstatus: proved\n")
	if _, err := Load(root); err == nil {
		t.Fatal("a paper record with an unknown field loads with no error")
	}
}

func TestPlannedNameIsNotAFault(t *testing.T) {
	root := fixture(t)
	write(t, root, "blueprint/nodes/LG00000006.yaml", "tag: LG00000006\nkind: theorem\ntitle: Local GL(2)\nstatement: LLC for GL(2).\ntier: T5\nlean:\n  statement: Langlands.LocalReciprocityGL1\n  proof: Langlands.localLanglandsGL2\n")
	write(t, root, "blueprint/nodes/LG00000007.yaml", "tag: LG00000007\nkind: lemma\ntitle: Open locus\nstatement: Open.\ntier: T1\nlean:\n  statement: Langlands.regularLocus_isOpen\n")
	write(t, root, "papers/Tat67.yaml", "key: Tat67\ntitle: Fourier analysis in number fields and Hecke zeta functions\ntag: P\ntier: T3\nwhy: Tate thesis\nuse: [reuse, bridge]\n")
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if probs := p.Check(); len(probs) != 0 {
		t.Fatalf("problems: %v", probs)
	}
	if got := p.Node("LG00000006").Status; got != Stated {
		t.Errorf("a statement with no proof in the records has status %s", got)
	}
	if got := p.Node("LG00000007").Status; got != Planned {
		t.Errorf("a name that is not in the records has status %s", got)
	}
}

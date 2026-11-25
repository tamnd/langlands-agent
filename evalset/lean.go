package evalset

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Decl is one declaration of a Lean file, as the scanner of this package finds it. The scanner does not run Lean. It reads the layout of the file: a declaration starts at column 0, and it ends before the next line at column 0.
type Decl struct {
	Name       string // the full name, with the namespaces
	Keyword    string // theorem, lemma, def, abbrev, instance, structure, class or inductive
	Start      int    // the first line, 1-based: the first attribute, or the keyword line
	End        int    // the last line that is not blank
	Doc        string // the docstring, with the markers removed
	Private    bool
	Deprecated bool
	Header     string // the text from Start up to the := of the body, with no trailing space
	Proof      string // the text after the :=, when the body is a single term
	HasProof   bool   // the body is one := at bracket depth 0, not a match or a where block
	ProofLines int    // the number of lines of the body that are not blank
}

// Text returns the lines Start to End of the file text.
func (d Decl) Text(lines []string) string {
	if d.Start < 1 || d.End > len(lines) || d.Start > d.End {
		return ""
	}
	return strings.Join(lines[d.Start-1:d.End], "\n")
}

var declKeywords = map[string]bool{"theorem": true, "lemma": true, "def": true, "abbrev": true, "instance": true, "structure": true, "class": true, "inductive": true, "opaque": true, "axiom": true}

var declModifiers = map[string]bool{"private": true, "protected": true, "public": true, "noncomputable": true, "nonrec": true, "unsafe": true, "partial": true, "scoped": true, "local": true, "meta": true}

// continuation is true for a line at column 0 that does not end the declaration above it.
func continuation(code string) bool {
	for _, p := range []string{"|", ")", "]", "}", "⟩", "termination_by", "decreasing_by", "deriving", "where"} {
		if strings.HasPrefix(code, p) {
			return true
		}
	}
	return false
}

// maskComments returns the text with each byte of a comment or a string literal changed to a space, so that the byte offsets do not change. Newlines stay.
func maskComments(text string) string {
	b := []byte(text)
	depth := 0
	inString := false
	for i := 0; i < len(b); i++ {
		switch {
		case depth > 0:
			if b[i] == '/' && i+1 < len(b) && b[i+1] == '-' {
				depth++
				b[i], b[i+1] = ' ', ' '
				i++
			} else if b[i] == '-' && i+1 < len(b) && b[i+1] == '/' {
				depth--
				b[i], b[i+1] = ' ', ' '
				i++
			} else if b[i] != '\n' {
				b[i] = ' '
			}
		case inString:
			if b[i] == '\\' && i+1 < len(b) && b[i+1] != '\n' {
				b[i], b[i+1] = ' ', ' '
				i++
			} else if b[i] == '"' {
				inString = false
				b[i] = ' '
			} else if b[i] != '\n' {
				b[i] = ' '
			}
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '-':
			depth = 1
			b[i], b[i+1] = ' ', ' '
			i++
		case b[i] == '-' && i+1 < len(b) && b[i+1] == '-':
			for i < len(b) && b[i] != '\n' {
				b[i] = ' '
				i++
			}
		case b[i] == '"':
			// A character literal such as '"' is not a string.
			if i > 0 && b[i-1] == '\'' && i+1 < len(b) && b[i+1] == '\'' {
				continue
			}
			inString = true
			b[i] = ' '
		}
	}
	return string(b)
}

// lineStarts returns the byte offset of the start of each line.
func lineStarts(text string) []int {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// identRune is true for a rune that can be part of a Lean name, with the dots.
func identRune(r rune) bool {
	if r == '_' || r == '.' || r == '\'' || r == '!' || r == '?' {
		return true
	}
	if r >= '₀' && r <= '₉' || r >= 'ₐ' && r <= 'ₜ' {
		return true
	}
	if r == 'λ' || r == 'Π' || r == 'Σ' {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// firstName returns the name at the start of s, which can be in «» quotes.
func firstName(s string) string {
	if strings.HasPrefix(s, "«") {
		if i := strings.Index(s, "»"); i > 0 {
			return s[:i+len("»")]
		}
	}
	end := 0
	for end < len(s) {
		r, size := utf8.DecodeRuneInString(s[end:])
		if !identRune(r) {
			break
		}
		end += size
	}
	return strings.TrimRight(s[:end], ".")
}

// stripAttributes removes the attribute groups @[...] at the start of code and returns the rest. It returns ok false when a group does not close on the line.
func stripAttributes(code string) (rest string, attrs string, ok bool) {
	rest = strings.TrimLeft(code, " ")
	for strings.HasPrefix(rest, "@[") {
		depth := 0
		closed := -1
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					closed = i
				}
			}
			if closed >= 0 {
				break
			}
		}
		if closed < 0 {
			return "", attrs + rest, false
		}
		attrs += rest[:closed+1]
		rest = strings.TrimLeft(rest[closed+1:], " ")
	}
	return rest, attrs, true
}

// ParseLean finds the declarations of a Lean file. It follows the namespaces, so the names are full names. It skips the declarations that have no name, such as most instances.
func ParseLean(text string) []Decl {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	masked := maskComments(text)
	starts := lineStarts(text)
	raw := strings.Split(text, "\n")
	code := strings.Split(masked, "\n")
	var decls []Decl
	var scopes []string // a namespace name, or "" for a section
	pending := 0        // the first line of the attributes or prefixes of the next declaration
	pendingAttrs := ""
	doc := ""
	docLine := 0
	cur := -1 // the index in decls of the open declaration

	closeDecl := func(next int) {
		if cur < 0 {
			return
		}
		d := &decls[cur]
		end := next - 1
		for end > d.Start && strings.TrimSpace(raw[end-1]) == "" {
			end--
		}
		d.End = end
		finish(d, text, masked, starts)
		cur = -1
	}

	for i := 0; i < len(raw); i++ {
		line := i + 1
		r := raw[i]
		c := code[i]
		if r == "" || r[0] == ' ' || r[0] == '\t' {
			continue
		}
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			// A comment at column 0. A docstring belongs to the next declaration.
			if strings.HasPrefix(r, "/--") {
				closeDecl(line)
				docLine = line
				j := i
				for ; j < len(raw); j++ {
					if strings.Contains(raw[j], "-/") && (j > i || strings.Index(raw[j], "-/") > 2) {
						break
					}
				}
				doc = strings.Join(raw[i:min(j+1, len(raw))], "\n")
				i = j
				continue
			}
			if strings.HasPrefix(r, "--") || strings.HasPrefix(r, "/-") {
				closeDecl(line)
				pending, pendingAttrs = 0, ""
			}
			continue
		}
		if cur >= 0 && continuation(trimmed) {
			continue
		}
		closeDecl(line)
		if docLine > 0 && docLine < line-50 {
			doc, docLine = "", 0
		}
		rest, attrs, ok := stripAttributes(trimmed)
		if !ok {
			// An attribute group on more than one line.
			if pending == 0 {
				pending = line
			}
			pendingAttrs += attrs
			for i+1 < len(raw) && !strings.Contains(code[i], "]") {
				i++
				pendingAttrs += code[i]
			}
			continue
		}
		words := strings.Fields(rest)
		if len(words) == 0 {
			if pending == 0 {
				pending = line
			}
			pendingAttrs += attrs
			continue
		}
		switch words[0] {
		case "namespace":
			if len(words) > 1 {
				scopes = append(scopes, words[1])
			}
			pending, pendingAttrs, doc = 0, "", ""
			continue
		case "section", "mutual":
			scopes = append(scopes, "")
			pending, pendingAttrs, doc = 0, "", ""
			continue
		case "end":
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			pending, pendingAttrs, doc = 0, "", ""
			continue
		}
		k := 0
		priv := false
		for k < len(words) && declModifiers[words[k]] {
			if words[k] == "private" {
				priv = true
			}
			k++
		}
		if k < len(words) && words[k] == "section" {
			scopes = append(scopes, "")
			pending, pendingAttrs, doc = 0, "", ""
			continue
		}
		if k >= len(words) || !declKeywords[words[k]] {
			// A prefix such as "variable {p} in" or "open Foo in" belongs to the next declaration.
			if strings.HasSuffix(trimmed, " in") {
				if pending == 0 {
					pending = line
				}
				continue
			}
			pending, pendingAttrs, doc = 0, "", ""
			continue
		}
		name := ""
		if k+1 < len(words) {
			name = firstName(words[k+1])
		}
		if name == "" || words[k] == "instance" && strings.HasPrefix(name, "[") {
			pending, pendingAttrs, doc = 0, "", ""
			continue
		}
		start := line
		if pending > 0 {
			start = pending
		}
		d := Decl{Name: fullName(scopes, name), Keyword: words[k], Start: start, Private: priv}
		allAttrs := pendingAttrs + attrs
		d.Deprecated = strings.Contains(allAttrs, "deprecated")
		if doc != "" {
			d.Doc = cleanDoc(doc)
		}
		decls = append(decls, d)
		cur = len(decls) - 1
		pending, pendingAttrs, doc, docLine = 0, "", "", 0
	}
	closeDecl(len(raw) + 1)
	return decls
}

func fullName(scopes []string, name string) string {
	if strings.HasPrefix(name, "_root_.") {
		return strings.TrimPrefix(name, "_root_.")
	}
	var parts []string
	for _, s := range scopes {
		if s != "" {
			parts = append(parts, s)
		}
	}
	parts = append(parts, name)
	return strings.Join(parts, ".")
}

func cleanDoc(doc string) string {
	doc = strings.TrimSpace(doc)
	doc = strings.TrimPrefix(doc, "/--")
	doc = strings.TrimSuffix(doc, "-/")
	return strings.Join(strings.Fields(doc), " ")
}

// finish fills the header and the body of a declaration from its lines.
func finish(d *Decl, text, masked string, starts []int) {
	from := starts[d.Start-1]
	to := len(text)
	if d.End < len(starts) {
		to = starts[d.End] - 1
	}
	if to < from {
		to = from
	}
	m := masked[from:to]
	depth := 0
	for i := 0; i < len(m); i++ {
		switch m[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '\n':
			// A match arm or a where block at the start of a line, at depth 0, means the body is not one term.
			rest := strings.TrimLeft(m[i+1:], " ")
			if depth == 0 && (strings.HasPrefix(rest, "| ") || strings.HasPrefix(rest, "where")) {
				d.Header = strings.TrimRight(text[from:from+i], " \n")
				return
			}
		case ':':
			if depth == 0 && i+1 < len(m) && m[i+1] == '=' {
				d.Header = strings.TrimRight(text[from:from+i], " \n")
				body := text[from+i+2 : to]
				d.Proof = strings.TrimSpace(body)
				d.HasProof = !strings.Contains(m[i+2:], "\n| ") && !hasWhere(m[i+2:])
				for _, l := range strings.Split(body, "\n") {
					if strings.TrimSpace(l) != "" {
						d.ProofLines++
					}
				}
				return
			}
		case 'w':
			// A where block after the type, as in "IsFoo x where", at depth 0.
			if depth == 0 && strings.HasPrefix(m[i:], "where") && i > 0 && (m[i-1] == ' ' || m[i-1] == '\n') && (i+5 == len(m) || m[i+5] == ' ' || m[i+5] == '\n') {
				d.Header = strings.TrimRight(text[from:from+i], " \n")
				return
			}
		default:
			if strings.HasPrefix(m[i:], "⟨") || strings.HasPrefix(m[i:], "⦃") {
				depth++
			} else if strings.HasPrefix(m[i:], "⟩") || strings.HasPrefix(m[i:], "⦄") {
				depth--
			}
		}
	}
	d.Header = strings.TrimRight(text[from:to], " \n")
}

func hasWhere(body string) bool {
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		if t == "where" || strings.HasSuffix(t, " where") {
			return true
		}
	}
	return false
}

// FindDecl returns the declaration with the full name, or false.
func FindDecl(decls []Decl, name string) (Decl, bool) {
	for _, d := range decls {
		if d.Name == name {
			return d, true
		}
	}
	return Decl{}, false
}

// SameText reports whether two Lean texts are equal up to white space.
func SameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

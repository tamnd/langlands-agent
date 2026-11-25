package evalset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// MaxDrop is the largest drop of a pass rate, in points, that a change can have and still be merged (spec 10.9).
const MaxDrop = 5.0

// The names of the three scored sets.
const (
	SetTasks   = "tasks"   // eval/tasks: all the checks of the task pass
	SetStater  = "stater"  // eval/statements: the stater writes a statement that passes the checks
	SetBattery = "battery" // eval/statements: the battery accepts the reviewed statements and rejects the wrong ones
)

// SetNames are the scored sets, in the order of the reports.
var SetNames = []string{SetTasks, SetStater, SetBattery}

// Answers is the file that a runner writes after one run of the sets. lgl does not run a model; it only scores this file.
type Answers struct {
	Model         string   `json:"model"`
	PromptVersion string   `json:"prompt_version"`
	Tasks         []Answer `json:"tasks"`
	Statements    []Answer `json:"statements"`
	Battery       []Answer `json:"battery"`
}

// Answer is the outcome of one unit: the checks that passed, or the verdict of the battery, and the cost.
type Answer struct {
	ID      string          `json:"id"`
	Checks  map[string]bool `json:"checks,omitempty"`
	Verdict string          `json:"verdict,omitempty"` // accept or reject
	CostUSD float64         `json:"cost_usd"`
	Tokens  int             `json:"tokens"`
}

// Count is the number of units of a group and the number that passed.
type Count struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
}

// SetResult is the score of one set.
type SetResult struct {
	Total    int              `json:"total"`
	Answered int              `json:"answered"`
	Passed   int              `json:"passed"`
	PassRate float64          `json:"pass_rate"` // in percent, from 0 to 100
	CostUSD  float64          `json:"cost_usd"`
	Tokens   int              `json:"tokens"`
	ByKind   map[string]Count `json:"by_kind,omitempty"`
	ByOrigin map[string]Count `json:"by_origin,omitempty"`
	Failed   []string         `json:"failed,omitempty"`
}

// Result is a file of eval/results.
type Result struct {
	Version       int                  `json:"version"`
	Date          string               `json:"date"`
	Label         string               `json:"label"`
	Model         string               `json:"model"`
	PromptVersion string               `json:"prompt_version"`
	Answers       string               `json:"answers"`
	AnswersSHA256 string               `json:"answers_sha256"`
	SetsSHA256    string               `json:"sets_sha256"`
	CostUSD       float64              `json:"cost_usd"`
	Sets          map[string]SetResult `json:"sets"`
}

// LoadAnswers reads an answers file and returns it with the sha256 of the file.
func LoadAnswers(path string) (*Answers, string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the answers file that the user names
	if err != nil {
		return nil, "", err
	}
	var a Answers
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return &a, hex.EncodeToString(sum[:]), nil
}

// LoadResult reads a result file.
func LoadResult(path string) (*Result, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- a result file that the user names
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if r.Version != 1 || r.Sets == nil {
		return nil, fmt.Errorf("%s is not a result file of lgl eval run", path)
	}
	return &r, nil
}

// unit is one scored item of a set.
type unit struct {
	id     string
	kind   string
	origin string
	checks []string // the checks that must pass, for the tasks and the stater
	expect string   // the verdict that must come, for the battery
}

func (s *Sets) units(set string) []unit {
	var out []unit
	switch set {
	case SetTasks:
		for _, t := range s.Tasks {
			out = append(out, unit{id: t.ID, kind: t.Kind, origin: t.Origin, checks: t.Checks})
		}
	case SetStater:
		for _, st := range s.Statements {
			out = append(out, unit{id: st.ID, kind: "state", checks: st.Checks})
		}
	case SetBattery:
		for _, st := range s.Statements {
			out = append(out, unit{id: st.ID, kind: "reviewed", expect: "accept"})
			for _, w := range st.Wrong {
				out = append(out, unit{id: w.ID, kind: "wrong", expect: "reject"})
			}
		}
	}
	return out
}

// Units returns the number of scored units of a set.
func (s *Sets) Units(set string) int { return len(s.units(set)) }

func (a *Answers) list(set string) []Answer {
	switch set {
	case SetTasks:
		return a.Tasks
	case SetStater:
		return a.Statements
	}
	return a.Battery
}

// Score compares the answers with the known answers. A unit with no answer fails. An answer with an unknown id or a second answer for an id is an error.
func Score(s *Sets, a *Answers) (map[string]SetResult, error) {
	out := map[string]SetResult{}
	var errs []string
	for _, set := range SetNames {
		units := s.units(set)
		known := map[string]bool{}
		for _, u := range units {
			known[u.id] = true
		}
		got := map[string]Answer{}
		var cost float64
		tokens := 0
		for _, ans := range a.list(set) {
			switch {
			case !known[ans.ID]:
				errs = append(errs, fmt.Sprintf("%s: the answer %q is not a unit of the set", set, ans.ID))
			case got[ans.ID].ID != "":
				errs = append(errs, fmt.Sprintf("%s: two answers for %s", set, ans.ID))
			}
			if set == SetBattery && ans.Verdict != "accept" && ans.Verdict != "reject" {
				errs = append(errs, fmt.Sprintf("battery: the verdict of %s is %q, not accept or reject", ans.ID, ans.Verdict))
			}
			if ans.CostUSD < 0 || ans.Tokens < 0 {
				errs = append(errs, fmt.Sprintf("%s: %s has a negative cost", set, ans.ID))
			}
			got[ans.ID] = ans
			cost += ans.CostUSD
			tokens += ans.Tokens
		}
		r := SetResult{Total: len(units), CostUSD: round(cost, 4), Tokens: tokens, ByKind: map[string]Count{}}
		if set == SetTasks {
			r.ByOrigin = map[string]Count{}
		}
		for _, u := range units {
			ans, ok := got[u.id]
			if ok {
				r.Answered++
			}
			pass := ok && u.passes(ans)
			k := r.ByKind[u.kind]
			k.Total++
			if pass {
				k.Passed++
				r.Passed++
			} else {
				r.Failed = append(r.Failed, u.id)
			}
			r.ByKind[u.kind] = k
			if r.ByOrigin != nil {
				o := r.ByOrigin[u.origin]
				o.Total++
				if pass {
					o.Passed++
				}
				r.ByOrigin[u.origin] = o
			}
		}
		if r.Total > 0 {
			r.PassRate = round(100*float64(r.Passed)/float64(r.Total), 2)
		}
		sort.Strings(r.Failed)
		out[set] = r
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

func (u unit) passes(a Answer) bool {
	if u.expect != "" {
		return a.Verdict == u.expect
	}
	for _, c := range u.checks {
		if !a.Checks[c] {
			return false
		}
	}
	return len(u.checks) > 0
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

// Key returns the answers of a runner that gets every unit right at no cost. Its score is 100 in each set, so it tests the scoring.
func Key(s *Sets) *Answers {
	a := &Answers{Model: "key", PromptVersion: "key", Tasks: []Answer{}, Statements: []Answer{}, Battery: []Answer{}}
	for _, set := range SetNames {
		for _, u := range s.units(set) {
			ans := Answer{ID: u.id, Verdict: u.expect}
			if u.expect == "" {
				ans.Checks = map[string]bool{}
				for _, c := range u.checks {
					ans.Checks[c] = true
				}
			}
			switch set {
			case SetTasks:
				a.Tasks = append(a.Tasks, ans)
			case SetStater:
				a.Statements = append(a.Statements, ans)
			default:
				a.Battery = append(a.Battery, ans)
			}
		}
	}
	return a
}

// Change is the change of one set between two results.
type Change struct {
	Set              string
	Old, New         float64 // the pass rates
	OldCost, NewCost float64
	Missing          bool // the set is in the old result and not in the new one
	Blocks           bool // the pass rate drops by more than MaxDrop points, or the set is missing
}

// Compare applies the rule of spec 10.9 to two results: a change that lowers the pass rate of a set by more than MaxDrop points is not merged. A set that is only in the new result does not block.
func Compare(old, cur *Result) []Change {
	var out []Change
	for _, set := range SetNames {
		o, inOld := old.Sets[set]
		n, inNew := cur.Sets[set]
		switch {
		case !inOld && !inNew:
			continue
		case !inOld:
			out = append(out, Change{Set: set, New: n.PassRate, NewCost: n.CostUSD})
		case !inNew:
			out = append(out, Change{Set: set, Old: o.PassRate, OldCost: o.CostUSD, Missing: true, Blocks: true})
		default:
			out = append(out, Change{Set: set, Old: o.PassRate, New: n.PassRate, OldCost: o.CostUSD, NewCost: n.CostUSD, Blocks: o.PassRate-n.PassRate > MaxDrop+1e-9})
		}
	}
	return out
}

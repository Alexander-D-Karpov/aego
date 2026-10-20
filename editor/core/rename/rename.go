package rename

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"aego/core/errs"
)

type Case uint8

const (
	CaseKeep Case = iota
	CaseLower
	CaseUpper
	CaseTitle
)

type Rule struct {
	Pattern string
	Find    string
	Replace string
	Regex   bool
	Case    Case
	Start   int
	Step    int
}

type Item struct {
	Name   string
	Ext    string
	Parent string
}

type Row struct {
	From     string
	To       string
	Conflict string
}

func (r Row) Changed() bool {
	return r.From != r.To
}

var token = regexp.MustCompile(`\{(name|n|parent|ext)(?::(0?\d+))?\}`)

func title(s string) string {
	rs := []rune(s)
	up := true
	for i, r := range rs {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if up {
				rs[i] = unicode.ToUpper(r)
			} else {
				rs[i] = unicode.ToLower(r)
			}
			up = false
			continue
		}
		up = true
	}
	return string(rs)
}

func applyCase(s string, c Case) string {
	switch c {
	case CaseLower:
		return strings.ToLower(s)
	case CaseUpper:
		return strings.ToUpper(s)
	case CaseTitle:
		return title(s)
	}
	return s
}

func number(n int, width string) string {
	s := strconv.Itoa(n)
	if width == "" {
		return s
	}
	w, _ := strconv.Atoi(width)
	if neg := n < 0; neg {
		return "-" + strings.Repeat("0", max(0, w-len(s))) + s[1:]
	}
	return strings.Repeat("0", max(0, w-len(s))) + s
}

func Plan(rule Rule, items []Item, valid func(string) bool, taken func(parent, name string) bool, fold bool) ([]Row, error) {
	var re *regexp.Regexp
	if rule.Find != "" && rule.Regex {
		var err error
		if re, err = regexp.Compile(rule.Find); err != nil {
			return nil, errs.New(errs.EditorCommand, "the find pattern is not a valid regular expression: "+err.Error())
		}
	}
	pattern := rule.Pattern
	if strings.TrimSpace(pattern) == "" {
		pattern = "{name}"
	}
	step := rule.Step
	if step == 0 {
		step = 1
	}
	key := func(parent, name string) string {
		if fold {
			name = strings.ToLower(name)
		}
		return parent + "\x00" + name
	}
	own := map[string]bool{}
	for _, it := range items {
		own[key(it.Parent, it.Name+it.Ext)] = true
	}
	rows := make([]Row, len(items))
	seen := map[string]int{}
	for i, it := range items {
		base := it.Name
		if rule.Find != "" {
			if re != nil {
				base = re.ReplaceAllString(base, rule.Replace)
			} else {
				base = strings.ReplaceAll(base, rule.Find, rule.Replace)
			}
		}
		n := rule.Start + i*step
		out := token.ReplaceAllStringFunc(pattern, func(m string) string {
			sub := token.FindStringSubmatch(m)
			switch sub[1] {
			case "name":
				return base
			case "n":
				return number(n, sub[2])
			case "parent":
				return it.Parent
			case "ext":
				return strings.TrimPrefix(it.Ext, ".")
			}
			return m
		})
		out = strings.TrimSpace(applyCase(out, rule.Case))
		rows[i] = Row{From: it.Name, To: out}
		switch {
		case out == "":
			rows[i].Conflict = "empty name"
		case valid != nil && !valid(out):
			rows[i].Conflict = "not a valid name"
		}
		k := key(it.Parent, out+it.Ext)
		if j, dup := seen[k]; dup {
			rows[i].Conflict = "same name as " + items[j].Name
			if rows[j].Conflict == "" {
				rows[j].Conflict = "same name as " + it.Name
			}
		} else {
			seen[k] = i
		}
		if rows[i].Conflict == "" && rows[i].Changed() && !own[k] && taken != nil && taken(it.Parent, out+it.Ext) {
			rows[i].Conflict = "already exists"
		}
	}
	return rows, nil
}

func Blocked(rows []Row) bool {
	for _, r := range rows {
		if r.Conflict != "" {
			return true
		}
	}
	return false
}

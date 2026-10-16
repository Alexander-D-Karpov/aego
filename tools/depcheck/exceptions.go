package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
)

// An exception lets one edge through; one that matches nothing is a violation.
type exception struct {
	line    int
	from    string
	to      string
	variant string
	reason  string
	used    bool
}

const exceptionsFile = "tools/depcheck/exceptions.txt"

var knownVariants = map[string]bool{
	"*": true, "default": true, "editor": true, "nonative": true, "runtime": true, "wasm": true,
	"test": true, "test-editor": true, "semantic": true,
}

func loadExceptions(file string) ([]*exception, []string, error) {
	f, err := os.Open(file)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var out []*exception
	var bad []string
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, err := parseException(line)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s:%d: [exception-syntax] %v", path.Base(file), n, err))
			continue
		}
		e.line = n
		out = append(out, e)
	}
	return out, bad, sc.Err()
}

func parseException(line string) (*exception, error) {
	from, rest, ok := strings.Cut(line, "->")
	if !ok {
		return nil, fmt.Errorf("want `from -> to, variant, reason`")
	}
	parts := strings.SplitN(rest, ",", 3)
	if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
		return nil, fmt.Errorf("exception %q has no reason", strings.TrimSpace(line))
	}
	e := &exception{
		from:    strings.TrimSpace(from),
		to:      strings.TrimSpace(parts[0]),
		variant: strings.TrimSpace(parts[1]),
		reason:  strings.TrimSpace(parts[2]),
	}
	if e.from == "" || e.to == "" || e.variant == "" {
		return nil, fmt.Errorf("exception %q has an empty field", strings.TrimSpace(line))
	}
	for _, v := range strings.Split(e.variant, "|") {
		if !knownVariants[v] {
			return nil, fmt.Errorf("exception %q names the unknown variant %q", strings.TrimSpace(line), v)
		}
	}
	return e, nil
}

// excepted reports whether an exception allows the edge and marks it used.
func excepted(list []*exception, variant, from, to string) bool {
	hit := false
	for _, e := range list {
		if !e.inVariant(variant) {
			continue
		}
		if !matchFrom(e.from, from) || !matchTo(e.to, to) {
			continue
		}
		e.used = true
		hit = true
	}
	return hit
}

func matchFrom(pattern, from string) bool {
	if strings.HasSuffix(pattern, ".go") {
		return pattern == from
	}
	if strings.HasSuffix(from, ".go") {
		from = path.Dir(from)
	}
	return match(pattern, from)
}

func matchTo(pattern, to string) bool {
	if strings.HasPrefix(pattern, `"`) {
		if !strings.HasPrefix(to, `"`) {
			return false
		}
		ok, _ := path.Match(strings.Trim(pattern, `"`), strings.Trim(to, `"`))
		return ok
	}
	return match(pattern, to)
}

func staleExceptions(list []*exception, variants map[string]bool) []string {
	var out []string
	for _, e := range list {
		if e.used || !e.checked(variants) {
			continue
		}
		out = append(out, fmt.Sprintf("exceptions.txt:%d: [exception-unused] %s -> %s (%s) matches nothing, remove it", e.line, e.from, e.to, e.variant))
	}
	return out
}

func (e *exception) inVariant(v string) bool {
	for _, x := range strings.Split(e.variant, "|") {
		if x == "*" || x == v {
			return true
		}
	}
	return false
}

// checked reports whether any variant of the exception ran.
func (e *exception) checked(ran map[string]bool) bool {
	for _, x := range strings.Split(e.variant, "|") {
		if x == "*" || ran[x] {
			return true
		}
	}
	return false
}

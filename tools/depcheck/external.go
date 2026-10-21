package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const externalFile = "tools/depcheck/external.txt"

type external struct {
	from   string
	to     string
	reason string
	line   string
	used   bool
}

func loadExternal(file string) ([]*external, []string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var out []*external
	var bad []string
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		where := fmt.Sprintf("%s:%d", externalFile, i+1)
		from, rest, ok1 := strings.Cut(line, "->")
		to, reason, ok2 := strings.Cut(rest, ",")
		from, to, reason = strings.TrimSpace(from), strings.TrimSpace(to), strings.TrimSpace(reason)
		if !ok1 || !ok2 || from == "" || to == "" || reason == "" {
			bad = append(bad, where+": "+line)
			continue
		}
		out = append(out, &external{from: from, to: to, reason: reason, line: where})
	}
	return out, bad, nil
}

func (c *collector) externalRule(from, imp string) string {
	if r, ok := ruleFor(from); ok && allowed(r.external, imp) {
		return ""
	}
	for _, e := range c.ext {
		if match(e.from, from) && (imp == e.to || strings.HasPrefix(imp, e.to+"/")) {
			e.used = true
			return ""
		}
	}
	return "external-import"
}

func unusedExternal(list []*external) []string {
	var out []string
	for _, e := range list {
		if !e.used {
			out = append(out, e.line+": "+e.from+" -> "+e.to)
		}
	}
	return out
}

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type pkgInfo struct {
	path      string // module-relative
	dir       string
	files     []string
	testFiles []string
	imports   []string // module-relative, own module only
	testImps  []string
	external  []string // non-module, non-std imports
	std       []string // standard library imports
}

type graph struct {
	variant string
	pkgs    map[string]*pkgInfo
	order   []string
	std     map[string][]string // standard library package -> its imports
}

type variantSpec struct {
	name     string
	tags     string
	env      []string
	patterns []string
}

const listFields = "ImportPath,Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,Imports,TestImports,XTestImports,Standard,Error,DepsErrors"

func loadGraph(root, module string, v variantSpec) (*graph, error) {
	args := []string{"list", "-e", "-deps", "-json=" + listFields}
	if v.tags != "" {
		args = append(args, "-tags", v.tags)
	}
	args = append(args, v.patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(append(os.Environ(), "GOWORK=off"), v.env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: go list: %w: %s", v.name, err, stderr.String())
	}
	g := &graph{variant: v.name, pkgs: map[string]*pkgInfo{}, std: map[string][]string{}}
	var errs []string
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var p struct {
			ImportPath                                   string
			Dir                                          string
			GoFiles, CgoFiles, TestGoFiles, XTestGoFiles []string
			Imports, TestImports, XTestImports           []string
			Standard                                     bool
			Error                                        *struct{ Err string }
			DepsErrors                                   []struct{ Err string }
		}
		if err := dec.Decode(&p); err != nil {
			return nil, err
		}
		if p.Standard {
			g.std[p.ImportPath] = p.Imports
			continue
		}
		local, ok := relPkg(module, p.ImportPath)
		if p.Error != nil {
			errs = append(errs, p.ImportPath+": "+p.Error.Err)
		}
		for _, d := range p.DepsErrors {
			errs = append(errs, p.ImportPath+": "+d.Err)
		}
		if !ok {
			continue
		}
		info := &pkgInfo{
			path:      local,
			dir:       p.Dir,
			files:     append(p.GoFiles, p.CgoFiles...),
			testFiles: append(p.TestGoFiles, p.XTestGoFiles...),
		}
		for _, imp := range p.Imports {
			if l, ok := relPkg(module, imp); ok {
				info.imports = append(info.imports, l)
			} else if imp != "C" && !isStdlib(imp) {
				info.external = append(info.external, imp)
			} else if imp != "C" {
				info.std = append(info.std, imp)
			}
		}
		for _, imp := range append(p.TestImports, p.XTestImports...) {
			if l, ok := relPkg(module, imp); ok && l != local {
				info.testImps = append(info.testImps, l)
			}
		}
		g.pkgs[local] = info
		g.order = append(g.order, local)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: go list reported errors:\n  %s", v.name, strings.Join(errs, "\n  "))
	}
	return g, nil
}

// path returns an import chain from one of roots to target, or nil.
func (g *graph) path(roots []string, target string) []string {
	parent := map[string]string{}
	queue := []string{}
	for _, r := range roots {
		if _, ok := g.pkgs[r]; ok {
			parent[r] = ""
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p == target {
			var chain []string
			for q := p; ; q = parent[q] {
				chain = append([]string{q}, chain...)
				if parent[q] == "" {
					return chain
				}
			}
		}
		for _, imp := range g.pkgs[p].imports {
			if _, seen := parent[imp]; !seen {
				if _, ok := g.pkgs[imp]; ok {
					parent[imp] = p
					queue = append(queue, imp)
				}
			}
		}
	}
	return nil
}

// stdPath returns a chain of standard library imports from one of p's imports to a package matching denied, or nil.
func (g *graph) stdPath(p string, denied []string) []string {
	parent := map[string]string{}
	queue := []string{}
	for _, s := range g.pkgs[p].std {
		if _, seen := parent[s]; !seen {
			parent[s] = ""
			queue = append(queue, s)
		}
	}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if allowed(denied, s) {
			var chain []string
			for q := s; q != ""; q = parent[q] {
				chain = append([]string{q}, chain...)
			}
			return chain
		}
		for _, imp := range g.std[s] {
			if _, seen := parent[imp]; !seen {
				parent[imp] = s
				queue = append(queue, imp)
			}
		}
	}
	return nil
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", errors.New("module directive not found in go.mod")
}

func relPkg(module, path string) (string, bool) {
	if path == module {
		return "", true
	}
	if rest, ok := strings.CutPrefix(path, module+"/"); ok {
		return rest, true
	}
	return "", false
}

func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

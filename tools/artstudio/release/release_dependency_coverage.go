package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// studioSourceCoversDir uses directory boundaries: "art-extra" must not be
// treated as covered by "art". The root itself is not an inventoried input.
func studioSourceCoversDir(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "." || !fs.ValidPath(rel) || strings.Contains(rel, "\\") {
		return false
	}
	for _, dir := range studioSourceDirs {
		if rel == dir || strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

// verifyStudioDependencyCoverage checks the actual build graph, not a parallel
// hand-maintained import allowlist. A local package outside sourceHash's inputs
// could otherwise embed an ignored file while the Git release guard is clean.
// This does not verify the Go toolchain or an external module cache, and is not
// a replacement for an isolated checkout and controlled build environment.
func verifyStudioDependencyCoverage(root string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	checkout, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	const format = `{{printf "%q\t%q\t%t" .ImportPath .Dir .Standard}}{{if .Module}}{{if .Module.Replace}}{{printf "\t%q\t%q" .Module.Replace.Path .Module.Replace.Version}}{{end}}{{end}}`
	list, err := command(root, []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"},
		studioGoArgs(true, "list", "-buildvcs=false", "-deps", "-f", format, "./tools/artstudio/wasm", "./tools/artstudio/previewwasm")...)
	if err != nil {
		return fmt.Errorf("cannot inspect release Go dependencies: %w", err)
	}
	roots := map[string]bool{"aego/tools/artstudio/wasm": false, "aego/tools/artstudio/previewwasm": false}
	for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 && len(fields) != 5 {
			return fmt.Errorf("invalid release Go dependency listing")
		}
		importPath, e1 := strconv.Unquote(fields[0])
		dir, e2 := strconv.Unquote(fields[1])
		standard, e3 := strconv.ParseBool(fields[2])
		if e1 != nil || e2 != nil || e3 != nil || importPath == "" {
			return fmt.Errorf("invalid release Go dependency listing")
		}
		if _, ok := roots[importPath]; ok {
			roots[importPath] = true
		}
		if standard {
			continue
		}
		if len(fields) == 5 {
			_, e1 := strconv.Unquote(fields[3])
			version, e2 := strconv.Unquote(fields[4])
			if e1 != nil || e2 != nil {
				return fmt.Errorf("invalid release Go dependency replacement")
			}
			if version == "" {
				return fmt.Errorf("local replacement for Go dependency %s is outside release source coverage", importPath)
			}
		}
		// Even a symlink resolving into a covered directory must not let an
		// uncovered main-module import path evade the Git/source inventory.
		if importPath == "aego" || strings.HasPrefix(importPath, "aego/") && !studioSourceCoversDir(strings.TrimPrefix(importPath, "aego/")) {
			return fmt.Errorf("release Go dependency %s is outside release source coverage", importPath)
		}
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("cannot identify release Go dependency %s", importPath)
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return fmt.Errorf("cannot identify release Go dependency %s: %w", importPath, err)
		}
		rel, relErr := filepath.Rel(checkout, resolved)
		inside := relErr == nil && (rel == "." || filepath.IsLocal(rel))
		rawRel, rawErr := filepath.Rel(absolute, dir)
		rawInside := rawErr == nil && (rawRel == "." || filepath.IsLocal(rawRel))
		if !rawInside && checkout != absolute {
			rawRel, rawErr = filepath.Rel(checkout, dir)
			rawInside = rawErr == nil && (rawRel == "." || filepath.IsLocal(rawRel))
		}
		if inside != rawInside || rawInside && !studioSourceCoversDir(rawRel) {
			return fmt.Errorf("release Go dependency %s is outside release source coverage", importPath)
		}
		if inside {
			if !studioSourceCoversDir(rel) {
				return fmt.Errorf("release Go dependency %s is outside release source coverage", importPath)
			}
			continue
		}
		if importPath == "aego" || strings.HasPrefix(importPath, "aego/") {
			return fmt.Errorf("release Go dependency %s is outside release source coverage", importPath)
		}
	}
	for _, found := range roots {
		if !found {
			return fmt.Errorf("release Go dependency listing is incomplete")
		}
	}
	return nil
}

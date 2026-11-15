package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
)

var engineSourceVersionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z][0-9A-Za-z.+-]*)?$`)

// sourceEngineVersion reads the exact source being packaged. Importing engine
// into this static builder would unnecessarily pull in native/render modules;
// using the builder executable's own version could describe a different tree.
// The older aegoVersion field records the requested compatible engine base.
func sourceEngineVersion(root string) (string, error) {
	file := filepath.Join(root, "engine", "config.go")
	data, err := readBounded(file, 1<<20)
	if err != nil {
		return "", fmt.Errorf("read engine source version: %w", err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, data, 0)
	if err != nil {
		return "", fmt.Errorf("parse engine source version: %w", err)
	}
	var version string
	found := false
	for _, declaration := range parsed.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || (group.Tok != token.VAR && group.Tok != token.CONST) {
			continue
		}
		for _, specification := range group.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range value.Names {
				if name.Name != "Version" {
					continue
				}
				if found || len(value.Values) != len(value.Names) {
					return "", fmt.Errorf("engine/config.go Version must have one explicit string value")
				}
				literal, ok := value.Values[index].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return "", fmt.Errorf("engine/config.go Version must be a string literal")
				}
				version, err = strconv.Unquote(literal.Value)
				if err != nil || !engineSourceVersionPattern.MatchString(version) {
					return "", fmt.Errorf("engine/config.go Version is not a supported version string")
				}
				found = true
			}
		}
	}
	if !found {
		return "", fmt.Errorf("engine/config.go has no explicit Version declaration")
	}
	return version, nil
}

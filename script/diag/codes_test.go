package diag

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	codeRe = regexp.MustCompile(`AES\d{4}`)
	nameRe = regexp.MustCompile(`\b(?:diag|vm)\.([A-Z]\w*)`)
)

var uncovered = map[string]string{
	BadConstType: "unreachable from source: every non-constant type is rejected earlier as NotConstant or TypeMismatch",
}

func declaredCodes(t *testing.T) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err == nil {
					out[n.Name] = v
				}
			}
		}
	}
	return out
}

func TestEveryCodeHasHelp(t *testing.T) {
	for name, c := range declaredCodes(t) {
		if _, ok := Explain(c); !ok {
			t.Errorf("%s (%s) has no explain entry", c, name)
		}
	}
}

func TestDocsMentionOnlyRealCodes(t *testing.T) {
	known := map[string]bool{}
	for _, c := range declaredCodes(t) {
		known[c] = true
	}
	var files []string
	for _, pat := range []string{"*/res/docs/*.md", "*/*/res/docs/*.md"} {
		m, _ := filepath.Glob(filepath.Join("..", "..", pat))
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("docs not found")
	}
	files = append(files, filepath.Join("..", "..", "ARCHITECTURE.md"))
	filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "doc.go" {
			files = append(files, p)
		}
		return nil
	})
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range codeRe.FindAllString(string(data), -1) {
			if !known[c] {
				t.Errorf("%s mentions %s, which is not declared in codes.go", f, c)
			}
		}
	}
}

func TestAllCodesHaveTest(t *testing.T) {
	codes := declaredCodes(t)
	seen := map[string]bool{}
	walk := func(root string) {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "diag" {
					return filepath.SkipDir
				}
				return nil
			}
			switch {
			case strings.HasSuffix(p, "_test.go"), strings.HasSuffix(p, ".diag"), strings.HasSuffix(p, ".ast"):
			default:
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			s := string(data)
			for _, c := range codeRe.FindAllString(s, -1) {
				seen[c] = true
			}
			for _, m := range nameRe.FindAllStringSubmatch(s, -1) {
				if c, ok := codes[m[1]]; ok {
					seen[c] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	walk("..")
	walk(filepath.Join("..", "..", "tests"))
	for name, c := range codes {
		if seen[c] {
			if _, listed := uncovered[c]; listed {
				t.Errorf("%s (%s) is tested now, remove it from the uncovered list", c, name)
			}
			continue
		}
		if _, ok := uncovered[c]; ok {
			continue
		}
		t.Errorf("%s (%s) has no test", c, name)
	}
}

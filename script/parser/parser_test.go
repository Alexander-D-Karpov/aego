package parser

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/script/ast"
	"aego/script/diag"
	"aego/script/source"
	"aego/script/token"
)

var update = flag.Bool("update", false, "rewrite golden AST dumps")

const scriptsDir = "../../tests/scripts"

func parse(t *testing.T, src string) (*ast.File, []diag.Diagnostic, *source.Manager) {
	t.Helper()
	sm := source.NewManager()
	f, ds := ParseSource(sm, "test.aes", []byte(src))
	return f, ds, sm
}

func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(scriptsDir, "*.aes"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no inputs in %s: %v", scriptsDir, err)
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sm := source.NewManager()
			f, ds := ParseSource(sm, name, src)
			var b bytes.Buffer
			ast.Fprint(&b, sm, f)
			if len(ds) > 0 {
				b.WriteString("--- diagnostics\n")
				diag.Print(&b, sm, ds)
			}
			golden := filepath.Join("testdata", strings.TrimSuffix(name, ".aes")+".ast")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, b.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden missing, run go test ./script/parser -update: %v", err)
			}
			if !bytes.Equal(want, b.Bytes()) {
				t.Errorf("AST dump changed\n--- got\n%s", b.String())
			}
		})
	}
}

func TestCleanFilesHaveNoDiagnostics(t *testing.T) {
	for _, name := range []string{"player.aes", "syntax.aes", "coroutines.aes", "signals.aes", "scriptref.aes"} {
		src, err := os.ReadFile(filepath.Join(scriptsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		sm := source.NewManager()
		if _, ds := ParseSource(sm, name, src); len(ds) > 0 {
			var b bytes.Buffer
			diag.Print(&b, sm, ds)
			t.Errorf("%s:\n%s", name, b.String())
		}
	}
}

func TestRecoveryReportsAllErrors(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(scriptsDir, "errors.aes"))
	if err != nil {
		t.Fatal(err)
	}
	sm := source.NewManager()
	f, ds := ParseSource(sm, "errors.aes", src)
	want := map[int]string{
		5:  diag.PackageVar,
		9:  diag.UnexpectedToken,
		12: diag.UnexpectedToken,
		14: diag.UnexpectedToken,
		17: diag.MisplacedAttr,
	}
	got := map[int]string{}
	for _, d := range ds {
		line, _ := sm.LineCol(d.Primary.File, d.Primary.Start)
		got[line] = d.Code
	}
	for line, code := range want {
		if got[line] != code {
			t.Errorf("line %d: got %q want %q", line, got[line], code)
		}
	}
	if len(got) != len(want) {
		var b bytes.Buffer
		diag.Print(&b, sm, ds)
		t.Errorf("unexpected diagnostics:\n%s", b.String())
	}
	funcs := 0
	for _, d := range f.Decls {
		if _, ok := d.(*ast.FuncDecl); ok {
			funcs++
		}
	}
	if funcs != 3 {
		t.Errorf("recovery lost functions, kept %d", funcs)
	}
}

func TestTypeArgsVersusLess(t *testing.T) {
	f, ds, _ := parse(t, "package p\nfunc f() {\n\tif node.Has<SpriteRenderer>() {\n\t}\n\tb := a < c\n\tx := g<A, B<C>>(1)\n}\n")
	if len(ds) > 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	var calls, less int
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if len(x.TypeArgs) > 0 {
				calls++
			}
		case *ast.BinaryExpr:
			if x.Op == token.Lt {
				less++
			}
		}
		return true
	})
	if calls != 2 || less != 1 {
		t.Fatalf("generic calls %d comparisons %d", calls, less)
	}
}

func TestCompositeInHeaders(t *testing.T) {
	f, ds, _ := parse(t, "package p\nfunc f() {\n\tif x == y {\n\t\ts := Pair{A: 1}\n\t}\n\tfor v {\n\t}\n\tl := []int{1, 2,}\n\tif (Pair{A: 1}) == p {\n\t}\n}\n")
	if len(ds) > 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	lits := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.CompositeLit); ok {
			lits++
		}
		return true
	})
	if lits != 3 {
		t.Fatalf("composite literals %d", lits)
	}
}

func TestAttributesOnOwnLine(t *testing.T) {
	f, ds, _ := parse(t, "package p\n@hide\nscript S {\n\t@export(min=-1, unit=\"px\")\n\t@former(\"Old\")\n\tvar Speed float = 1\n}\n")
	if len(ds) > 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	s := f.Decls[0].(*ast.ScriptDecl)
	if len(s.Attrs) != 1 || s.Attrs[0].Name.Name != "hide" {
		t.Fatal("script attribute lost")
	}
	v := s.Vars[0]
	if len(v.Attrs) != 2 {
		t.Fatalf("field attrs %d", len(v.Attrs))
	}
	arg, ok := v.Attrs[0].Arg("min")
	if !ok {
		t.Fatal("min missing")
	}
	if u, ok := arg.Value.(*ast.UnaryExpr); !ok || u.Op != token.Sub {
		t.Fatal("negative attribute value must parse as unary minus")
	}
}

func TestMissingPackageAndLateImport(t *testing.T) {
	_, ds, _ := parse(t, "func f() {\n}\nimport \"x\"\n")
	codes := map[string]bool{}
	for _, d := range ds {
		codes[d.Code] = true
	}
	if !codes[diag.MissingPackage] || !codes[diag.LateImport] {
		t.Fatalf("codes %v", codes)
	}
}

func TestDeepNesting(t *testing.T) {
	src := "package p\nconst X = " + strings.Repeat("(", 5000) + "1" + strings.Repeat(")", 5000) + "\nfunc f() " + strings.Repeat("{", 3000) + strings.Repeat("}", 3000) + "\n"
	_, ds, _ := parse(t, src)
	found := false
	for _, d := range ds {
		if d.Code == diag.TooDeep {
			found = true
		}
	}
	if !found {
		t.Fatal("deep nesting must be reported")
	}
}

func FuzzParse(f *testing.F) {
	if files, err := filepath.Glob(filepath.Join(scriptsDir, "*.aes")); err == nil {
		for _, path := range files {
			if src, err := os.ReadFile(path); err == nil {
				f.Add(src)
			}
		}
	}
	f.Add([]byte("package p\nscript S { var A int = ( }"))
	f.Add([]byte("package p\nfunc f() { for ;; { switch x { case 1, : } } }"))
	f.Add([]byte("@@@ { [ ( < > ) ] }"))
	f.Fuzz(func(t *testing.T, src []byte) {
		sm := source.NewManager()
		file, _ := ParseSource(sm, "fuzz.aes", src)
		if file == nil {
			t.Fatal("nil file")
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sp := n.Span()
			if sp.Start > sp.End || int(sp.End) > len(src) {
				t.Fatalf("bad span %+v in %s", sp, ast.Label(n))
			}
			return true
		})
	})
}

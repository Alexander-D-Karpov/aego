package parser

import (
	"testing"

	"aego/script/ast"
	"aego/script/diag"
	"aego/script/token"
)

func hasCode(ds []diag.Diagnostic, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestAttrArgMustBeLiteral(t *testing.T) {
	if _, ds, _ := parse(t, "package p\n@a(min=f())\nscript S {\n\tvar X int\n}\n"); !hasCode(ds, diag.BadAttrArg) {
		t.Errorf("call in attribute arg must be rejected: %v", ds)
	}
	if _, ds, _ := parse(t, "package p\n@a(min=a+b)\nscript S {\n\tvar X int\n}\n"); !hasCode(ds, diag.BadAttrArg) {
		t.Errorf("binary expr in attribute arg must be rejected: %v", ds)
	}
	// A plain literal and a negated literal are the two allowed forms.
	if _, ds, _ := parse(t, "package p\n@a(min=-1, max=10, unit=\"px\")\nscript S {\n\tvar X int\n}\n"); len(ds) != 0 {
		t.Errorf("literal attribute args must be accepted: %v", ds)
	}
}

func TestFunctionAttributesAttach(t *testing.T) {
	// Top-level function.
	f, ds, _ := parse(t, "package p\n@rpc\nfunc f() {\n}\n")
	if len(ds) != 0 {
		t.Fatalf("attributes on a top-level func must be accepted: %v", ds)
	}
	fn, ok := f.Decls[0].(*ast.FuncDecl)
	if !ok || len(fn.Attrs) != 1 || fn.Attrs[0].Name.Name != "rpc" {
		t.Fatalf("top-level func attribute lost: %+v", f.Decls[0])
	}
	// Script method.
	f, ds, _ = parse(t, "package p\nscript S {\n\t@rpc\n\tfunc M() {\n\t}\n}\n")
	if len(ds) != 0 {
		t.Fatalf("attributes on a script method must be accepted: %v", ds)
	}
	s := f.Decls[0].(*ast.ScriptDecl)
	if len(s.Funcs) != 1 || len(s.Funcs[0].Attrs) != 1 || s.Funcs[0].Attrs[0].Name.Name != "rpc" {
		t.Fatalf("method attribute lost: %+v", s.Funcs)
	}
}

func TestAttrMisplacedOnOtherDecls(t *testing.T) {
	// const/type/enum still reject attributes.
	if _, ds, _ := parse(t, "package p\n@a\nconst X = 1\n"); !hasCode(ds, diag.MisplacedAttr) {
		t.Errorf("attribute on const must be misplaced: %v", ds)
	}
}

func TestElseOnNewLine(t *testing.T) {
	// A single diagnostic, not a cascade from the orphaned block.
	_, ds, _ := parse(t, "package p\nfunc f() {\n\tif x {\n\t}\n\telse {\n\t}\n}\n")
	if len(ds) != 1 || ds[0].Code != diag.ElseOnNewLine {
		t.Errorf("else on new line: %v", ds)
	}
	_, ds, _ = parse(t, "package p\nfunc f() {\n\tif x {\n\t}\n\telse if y {\n\t}\n}\n")
	if len(ds) != 1 || ds[0].Code != diag.ElseOnNewLine {
		t.Errorf("else-if on new line: %v", ds)
	}
}

func TestAttrInFunctionBody(t *testing.T) {
	_, ds, _ := parse(t, "package p\nfunc f() {\n\t@export\n\tx := 1\n}\n")
	if !hasCode(ds, diag.MisplacedAttr) {
		t.Errorf("attribute inside a function body must be reported: %v", ds)
	}
}

func TestMultiAssignRejected(t *testing.T) {
	_, ds, _ := parse(t, "package p\nfunc f() {\n\tx, y := g()\n}\n")
	if !hasCode(ds, diag.MultiAssign) {
		t.Errorf("multiple assignment must be reported: %v", ds)
	}
}

func TestCompositeInControlHeaderRejected(t *testing.T) {
	// Like Go: a composite literal in an if/for header needs parens.
	if _, ds, _ := parse(t, "package p\nfunc f() {\n\tif x == Stats{} {\n\t}\n}\n"); len(ds) == 0 {
		t.Error("bare composite in if header must be rejected")
	}
	if _, ds, _ := parse(t, "package p\nfunc f() {\n\tfor s := Stats{}; s.ok; {\n\t}\n}\n"); len(ds) == 0 {
		t.Error("bare composite in for header must be rejected")
	}
}

func TestLessVersusTypeArgsWithCall(t *testing.T) {
	// `count < max(a, b)` is a comparison, not Get<...>() type args.
	f, ds, _ := parse(t, "package p\nfunc f() {\n\tb := count < max(a, b)\n}\n")
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	var less, targCalls int
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op == token.Lt {
				less++
			}
		case *ast.CallExpr:
			if len(x.TypeArgs) > 0 {
				targCalls++
			}
		}
		return true
	})
	if less != 1 || targCalls != 0 {
		t.Fatalf("comparisons %d type-arg calls %d", less, targCalls)
	}
}

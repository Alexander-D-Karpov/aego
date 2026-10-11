package checker

import (
	"testing"

	"aego/script/diag"
)

func TestRemainingCodes(t *testing.T) {
	cases := []struct {
		code string
		src  string
	}{
		{diag.NotAType, "package t\nconst X = 1\nfunc f() {\n\tvar y X\n}\n"},
		{diag.InvalidOperation, "package t\nfunc f() bool {\n\treturn !1\n}\n"},
		{diag.NotCallable, "package t\nfunc f() {\n\tx := 1\n\tx()\n}\n"},
		{diag.UnknownField, "package t\nfunc f(v Vec2) float {\n\treturn v.Q\n}\n"},
		{diag.NotConstant, "package t\nfunc g() int {\n\treturn 1\n}\nconst X = g()\n"},
		{diag.Redeclared, "package t\nfunc f() int {\n\tx := 1\n\tx := 2\n\treturn x\n}\n"},
		{diag.ReturnMismatch, "package t\nfunc f() {\n\treturn 1\n}\n"},
		{diag.NotValue, "package t\nfunc f() {\n\tx := int\n}\n"},
		{diag.InvalidComposite, "package t\ntype P struct {\n\tA int\n}\nfunc f() P {\n\treturn P{1, 2}\n}\n"},
		{diag.DuplicateAttr, "package t\nscript S {\n\t@export\n\t@export\n\tvar A int\n}\n"},
		{diag.DuplicateImport, "package t\nimport (\n\t\"aego/log\"\n\t\"aego/log\"\n)\nfunc f() {\n\tlog.Info(\"x\")\n}\n"},
		{diag.SelfImport, "package t\nimport \"game/t\"\n"},
		{diag.DuplicateDecl, "package t\nconst A = 1\nconst A = 2\n"},
		{diag.ImportConflict, "package t\nimport \"aego/log\"\nconst log = 1\n"},
	}
	for _, tc := range cases {
		_, sm, ds := check(t, map[string]string{"game/t": tc.src})
		if codes(ds)[tc.code] == 0 {
			t.Errorf("%s not reported:\n%s", tc.code, dump(sm, ds))
		}
	}
}

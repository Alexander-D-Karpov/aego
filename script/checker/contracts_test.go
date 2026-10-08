package checker

import (
	"testing"

	"aego/script/diag"
)

func TestPackageNameMustMatchDir(t *testing.T) {
	_, sm, ds := check(t, map[string]string{"game/wrong": "package notwrong\nfunc f() {\n}\n"})
	if codes(ds)[diag.PackageName] == 0 {
		t.Fatalf("package name must match its directory:\n%s", dump(sm, ds))
	}
	if _, _, ds := check(t, map[string]string{"game/right": "package right\nfunc f() {\n}\n"}); codes(ds)[diag.PackageName] != 0 {
		t.Fatalf("matching name must be accepted: %v", codes(ds))
	}
}

func TestFunctionAttributesRejected(t *testing.T) {
	_, sm, ds := check(t, map[string]string{"game/t": "package t\n@rpc\nfunc f() {\n}\n"})
	if codes(ds)[diag.AttrTarget] == 0 {
		t.Fatalf("attribute on a top-level func must be rejected:\n%s", dump(sm, ds))
	}
	_, sm, ds = check(t, map[string]string{"game/t": "package t\nscript S {\n\t@rpc\n\tfunc M() {\n\t}\n}\n"})
	if codes(ds)[diag.AttrTarget] == 0 {
		t.Fatalf("attribute on a script method must be rejected:\n%s", dump(sm, ds))
	}
}

func TestUnusedLocalVar(t *testing.T) {
	_, sm, ds := check(t, map[string]string{"game/t": "package t\nfunc f() {\n\tx := 1\n}\n"})
	if codes(ds)[diag.UnusedVar] == 0 {
		t.Fatalf("unused local must warn:\n%s", dump(sm, ds))
	}
	// A var touched only through an assignment counts as used (matches the
	// checker, where the assign target is evaluated and marked used).
	if _, _, ds := check(t, map[string]string{"game/t": "package t\nfunc f() int {\n\tx := 1\n\treturn x\n}\n"}); codes(ds)[diag.UnusedVar] != 0 {
		t.Fatalf("used local must not warn: %v", codes(ds))
	}
}

func TestVectorConstructor(t *testing.T) {
	// Composite form is the only constructor; the call form is an arity error
	// (Color(x) is a one-argument conversion). Documented in docs/script.md.
	if _, sm, ds := check(t, map[string]string{"game/t": "package t\nfunc f() Color {\n\treturn Color{1, 0, 0, 1}\n}\n"}); len(ds) != 0 {
		t.Fatalf("vector composite must be accepted:\n%s", dump(sm, ds))
	}
	if _, _, ds := check(t, map[string]string{"game/t": "package t\nfunc f() Color {\n\treturn Color(1, 0, 0, 1)\n}\n"}); len(ds) == 0 {
		t.Fatal("vector call form must be rejected")
	}
}

func TestCyclicPackageSkipsBodies(t *testing.T) {
	// The cycle is reported once; bodies of cyclic packages are not checked, so
	// there is no undefined/not-exported cascade on top of it.
	_, sm, ds := check(t, map[string]string{
		"game/a": "package a\nimport \"game/b\"\nfunc fa() {\n\tb.fb()\n}\n",
		"game/b": "package b\nimport \"game/a\"\nfunc fb() {\n\ta.fa()\n}\n",
	})
	c := codes(ds)
	if c[diag.ImportCycle] == 0 {
		t.Fatalf("cycle not reported:\n%s", dump(sm, ds))
	}
	if c[diag.UndefinedName] != 0 || c[diag.NotExported] != 0 {
		t.Fatalf("cyclic package bodies must be skipped, no cascade:\n%s", dump(sm, ds))
	}
}

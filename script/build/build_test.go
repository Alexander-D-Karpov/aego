package build_test

import (
	"strings"
	"testing"

	"aego/script/abi"
	"aego/script/bind"
	"aego/script/build"
	"aego/script/diag"
)

func surface(t *testing.T) abi.Surface {
	t.Helper()
	r := bind.New()
	bind.Proc0(r.Module("game/combat"), "Hit", func(*bind.Ctx) {})
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	return r.Surface()
}

func src(path, text string) build.File {
	return build.File{Path: path, Src: []byte(text)}
}

func TestBrokenDependencyBlocksDependents(t *testing.T) {
	res := build.Build(map[string][]build.File{
		"game/a": {src("scripts/a/a.aes", "package a\n\nfunc F() int {\n\treturn 1 +\n}\n")},
		"game/b": {src("scripts/b/b.aes", "package b\n\nimport \"game/a\"\n\nfunc G() int {\n\treturn a.F()\n}\n")},
		"game/c": {src("scripts/c/c.aes", "package c\n\nfunc H() int {\n\treturn 3\n}\n")},
	}, surface(t), build.Options{})
	if !res.HasErrors() {
		t.Fatal("parse error lost")
	}
	if _, ok := res.Modules["game/b"]; ok {
		t.Fatal("dependent of a broken package was compiled")
	}
	if _, ok := res.Modules["game/c"]; !ok {
		t.Fatal("independent package was not compiled")
	}
	if got := strings.Join(res.Invalid, ","); got != "game/a,game/b" {
		t.Fatalf("invalid = %s", got)
	}
}

func TestTestFilesOnlyInTestMode(t *testing.T) {
	pkgs := map[string][]build.File{"game/t": {
		src("scripts/t/t.aes", "package t\n"),
		src("scripts/t/t_test.aes", "package t\n\nfunc TestX() {\n}\n"),
	}}
	has := func(res *build.Result) bool {
		return res.Modules["game/t"].FuncIndex("TestX") >= 0
	}
	if has(build.Build(pkgs, surface(t), build.Options{})) {
		t.Fatal("test function compiled outside test mode")
	}
	if !has(build.Build(pkgs, surface(t), build.Options{Tests: true})) {
		t.Fatal("test function missing in test mode")
	}
}

func TestModuleConflict(t *testing.T) {
	res := build.Build(map[string][]build.File{
		"game/combat": {src("scripts/combat/combat.aes", "package combat\n")},
	}, surface(t), build.Options{})
	for _, d := range res.Diagnostics {
		if d.Code == diag.ModuleConflict {
			return
		}
	}
	t.Fatalf("conflict not reported: %v", res.Diagnostics)
}

func TestPackagePath(t *testing.T) {
	for in, want := range map[string]string{".": "mygame", "player": "mygame/player", "enemy/ai": "mygame/enemy/ai"} {
		if got := build.PackagePath("mygame", in); got != want {
			t.Fatalf("%s -> %s", in, got)
		}
	}
}

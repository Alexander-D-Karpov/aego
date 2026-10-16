package checker

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
	"aego/script/types"
)

var update = flag.Bool("update", false, "rewrite golden diagnostics")

const scriptsDir = "../../tests/scripts"

func surface() *abi.Table {
	t := abi.NewTable()
	f := abi.T(abi.Float)
	t.AddFunc("aego/log", "Info", abi.Sig(abi.T(abi.Void), abi.T(abi.String)))
	t.AddFunc("aego/math", "Clamp", abi.Sig(f, f, f, f))
	t.AddFunc("aego/math", "Abs", abi.Sig(f, f))
	t.AddFunc("aego/input", "Axis", abi.Sig(f, abi.T(abi.String)))
	t.AddEnum(&abi.EnumDef{Module: "aego/input", Name: "Key", Members: []abi.EnumMember{{Name: "A", Value: 4}, {Name: "Space", Value: 44}}})
	t.AddConst("aego/input", "KeySpace", abi.Const{Type: abi.EnumOf("aego/input.Key"), Int: 44})
	t.AddFunc("aego/input", "Pressed", abi.Sig(abi.T(abi.Bool), abi.EnumOf("aego/input.Key")))
	t.AddFunc("mygame/combat", "Damage", abi.Sig(abi.T(abi.Void), abi.T(abi.NodeRef), abi.T(abi.Int)))
	t.AddStruct(&abi.StructDef{Module: "aego/probe", Name: "RayHit", Slots: 3, Fields: []abi.StructField{
		{Name: "Point", Type: abi.T(abi.Vec2)},
		{Name: "Distance", Type: f},
		{Name: "Node", Type: abi.T(abi.NodeRef)},
	}})
	t.AddFunc("aego/probe", "Raycast", abi.Sig(abi.StructOf("aego/probe.RayHit"), abi.T(abi.Vec2), abi.T(abi.Vec2)))
	t.AddEvent(abi.EventDef{Name: "BodyEntered", Params: []abi.TypeRef{abi.T(abi.NodeRef)}})
	t.AddMethod(abi.NodeRef, "Transform2D", abi.Sig(abi.T(abi.Transform2D)))
	t.AddMethod(abi.NodeRef, "SetTransform2D", abi.Sig(abi.T(abi.Void), abi.T(abi.Transform2D)))
	t.AddComponent(&abi.Component{Name: "engine.SpriteRenderer", Fields: []abi.Field{
		{Name: "FlipX", Type: abi.T(abi.Bool)},
		{Name: "Order", Type: abi.T(abi.Int)},
		{Name: "Id", Type: abi.T(abi.Int), ReadOnly: true},
	}})
	t.AddComponent(&abi.Component{Name: "engine.Camera2D", Fields: []abi.Field{{Name: "Zoom", Type: f}}})
	t.AddComponentMethod("engine.SpriteRenderer", "Flip", abi.Sig(abi.T(abi.Void), abi.T(abi.Bool)))
	t.AddComponentMethod("engine.SpriteRenderer", "Pulse", abi.Sig(f, f, abi.T(abi.Vec2)))
	t.AddComponent(&abi.Component{Name: "game.Camera2D", Fields: []abi.Field{{Name: "Shake", Type: f}}})
	t.Freeze()
	return t
}

func check(t *testing.T, pkgs map[string]string) (*Analysis, *source.Manager, []diag.Diagnostic) {
	t.Helper()
	sm := source.NewManager()
	files := map[string][]*ast.File{}
	var all []diag.Diagnostic
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f, ds := parser.ParseSource(sm, p+"/main.aes", []byte(pkgs[p]))
		files[p] = append(files[p], f)
		all = append(all, ds...)
	}
	a := Check(sm, files, surface(), Options{})
	all = append(all, a.Diagnostics...)
	diag.Sort(all)
	return a, sm, all
}

func codes(ds []diag.Diagnostic) map[string]int {
	out := map[string]int{}
	for _, d := range ds {
		out[d.Code]++
	}
	return out
}

func dump(sm *source.Manager, ds []diag.Diagnostic) string {
	var b bytes.Buffer
	diag.Print(&b, sm, ds)
	return b.String()
}

var goldenPackages = map[string]string{
	"player.aes": "mygame/player",
	"syntax.aes": "game/syntax",
	"errors.aes": "game/broken",
}

func TestGoldenDiagnostics(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(scriptsDir, "*.aes"))
	if len(files) == 0 {
		t.Fatal("no inputs")
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			pkg, ok := goldenPackages[name]
			if !ok {
				pkg = "game/" + strings.TrimSuffix(name, ".aes")
			}
			_, sm, ds := check(t, map[string]string{pkg: string(src)})
			got := dump(sm, ds)
			golden := filepath.Join("testdata", strings.TrimSuffix(name, ".aes")+".diag")
			if *update {
				os.MkdirAll("testdata", 0o755)
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden missing, run go test ./script/checker -update: %v", err)
			}
			if string(want) != got {
				t.Errorf("diagnostics changed\n--- got\n%s", got)
			}
		})
	}
}

func TestPlayerIsClean(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(scriptsDir, "player.aes"))
	if err != nil {
		t.Fatal(err)
	}
	a, sm, ds := check(t, map[string]string{"mygame/player": string(src)})
	if len(ds) > 0 {
		t.Fatalf("player.aes:\n%s", dump(sm, ds))
	}
	var get, host int
	for _, f := range a.Packages["mygame/player"].Res.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				switch k := a.Calls[call]; {
				case k == nil:
				case k.Kind == CallGetComponent && k.Component.Name == "engine.SpriteRenderer":
					get++
				case k.Kind == CallHost && k.Module == "mygame/combat" && k.Name == "Damage":
					host++
				}
			}
			return true
		})
	}
	if get != 1 || host != 1 {
		t.Fatalf("get %d host %d", get, host)
	}
}

func TestSyntaxHasOnlyMisplacedContinue(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(scriptsDir, "syntax.aes"))
	if err != nil {
		t.Fatal(err)
	}
	_, sm, ds := check(t, map[string]string{"game/syntax": string(src)})
	if len(ds) != 1 || ds[0].Code != diag.MisplacedBranch {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
}

func TestDiagnosticCodes(t *testing.T) {
	script := func(body string) string {
		return "package t\nscript S {\n\tfunc Ready() {\n" + body + "\n\t}\n}\n"
	}
	cases := []struct {
		name string
		src  string
		code string
	}{
		{"mismatch", "package t\nfunc f() {\n\tvar x int = 1.5\n}\n", diag.TypeMismatch},
		{"undefined", "package t\nfunc f() {\n\ty := z\n}\n", diag.UndefinedName},
		{"exhaustive", "package t\nenum E {\n\tA\n\tB\n}\nfunc f(e E) {\n\tswitch e {\n\tcase E.A:\n\t}\n}\n", diag.NotExhaustive},
		{"missingReturn", "package t\nfunc f() int {\n\tif true {\n\t\treturn 1\n\t}\n}\n", diag.MissingReturn},
		{"exportType", "package t\nscript S {\n\t@export\n\tvar A []int\n}\n", diag.NotExportable},
		{"exportArg", "package t\nscript S {\n\t@export(colour=1)\n\tvar A int\n}\n", diag.BadAttrArg},
		{"exportRange", "package t\nscript S {\n\t@export(min=5, max=1)\n\tvar A int\n}\n", diag.BadAttrArg},
		{"gizmoKind", "package t\nscript S {\n\t@export(gizmo=\"vector\")\n\tvar A float\n}\n", diag.BadAttrArg},
		{"gizmoSpace", "package t\nscript S {\n\t@export(space=\"world\")\n\tvar A Vec2\n}\n", diag.BadAttrArg},
		{"unknownAttr", "package t\n@tool\nscript S {\n}\n", diag.UnknownAttr},
		{"lifecycle", "package t\nscript S {\n\tfunc Update() {\n\t}\n}\n", diag.Lifecycle},
		{"recursive", "package t\ntype A struct {\n\tB B\n}\ntype B struct {\n\tA A\n}\n", diag.RecursiveStruct},
		{"constCycle", "package t\nconst A = B\nconst B = A\n", diag.ConstCycle},
		{"unknownComponent", script("\t\tx := node.Has<Missing>()\n\t\tif x {\n\t\t}"), diag.BadTypeArg},
		{"ambiguousComponent", script("\t\tc := node.Get<Camera2D>()\n\t\tif c != nil {\n\t\t}"), diag.BadTypeArg},
		{"typeArgsElsewhere", "package t\nfunc f() int {\n\treturn len<int>(\"x\")\n}\n", diag.BadTypeArg},
		{"nil", "package t\nfunc f() {\n\tx := nil\n}\n", diag.NilMisuse},
		{"break", "package t\nfunc f() {\n\tbreak\n}\n", diag.MisplacedBranch},
		{"dupCase", "package t\nfunc f(x int) {\n\tswitch x {\n\tcase 1:\n\tcase 1:\n\t}\n}\n", diag.DuplicateCase},
		{"notAssignable", script("\t\tnode.Transform2D().Rotation = 1"), diag.NotAssignable},
		{"readonly", script("\t\tnode.Get<SpriteRenderer>().Id = 3"), diag.NotAssignable},
		{"overflow", "package t\nconst N = 9223372036854775808\n", diag.ConstOverflow},
		{"divZero", "package t\nconst N = 1 / 0\n", diag.DivisionByZero},
		{"unusedImport", "package t\nimport \"aego/log\"\n", diag.UnusedImport},
		{"unknownImport", "package t\nimport \"nope/x\"\n", diag.UnknownImport},
		{"unknownHost", "package t\nimport \"aego/log\"\nfunc f() {\n\tlog.Shout(\"x\")\n}\n", diag.UnknownHost},
		{"unused", "package t\nfunc f(x int) {\n\tx + 1\n}\n", diag.Unused},
		{"argCount", "package t\nfunc g(a int) {\n}\nfunc f() {\n\tg(1, 2)\n}\n", diag.ArgCount},
		{"conversion", "package t\nfunc f() bool {\n\treturn bool(1)\n}\n", diag.BadConversion},
		{"fieldInit", "package t\nfunc v() int {\n\treturn 1\n}\nscript S {\n\tvar A int = v()\n}\n", diag.FieldInit},
		{"reserved", "package aego\n", diag.ReservedPackage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "game/t"
			if tc.code == diag.ReservedPackage {
				path = "aego/x"
			}
			_, sm, ds := check(t, map[string]string{path: tc.src})
			if codes(ds)[tc.code] == 0 {
				t.Fatalf("want %s, got:\n%s", tc.code, dump(sm, ds))
			}
		})
	}
}

func TestCleanSnippets(t *testing.T) {
	for name, src := range map[string]string{
		"vectorMath": "package t\nfunc f(a Vec2, s float) Vec2 {\n\treturn (a + a) * s / 2\n}\n",
		"minInt":     "package t\nconst M = -9223372036854775808\n",
		"untyped":    "package t\nconst R = .5\nfunc f(x float) bool {\n\treturn x*2 > R && 1 < 2\n}\n",
		"strings":    "package t\nfunc f(n int) string {\n\treturn \"n=\" + string(n) + string(1.5)\n}\n",
		"slices":     "package t\nfunc f() int {\n\ts := []int{1, 2}\n\ts = append(s, 3)\n\ts[0] += 4\n\treturn len(s) + s[0]\n}\n",
		"loopReturn": "package t\nfunc f() int {\n\tfor {\n\t\treturn 1\n\t}\n}\n",
		"enumSwitch": "package t\nenum E {\n\tA\n\tB\n}\nfunc f(e E) int {\n\tswitch e {\n\tcase E.A:\n\t\treturn 1\n\tdefault:\n\t\treturn 2\n\t}\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, sm, ds := check(t, map[string]string{"game/t": src})
			if len(ds) > 0 {
				t.Fatalf("diagnostics:\n%s", dump(sm, ds))
			}
		})
	}
}

func TestScriptMetadata(t *testing.T) {
	src := `package t

@hide
@former("Old")
script Mover {
	@export(min=0, max=10, step=0.5, unit="m/s", display="Speed", tip="how fast", category="Motion")
	@former("Velocity")
	var Speed float = 2
	var Pose Transform2D
	@export
	var Mode Kind
	var names []string

	func Ready() {
	}

	func Update(dt float) {
	}
}

enum Kind {
	A
	B
}
`
	a, sm, ds := check(t, map[string]string{"game/t": src})
	if len(ds) > 0 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	st := a.Scripts[0]
	if st.FullName != "game.t.Mover" || !st.Hidden || len(st.Former) != 1 || st.Former[0] != "Old" {
		t.Fatalf("script %+v", st)
	}
	if st.Slots != 6 || st.Fields[1].Slot != 1 || st.Fields[2].Slot != 4 || st.Fields[3].Slot != 5 {
		t.Fatalf("layout slots=%d", st.Slots)
	}
	sp := st.Fields[0]
	if !sp.Export || sp.Hints.Max != 10 || sp.Hints.Step != 0.5 || sp.Hints.Unit != "m/s" || sp.Former[0] != "Velocity" {
		t.Fatalf("speed %+v", sp)
	}
	if sp.Default == nil || sp.Default.Float != 2 || sp.Default.Kind != types.CFloat {
		t.Fatalf("default %+v", sp.Default)
	}
	if st.Ready == nil || st.Update == nil || st.FixedUpdate != nil {
		t.Fatal("lifecycle")
	}
}

func TestExportGizmo(t *testing.T) {
	src := "package t\nscript S {\n\t@export(gizmo=\"vector\", space=\"world\")\n\tvar Push Vec2\n\t@export(gizmo=\"radius\")\n\tvar R float\n}\n"
	a, sm, ds := check(t, map[string]string{"game/t": src})
	if len(ds) > 0 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	fs := a.Scripts[0].Fields
	if fs[0].Hints.Gizmo != "vector" || fs[0].Hints.Space != "world" || fs[1].Hints.Gizmo != "radius" {
		t.Fatalf("hints %+v %+v", fs[0].Hints, fs[1].Hints)
	}
}

func TestPackages(t *testing.T) {
	_, sm, ds := check(t, map[string]string{
		"game/a": "package a\nconst Public = 1\nconst hidden = 2\ntype P struct {\n\tX int\n\ty int\n}\n",
		"game/b": "package b\nimport \"game/a\"\nfunc f() int {\n\tp := a.P{X: 1}\n\treturn a.Public + p.X + a.hidden\n}\n",
	})
	if c := codes(ds); c[diag.NotExported] != 1 || len(ds) != 1 {
		t.Fatalf("diagnostics:\n%s", dump(sm, ds))
	}
	_, sm, ds = check(t, map[string]string{
		"game/a": "package a\nimport \"game/b\"\nconst A = b.B\n",
		"game/b": "package b\nimport \"game/a\"\nconst B = 1\nconst C = a.A\n",
	})
	if codes(ds)[diag.ImportCycle] == 0 {
		t.Fatalf("cycle not found:\n%s", dump(sm, ds))
	}
}

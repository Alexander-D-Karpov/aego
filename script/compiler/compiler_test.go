package compiler

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
	"aego/script/verify"
	"aego/script/vm"
)

var update = flag.Bool("update", false, "rewrite golden disassembly")

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
	t.AddMethod(abi.NodeRef, "Transform2D", abi.Sig(abi.T(abi.Transform2D)))
	t.AddMethod(abi.NodeRef, "SetTransform2D", abi.Sig(abi.T(abi.Void), abi.T(abi.Transform2D)))
	t.AddComponent(&abi.Component{Name: "engine.SpriteRenderer", Fields: []abi.Field{{Name: "FlipX", Type: abi.T(abi.Bool)}}})
	t.Freeze()
	return t
}

type record struct {
	pose   [3]vm.Value
	damage []int64
	logs   []string
}

type comps map[int]vm.Value

func (c comps) GetField(ref vm.Value, f int, dst *vm.Value) error {
	*dst = c[f]
	return nil
}

func (c comps) SetField(ref vm.Value, f int, v vm.Value) error {
	c[f] = v
	return nil
}

func hosts(rec *record) func(bytecode.HostImport) (vm.HostFunc, bool) {
	table := map[string]vm.HostFunc{
		"aego/log.Info": func(m *vm.Machine, w []vm.Value) error {
			s, _ := m.Str(w[0])
			rec.logs = append(rec.logs, s)
			return nil
		},
		"aego/math.Clamp": func(m *vm.Machine, w []vm.Value) error {
			v, lo, hi := w[0].Float(), w[1].Float(), w[2].Float()
			w[0] = vm.Float(min(max(v, lo), hi))
			return nil
		},
		"aego/math.Abs": func(m *vm.Machine, w []vm.Value) error {
			w[0] = vm.Float(max(w[0].Float(), -w[0].Float()))
			return nil
		},
		"aego/input.Axis":    func(m *vm.Machine, w []vm.Value) error { w[0] = vm.Float(1); return nil },
		"aego/input.Pressed": func(m *vm.Machine, w []vm.Value) error { w[0] = vm.Bool(w[0].Int() == 44); return nil },
		"mygame/combat.Damage": func(m *vm.Machine, w []vm.Value) error {
			rec.damage = append(rec.damage, w[1].Int())
			return nil
		},
		"#NodeRef.Transform2D": func(m *vm.Machine, w []vm.Value) error {
			w[0], w[1], w[2] = vm.Vec(1, 2, 0, 0), vm.Float(0), vm.Vec(1, 1, 0, 0)
			return nil
		},
		"#NodeRef.SetTransform2D": func(m *vm.Machine, w []vm.Value) error {
			copy(rec.pose[:], w[1:4])
			return nil
		},
		"#NodeRef.Has<engine.SpriteRenderer>": func(m *vm.Machine, w []vm.Value) error { w[0] = vm.Bool(true); return nil },
		"#NodeRef.Get<engine.SpriteRenderer>": func(m *vm.Machine, w []vm.Value) error { w[0] = vm.Value{A: 1}; return nil },
	}
	return func(imp bytecode.HostImport) (vm.HostFunc, bool) {
		h, ok := table[imp.Module+"."+imp.Name]
		return h, ok
	}
}

func build(t *testing.T, pkgs map[string]string) (*checker.Analysis, *source.Manager) {
	t.Helper()
	sm := source.NewManager()
	files := map[string][]*ast.File{}
	for p, src := range pkgs {
		f, ds := parser.ParseSource(sm, p+"/main.aes", []byte(src))
		if diag.HasErrors(ds) {
			var b bytes.Buffer
			diag.Print(&b, sm, ds)
			t.Fatal(b.String())
		}
		files[p] = append(files[p], f)
	}
	a := checker.Check(sm, files, surface(), checker.Options{})
	if a.HasErrors() {
		var b bytes.Buffer
		diag.Print(&b, sm, a.Diagnostics)
		t.Fatal(b.String())
	}
	return a, sm
}

func compile(t *testing.T, a *checker.Analysis, sm *source.Manager, pkg string) *bytecode.Module {
	t.Helper()
	m, ds := Compile(a, pkg, Options{Sources: sm})
	if m == nil || diag.HasErrors(ds) {
		var b bytes.Buffer
		diag.Print(&b, sm, ds)
		t.Fatalf("compile failed:\n%s", b.String())
	}
	if vd := verify.Module(m); len(vd) > 0 {
		var b bytes.Buffer
		bytecode.Disassemble(&b, m)
		t.Fatalf("verify: %s\n%s", vd[0].Message, b.String())
	}
	return m
}

func runFunc(t *testing.T, src, name string, args ...vm.Value) ([]vm.Value, *vm.Fault) {
	t.Helper()
	a, sm := build(t, map[string]string{"game/t": src})
	mod := compile(t, a, sm, "game/t")
	m := vm.New(vm.Options{Budget: 10_000_000})
	lm, err := m.Load(mod, hosts(&record{}))
	if err != nil {
		t.Fatal(err)
	}
	out, fl := m.Call(lm.Func(name), nil, args)
	return append([]vm.Value(nil), out...), fl
}

func TestConformance(t *testing.T) {
	cases := []struct {
		name string
		src  string
		fn   string
		args []vm.Value
		want int64
	}{
		{"fib", "package t\nfunc Fib(n int) int {\n\tif n < 2 {\n\t\treturn n\n\t}\n\treturn Fib(n-1) + Fib(n-2)\n}\n", "Fib", []vm.Value{vm.Int(20)}, 6765},
		{"loop", "package t\nfunc F(n int) int {\n\ts := 0\n\tfor i := 0; i < n; i++ {\n\t\tif i%2 == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tif i > 50 {\n\t\t\tbreak\n\t\t}\n\t\ts += i\n\t}\n\treturn s\n}\n", "F", []vm.Value{vm.Int(100)}, 625},
		{"convert", "package t\nfunc F(f float) int {\n\treturn int(f*2) + int(float(3)/2)\n}\n", "F", []vm.Value{vm.Float(2.75)}, 6},
		{"strings", "package t\nfunc F(n int) int {\n\ts := \"a\" + string(n) + \"b\"\n\tif s == \"a12b\" && s < \"b\" {\n\t\treturn len(s)\n\t}\n\treturn -1\n}\n", "F", []vm.Value{vm.Int(12)}, 4},
		{"structs", "package t\ntype P struct {\n\tX int\n\tY int\n}\nfunc F() int {\n\tp := P{X: 1, Y: 2}\n\tq := p\n\tq.X = 10\n\tif p == (P{X: 1, Y: 2}) && q != p {\n\t\treturn q.X + p.Y\n\t}\n\treturn -1\n}\n", "F", nil, 12},
		{"enumSwitch", "package t\nenum E {\n\tA\n\tB\n\tC\n}\nfunc F(e E) int {\n\tswitch e {\n\tcase E.A:\n\t\treturn 1\n\tcase E.B, E.C:\n\t\treturn 2\n\t}\n\treturn 0\n}\n", "F", []vm.Value{vm.Int(2)}, 2},
		{"slices", "package t\nfunc F() int {\n\ts := []int{1, 2}\n\ts = append(s, 3)\n\ts[0] += 4\n\tt := 0\n\tfor i := 0; i < len(s); i++ {\n\t\tt += s[i]\n\t}\n\treturn t\n}\n", "F", nil, 10},
		{"structSlice", "package t\ntype P struct {\n\tX int\n\tY int\n}\nfunc F() int {\n\tvar ps []P\n\tps = append(ps, P{X: 1, Y: 2})\n\tps = append(ps, P{X: 3, Y: 4})\n\tps[1].Y = 40\n\treturn ps[0].X + ps[1].Y\n}\n", "F", nil, 41},
		{"shortCircuit", "package t\nfunc F(n int) int {\n\tif n > 0 && 10/n > 2 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n", "F", []vm.Value{vm.Int(0)}, 0},
		{"consts", "package t\nconst K = 3 * 4\nconst M = -9223372036854775808\nfunc F() int {\n\tif M < 0 {\n\t\treturn K + 1\n\t}\n\treturn 0\n}\n", "F", nil, 13},
		{"bigConst", "package t\nfunc F() int {\n\treturn 1_000_000 + 7\n}\n", "F", nil, 1000007},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, fl := runFunc(t, tc.src, tc.fn, tc.args...)
			if fl != nil {
				t.Fatalf("fault: %v", fl)
			}
			if len(out) != 1 || out[0].Int() != tc.want {
				t.Fatalf("got %v want %d", out, tc.want)
			}
		})
	}
}

func TestFloatPrograms(t *testing.T) {
	cases := map[string]struct {
		src  string
		want float32
	}{
		"vectors":   {"package t\nfunc F() float {\n\tv := Vec2{X: 1, Y: 2}\n\tw := (v + v) * 0.5\n\tw.Y += 1\n\treturn w.X + w.Y\n}\n", 4},
		"transform": {"package t\nfunc F() float {\n\tt := Transform2D{}\n\tt.Position.X += 2\n\tt.Scale = Vec2{X: 3, Y: 3}\n\treturn t.Position.X * t.Scale.Y\n}\n", 6},
		"negate":    {"package t\nfunc F() float {\n\tv := -Vec2{X: 1, Y: 2}\n\tx := 2.5\n\treturn v.Y - x\n}\n", -4.5},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out, fl := runFunc(t, tc.src, "F")
			if fl != nil || out[0].Float() != tc.want {
				t.Fatalf("got %v %v want %v", out, fl, tc.want)
			}
		})
	}
}

func TestRuntimeFaults(t *testing.T) {
	_, fl := runFunc(t, "package t\nfunc Spin() int {\n\tfor {\n\t}\n}\n", "Spin")
	if fl == nil || fl.Code != vm.CodeBudget {
		t.Fatalf("budget: %v", fl)
	}
	_, fl = runFunc(t, "package t\nfunc F() int {\n\ts := []int{1}\n\treturn s[3]\n}\n", "F")
	if fl == nil || fl.Code != vm.CodeIndex || fl.Stack[0].Line != 4 {
		t.Fatalf("index: %+v", fl)
	}
}

func TestScriptSelf(t *testing.T) {
	src := "package t\nscript Counter {\n\t@export\n\tvar N int = 5\n\tfunc Update(dt float) {\n\t\tN++\n\t}\n}\n"
	a, sm := build(t, map[string]string{"game/t": src})
	mod := compile(t, a, sm, "game/t")
	sd := mod.Scripts[0]
	if sd.Slots != 2 || sd.Fields[0].Slot != 1 || !sd.Fields[0].HasDefault || sd.Update < 0 {
		t.Fatalf("script def %+v", sd)
	}
	m := vm.New(vm.Options{})
	lm, err := m.Load(mod, nil)
	if err != nil {
		t.Fatal(err)
	}
	self := []vm.Value{{A: 1}, {A: sd.Fields[0].Default.A}}
	upd := lm.Funcs[sd.Update]
	for i := 0; i < 3; i++ {
		if _, fl := m.Call(upd, self, []vm.Value{vm.Float(0.016)}); fl != nil {
			t.Fatal(fl)
		}
	}
	if self[1].Int() != 8 {
		t.Fatalf("N = %d", self[1].Int())
	}
}

func readPlayer(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(scriptsDir, "player.aes"))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

func TestPlayerRuns(t *testing.T) {
	a, sm := build(t, map[string]string{"mygame/player": readPlayer(t)})
	mod := compile(t, a, sm, "mygame/player")
	rec := &record{}
	m := vm.New(vm.Options{})
	cs := comps{}
	m.Comps = cs
	lm, err := m.Load(mod, hosts(rec))
	if err != nil {
		t.Fatal(err)
	}
	sd := mod.Scripts[0]
	self := make([]vm.Value, sd.Slots)
	self[0] = vm.Value{A: 1}
	for _, f := range sd.Fields {
		if f.HasDefault {
			self[f.Slot] = vm.Value{A: f.Default.A, B: f.Default.B}
		}
		if f.Name == "Target" {
			self[f.Slot] = vm.Value{A: 7}
		}
	}
	if _, fl := m.Call(lm.Funcs[sd.Ready], self, nil); fl != nil {
		t.Fatal(fl)
	}
	if _, fl := m.Call(lm.Funcs[sd.Update], self, []vm.Value{vm.Float(0.5)}); fl != nil {
		t.Fatal(fl)
	}
	if x := rec.pose[0].Lane(0); x != 111 {
		t.Fatalf("player moved to x=%v, want 111", x)
	}
	if len(rec.damage) != 1 || rec.damage[0] != 5 || len(rec.logs) != 1 || rec.logs[0] != "ready" {
		t.Fatalf("host calls %+v", rec)
	}
	if v, ok := cs[0]; !ok || v.Bool() {
		t.Fatal("FlipX was not written as false")
	}
}

func TestGoldenDisassembly(t *testing.T) {
	a, sm := build(t, map[string]string{"mygame/player": readPlayer(t)})
	mod := compile(t, a, sm, "mygame/player")
	var b bytes.Buffer
	bytecode.DisassembleSource(&b, mod, func(file string, line int) string {
		id, ok := sm.Lookup(file)
		if !ok {
			return ""
		}
		return sm.Line(id, line)
	})
	golden := filepath.Join("testdata", "player.dis")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(golden, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden missing, run go test ./script/compiler -update: %v", err)
	}
	if !bytes.Equal(want, b.Bytes()) {
		t.Errorf("disassembly changed\n%s", b.String())
	}
}

func TestInterfaceHashAndCacheKey(t *testing.T) {
	hash := func(src string) [32]byte {
		a, _ := build(t, map[string]string{"game/t": src})
		return InterfaceHash(a, "game/t")
	}
	base := hash("package t\nfunc Add(a int, b int) int {\n\treturn a + b\n}\nfunc helper() int {\n\treturn 1\n}\n")
	body := hash("package t\nfunc Add(a int, b int) int {\n\treturn b + a\n}\nfunc helper() int {\n\treturn 2\n}\n")
	sig := hash("package t\nfunc Add(a int, b float) int {\n\treturn a\n}\nfunc helper() int {\n\treturn 1\n}\n")
	if base != body {
		t.Fatal("body-only edit changed the interface hash")
	}
	if base == sig {
		t.Fatal("signature change kept the interface hash")
	}
	src := map[string][]byte{"main.aes": []byte("package t")}
	k1 := CacheKey(src, nil, [32]byte{1}, Options{})
	k2 := CacheKey(src, nil, [32]byte{2}, Options{})
	k3 := CacheKey(src, map[string][32]byte{"game/a": {9}}, [32]byte{1}, Options{})
	if k1 == k2 || k1 == k3 || k1 != CacheKey(src, nil, [32]byte{1}, Options{}) {
		t.Fatal("cache key is not sensitive or not stable")
	}
}

func TestRegisterLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("package t\nfunc Big() int {\n")
	for i := 0; i < 300; i++ {
		b.WriteString("\tvar v" + strconv.Itoa(i) + " int\n")
	}
	b.WriteString("\treturn v0\n}\n")
	a, sm := build(t, map[string]string{"game/t": b.String()})
	m, ds := Compile(a, "game/t", Options{Sources: sm})
	if m != nil || len(ds) == 0 || ds[0].Code != diag.RegisterLimit {
		t.Fatalf("limit not reported: %v", ds)
	}
}

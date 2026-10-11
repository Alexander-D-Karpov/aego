package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/script/bytecode"
)

func TestGoldenAsync(t *testing.T) {
	names := []string{"coroutines", "scriptref", "signals"}
	pkgs := map[string]string{}
	for _, n := range names {
		src, err := os.ReadFile(filepath.Join(scriptsDir, n+".aes"))
		if err != nil {
			t.Fatal(err)
		}
		pkgs["game/"+n] = string(src)
	}
	a, sm := build(t, pkgs)
	var b bytes.Buffer
	for _, n := range names {
		mod := compile(t, a, sm, "game/"+n)
		bytecode.DisassembleSource(&b, mod, func(file string, line int) string {
			id, ok := sm.Lookup(file)
			if !ok {
				return ""
			}
			return sm.Line(id, line)
		})
		b.WriteByte('\n')
	}
	for _, op := range []string{"START", "YIELD", "EMIT", "CONNECT", "SELFREF", "GETSCRIPT", "GETXF", "SETXF", "CALLR", "COSTOP", "CORUN", "DISCONNECT", "CONNECTED"} {
		if !bytes.Contains(b.Bytes(), []byte(op)) {
			t.Errorf("disassembly has no %s", op)
		}
	}
	golden := filepath.Join("testdata", "async.dis")
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

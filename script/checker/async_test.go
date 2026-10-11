package checker

import (
	"os"
	"path/filepath"
	"testing"

	"aego/script/diag"
)

func TestAsyncFilesAreClean(t *testing.T) {
	for _, name := range []string{"coroutines", "signals", "scriptref"} {
		src, err := os.ReadFile(filepath.Join(scriptsDir, name+".aes"))
		if err != nil {
			t.Fatal(err)
		}
		_, sm, ds := check(t, map[string]string{"game/" + name: string(src)})
		if len(ds) > 0 {
			t.Errorf("%s:\n%s", name, dump(sm, ds))
		}
	}
}

func TestAsyncErrorsEachOnce(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(scriptsDir, "async_errors.aes"))
	if err != nil {
		t.Fatal(err)
	}
	_, sm, ds := check(t, map[string]string{"game/async_errors": string(src)})
	c := codes(ds)
	for _, code := range []string{
		diag.YieldOutside, diag.CoroutineCall, diag.StartTarget, diag.CoroutineDecl, diag.HandlerMismatch,
		diag.BadConnect, diag.CoroutineLifecycle, diag.YieldArg, diag.ScriptRefTarget, diag.ScriptRefCall,
	} {
		if c[code] != 1 {
			t.Errorf("%s reported %d times\n%s", code, c[code], dump(sm, ds))
		}
	}
}

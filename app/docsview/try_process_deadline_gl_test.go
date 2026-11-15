//go:build aego_editor && aego_gltest && linux && cgo && !aego_nonative

package docsview

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserProcessDeadlineFailsClosed(t *testing.T) {
	const runner = `
const readline = require('node:readline');
readline.createInterface({input: process.stdin}).on('line', line => {
  const req = JSON.parse(line);
  if (req.method === 'hang') return;
  const id = req.method === 'wrong-id' ? req.id + 1 : req.id;
  const error = req.method === 'error' ? 'deliberate failure' : undefined;
  process.stdout.write(JSON.stringify({id, error, result: {ok: true}}) + '\n');
});`
	file := filepath.Join(t.TempDir(), "runner.cjs")
	if err := os.WriteFile(file, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := startBrowserProcess(context.Background(), t.TempDir(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	var reply struct {
		OK bool `json:"ok"`
	}
	if err := p.call(context.Background(), "ok", nil, &reply); err != nil || !reply.OK {
		t.Fatalf("valid browser reply failed: %+v, %v", reply, err)
	}
	if err := p.call(context.Background(), "wrong-id", nil, nil); err == nil || !strings.Contains(err.Error(), "ID mismatch") {
		t.Fatalf("wrong response ID accepted: %v", err)
	}
	if err := p.call(context.Background(), "error", nil, nil); err == nil || !strings.Contains(err.Error(), "deliberate failure") {
		t.Fatalf("browser error accepted: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := p.call(ctx, "hang", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hung browser must fail with context deadline, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("hung browser did not stop promptly: %v", elapsed)
	}
	if err := p.call(context.Background(), "ok", nil, &reply); err == nil {
		t.Fatal("a killed runner accepted a later probe")
	}
}

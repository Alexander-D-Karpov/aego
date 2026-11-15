package webaudio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptsUseEngineSourcesAndRejectIncompletePair(t *testing.T) {
	engine, out := t.TempDir(), t.TempDir()
	src := filepath.Join(engine, "platform", "web")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "audio_worker_host.js"), []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Scripts(engine, out); err == nil {
		t.Fatal("accepted a missing Worker")
	}
	if _, err := os.Stat(filepath.Join(out, Host)); !os.IsNotExist(err) {
		t.Fatal("wrote the host before confirming the complete pair")
	}
	if err := os.WriteFile(filepath.Join(src, "audio_worker.js"), []byte("worker"), 0o644); err != nil {
		t.Fatal(err)
	}
	host, worker, err := Scripts(engine, out)
	if err != nil || string(host) != "host" || string(worker) != "worker" {
		t.Fatalf("unexpected source scripts: %q %q %v", host, worker, err)
	}
	for name, want := range map[string]string{Host: "host", Worker: "worker"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q: %v", name, got, err)
		}
	}
}

func TestBuildFailureDoesNotReplaceExistingReactor(t *testing.T) {
	engine, out := t.TempDir(), t.TempDir()
	path := filepath.Join(out, Reactor)
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Build(engine, out, true); err == nil {
		t.Fatal("accepted an engine without the reactor source")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "existing" {
		t.Fatalf("failed build changed prior reactor: %q %v", got, err)
	}
}

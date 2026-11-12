// Package webaudio builds and packages the isolated browser audio reactor for web exports.
package webaudio

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	Reactor = "audio-mixer.wasm"
	Worker  = "audio-worker.js"
	Host    = "audio-worker-host.js"
	// This is also enforced by the page before compiling untrusted responses.
	MaxReactorBytes = 32 << 20
)

// Build compiles the reactor from the same engine tree as the game. A private
// temporary output avoids replacing an existing artifact on build failure.
func Build(engine, out string, release bool) error {
	if engine == "" || out == "" {
		return fmt.Errorf("web audio needs an engine and output directory")
	}
	var err error
	engine, err = filepath.Abs(engine)
	if err != nil {
		return fmt.Errorf("resolve web audio engine: %w", err)
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return fmt.Errorf("resolve web audio output: %w", err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("create web audio output: %w", err)
	}
	tmp, err := os.CreateTemp(out, ".aego-audio-*.wasm")
	if err != nil {
		return fmt.Errorf("create temporary web audio reactor: %w", err)
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	defer os.Remove(name)
	args := []string{"build", "-buildmode=c-shared", "-buildvcs=false"}
	if release {
		args = append(args, "-trimpath", "-ldflags", "-s -w")
	}
	args = append(args, "-o", name, "./cmd/aego-audio-worker")
	cmd := exec.Command("go", args...)
	cmd.Dir = engine
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build web audio reactor: %w: %s", err, strings.TrimSpace(string(data)))
	}
	f, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("open built web audio reactor: %w", err)
	}
	info, statErr := f.Stat()
	var magic [4]byte
	_, readErr := io.ReadFull(f, magic[:])
	closeErr := f.Close()
	if statErr != nil || readErr != nil || closeErr != nil || info.Size() > MaxReactorBytes || string(magic[:]) != "\x00asm" {
		return fmt.Errorf("built web audio reactor is invalid or too large")
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return fmt.Errorf("set web audio reactor permissions: %w", err)
	}
	if err := os.Rename(name, filepath.Join(out, Reactor)); err != nil {
		return fmt.Errorf("publish web audio reactor: %w", err)
	}
	return nil
}

// Scripts copies the host and Worker from the exact tree used for the reactor.
// Both are read before writing, so a missing source cannot create half a pair.
func Scripts(engine, out string) (host, worker []byte, err error) {
	host, err = os.ReadFile(filepath.Join(engine, "platform", "web", "audio_worker_host.js"))
	if err != nil {
		return nil, nil, fmt.Errorf("read web audio host: %w", err)
	}
	worker, err = os.ReadFile(filepath.Join(engine, "platform", "web", "audio_worker.js"))
	if err != nil {
		return nil, nil, fmt.Errorf("read web audio worker: %w", err)
	}
	if len(host) == 0 || len(worker) == 0 {
		return nil, nil, fmt.Errorf("web audio scripts must not be empty")
	}
	if err = os.WriteFile(filepath.Join(out, Host), host, 0o644); err != nil {
		return nil, nil, fmt.Errorf("write web audio host: %w", err)
	}
	if err = os.WriteFile(filepath.Join(out, Worker), worker, 0o644); err != nil {
		return nil, nil, fmt.Errorf("write web audio worker: %w", err)
	}
	return host, worker, nil
}

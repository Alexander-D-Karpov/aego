package main

import (
	"os/exec"
	"path/filepath"
	"testing"

	"aego/engine"
	"aego/engine/bootstrap"
	"aego/project"
)

func TestNativeRotatorRunsHeadless(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the example")
	}
	p, err := project.Load(filepath.Join("..", "..", "examples", "native-rotator"))
	if err != nil {
		t.Fatal(err)
	}
	bin, err := bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--headless", "--frames", "60")
	cmd.Dir = p.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

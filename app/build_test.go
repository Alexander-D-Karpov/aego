package app

import (
	"errors"
	"strings"
	"testing"

	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/project"
)

type failInit struct{}

func (failInit) Info() module.Info {
	return module.Info{ID: "boom"}
}

func (failInit) Register(*module.Registry) error {
	return nil
}

func (failInit) Init(*module.InitContext) error {
	return errors.New("init failed")
}

func TestBuildRollbackOnError(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Rollback", false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Engine: engine.Config{Headless: true}, Logger: log.Nop()}
	h, err := Build(p, cfg, failInit{})
	if err == nil || h != nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("host %v err %v", h, err)
	}
	h, err = Build(p, cfg)
	if err != nil {
		t.Fatalf("a build after a rolled back one failed: %v", err)
	}
	h.Close()
	h.Close()
}

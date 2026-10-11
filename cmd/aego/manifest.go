package main

import (
	"os"

	"aego/core/log"
	"aego/engine"
	"aego/engine/bootstrap"
	"aego/engine/builtin"
	"aego/project"
	"aego/script/bind/manifest"
	"aego/script/workspace"
)

func loadManifest(p *project.Project, path string) (*manifest.Manifest, error) {
	if path == "" {
		if !bootstrap.NeedsNative(p) {
			return builtin.Manifest()
		}
		built, err := bootstrap.BuildManifest(p, engine.Version, log.New(log.NewTextSink(os.Stderr)))
		if err != nil {
			return nil, err
		}
		path = built
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return manifest.Decode(data)
}

func openWorkspace(p *project.Project, manifestPath string, tests bool) (*workspace.Workspace, error) {
	man, err := loadManifest(p, manifestPath)
	if err != nil {
		return nil, err
	}
	tab, err := man.Table()
	if err != nil {
		return nil, err
	}
	return workspace.Open(workspace.Config{
		Root:     p.Root,
		Module:   p.Module,
		Scripts:  p.ScriptsDir(),
		Bindings: tab,
		Docs:     man,
		Tests:    tests,
	})
}

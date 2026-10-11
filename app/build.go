package app

import (
	"os"
	"strings"

	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
	"aego/project"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene"
	"aego/scene/frame"
	"aego/script/bind"
	"aego/script/bind/manifest"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
	"aego/script/workspace"
)

type Config struct {
	Engine             engine.Config
	Logger             *log.Logger
	Debug              bool
	LoopScope          engine.SystemScope
	LocateBindings     bool
	Docs               bool
	Editor             bool
	FailOnScriptErrors bool
}

type Host struct {
	Project     *project.Project
	Engine      *engine.Engine
	Pipeline    *pipeline.Pipeline
	Loader      *loader.Loader
	Renderer    *render.Renderer
	RenderStats func() render.Stats
	Types       *scene.Types
	Bindings    *bind.Registry
	Workspace   *workspace.Workspace
	Modules     *module.Manifest
	Importers   *pipeline.ImporterRegistry
	Log         *log.Logger
	closed      bool
}

func Build(p *project.Project, cfg Config, mods ...module.Module) (*Host, error) {
	lg := cfg.Logger
	if lg == nil {
		lg = log.New(log.NewTextSink(os.Stderr))
		if cfg.Debug {
			lg.SetLevel(log.Debug)
		}
	}
	h := &Host{Project: p, Log: lg}
	if err := h.build(cfg, mods); err != nil {
		h.Close()
		return nil, err
	}
	return h, nil
}

func (h *Host) build(cfg Config, mods []module.Module) error {
	p, lg := h.Project, h.Log
	ch := lg.With(log.Engine)
	var err error
	if h.Workspace, err = workspace.Open(workspace.Config{Root: p.Root, Module: p.Module, Scripts: p.ScriptsDir()}); err != nil {
		return err
	}
	rmod := rendermodule.New(render.Options{MaxSprites: 65536})
	ecfg := cfg.Engine
	if ecfg.ResRoot == "" {
		ecfg.ResRoot = p.AssetsDir()
	}
	if cfg.LoopScope != 0 {
		ecfg.LoopScope = cfg.LoopScope
	}
	builder := engine.NewBuilder(ecfg).WithLogger(lg).Use(rmod)
	h.Types = NewTypes()
	h.Bindings = bind.New()
	h.Bindings.Locate(cfg.LocateBindings)
	if err = frame.Register(builder.Registry()); err != nil {
		return err
	}
	if h.Modules, err = BuildModules(h.Types, h.Bindings, stdlib.Options{Input: &p.Input}, mods); err != nil {
		return err
	}
	builder.Use(h.Modules.Engine())
	if h.Importers, err = DevImporters(h.Workspace, h.Modules); err != nil {
		return err
	}
	if err = h.Types.Schema().Freeze(); err != nil {
		return err
	}
	if err = scriptmodule.Freeze(h.Bindings, h.Types.Schema()); err != nil {
		return errs.Wrap(err, errs.EngineModule, "script bindings are invalid")
	}
	h.Workspace.SetBindings(h.Bindings.Table())
	if cfg.Docs {
		if m, derr := manifest.FromRegistry(h.Bindings, h.Types.Schema()); derr == nil {
			h.Workspace.SetDocs(m)
		} else {
			ch.Warn("cannot describe the script bindings", log.F("err", derr.Error()))
		}
	}
	if h.Pipeline, err = pipeline.Open(p, h.Importers, lg); err != nil {
		return err
	}
	if _, _, err = h.Pipeline.Scan(); err != nil {
		return err
	}
	batch, err := h.Pipeline.ImportAll()
	if err != nil {
		return err
	}
	for id, ierr := range batch.Failed {
		ch.Error("asset import failed", log.F("asset", id.String()), log.F("err", ierr.Error()))
	}
	if cfg.FailOnScriptErrors {
		if bad := ScriptErrors(h.Pipeline); len(bad) > 0 {
			msg := strings.Join(bad, "\n")
			if si := ScriptImporter(h.Importers); si != nil {
				if r := si.Report(); r != "" {
					msg = r
				}
			}
			return errs.New(errs.BuildFailed, "script packages have errors, fix them before running:\n"+msg)
		}
	}
	if h.Engine, err = builder.Build(); err != nil {
		return err
	}
	h.Renderer = rmod.Renderer()
	h.RenderStats = rmod.Stats
	h.Loader = loader.New(h.Engine.GPU(), h.Renderer.Textures(), lg)
	if err = h.Loader.Mount(h.Pipeline.Index(), h.Pipeline.Artifacts()); err != nil {
		return err
	}
	h.Engine.SetService(loader.ServiceID, h.Loader)
	h.Engine.SetService(rendermodule.ServiceID, h.Renderer)
	if err = h.Modules.Init(&module.InitContext{Engine: h.Engine, Assets: h.Loader, Log: lg, Editor: cfg.Editor}); err != nil {
		return err
	}
	for _, w := range h.Modules.Warnings {
		ch.Warn(w)
	}
	return nil
}

func (h *Host) Close() {
	if h.closed {
		return
	}
	h.closed = true
	if h.Engine != nil {
		h.Engine.Close()
	}
	if h.Loader != nil {
		h.Loader.Close()
	}
	if h.Pipeline != nil {
		h.Pipeline.Close()
	}
}

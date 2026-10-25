package shell

import (
	"path"
	"slices"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/animedit"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	animpanel "aego/editor/panels/animation"
	"aego/editor/shell/addpalette"
	"aego/engine/i18n"
	"aego/modules/animation"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
)

func (a *App) wireAnim() {
	ctx := a.ctx
	files := animedit.PipelineFiles(ctx.Pipeline, ctx.ApplyBatch)
	ctx.AnimFiles = files
	ctx.Anim = animedit.NewRegistry(files, func(msg string) { ctx.Notify(context.LevelInfo, msg) })
	ctx.Docs.SetSaveGuard(ctx.Anim.SaveGuard)
	ctx.Docs.Subscribe(func(ev document.Event) {
		if ev.Kind == document.EvClosed || ev.Kind == document.EvReloaded {
			ctx.Anim.Drop(ev.Doc)
		}
	})
	a.play.SetBeforeStart(ctx.Anim.StopAll)
	a.registerPathGizmo()
	a.registerSlicer()
}

func (a *App) animDriven(d *document.SceneDocument, node ids.NodeID, t schema.TypeID, f schema.FieldID) bool {
	return a.ctx.Anim != nil && a.ctx.Anim.For(d).Driven(node, t, f)
}

var builtinAnimEvents = []string{"Footstep", "Hit"}

// animEvents lists the events a clip may use: the built-in ones and those of the project section.
func (a *App) animEvents() []string {
	out := slices.Clone(builtinAnimEvents)
	if s := a.ctx.Sections; s != nil {
		for _, n := range s.Names("animation", "events") {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

func (a *App) newAnimPanel(d animpanel.Deps) *animpanel.Panel {
	a.animPanel = animpanel.New(d)
	return a.animPanel
}

// AnimationPanel is the Animation panel, for tests and screenshots.
func (a *App) AnimationPanel() *animpanel.Panel {
	return a.animPanel
}

func (a *App) registerAnimCommands() error {
	p := a.animPanel
	on := p.Active
	for _, d := range []command.Desc{
		{ID: animpanel.CommandPlay, Name: "Play or Pause Clip", Category: "Animation", Order: 100, Scope: animpanel.ID, Enabled: on,
			Shortcut: command.Key(input.KeySpace, 0), Run: func() error { p.TogglePlay(); return nil }},
		{ID: animpanel.CommandKey, Name: "Key Changed Fields", Category: "Animation", Order: 110, Scope: animpanel.ID, Enabled: on,
			Shortcut: command.Key(input.KeyK, 0), Run: p.KeyNode},
		{ID: animpanel.CommandDelete, Name: "Delete Keys", Category: "Animation", Order: 120, Scope: animpanel.ID, Enabled: on,
			Shortcut: command.Key(input.KeyDelete, 0), Run: p.DeleteKeys},
		{ID: animpanel.CommandCopy, Name: "Copy Keys", Category: "Animation", Order: 130, Scope: animpanel.ID, Enabled: on,
			Shortcut: command.Key(input.KeyC, command.ModPrimary), Run: func() error { p.CopyKeys(); return nil }},
		{ID: animpanel.CommandPreview, Name: "Preview", Category: "Animation", Order: 10, Enabled: on, Checked: p.Previewing,
			Run: func() error { p.TogglePreview(); return nil }},
		{ID: animpanel.CommandRecord, Name: "Record", Category: "Animation", Order: 20, Enabled: on, Checked: p.Recording,
			Run: func() error { p.ToggleRecord(); return nil }},
		{ID: animpanel.CommandCurves, Name: "Curves", Category: "Animation", Order: 30, Enabled: on, Checked: p.Curves,
			Run: func() error { p.ToggleCurves(); return nil }},
		{ID: animpanel.CommandPaste, Name: "Paste Keys", Category: "Animation", Order: 140, Scope: animpanel.ID, Enabled: on,
			Shortcut: command.Key(input.KeyV, command.ModPrimary), Run: p.PasteKeys},
	} {
		if err := a.ctx.Commands.Register(d); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) animActions() []addpalette.Action {
	return []addpalette.Action{
		{Key: "anim.clip", Name: "Clip", Category: "Animation", Icon: "anim.key",
			Doc: "A new one-second looping clip next to the scene. A node without an Animation Player or an Animator gets an Animation Player that plays it.",
			Run: a.createClip},
		{Key: "anim.flipbook", Name: "Flipbook from Selection", Category: "Animation", Icon: "anim.slice",
			Doc: "A clip that shows the sprites selected in Assets, or the cells of a sliced sheet, one after another at 12 frames per second. A node whose Animation Player already has a clip gets the frames in that clip.",
			Run: a.createFlipbook},
		{Key: "anim.controller", Name: "Controller", Category: "Animation", Icon: "anim.graph",
			Doc: "A new controller with one Idle state next to the scene. A node without an Animator or an Animation Player gets an Animator that uses it.",
			Run: a.createController},
	}
}

func (a *App) animTarget(parent ids.NodeID) (*document.SceneDocument, scene.NodeHandle, string, error) {
	d := a.ctx.Doc()
	if d == nil {
		return nil, scene.NodeHandle{}, "", errs.New(errs.EditorCommand, "open a scene first")
	}
	s := d.Scene()
	h := s.Handle(parent)
	if h.IsZero() || h == s.Root() {
		return nil, scene.NodeHandle{}, "", errs.New(errs.EditorCommand, "select the node to animate first")
	}
	dir := path.Dir(d.Path)
	if d.Path == "" || dir == "." {
		dir = "assets/anim"
	}
	return d, h, dir, nil
}

func (a *App) hasComp(s *scene.Scene, h scene.NodeHandle, name string) bool {
	td, ok := s.Registry().ByName(name)
	if !ok {
		return false
	}
	_, has := s.GetByType(h, td.ID)
	return has
}

// addWith adds a component with one asset field set, as one History step.
func (a *App) addWith(d *document.SceneDocument, h scene.NodeHandle, comp, field string, id ids.AssetID) error {
	s := d.Scene()
	td, ok := s.Registry().ByName(comp)
	if !ok {
		return nil
	}
	fd, ok := td.Field(field)
	if !ok {
		return nil
	}
	env := commands.WithOnAdd(commands.EnvFor(s), a.ctx.OnAdd)
	vals := append(a.inspectors.Defaults(td.ID), commands.FieldValue{Field: fd.ID, Value: schema.Asset(id)})
	return d.History.Exec(commands.NewMutation(i18n.Tr("editor.history.add", "Add {name}", i18n.Str("name", schema.TypeLabel(td))), env, commands.AddComp(s, s.ID(h), td, vals)))
}

func (a *App) createClip(parent ids.NodeID) error {
	d, h, dir, err := a.animTarget(parent)
	if err != nil {
		return err
	}
	s := d.Scene()
	rel := assetops.UniqueFile(a.ctx, dir, s.Name(h), ".aeanim")
	id, err := assetops.CreateFile(a.ctx, rel, animedit.NewClipText())
	if err != nil {
		return err
	}
	if !a.hasComp(s, h, animation.NamePlayer) && !a.hasComp(s, h, animation.NameAnimator) {
		if err := a.addWith(d, h, animation.NamePlayer, "clip", id); err != nil {
			return err
		}
	}
	a.ctx.Select(s.ID(h))
	a.ws.Show(animpanel.ID)
	return nil
}

func (a *App) createController(parent ids.NodeID) error {
	d, h, dir, err := a.animTarget(parent)
	if err != nil {
		return err
	}
	data, err := animedit.NewControllerText()
	if err != nil {
		return err
	}
	s := d.Scene()
	rel := assetops.UniqueFile(a.ctx, dir, s.Name(h), ".aectrl")
	id, err := assetops.CreateFile(a.ctx, rel, data)
	if err != nil {
		return err
	}
	if !a.hasComp(s, h, animation.NameAnimator) && !a.hasComp(s, h, animation.NamePlayer) {
		if err := a.addWith(d, h, animation.NameAnimator, "controller", id); err != nil {
			return err
		}
	}
	a.ctx.Select(s.ID(h))
	return nil
}

// flipFrames are the sprites of the Assets selection, a sliced sheet giving its cells; the inspected sheet when nothing is selected.
func (a *App) flipFrames() []ids.AssetID {
	pl := a.opt.Pipeline
	var src []ids.AssetID
	if a.assetsPanel != nil {
		src = a.assetsPanel.Selection()
	}
	if len(src) == 0 && a.data != nil {
		if id := a.data.Inspected(); !id.IsZero() {
			src = []ids.AssetID{id}
		}
	}
	var out []ids.AssetID
	for _, id := range src {
		if m, ok := pl.MetaOf(id); ok && len(m.Subs) > 0 {
			for _, s := range m.Subs {
				out = append(out, s.ID)
			}
			continue
		}
		if e, ok := pl.Entry(id); ok && (e.Kind == asset.ArtTexture || e.Kind == asset.ArtSpriteRegion) {
			out = append(out, id)
		}
	}
	return out
}

func (a *App) createFlipbook(parent ids.NodeID) error {
	frames := a.flipFrames()
	if len(frames) == 0 {
		return errs.New(errs.EditorCommand, "select sprites or a sliced sheet in Assets first")
	}
	d, h, dir, err := a.animTarget(parent)
	if err != nil {
		return err
	}
	s := d.Scene()
	if p, ok := scene.Get[animation.AnimationPlayer](s, h); ok && !p.Clip.ID.IsZero() {
		rel, ok := a.opt.Pipeline.PathOf(p.Clip.ID)
		if !ok {
			return errs.New(errs.EditorCommand, "the clip of the Animation Player is not in the project")
		}
		svc := a.ctx.Anim.For(d)
		if err := svc.SetClip(rel); err != nil {
			return err
		}
		if err := svc.FlipbookInto(frames); err != nil {
			return err
		}
	} else {
		rel := assetops.UniqueFile(a.ctx, dir, s.Name(h), ".aeanim")
		id, err := assetops.CreateFile(a.ctx, rel, animedit.NewFlipbookText(frames))
		if err != nil {
			return err
		}
		if !a.hasComp(s, h, animation.NamePlayer) && !a.hasComp(s, h, animation.NameAnimator) {
			if err := a.addWith(d, h, animation.NamePlayer, "clip", id); err != nil {
				return err
			}
		}
	}
	a.ctx.Select(s.ID(h))
	a.ws.Show(animpanel.ID)
	return nil
}

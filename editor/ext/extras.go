package ext

import (
	"fmt"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/vectors"
)

func (r *Registry) SettingsPage(section string, draw func(*context.Editor, any) bool, keywords ...string) error {
	ed := r.deps.Editor
	ext := r.ext()
	if ed == nil || draw == nil {
		return errs.New(errs.EditorPanel, "settings page needs a draw function", log.F("module", ext))
	}
	sec, ok := ed.Sections.Desc(section)
	if !ok {
		return errs.New(errs.EditorPanel, "settings page "+section+" has no project section", log.F("module", ext))
	}
	for _, p := range ed.Pages {
		if p.Section == section {
			return errs.New(errs.EditorPanel, "settings page "+section+" is already registered", log.F("module", ext))
		}
	}
	ed.Pages = append(ed.Pages, context.SettingsPage{Section: section, Title: sec.Title, Order: sec.Order, Draw: func(d any) bool {
		changed := false
		Safe(ext, r, func() { changed = draw(ed, d) })
		return changed
	}, Keywords: keywords})
	return nil
}

func (r *Registry) Vectors(id, title string, fn func(*context.VectorSink)) error {
	ed := r.deps.Editor
	ext := r.ext()
	if ed == nil || id == "" || fn == nil {
		return errs.New(errs.EditorPanel, "vector layer needs an id and a draw function", log.F("module", ext))
	}
	for _, l := range ed.Vectors {
		if l.ID == id {
			return errs.New(errs.EditorPanel, "vector layer "+id+" is already registered", log.F("module", ext))
		}
	}
	ed.Vectors = append(ed.Vectors, context.VectorLayer{ID: id, Title: title, Draw: func(v *context.VectorSink) {
		Safe(ext, r, func() { fn(v) })
	}})
	if r.deps.Commands == nil {
		return nil
	}
	cid := vectors.Prefix + id
	o := &ed.Overlay
	o.Set(cid, true)
	return r.Command(command.Desc{
		ID: cid, Name: title, Category: vectors.Category, Order: 200 + len(ed.Vectors),
		Checked: func() bool { return o.On(cid) },
		Run: func() error {
			o.Toggle(cid)
			return nil
		},
	})
}

func (r *Registry) Profiler(title string, play bool, fn func(*context.Editor)) {
	ed := r.deps.Editor
	if ed == nil || fn == nil {
		return
	}
	ext := r.ext()
	ed.Profiler = append(ed.Profiler, context.ProfilerSection{Title: title, Play: play, Draw: func() {
		Safe(ext, r, func() { fn(ed) })
	}})
}

func (r *Registry) AssetInspector(kind string, fn context.AssetView) error {
	ed := r.deps.Editor
	ext := r.ext()
	if ed == nil {
		return nil
	}
	if kind == "" || fn == nil {
		return errs.New(errs.EditorPanel, "asset inspector needs an artifact kind and a draw function", log.F("module", ext))
	}
	if _, dup := ed.AssetViews[kind]; dup {
		return errs.New(errs.EditorPanel, "asset inspector for "+kind+" is already registered", log.F("module", ext))
	}
	if ed.AssetViews == nil {
		ed.AssetViews = map[string]context.AssetView{}
	}
	ed.AssetViews[kind] = func(ctx *context.Editor, id ids.AssetID) {
		mark := ctx.UI.Mark()
		defer func() {
			if rec := recover(); rec != nil {
				ctx.UI.Restore(mark)
				r.Disable(ext, fmt.Sprint(rec))
			}
		}()
		fn(ctx, id)
	}
	o := r.own(ext)
	o.assets = append(o.assets, kind)
	return nil
}

// AssetPlayer registers an audition for an artifact kind or a data type's full name.
func (r *Registry) AssetPlayer(key string, pl context.AssetPlayer) error {
	ed := r.deps.Editor
	ext := r.ext()
	if ed == nil {
		return nil
	}
	if key == "" || pl.Play == nil || pl.Stop == nil || pl.Playing == nil {
		return errs.New(errs.EditorPanel, "asset player needs a kind and Play, Stop and Playing functions", log.F("module", ext))
	}
	if _, dup := ed.AssetPlayers[key]; dup {
		return errs.New(errs.EditorPanel, "asset player for "+key+" is already registered", log.F("module", ext))
	}
	if ed.AssetPlayers == nil {
		ed.AssetPlayers = map[string]context.AssetPlayer{}
	}
	guard := func(fn func()) {
		defer func() {
			if rec := recover(); rec != nil {
				r.Disable(ext, fmt.Sprint(rec))
			}
		}()
		fn()
	}
	ed.AssetPlayers[key] = context.AssetPlayer{
		Play: func(ctx *context.Editor, id ids.AssetID) (err error) {
			guard(func() { err = pl.Play(ctx, id) })
			return err
		},
		Stop: func(ctx *context.Editor) { guard(func() { pl.Stop(ctx) }) },
		Playing: func(ctx *context.Editor) (id ids.AssetID) {
			guard(func() { id = pl.Playing(ctx) })
			return id
		},
	}
	o := r.own(ext)
	o.players = append(o.players, key)
	return nil
}

package ext

import (
	"fmt"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/vectors"
	"aego/engine/i18n"
)

func (r *Registry) SettingsPage(section string, draw func(*context.Editor, any) bool, keywords ...i18n.Text) error {
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

func (r *Registry) Vectors(id string, title i18n.Text, fn func(*context.VectorSink)) error {
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
		ID: cid, Name: title.En, Label: title.String, Category: vectors.Category, Order: 200 + len(ed.Vectors),
		Checked: func() bool { return o.On(cid) },
		Run: func() error {
			o.Toggle(cid)
			return nil
		},
	})
}

func (r *Registry) Profiler(title i18n.Text, play bool, fn func(*context.Editor)) {
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

// CreateAction registers a module-owned entry in the searchable Create palette.
func (r *Registry) CreateAction(a context.CreateAction) error {
	ed := r.deps.Editor
	if ed == nil {
		return nil
	}
	if a.Key == "" || a.Name == "" || a.Run == nil {
		return errs.New(errs.EditorPanel, "create action needs key, name and handler")
	}
	for _, current := range ed.CreateActions {
		if current.Key == a.Key {
			return errs.New(errs.EditorPanel, "duplicate create action "+a.Key)
		}
	}
	ext := r.ext()
	run := a.Run
	a.Run = func(parent ids.NodeID) (err error) { Safe(ext, r, func() { err = run(parent) }); return }
	if runInFolder := a.RunInFolder; runInFolder != nil {
		a.RunInFolder = func(dir string) (err error) { Safe(ext, r, func() { err = runInFolder(dir) }); return }
	}
	ed.CreateActions = append(ed.CreateActions, a)
	r.own(ext).createActions = append(r.own(ext).createActions, a.Key)
	return nil
}

// ExternalFileImport registers a module-owned external source converter.
func (r *Registry) ExternalFileImport(h context.ExternalFileImport) error {
	ed := r.deps.Editor
	if ed == nil {
		return nil
	}
	if h.Key == "" || len(h.Extensions) == 0 || h.Import == nil {
		return errs.New(errs.EditorPanel, "external file import needs a key, extensions and handler")
	}
	seen := map[string]bool{}
	for _, ext := range h.Extensions {
		if len(ext) < 2 || !strings.HasPrefix(ext, ".") || strings.ContainsAny(ext, "/\\") {
			return errs.New(errs.EditorPanel, "invalid external file extension "+ext)
		}
		key := strings.ToLower(ext)
		if seen[key] {
			return errs.New(errs.EditorPanel, "duplicate external file extension "+ext)
		}
		seen[key] = true
		for _, old := range ed.ExternalFileImports {
			if old.Key == h.Key {
				return errs.New(errs.EditorPanel, "duplicate external file import "+h.Key)
			}
			for _, existing := range old.Extensions {
				if strings.EqualFold(existing, ext) {
					return errs.New(errs.EditorPanel, "external file extension is already registered "+ext)
				}
			}
		}
	}
	owner, run := r.ext(), h.Import
	h.Extensions = append([]string(nil), h.Extensions...)
	h.Import = func(source, dir string) (id ids.AssetID, pending bool, err error) {
		finished := false
		Safe(owner, r, func() { id, pending, err = run(source, dir); finished = true })
		if !finished {
			err = errs.New(errs.EditorPanel, "external file import failed in extension "+owner)
		}
		return
	}
	ed.ExternalFileImports = append(ed.ExternalFileImports, h)
	r.own(owner).externalImports = append(r.own(owner).externalImports, h.Key)
	return nil
}

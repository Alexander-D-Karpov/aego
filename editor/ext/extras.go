package ext

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/vectors"
)

func (r *Registry) SettingsPage(section string, draw func(*context.Editor, any) bool) error {
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
	}})
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

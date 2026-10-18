package projectsettings

import (
	"bytes"
	"maps"

	es "aego/engine/settings"
	"aego/platform/input"
	"aego/project"
	"aego/schema"
	"aego/ui/core"
)

const (
	CommandUndo = "project_settings.undo"
	CommandRedo = "project_settings.redo"
	Scope       = "project_settings"
)

type state struct {
	drafts map[string]any
	work   *project.Project
	proj   map[es.Key]schema.Value
}

func (p *Panel) Scope() string {
	return Scope
}

func (p *Panel) copyDrafts(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for id, d := range src {
		if c, err := p.d.Sections.Copy(id, d); err == nil {
			out[id] = c
		}
	}
	return out
}

func (p *Panel) capture() state {
	return state{drafts: p.copyDrafts(p.drafts), work: clone(p.work), proj: maps.Clone(p.proj)}
}

func (p *Panel) restore(s state) {
	p.drafts = p.copyDrafts(s.drafts)
	p.work = clone(s.work)
	p.proj = maps.Clone(s.proj)
	p.recheck()
}

func (p *Panel) touch() {
	if !p.gesture {
		p.undo = append(p.undo, p.base)
		p.redo = p.redo[:0]
		p.gesture = true
	}
	p.touched = true
	p.recheck()
}

func (p *Panel) settle(ui *core.Context) {
	if p.gesture && !p.touched && !ui.MouseDown(input.MouseLeft) {
		p.gesture = false
		p.base = p.capture()
	}
	p.touched = false
}

func (p *Panel) CanUndo() bool {
	return p.work != nil && (len(p.undo) > 0 || p.gesture)
}

func (p *Panel) CanRedo() bool {
	return p.work != nil && len(p.redo) > 0
}

func (p *Panel) Undo() {
	if p.work == nil {
		return
	}
	if p.gesture {
		p.gesture = false
		p.base = p.capture()
	}
	if len(p.undo) == 0 {
		return
	}
	s := p.undo[len(p.undo)-1]
	p.undo = p.undo[:len(p.undo)-1]
	p.redo = append(p.redo, p.capture())
	p.restore(s)
	p.base = p.capture()
}

func (p *Panel) Redo() {
	if p.work == nil || len(p.redo) == 0 {
		return
	}
	s := p.redo[len(p.redo)-1]
	p.redo = p.redo[:len(p.redo)-1]
	p.undo = append(p.undo, p.capture())
	p.restore(s)
	p.base = p.capture()
}

func (p *Panel) recheck() {
	dirty := !bytes.Equal(p.work.Encode(), p.d.Project.Encode())
	for id, d := range p.drafts {
		if live, ok := p.d.Sections.Get(id); ok && !p.d.Sections.Same(id, live, d) {
			dirty = true
		}
	}
	p.projDirty = false
	if st := p.d.Settings; st != nil {
		saved := st.ProjectValues()
		if len(saved) != len(p.proj) {
			p.projDirty = true
		}
		for k, v := range p.proj {
			if s, ok := saved[k]; !ok || !es.Equal(s, v) {
				p.projDirty = true
			}
		}
	}
	p.dirty = dirty || p.projDirty
}

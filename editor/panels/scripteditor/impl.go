package scripteditor

import (
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/script/source"
	"aego/ui/core"
)

const builtinNote = "engine builtin, no implementation in the project"

func (p *Panel) definition(ui *core.Context) {
	if p.doc.ReadOnly {
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.library_stub_hint", "Library stub, Ctrl+Alt+B opens the Go implementation when it lives in native/"))
		return
	}
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		p.flash(ui.Time, analyzing())
		return
	}
	loc, ok := p.host.lang.DefinitionIn(p.snap, file, uint32(off))
	switch {
	case !ok:
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.no_definition", "No definition found"))
	case loc.Native && loc.Stub != "" && p.host.openStub(loc):
	case loc.Native:
		msg := builtinNote
		if h, ok := p.host.lang.HoverIn(p.snap, file, uint32(off)); ok {
			msg = h.Signature + "\n" + msg
		}
		p.flash(ui.Time, msg)
	default:
		p.jump(loc)
	}
}

func (p *Panel) implementation(ui *core.Context) {
	if p.doc.ReadOnly {
		p.stubSource(ui)
		return
	}
	file, ok := p.file()
	off, ok2 := p.snapOffset()
	if !ok || !ok2 {
		p.flash(ui.Time, analyzing())
		return
	}
	loc, ok := p.host.lang.DefinitionIn(p.snap, file, uint32(off))
	switch {
	case !ok:
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.no_implementation", "No implementation found"))
	case !loc.Native:
		p.jump(loc)
	default:
		impl, ok := p.host.lang.Implementation(loc)
		if !ok {
			p.flash(ui.Time, builtinNote)
			return
		}
		p.host.Goto(context.At(context.GoPrefix+impl.Path, impl.Line, 1))
	}
}

func (p *Panel) builtinAt(file source.FileID, off int) bool {
	loc, ok := p.host.lang.DefinitionIn(p.snap, file, uint32(off))
	if !ok || !loc.Native {
		return false
	}
	_, impl := p.host.lang.Implementation(loc)
	return !impl
}

func (p *Panel) brackets() {
	c := p.doc.Code
	k := mark{rev: c.Rev(), off: c.Cursor()}
	if k == p.brKey {
		return
	}
	p.brKey = k
	p.brA, p.brB, p.brState = c.BracketAt(c.Cursor())
}

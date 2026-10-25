package scripteditor

import (
	"os"
	"sort"
	"strings"

	"aego/core/errs"
	"aego/editor/core/code"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/script/langsvc"
	"aego/ui/core"
)

const ()

func (h *Host) virtual(p string) (*document.ScriptDocument, error) {
	switch {
	case strings.HasPrefix(p, context.StubPrefix):
		h.ensure()
		st, ok := h.lang.Stub(p)
		if !ok {
			return nil, errs.New(errs.EditorDocument, "no library stub for "+strings.TrimPrefix(p, context.StubPrefix))
		}
		return &document.ScriptDocument{Path: p, Code: code.New([]byte(st.Text)), ReadOnly: true, Note: i18n.Tr("editor.scripteditor.stub_note", "Library stub generated from the engine bindings · read-only")}, nil
	case strings.HasPrefix(p, context.GoPrefix):
		data, err := os.ReadFile(strings.TrimPrefix(p, context.GoPrefix))
		if err != nil {
			return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the Go source")
		}
		return &document.ScriptDocument{Path: p, Code: code.New(data), ReadOnly: true, Note: i18n.Tr("editor.scripteditor.go_note", "Go source · read-only")}, nil
	}
	return nil, errs.New(errs.EditorDocument, "unknown virtual document "+p)
}

func (h *Host) openStub(l langsvc.Location) bool {
	st, ok := h.lang.Stub(l.Stub)
	if !ok {
		return false
	}
	h.Goto(context.At(l.Stub, st.Lines[l.Symbol]+1, 1))
	return true
}

func (p *Panel) stubSource(ui *core.Context) {
	if !strings.HasPrefix(p.doc.Path, context.StubPrefix) {
		p.flash(ui.Time, i18n.Tr("editor.scripteditor.read_only_go", "This is read-only Go source"))
		return
	}
	st, ok := p.host.lang.Stub(p.doc.Path)
	if !ok {
		return
	}
	cur := p.doc.Code.Line(p.doc.Code.Cursor())
	type entry struct {
		key  string
		line int
	}
	var list []entry
	for k, l := range st.Lines {
		if k != "" {
			list = append(list, entry{k, l})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].line < list[j].line })
	key := ""
	for _, e := range list {
		if e.line >= cur {
			key = e.key
			break
		}
	}
	impl, ok := p.host.lang.StubImplementation(p.doc.Path, key)
	if !ok {
		p.flash(ui.Time, builtinNote)
		return
	}
	p.host.Goto(context.At(context.GoPrefix+impl.Path, impl.Line, 1))
}

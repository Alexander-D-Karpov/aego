package scripteditor

import (
	"path"
	"strings"
)

func (h *Host) tabTitle(rel string) string {
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	for other := range h.panels {
		if other != rel && path.Base(other) == base {
			return stem + " — " + path.Base(path.Dir(rel))
		}
	}
	return stem
}

func (p *Panel) Icon() string {
	return "file.script"
}

func (p *Panel) Dirty() bool {
	return !p.doc.ReadOnly && p.doc.Dirty()
}

func (f *FindPanel) Icon() string {
	return "panel.find"
}

package module

import (
	"strings"

	"aego/contracts/persistence"
)

type OwnedSection struct {
	Owner   string
	ID      string
	Section persistence.Section
}

func (r *Registry) SaveSection(id string, s persistence.Section) {
	owner := r.Owner()
	switch {
	case r.cur == nil:
		r.fail("save section " + id + " is registered outside a module")
		return
	case s == nil:
		r.fail("save section " + id + " is nil")
		return
	case id != owner && !strings.HasPrefix(id, owner+"."):
		r.fail("save section " + id + " must be named " + owner + " or " + owner + ".<name>")
		return
	}
	for _, o := range r.saves {
		if o.ID == id {
			r.fail("save section " + id + " is already registered by " + o.Owner)
			return
		}
	}
	r.saves = append(r.saves, OwnedSection{Owner: owner, ID: id, Section: s})
}

func (r *Registry) SaveSections() []OwnedSection {
	return r.saves
}

func (r *Registry) GameModules() []Info {
	var out []Info
	for _, e := range r.modules {
		if !e.info.Builtin {
			out = append(out, e.info)
		}
	}
	return out
}

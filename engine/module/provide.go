package module

type Provision struct {
	Owner string
	Value any
}

type provision struct {
	kind string
	Provision
}

func (r *Registry) Provide(kind string, v any) {
	if kind == "" || v == nil {
		r.fail("provision needs a kind and a value")
		return
	}
	r.provided = append(r.provided, provision{kind: kind, Provision: Provision{Owner: r.Owner(), Value: v}})
}

func (m *Manifest) Provided(kind string) []Provision {
	var out []Provision
	for _, p := range m.provided {
		if p.kind == kind {
			out = append(out, p.Provision)
		}
	}
	return out
}

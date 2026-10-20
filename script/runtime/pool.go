package runtime

import (
	"slices"

	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/script/bind"
)

func (r *Runtime) ParkNode(h scene.NodeHandle) bool {
	if slices.Contains(r.spawn, h) {
		return false
	}
	if r.m != nil && !r.m.Busy() {
		r.Flush()
	}
	for _, id := range r.byNode[h] {
		in := &r.inst[id-1]
		if in.state == dead {
			continue
		}
		if r.co != nil {
			r.co.CancelOwner(id)
		}
		if r.sig != nil {
			r.sig.DropInstance(id)
		}
		in.state = Disabled
	}
	return true
}

func (r *Runtime) ReviveNode(h scene.NodeHandle) {
	if r.scene == nil {
		return
	}
	sc, _ := scriptcomp.ScriptsOf(r.scene, h)
	for _, id := range r.byNode[h] {
		in := &r.inst[id-1]
		if in.state == dead || in.base < 0 {
			continue
		}
		t := in.typ
		self := t.vals[in.base : in.base+t.slots]
		copy(self, t.init)
		self[0] = bind.NodeValue(h)
		if sc != nil {
			if i := sc.Index(t.name); i >= 0 {
				props := &sc.Attachments[i].Props
				for _, f := range t.fields {
					if v, ok := props.Get(f.name); ok {
						self[f.slot] = r.convert(t, f, v, h)
					}
				}
			}
		}
		in.state = Running
	}
}

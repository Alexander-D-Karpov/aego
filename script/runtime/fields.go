package runtime

import (
	"aego/contracts/scriptfields"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

var _ scriptfields.Fields = (*Runtime)(nil)

func (r *Runtime) ResolveField(h scene.NodeHandle, script, field string) (scriptfields.Ref, bool) {
	for _, id := range r.byNode[h] {
		in := &r.inst[id-1]
		if in.state == dead || in.base < 0 || (in.typ.name != script && shortName(in.typ.name) != script) {
			continue
		}
		f := in.typ.def.Field(field)
		if f == nil {
			return scriptfields.Ref{}, false
		}
		t, ok := abi.Parse(f.Type)
		if !ok {
			return scriptfields.Ref{}, false
		}
		return scriptfields.Ref{
			Node: h, Instance: id, Script: in.typ.name, Field: f.Name,
			Kind: t.Kind, Slot: int(f.Slot), Layout: in.typ,
		}, true
	}
	return scriptfields.Ref{}, false
}

func (r *Runtime) fieldSlot(ref *scriptfields.Ref) ([]vm.Value, int, bool) {
	in := r.live(ref.Instance)
	if in == nil || in.node != ref.Node || in.typ.name != ref.Script {
		return nil, 0, false
	}
	if lay, ok := ref.Layout.(*scriptType); !ok || lay != in.typ {
		f := in.typ.def.Field(ref.Field)
		if f == nil {
			return nil, 0, false
		}
		if t, ok := abi.Parse(f.Type); !ok || t.Kind != ref.Kind {
			return nil, 0, false
		}
		ref.Slot, ref.Layout = int(f.Slot), in.typ
	}
	return in.typ.vals, in.base + ref.Slot, true
}

func (r *Runtime) LoadField(ref *scriptfields.Ref) (vm.Value, bool) {
	vals, i, ok := r.fieldSlot(ref)
	if !ok {
		return vm.Value{}, false
	}
	return vals[i], true
}

func (r *Runtime) StoreField(ref *scriptfields.Ref, v vm.Value) bool {
	vals, i, ok := r.fieldSlot(ref)
	if !ok {
		return false
	}
	vals[i] = v
	return true
}

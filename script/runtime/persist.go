package runtime

import (
	"slices"

	"aego/contracts/persistence"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bytecode"
)

var _ persistence.Scripts = (*Runtime)(nil)

func (r *Runtime) Prepare() {
	if r.scene == nil {
		return
	}
	for i := 0; i < len(r.spawn); i++ {
		r.spawnNode(r.spawn[i])
	}
	r.spawn = r.spawn[:0]
}

func (r *Runtime) Persisted(h scene.NodeHandle, dst []persistence.ScriptValue) []persistence.ScriptValue {
	for _, id := range r.byNode[h] {
		in := &r.inst[id-1]
		if in.state == dead || in.base < 0 {
			continue
		}
		t := in.typ
		for i := range t.def.Fields {
			f := &t.def.Fields[i]
			if !f.Persist {
				continue
			}
			tr, ok := abi.Parse(f.Type)
			if !ok {
				continue
			}
			sv := persistence.ScriptValue{Script: t.name, Field: f.Name, Kind: tr.Kind, Value: t.vals[in.base+int(f.Slot)]}
			if tr.Kind == abi.String {
				sv.Text, _ = r.m.Str(sv.Value)
			}
			dst = append(dst, sv)
		}
	}
	return dst
}

func persistedField(def *bytecode.ScriptDef, name string) *bytecode.FieldDef {
	if f := def.Field(name); f != nil {
		return f
	}
	for i := range def.Fields {
		if slices.Contains(def.Fields[i].Former, name) {
			return &def.Fields[i]
		}
	}
	return nil
}

func (r *Runtime) RestoreField(h scene.NodeHandle, sv persistence.ScriptValue) bool {
	for _, id := range r.byNode[h] {
		in := &r.inst[id-1]
		t := in.typ
		if in.state == dead || in.base < 0 || (t.name != sv.Script && !slices.Contains(t.former, sv.Script)) {
			continue
		}
		f := persistedField(t.def, sv.Field)
		if f == nil || !f.Persist {
			return false
		}
		tr, ok := abi.Parse(f.Type)
		if !ok || tr.Kind != sv.Kind {
			return false
		}
		v := sv.Value
		if tr.Kind == abi.String {
			v = r.m.NewString(sv.Text)
		}
		t.vals[in.base+int(f.Slot)] = v
		return true
	}
	return false
}

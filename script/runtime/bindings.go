package runtime

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/vm"
)

const unknownComponent = "engine.Unknown"

var errNoScene = errors.New("no scene is bound to the script runtime")

func fieldABI(fd *schema.FieldDescriptor) (abi.TypeRef, bool) {
	switch fd.Kind {
	case schema.KindBool:
		return abi.T(abi.Bool), true
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return abi.T(abi.Int), true
	case schema.KindFloat32, schema.KindFloat64:
		return abi.T(abi.Float), true
	case schema.KindString:
		return abi.T(abi.String), true
	case schema.KindVec2:
		return abi.T(abi.Vec2), true
	case schema.KindVec3:
		return abi.T(abi.Vec3), true
	case schema.KindColor:
		return abi.T(abi.Color), true
	case schema.KindQuat:
		return abi.T(abi.Quat), true
	case schema.KindRect:
		return abi.T(abi.Rect), true
	case schema.KindCurve:
		return abi.T(abi.Curve), true
	case schema.KindNodeRef:
		return abi.T(abi.NodeRef), true
	case schema.KindAssetRef:
		kind := fd.AssetKind
		if kind == "" {
			kind = "texture"
		}
		return abi.AssetOf(kind), true
	case schema.KindList:
		switch fd.Elem {
		case schema.KindVec2, schema.KindFloat32, schema.KindInt32, schema.KindInt64, schema.KindBool:
			e, _ := fieldABI(&schema.FieldDescriptor{Kind: fd.Elem})
			return abi.SliceOf(e), true
		}
	}
	return abi.TypeRef{}, false
}

func readOnly(fd *schema.FieldDescriptor) bool {
	return fd.Flags.Has(schema.FlagReadOnly) || fd.Kind == schema.KindList || fd.Kind == schema.KindCurve
}

func exportName(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func ComponentFields(td *schema.TypeDescriptor) []*schema.FieldDescriptor {
	var out []*schema.FieldDescriptor
	for i := range td.Fields {
		fd := &td.Fields[i]
		if !td.Scripted(fd) {
			continue
		}
		if _, ok := fieldABI(fd); ok {
			out = append(out, fd)
		}
	}
	return out
}

func scriptable(td *schema.TypeDescriptor) bool {
	return td.FullName != scriptcomp.ScriptName && td.FullName != unknownComponent
}

func fieldLimit(td *schema.TypeDescriptor, n int) error {
	if n <= bytecode.MaxComponentFields {
		return nil
	}
	return fmt.Errorf("component %s of module %s exposes %d script fields, the limit is %d", td.FullName, td.Owner, n, bytecode.MaxComponentFields)
}

func BindComponents(reg *bind.Registry, sr *schema.Registry) {
	for _, td := range sr.Components() {
		if !scriptable(td) {
			continue
		}
		fields := ComponentFields(td)
		if err := fieldLimit(td, len(fields)); err != nil {
			reg.Fail(err)
			continue
		}
		c := &abi.Component{Name: td.FullName}
		for _, fd := range fields {
			t, _ := fieldABI(fd)
			c.Fields = append(c.Fields, abi.Field{Name: exportName(fd.Name), Type: t, ReadOnly: readOnly(fd)})
		}
		reg.Component(c)
		idx, tid := uint64(c.Index), td.ID
		reg.Intrinsic("#NodeRef", "Get<"+td.FullName+">", abi.Sig(abi.ComponentOf(td.FullName), abi.T(abi.NodeRef)),
			func(m *vm.Machine, w []vm.Value) error {
				ctx, _ := m.Env.(*bind.Ctx)
				if ctx == nil || ctx.Scene == nil {
					return errNoScene
				}
				sc := ctx.Scene
				h := bind.NodeOf(w[0])
				if h.IsZero() {
					return &vm.Fault{Code: vm.CodeNil, Message: "node reference is nil"}
				}
				if _, ok := sc.GetByType(h, tid); ok && sc.IsAlive(h) {
					w[0] = vm.Value{A: w[0].A, B: idx}
				} else {
					w[0] = vm.Value{}
				}
				return nil
			})
		reg.Intrinsic("#NodeRef", "Has<"+td.FullName+">", abi.Sig(abi.T(abi.Bool), abi.T(abi.NodeRef)),
			func(m *vm.Machine, w []vm.Value) error {
				ctx, _ := m.Env.(*bind.Ctx)
				h := bind.NodeOf(w[0])
				ok := false
				if ctx != nil && ctx.Scene != nil && ctx.Scene.IsAlive(h) {
					_, ok = ctx.Scene.GetByType(h, tid)
				}
				w[0] = vm.Bool(ok)
				return nil
			})
	}
}

type compInfo struct {
	typ    schema.TypeID
	fields []*schema.FieldDescriptor
}

func linkComponents(reg *bind.Registry, sr *schema.Registry) []compInfo {
	if sr == nil {
		return nil
	}
	list := reg.Table().AllComponents()
	out := make([]compInfo, len(list))
	for i, c := range list {
		if td, ok := sr.ByName(c.Name); ok {
			out[i] = compInfo{typ: td.ID, fields: ComponentFields(td)}
		}
	}
	return out
}

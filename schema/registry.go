package schema

import (
	"reflect"
	"strings"
	"unsafe"

	"aego/core/curve"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
)

type vec2 = mathx.Vec2

type vec3 = mathx.Vec3

type vec4 = mathx.Vec4

type quat = mathx.Quat

type color = mathx.Color

type rect = mathx.Rect

var (
	vec2Type     = reflect.TypeFor[mathx.Vec2]()
	vec3Type     = reflect.TypeFor[mathx.Vec3]()
	vec4Type     = reflect.TypeFor[mathx.Vec4]()
	quatType     = reflect.TypeFor[mathx.Quat]()
	colorType    = reflect.TypeFor[mathx.Color]()
	rectType     = reflect.TypeFor[mathx.Rect]()
	curveType    = reflect.TypeFor[curve.Curve]()
	assetRefType = reflect.TypeFor[AssetRef]()
	nodeRefType  = reflect.TypeFor[NodeRef]()
)

type enumInfo struct {
	name   string
	values []EnumValue
}

type Registry struct {
	frozen bool
	byID   map[TypeID]*TypeDescriptor
	byName map[string]*TypeDescriptor
	byType map[reflect.Type]*TypeDescriptor
	enums  map[reflect.Type]*enumInfo
	order  []*TypeDescriptor
	dyn    dynamic
}

func NewRegistry() *Registry {
	return &Registry{
		byID:   map[TypeID]*TypeDescriptor{},
		byName: map[string]*TypeDescriptor{},
		byType: map[reflect.Type]*TypeDescriptor{},
		enums:  map[reflect.Type]*enumInfo{},
	}
}

func (r *Registry) Frozen() bool {
	return r.frozen
}

func (r *Registry) ByID(id TypeID) (*TypeDescriptor, bool) {
	if t, ok := r.byID[id]; ok {
		return t, true
	}
	if l := r.dyn.layer.Load(); l != nil {
		t, ok := l.byID[id]
		return t, ok
	}
	return nil, false
}

func (r *Registry) ByName(name string) (*TypeDescriptor, bool) {
	if t, ok := r.byName[name]; ok {
		return t, true
	}
	if l := r.dyn.layer.Load(); l != nil {
		t, ok := l.byName[name]
		return t, ok
	}
	return nil, false
}

func (r *Registry) ByGoType(t reflect.Type) (*TypeDescriptor, bool) {
	d, ok := r.byType[t]
	return d, ok
}

func (r *Registry) Field(t TypeID, name string) (*FieldDescriptor, bool) {
	td, ok := r.ByID(t)
	if !ok {
		return nil, false
	}
	return td.Field(name)
}

func (r *Registry) Each(fn func(*TypeDescriptor)) {
	for _, t := range r.order {
		fn(t)
	}
}

func (r *Registry) Components() []*TypeDescriptor {
	out := make([]*TypeDescriptor, 0, len(r.order))
	for _, t := range r.order {
		if t.Kind == TypeComponent {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) Len() int {
	return len(r.order)
}

func RegisterEnum[T ~int8 | ~int16 | ~int32 | ~int64 | ~int | ~uint8 | ~uint16 | ~uint32](r *Registry, name string, values ...EnumValue) error {
	if r.frozen {
		return errs.New(errs.SchemaFrozen, "registry is frozen", log.F("enum", name))
	}
	rt := reflect.TypeFor[T]()
	if _, dup := r.enums[rt]; dup {
		return errs.New(errs.SchemaDuplicate, "enum type is already registered", log.F("enum", name))
	}
	if len(values) == 0 {
		return errs.New(errs.SchemaBadValue, "enum needs at least one value", log.F("enum", name))
	}
	r.enums[rt] = &enumInfo{name: name, values: values}
	return nil
}

func Register[T any](r *Registry, opts ...Option) (*TypeDescriptor, error) {
	return r.register(reflect.TypeFor[T](), TypeComponent, opts)
}

func RegisterStruct[T any](r *Registry, opts ...Option) (*TypeDescriptor, error) {
	return r.register(reflect.TypeFor[T](), TypeStruct, opts)
}

func RegisterData[T any](r *Registry, opts ...Option) (*TypeDescriptor, error) {
	return r.register(reflect.TypeFor[T](), TypeAsset, opts)
}

func (r *Registry) DataTypes() []*TypeDescriptor {
	var out []*TypeDescriptor
	for _, t := range r.order {
		if t.Kind == TypeAsset {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) register(rt reflect.Type, kind TypeKind, opts []Option) (*TypeDescriptor, error) {
	if r.frozen {
		return nil, errs.New(errs.SchemaFrozen, "registry is frozen", log.F("type", rt.String()))
	}
	if rt.Kind() != reflect.Struct {
		return nil, errs.New(errs.SchemaNotStruct, "only structs can be registered",
			log.F("type", rt.String()))
	}
	var o regOptions
	for _, fn := range opts {
		fn(&o)
	}
	full := o.name
	if full == "" {
		full = defaultName(rt)
	}
	if existing, dup := r.byName[full]; dup {
		return nil, errs.New(errs.SchemaDuplicate, "type name is already registered",
			log.F("name", full), log.F("existing", existing.FullName))
	}
	if _, dup := r.byType[rt]; dup {
		return nil, errs.New(errs.SchemaDuplicate, "go type is already registered",
			log.F("type", rt.String()))
	}
	id := HashName(full)
	if existing, clash := r.byID[id]; clash {
		return nil, errs.New(errs.SchemaCollision, "type name hash collides",
			log.F("name", full), log.F("existing", existing.FullName))
	}

	td := &TypeDescriptor{
		ID:       id,
		FullName: full,
		Former:   o.former,
		Version:  o.version,
		Size:     rt.Size(),
		Kind:     kind,
		Space:    o.space,
		Doc:      o.doc,
		index:    map[string]int{},
		byID:     map[FieldID]int{},
		New: func() unsafe.Pointer {
			return reflect.New(rt).UnsafePointer()
		},
		Copy: func(dst, src unsafe.Pointer) {
			reflect.NewAt(rt, dst).Elem().Set(reflect.NewAt(rt, src).Elem())
		},
	}
	if td.Version == 0 {
		td.Version = 1
	}

	if err := r.addFields(td, rt, 0, &o); err != nil {
		return nil, err
	}
	if err := finishClone(td, rt, &o); err != nil {
		return nil, err
	}
	for name := range o.fieldDocs {
		if _, ok := td.index[name]; !ok {
			return nil, errs.New(errs.SchemaBadField, "FieldDoc names an unknown field",
				log.F("type", full), log.F("field", name))
		}
	}
	for name := range o.gizmos {
		if _, ok := td.index[name]; !ok {
			return nil, errs.New(errs.SchemaBadField, "Gizmo names an unknown field",
				log.F("type", full), log.F("field", name))
		}
	}
	for _, m := range []map[string]HintSource{o.choices, o.bitsets} {
		for name := range m {
			if _, ok := td.index[name]; !ok {
				return nil, errs.New(errs.SchemaBadField, "hint names an unknown field", log.F("type", full), log.F("field", name))
			}
		}
	}
	for _, m := range []map[string]bool{o.runtime, o.persist} {
		for name := range m {
			if _, ok := td.index[name]; !ok {
				return nil, errs.New(errs.SchemaBadField, "Runtime or Persist names an unknown field", log.F("type", full), log.F("field", name))
			}
		}
	}

	r.byID[id] = td
	r.byName[full] = td
	r.byType[rt] = td
	r.order = append(r.order, td)
	return td, nil
}

// addFields flattens embedded exported structs with offsets relative to the owner.
func (r *Registry) addFields(td *TypeDescriptor, rt reflect.Type, base uintptr, o *regOptions) error {
	full := td.FullName
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("aego")
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			if tag == "-" {
				continue
			}
			if err := r.addFields(td, sf.Type, base+sf.Offset, o); err != nil {
				return err
			}
			continue
		}
		info, err := parseTag(full, sf.Name, tag)
		if err != nil {
			return err
		}
		if info.skip {
			continue
		}
		sf.Offset += base
		fd, err := r.buildField(full, sf, info)
		if err != nil {
			return err
		}
		if former, ok := o.formerFields[fd.Name]; ok {
			fd.Former = append(fd.Former, former...)
		}
		if doc, ok := o.fieldDocs[fd.Name]; ok {
			fd.Doc = doc
			if fd.Hints.Tooltip == "" {
				fd.Hints.Tooltip = doc
			}
		}
		if g, ok := o.gizmos[fd.Name]; ok {
			fd.Hints.Gizmo, fd.Hints.GizmoSpace = g[0], g[1]
		}
		if h, ok := o.choices[fd.Name]; ok {
			fd.Hints.Choices = h
		}
		if h, ok := o.bitsets[fd.Name]; ok {
			fd.Hints.Bitset = h
		}
		if (fd.Hints.Choices.Section != "" || fd.Hints.Bitset.Section != "") && fd.Kind != KindInt32 && fd.Kind != KindInt64 {
			return errs.New(errs.SchemaBadField, "choices and bitset hints need an integer field", log.F("type", full), log.F("field", fd.Name))
		}
		if o.runtime[fd.Name] {
			fd.Hints.Runtime = true
			fd.Flags |= FlagReadOnly
		}
		if o.persist[fd.Name] || (o.persistAll && fd.Flags.Has(FlagSerialize)) {
			fd.Flags |= FlagPersist
		}
		if err := ValidGizmo(fd.Kind, fd.Hints.Gizmo, fd.Hints.GizmoSpace); err != nil {
			return errs.Wrap(err, errs.SchemaBadField, "bad gizmo").With(log.F("type", full), log.F("field", fd.Name))
		}
		fd.ID = HashField(td.ID, fd.Name)
		if prev, dup := td.index[fd.Name]; dup {
			return errs.New(errs.SchemaDuplicate, "duplicate field name",
				log.F("type", full), log.F("field", fd.Name), log.F("previous", td.Fields[prev].Name))
		}
		if prev, clash := td.byID[fd.ID]; clash {
			return errs.New(errs.SchemaCollision, "field name hash collides",
				log.F("type", full), log.F("field", fd.Name), log.F("existing", td.Fields[prev].Name))
		}
		td.index[fd.Name] = len(td.Fields)
		td.byID[fd.ID] = len(td.Fields)
		td.Fields = append(td.Fields, fd)
	}
	return nil
}

var (
	assetIDType = reflect.TypeFor[ids.AssetID]()
	nodeIDType  = reflect.TypeFor[ids.NodeID]()
)

func directRef(ft reflect.Type, k Kind) bool {
	want := assetIDType
	if k == KindNodeRef {
		want = nodeIDType
	}
	return ft.Kind() == reflect.Struct && ft.NumField() == 1 &&
		ft.Field(0).Type == want && ft.Field(0).Offset == 0 && ft.Size() == want.Size()
}

func (r *Registry) buildField(typeName string, sf reflect.StructField, info tagInfo) (FieldDescriptor, error) {
	fd := FieldDescriptor{
		Name:      info.name,
		Former:    info.former,
		Flags:     info.flags,
		AssetKind: info.assetKind,
		Min:       info.min,
		Max:       info.max,
		HasMin:    info.hasMin,
		HasMax:    info.hasMax,
		Hints:     info.hints,
		Doc:       info.doc,
		Offset:    sf.Offset,
		Size:      sf.Type.Size(),
	}
	ft := sf.Type
	kind, elem, structID, err := r.kindOf(typeName, info.name, ft)
	if err != nil {
		return fd, err
	}
	fd.Kind = kind
	fd.Elem = elem
	fd.Struct = structID
	switch kind {
	case KindEnum:
		if ei, ok := r.enums[ft]; ok {
			fd.Enum = ei.values
			fd.EnumName = ei.name
		}
		switch ft.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint32:
			fd.Unsigned = true
		}
	case KindAssetRef:
		if fd.AssetKind == "" {
			fd.AssetKind = reflect.New(ft).Interface().(AssetRef).AssetKind()
		}
		fd.Direct = directRef(ft, kind)
	case KindNodeRef:
		fd.Direct = directRef(ft, kind)
	}
	fd.Fast = kind.IsPOD()
	if err := r.makeAccessors(&fd, ft, sf.Offset); err != nil {
		return fd, err
	}
	return fd, nil
}

func (r *Registry) kindOf(typeName, field string, ft reflect.Type) (Kind, Kind, TypeID, error) {
	if _, ok := r.enums[ft]; ok {
		return KindEnum, KindInvalid, 0, nil
	}
	if reflect.PointerTo(ft).Implements(assetRefType) {
		return KindAssetRef, KindInvalid, 0, nil
	}
	if reflect.PointerTo(ft).Implements(nodeRefType) {
		return KindNodeRef, KindInvalid, 0, nil
	}
	switch ft {
	case vec2Type:
		return KindVec2, KindInvalid, 0, nil
	case vec3Type:
		return KindVec3, KindInvalid, 0, nil
	case vec4Type:
		return KindVec4, KindInvalid, 0, nil
	case quatType:
		return KindQuat, KindInvalid, 0, nil
	case colorType:
		return KindColor, KindInvalid, 0, nil
	case rectType:
		return KindRect, KindInvalid, 0, nil
	case curveType:
		return KindCurve, KindInvalid, 0, nil
	}
	switch ft.Kind() {
	case reflect.Bool:
		return KindBool, KindInvalid, 0, nil
	case reflect.Int32:
		return KindInt32, KindInvalid, 0, nil
	case reflect.Int, reflect.Int64:
		return KindInt64, KindInvalid, 0, nil
	case reflect.Float32:
		return KindFloat32, KindInvalid, 0, nil
	case reflect.Float64:
		return KindFloat64, KindInvalid, 0, nil
	case reflect.String:
		return KindString, KindInvalid, 0, nil
	case reflect.Slice:
		elemKind, _, structID, err := r.kindOf(typeName, field, ft.Elem())
		if err != nil {
			return KindInvalid, KindInvalid, 0, err
		}
		if elemKind == KindList {
			return KindInvalid, KindInvalid, 0, tagError(typeName, field, "nested slices are not supported")
		}
		return KindList, elemKind, structID, nil
	case reflect.Struct:
		td, ok := r.byType[ft]
		if !ok {
			return KindInvalid, KindInvalid, 0, errs.New(errs.SchemaUnknownType,
				"struct field type must be registered with RegisterStruct first",
				log.F("type", typeName), log.F("field", field), log.F("fieldType", ft.String()))
		}
		return KindStruct, KindInvalid, td.ID, nil
	}
	return KindInvalid, KindInvalid, 0, tagError(typeName, field,
		"unsupported field type "+ft.String()+"; tag it with aego:\"-\" to skip")
}

func (r *Registry) Freeze() error {
	if r.frozen {
		return nil
	}
	seen := map[string]*TypeDescriptor{}
	for _, td := range r.order {
		seen[td.FullName] = td
	}
	for _, td := range r.order {
		for _, alias := range td.Former {
			if other, dup := seen[alias]; dup {
				return errs.New(errs.SchemaDuplicate, "former name collides with an existing type",
					log.F("type", td.FullName), log.F("former", alias), log.F("other", other.FullName))
			}
			id := HashName(alias)
			if other, clash := r.byID[id]; clash && other != td {
				return errs.New(errs.SchemaCollision, "former name hash collides",
					log.F("type", td.FullName), log.F("former", alias), log.F("other", other.FullName))
			}
			seen[alias] = td
			r.byName[alias] = td
			r.byID[id] = td
		}
		for i := range td.Fields {
			fd := &td.Fields[i]
			if fd.Kind == KindStruct || fd.Elem == KindStruct {
				if _, ok := r.byID[fd.Struct]; !ok {
					return errs.New(errs.SchemaUnknownType, "field references an unregistered struct",
						log.F("type", td.FullName), log.F("field", fd.Name))
				}
			}
			for _, alias := range fd.Former {
				if _, dup := td.index[alias]; dup {
					return errs.New(errs.SchemaDuplicate, "former field name collides with an existing field",
						log.F("type", td.FullName), log.F("field", fd.Name), log.F("former", alias))
				}
				td.index[alias] = td.index[fd.Name]
			}
		}
	}
	r.frozen = true
	return nil
}

func defaultName(rt reflect.Type) string {
	pkg := rt.PkgPath()
	if i := strings.LastIndexByte(pkg, '/'); i >= 0 {
		pkg = pkg[i+1:]
	}
	if pkg == "" {
		return rt.Name()
	}
	return pkg + "." + rt.Name()
}

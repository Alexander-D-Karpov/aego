package module

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

type Integer interface {
	~int8 | ~int16 | ~int32 | ~int64 | ~int | ~uint8 | ~uint16 | ~uint32
}

type ComponentBuilder[T any] struct {
	r    *Registry
	desc *schema.TypeDescriptor
	err  error
}

func Component[T any](r *Registry, opts ...schema.Option) *ComponentBuilder[T] {
	b := &ComponentBuilder[T]{r: r}
	rt := reflect.TypeFor[T]()
	if err := scene.RegisterComponent[T](r.types, opts...); err != nil {
		b.err = r.wrap(err, "component "+rt.String()+" registration failed")
		return b
	}
	td, ok := r.Schema().ByGoType(rt)
	if !ok {
		b.err = r.fail("component " + rt.String() + " has no descriptor after registration")
		return b
	}
	td.Owner = r.Owner()
	td.Builtin = r.builtin()
	b.desc = td
	r.comps = append(r.comps, td)
	return b
}

func Struct[T any](r *Registry, opts ...schema.Option) error {
	if _, err := schema.RegisterStruct[T](r.Schema(), opts...); err != nil {
		return r.wrap(err, "struct "+reflect.TypeFor[T]().String()+" registration failed")
	}
	return nil
}

func Enum[T Integer](r *Registry, name string, values ...schema.EnumValue) error {
	if err := schema.RegisterEnum[T](r.Schema(), name, values...); err != nil {
		return r.wrap(err, "enum "+name+" registration failed")
	}
	return nil
}

func (b *ComponentBuilder[T]) Err() error {
	return b.err
}

func (b *ComponentBuilder[T]) Descriptor() *schema.TypeDescriptor {
	return b.desc
}

func (b *ComponentBuilder[T]) fail(msg string) {
	if err := b.r.fail(msg); b.err == nil {
		b.err = err
	}
}

func (b *ComponentBuilder[T]) edit() *schema.TypeDescriptor {
	if b.desc == nil {
		return nil
	}
	if b.r.Schema().Frozen() {
		if b.err == nil {
			b.err = errs.New(errs.EngineFrozen, "component "+b.desc.FullName+" is changed after the schema was frozen", log.F("module", b.r.Owner()))
		}
		return nil
	}
	return b.desc
}

func (b *ComponentBuilder[T]) Category(name string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.Category = name
	}
	return b
}

func (b *ComponentBuilder[T]) Display(name string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.DisplayName = name
	}
	return b
}

// SearchAliases retains former public names without changing the technical ID.
func (b *ComponentBuilder[T]) SearchAliases(names ...string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.SearchAliases = append(d.SearchAliases, names...)
	}
	return b
}

func (b *ComponentBuilder[T]) Icon(name string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.Icon = name
	}
	return b
}

func (b *ComponentBuilder[T]) Default(fn func() T) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if fn == nil {
		b.fail("component " + d.FullName + " has a nil default")
		return b
	}
	d.Default = func() unsafe.Pointer {
		v := fn()
		return unsafe.Pointer(&v)
	}
	return b
}

func scriptField(d *schema.TypeDescriptor, name string) (*schema.FieldDescriptor, bool) {
	if fd, ok := d.Field(name); ok {
		return fd, true
	}
	r, n := utf8.DecodeRuneInString(name)
	return d.Field(string(unicode.ToLower(r)) + name[n:])
}

func (b *ComponentBuilder[T]) Script(fields ...string) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	ids := make([]schema.FieldID, 0, len(fields))
	for _, name := range fields {
		fd, ok := scriptField(d, name)
		switch {
		case !ok:
			b.fail("component " + d.FullName + " has no field " + name)
		case !fd.Flags.Has(schema.FlagScript):
			b.fail("field " + d.FullName + "." + fd.Name + " is tagged noscript")
		default:
			ids = append(ids, fd.ID)
		}
	}
	d.ScriptFields = ids
	return b
}

func (b *ComponentBuilder[T]) ScriptNone() *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.ScriptFields = []schema.FieldID{}
	}
	return b
}

func (b *ComponentBuilder[T]) declared(d *schema.TypeDescriptor, name string) bool {
	for _, m := range d.ScriptMethods {
		if m.Name == name {
			b.fail("script method " + name + " of " + d.FullName + " is declared twice")
			return true
		}
	}
	return false
}

func (b *ComponentBuilder[T]) ScriptMethod(name string, fn any, opts ...bind.Option) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if name == "" || fn == nil || reflect.TypeOf(fn).Kind() != reflect.Func {
		b.fail("script method " + name + " of " + d.FullName + " needs a name and a function")
		return b
	}
	if b.declared(d, name) {
		return b
	}
	conv := make([]any, len(opts))
	for i, o := range opts {
		conv[i] = o
	}
	d.ScriptMethods = append(d.ScriptMethods, schema.ScriptMethod{Name: name, Fn: fn, Opts: conv})
	return b
}

func (b *ComponentBuilder[T]) typed(name string, missing bool, adapt func(*bind.Module, schema.TypeID)) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if name == "" || missing {
		b.fail("script method " + name + " of " + d.FullName + " needs a name and a function")
		return b
	}
	if b.declared(d, name) {
		return b
	}
	d.ScriptMethods = append(d.ScriptMethods, schema.ScriptMethod{Name: name, Bind: adapt})
	return b
}

func (b *ComponentBuilder[T]) Method0[R any](name string, fn func(*bind.Ctx, *T) R, opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.Method0(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Method1[A, R any](name string, fn func(*bind.Ctx, *T, A) R, opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.Method1(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Method2[A, B, R any](name string, fn func(*bind.Ctx, *T, A, B) R, opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.Method2(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Proc0(name string, fn func(*bind.Ctx, *T), opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.MethodProc0(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Proc1[A any](name string, fn func(*bind.Ctx, *T, A), opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.MethodProc1(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Proc2[A, B any](name string, fn func(*bind.Ctx, *T, A, B), opts ...bind.Option) *ComponentBuilder[T] {
	return b.typed(name, fn == nil, func(m *bind.Module, t schema.TypeID) { bind.MethodProc2(m, name, t, fn, opts...) })
}

func (b *ComponentBuilder[T]) Template(name string, setup func(*scene.Scene, scene.NodeHandle)) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if d.Template != nil {
		b.fail("component " + d.FullName + " declares its template twice")
		return b
	}
	if name == "" {
		name = d.FullName[strings.LastIndexByte(d.FullName, '.')+1:]
	}
	d.Template = &schema.TemplateDesc{Name: name, Space: d.Space, Setup: componentSetup(d, setup)}
	b.r.addTemplate(d.Template)
	return b
}

func (b *ComponentBuilder[T]) Requires(fullNames ...string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.Requires = append(d.Requires, fullNames...)
	}
	return b
}

// RequiresOneOf needs any one of the components; adding picks the first when the node has none.
func (b *ComponentBuilder[T]) RequiresOneOf(fullNames ...string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil && len(fullNames) > 0 {
		d.RequiresAny = append(d.RequiresAny, fullNames)
	}
	return b
}

func (b *ComponentBuilder[T]) Suggests(fullNames ...string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.Suggests = append(d.Suggests, fullNames...)
	}
	return b
}

func (b *ComponentBuilder[T]) Conflicts(fullNames ...string) *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.Conflicts = append(d.Conflicts, fullNames...)
	}
	return b
}

func (b *ComponentBuilder[T]) SceneUnique() *ComponentBuilder[T] {
	if d := b.edit(); d != nil {
		d.SceneUnique = true
	}
	return b
}

func (b *ComponentBuilder[T]) Inspector(fn any) *ComponentBuilder[T] {
	d := b.edit()
	if d == nil {
		return b
	}
	if fn == nil {
		b.fail("component " + d.FullName + " has a nil inspector")
		return b
	}
	d.Inspector = fn
	return b
}

func componentSetup(d *schema.TypeDescriptor, user func(*scene.Scene, scene.NodeHandle)) func(*scene.Scene, scene.NodeHandle) {
	return func(s *scene.Scene, h scene.NodeHandle) {
		for _, name := range d.Requires {
			if td, ok := s.Registry().ByName(name); ok {
				Ensure(s, h, td)
			}
		}
		for _, alts := range d.RequiresAny {
			if !hasAny(s, h, alts) {
				if td, ok := s.Registry().ByName(alts[0]); ok {
					Ensure(s, h, td)
				}
			}
		}
		Ensure(s, h, d)
		if user != nil {
			user(s, h)
		}
	}
}

func hasAny(s *scene.Scene, h scene.NodeHandle, names []string) bool {
	for _, n := range names {
		if td, ok := s.Registry().ByName(n); ok {
			if _, has := s.GetByType(h, td.ID); has {
				return true
			}
		}
	}
	return false
}

func Ensure(s *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor) {
	if _, ok := s.GetByType(h, td.ID); ok {
		return
	}
	p, err := s.AddByType(h, td.ID)
	if err != nil || td.Default == nil || td.Copy == nil {
		return
	}
	td.Copy(p, td.Default())
}

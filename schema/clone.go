package schema

import (
	"reflect"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
)

type fixer func(dst, src unsafe.Pointer)

func Clone[T any](fn func(dst, src *T)) Option {
	return func(o *regOptions) {
		o.clone = func(dst, src unsafe.Pointer) { fn((*T)(dst), (*T)(src)) }
	}
}

type cloneBuilder struct {
	slices map[reflect.Type]bool
	fixers map[reflect.Type]*fixer
}

func newCloneBuilder() *cloneBuilder {
	return &cloneBuilder{slices: map[reflect.Type]bool{}, fixers: map[reflect.Type]*fixer{}}
}

func (b *cloneBuilder) hasSlices(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice:
		return true
	case reflect.Array:
		return t.Len() > 0 && b.hasSlices(t.Elem())
	case reflect.Struct:
		if v, ok := b.slices[t]; ok {
			return v
		}
		b.slices[t] = false
		for i := 0; i < t.NumField(); i++ {
			if b.hasSlices(t.Field(i).Type) {
				b.slices[t] = true
				return true
			}
		}
	}
	return false
}

func (b *cloneBuilder) build(t reflect.Type) fixer {
	if !b.hasSlices(t) {
		return nil
	}
	if p, ok := b.fixers[t]; ok {
		return func(dst, src unsafe.Pointer) { (*p)(dst, src) }
	}
	p := new(fixer)
	b.fixers[t] = p
	var f fixer
	switch t.Kind() {
	case reflect.Slice:
		et := t.Elem()
		sub, size := b.build(et), et.Size()
		f = func(dst, src unsafe.Pointer) {
			sv := reflect.NewAt(t, src).Elem()
			if sv.IsNil() {
				return
			}
			n := sv.Len()
			nv := reflect.MakeSlice(t, n, n)
			reflect.Copy(nv, sv)
			if sub != nil && n > 0 {
				db, sb := nv.UnsafePointer(), sv.UnsafePointer()
				for i := 0; i < n; i++ {
					off := uintptr(i) * size
					sub(unsafe.Add(db, off), unsafe.Add(sb, off))
				}
			}
			reflect.NewAt(t, dst).Elem().Set(nv)
		}
	case reflect.Array:
		sub, size, n := b.build(t.Elem()), t.Elem().Size(), t.Len()
		f = func(dst, src unsafe.Pointer) {
			for i := 0; i < n; i++ {
				off := uintptr(i) * size
				sub(unsafe.Add(dst, off), unsafe.Add(src, off))
			}
		}
	case reflect.Struct:
		type part struct {
			off uintptr
			fix fixer
		}
		var parts []part
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if fx := b.build(sf.Type); fx != nil {
				parts = append(parts, part{off: sf.Offset, fix: fx})
			}
		}
		f = func(dst, src unsafe.Pointer) {
			for _, pt := range parts {
				pt.fix(unsafe.Add(dst, pt.off), unsafe.Add(src, pt.off))
			}
		}
	}
	*p = f
	return f
}

func holdsRefs(t reflect.Type, seen map[reflect.Type]bool) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return t.Len() > 0 && holdsRefs(t.Elem(), seen)
	case reflect.Struct:
		if seen[t] {
			return false
		}
		seen[t] = true
		for i := 0; i < t.NumField(); i++ {
			if holdsRefs(t.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

func skippedRef(rt reflect.Type) (string, bool) {
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		skip := sf.Tag.Get("aego") == "-"
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct && !skip {
			if name, ok := skippedRef(sf.Type); ok {
				return name, true
			}
			continue
		}
		if skip && holdsRefs(sf.Type, map[reflect.Type]bool{}) {
			return sf.Name, true
		}
	}
	return "", false
}

func finishClone(td *TypeDescriptor, rt reflect.Type, o *regOptions) error {
	if o.clone == nil {
		if name, ok := skippedRef(rt); ok {
			return errs.New(errs.SchemaBadField,
				"field "+name+" of "+td.FullName+" is skipped with aego:\"-\" and holds references; register schema.Clone for the type",
				log.F("type", td.FullName), log.F("field", name))
		}
	}
	fix := newCloneBuilder().build(rt)
	cp, user := td.Copy, o.clone
	switch {
	case user != nil:
		td.CloneInto = func(dst, src unsafe.Pointer) {
			cp(dst, src)
			if fix != nil {
				fix(dst, src)
			}
			user(dst, src)
		}
	case fix != nil:
		td.CloneInto = func(dst, src unsafe.Pointer) {
			cp(dst, src)
			fix(dst, src)
		}
	default:
		td.Flat = true
		td.CloneInto = cp
	}
	return nil
}

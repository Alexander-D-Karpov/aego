package module

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"unsafe"

	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
	"aego/schema"
)

const VersionAttr = "version"

type SectionDesc[T any] struct {
	ID       string
	Former   []string
	Title    string
	Order    int
	Version  int
	Default  func() T
	Decode   func(n *aetext.Node, v *T) error
	Encode   func(v *T, n *aetext.Node)
	Validate func(v *T) error
	Migrate  func(n *aetext.Node, from, to int) error
	Names    func(v *T, list string) []string
}

type Section struct {
	ID       string
	Former   []string
	Title    string
	Order    int
	Version  int
	Owner    string
	Type     *schema.TypeDescriptor
	def      func() any
	decode   func(*aetext.Node) (any, error)
	encode   func(any) *aetext.Node
	validate func(any) error
	migrate  func(*aetext.Node, int, int) error
	names    func(any, string) []string
}

func (s *Section) DefaultValue() any {
	return s.def()
}

func (s *Section) DefaultBlock() *aetext.Node {
	return s.encode(s.def())
}

func (s *Section) Heads() []string {
	return append([]string{s.ID}, s.Former...)
}

func sectionErr(id, msg string) error {
	return errs.New(errs.ProjectBadFile, id+": "+msg, log.F("section", id))
}

func (r *Registry) ProjectSection[T any](d SectionDesc[T]) {
	if r.cur == nil {
		r.fail("project section " + d.ID + " is registered outside a module")
		return
	}
	if d.ID == "" {
		r.fail("project section needs an id")
		return
	}
	if d.Version <= 0 {
		d.Version = 1
	}
	if d.Default == nil {
		d.Default = func() T {
			var zero T
			return zero
		}
	}
	s := &Section{ID: d.ID, Former: d.Former, Title: d.Title, Order: d.Order, Version: d.Version, Owner: r.Owner()}
	if s.Title == "" {
		s.Title = d.ID
	}
	if td, ok := r.Schema().ByGoType(reflect.TypeFor[T]()); ok {
		s.Type = td
	}
	dec, enc := d.Decode, d.Encode
	if dec == nil || enc == nil {
		td := s.Type
		if td == nil {
			r.fail("project section " + d.ID + " needs Decode and Encode or a type registered with module.Struct")
			return
		}
		if err := codecFits(td); err != nil {
			r.wrap(err, "project section "+d.ID+" cannot use the schema codec")
			return
		}
		if dec == nil {
			dec = func(n *aetext.Node, v *T) error { return decodeFields(td, unsafe.Pointer(v), n) }
		}
		if enc == nil {
			enc = func(v *T, n *aetext.Node) { encodeFields(td, unsafe.Pointer(v), n) }
		}
	}
	s.def = func() any {
		v := d.Default()
		return &v
	}
	s.decode = func(n *aetext.Node) (any, error) {
		v := d.Default()
		if err := dec(n, &v); err != nil {
			return nil, err
		}
		return &v, nil
	}
	s.encode = func(v any) *aetext.Node {
		n := &aetext.Node{Head: d.ID}
		if d.Version > 1 {
			n.Attrs = append(n.Attrs, aetext.Field{Name: VersionAttr, Value: aetext.Int(int64(d.Version))})
		}
		enc(v.(*T), n)
		return n
	}
	s.validate = func(v any) error {
		p, ok := v.(*T)
		if !ok || p == nil {
			return sectionErr(d.ID, "value has the wrong type")
		}
		if d.Validate == nil {
			return nil
		}
		return d.Validate(p)
	}
	s.migrate = d.Migrate
	if d.Names != nil {
		s.names = func(v any, list string) []string { return d.Names(v.(*T), list) }
	}
	r.sections = append(r.sections, s)
}

func (s *Section) load(n *aetext.Node) (any, error) {
	if n == nil {
		v := s.def()
		return v, s.validate(v)
	}
	ver := 1
	if a, ok := n.Attr(VersionAttr); ok {
		ver = int(a.AsInt())
	}
	switch {
	case ver > s.Version:
		return nil, sectionErr(s.ID, "written by a newer engine, version "+strconv.Itoa(ver)+", supported "+strconv.Itoa(s.Version))
	case ver < s.Version:
		if s.migrate == nil {
			return nil, sectionErr(s.ID, "version "+strconv.Itoa(ver)+" has no migration to "+strconv.Itoa(s.Version))
		}
		n = n.Clone()
		if err := s.migrate(n, ver, s.Version); err != nil {
			return nil, err
		}
	}
	v, err := s.decode(n)
	if err != nil {
		return nil, err
	}
	return v, s.validate(v)
}

type Sections struct {
	list []*Section
	vals map[string]any
}

func (m *Manifest) LoadSections(block func(head string) *aetext.Node, reserved ...string) (*Sections, error) {
	s := &Sections{vals: map[string]any{}}
	var all []error
	for _, d := range m.Sections {
		if slices.Contains(reserved, d.ID) {
			all = append(all, sectionErr(d.ID, "collides with a core project block"))
			continue
		}
		var n *aetext.Node
		for _, h := range d.Heads() {
			if n = block(h); n != nil {
				break
			}
		}
		v, err := d.load(n)
		if err != nil {
			all = append(all, err)
			continue
		}
		s.list = append(s.list, d)
		s.vals[d.ID] = v
	}
	if len(all) > 0 {
		return nil, errors.Join(all...)
	}
	return s, nil
}

func (s *Sections) List() []*Section {
	if s == nil {
		return nil
	}
	return s.list
}

func (s *Sections) Names(id, list string) []string {
	d, ok := s.Desc(id)
	if !ok || d.names == nil {
		return nil
	}
	return d.names(s.vals[id], list)
}

func (s *Sections) Desc(id string) (*Section, bool) {
	for _, d := range s.List() {
		if d.ID == id {
			return d, true
		}
	}
	return nil, false
}

func (s *Sections) Get(id string) (any, bool) {
	if s == nil {
		return nil, false
	}
	v, ok := s.vals[id]
	return v, ok
}

func SectionValue[T any](s *Sections, id string) (*T, bool) {
	v, ok := s.Get(id)
	if !ok {
		return nil, false
	}
	p, ok := v.(*T)
	return p, ok
}

func (s *Sections) Draft(id string) (any, error) {
	d, ok := s.Desc(id)
	if !ok {
		return nil, sectionErr(id, "no such project section")
	}
	return d.decode(d.encode(s.vals[id]))
}

func (s *Sections) Check(id string, v any) error {
	d, ok := s.Desc(id)
	if !ok {
		return sectionErr(id, "no such project section")
	}
	return d.validate(v)
}

func (s *Sections) Node(id string, v any) (*aetext.Node, error) {
	d, ok := s.Desc(id)
	if !ok {
		return nil, sectionErr(id, "no such project section")
	}
	if err := d.validate(v); err != nil {
		return nil, err
	}
	n := d.encode(v)
	if bytes.Equal(aetext.Write(n), aetext.Write(d.encode(d.def()))) {
		return nil, nil
	}
	return n, nil
}

func (s *Sections) Set(id string, v any) error {
	if err := s.Check(id, v); err != nil {
		return err
	}
	s.vals[id] = v
	return nil
}

func (s *Sections) Copy(id string, v any) (any, error) {
	d, ok := s.Desc(id)
	if !ok {
		return nil, sectionErr(id, "no such project section")
	}
	return d.decode(d.encode(v))
}

func (s *Sections) Same(id string, a, b any) bool {
	d, ok := s.Desc(id)
	if !ok {
		return false
	}
	return bytes.Equal(aetext.Write(d.encode(a)), aetext.Write(d.encode(b)))
}

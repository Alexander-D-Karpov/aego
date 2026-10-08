package components

import (
	"unsafe"

	"aego/format/aetext"
	"aego/scene"
	"aego/schema"
)

const ScriptName = "engine.Script"

var ScriptTypeID = schema.HashName(ScriptName)

func ScriptsOf(s *scene.Scene, h scene.NodeHandle) (*ScriptComponent, bool) {
	p, ok := s.GetByType(h, ScriptTypeID)
	if !ok {
		return nil, false
	}
	return (*ScriptComponent)(p), true
}

type Attachment struct {
	TypeName string
	Enabled  bool
	Props    schema.PropertyBag
	Unknown  []aetext.Field
}

func (a *Attachment) Clone() Attachment {
	out := *a
	out.Props = a.Props.Clone()
	out.Unknown = append([]aetext.Field(nil), a.Unknown...)
	return out
}

func (a *Attachment) Is(td *schema.TypeDescriptor) bool {
	if a.TypeName == td.FullName {
		return true
	}
	for _, f := range td.Former {
		if a.TypeName == f {
			return true
		}
	}
	return false
}

type ScriptComponent struct {
	Attachments []Attachment `aego:"-"`
}

func (c *ScriptComponent) Index(name string) int {
	for i := range c.Attachments {
		if c.Attachments[i].TypeName == name {
			return i
		}
	}
	return -1
}

func (c *ScriptComponent) Find(td *schema.TypeDescriptor) *Attachment {
	for i := range c.Attachments {
		if c.Attachments[i].Is(td) {
			return &c.Attachments[i]
		}
	}
	return nil
}

type scriptOverlay struct{}

func (scriptOverlay) Bag(s *scene.Scene, h scene.NodeHandle, td *schema.TypeDescriptor) (unsafe.Pointer, bool) {
	sc, ok := ScriptsOf(s, h)
	if !ok {
		return nil, false
	}
	a := sc.Find(td)
	if a == nil {
		return nil, false
	}
	return unsafe.Pointer(&a.Props), true
}

func registerScripts(t *scene.Types) error {
	if err := scene.RegisterComponent[ScriptComponent](t,
		schema.Name(ScriptName),
		schema.Doc("holds the Aego Script attachments of a node")); err != nil {
		return err
	}
	t.SetOverlay(scriptOverlay{})
	return nil
}

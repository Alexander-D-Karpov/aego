package clipboard

import (
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

type compData struct {
	td     *schema.TypeDescriptor
	values []commands.FieldValue
}

func (e Env) pasteInto(tx *history.Transaction, d *document.SceneDocument, nodes []ids.NodeID, td *schema.TypeDescriptor, values []commands.FieldValue) error {
	s := d.Scene()
	for _, n := range nodes {
		h := s.Handle(n)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		if _, has := s.GetByType(h, td.ID); !has {
			if !s.Accepts(h, td.ID) {
				continue
			}
			if err := tx.Exec(&commands.AddComponent{Node: n, Type: td.ID, Name: td.FullName, Values: values, OnAdd: e.onAdd(td)}); err != nil {
				return err
			}
			continue
		}
		for _, fv := range values {
			fd, ok := td.FieldByID(fv.Field)
			if !ok || fd.Flags.Has(schema.FlagReadOnly) {
				continue
			}
			if err := d.Props.Set(tx, []ids.NodeID{n}, td.ID, fv.Field, fv.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e Env) EncodeComponent(s *scene.Scene, node ids.NodeID, t schema.TypeID) ([]byte, error) {
	td, ok := s.Registry().ByID(t)
	if !ok {
		return nil, errs.New(errs.SchemaUnknownType, "component type is not registered")
	}
	values, err := componentValues(s, node, td)
	if err != nil {
		return nil, err
	}
	return e.encodeComponent(td, values)
}

func (e Env) ApplyComponentData(d *document.SceneDocument, nodes []ids.NodeID, data []byte, label string) error {
	td, values, err := e.decodeComponent(data)
	if err != nil {
		return err
	}
	tx := d.History.Begin(label)
	if err := e.pasteInto(tx, d, nodes, td, values); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
}

func (e Env) CopyComponents(s *scene.Scene, node ids.NodeID, types []schema.TypeID) error {
	if e.Clip == nil {
		return errs.New(errs.EditorCommand, "clipboard is unavailable")
	}
	if len(types) == 0 {
		return errs.New(errs.EditorCommand, "the node has no components to copy")
	}
	sc := e.scratch()
	h := sc.Create(sc.Root(), "components")
	sc.Flush()
	for _, t := range types {
		td, ok := s.Registry().ByID(t)
		if !ok {
			continue
		}
		values, err := componentValues(s, node, td)
		if err != nil {
			return err
		}
		add := &commands.AddComponent{Node: sc.ID(h), Type: td.ID, Name: td.FullName, Values: values}
		if err := add.Do(target{sc}); err != nil {
			return err
		}
	}
	data := sceneio.SaveSubtree(sc, h, true)
	e.Clip.SetData(MIMEComponents, data, componentsText+string(data))
	return nil
}

func (e Env) readComponents() ([]compData, bool) {
	c := e.Clip
	if c == nil {
		return nil, false
	}
	data, ok := c.Data(MIMEComponents)
	if !ok || len(data) == 0 {
		t := c.Text()
		if !strings.HasPrefix(t, componentsText) {
			return nil, false
		}
		data = []byte(strings.TrimPrefix(t, componentsText))
	}
	sc := e.scratch()
	cr := &commands.Create{Index: -1, Fragment: data}
	if err := cr.Do(target{sc}); err != nil || len(cr.Roots()) == 0 {
		return nil, false
	}
	root := cr.Roots()[0]
	reg := sc.Registry()
	unknown, hasUnknown := reg.ByName("engine.Unknown")
	var out []compData
	var fail bool
	sc.ComponentsOf(sc.Handle(root), func(t schema.TypeID) {
		if fail || (hasUnknown && t == unknown.ID) || t == scriptcomp.ScriptTypeID {
			return
		}
		td, ok := reg.ByID(t)
		if !ok || td.Hidden {
			return
		}
		values, err := componentValues(sc, root, td)
		if err != nil {
			fail = true
			return
		}
		out = append(out, compData{td: td, values: values})
	})
	return out, !fail && len(out) > 0
}

func (e Env) PasteComponents(d *document.SceneDocument, nodes []ids.NodeID) error {
	list, ok := e.readComponents()
	if !ok {
		return errs.New(errs.EditorCommand, "clipboard does not hold components")
	}
	if len(nodes) == 0 {
		return errs.New(errs.EditorCommand, "select the nodes to paste the components onto")
	}
	tx := d.History.Begin("Paste Components")
	for _, c := range list {
		if err := e.pasteInto(tx, d, nodes, c.td, c.values); err != nil {
			tx.Abort()
			return err
		}
	}
	tx.Commit()
	return nil
}

func (e Env) EncodeScript(s *scene.Scene, node ids.NodeID, name string) ([]byte, error) {
	sc, ok := scriptcomp.ScriptsOf(s, s.Handle(node))
	i := -1
	if ok {
		i = sc.Index(name)
	}
	if i < 0 {
		return nil, errs.New(errs.EditorCommand, "the node has no script "+name)
	}
	scr := e.scratch()
	h := scr.Create(scr.Root(), "preset")
	scr.Flush()
	bag := schema.PropertyBag{Props: slices.Clone(sc.Attachments[i].Props.Props)}
	add := &commands.AddScript{Node: scr.ID(h), TypeName: name, Props: bag}
	if err := add.Do(target{scr}); err != nil {
		return nil, err
	}
	return sceneio.SaveSubtree(scr, h, true), nil
}

func (e Env) decodeScript(data []byte) (string, schema.PropertyBag, error) {
	sc := e.scratch()
	cr := &commands.Create{Index: -1, Fragment: data}
	if err := cr.Do(target{sc}); err != nil {
		return "", schema.PropertyBag{}, err
	}
	if len(cr.Roots()) == 0 {
		return "", schema.PropertyBag{}, errs.New(errs.EditorCommand, "the preset is empty")
	}
	scs, ok := scriptcomp.ScriptsOf(sc, sc.Handle(cr.Roots()[0]))
	if !ok || len(scs.Attachments) == 0 {
		return "", schema.PropertyBag{}, errs.New(errs.EditorCommand, "the preset holds no script")
	}
	a := scs.Attachments[0]
	return a.TypeName, a.Props, nil
}

func (e Env) ApplyScriptData(d *document.SceneDocument, nodes []ids.NodeID, data []byte, label string) error {
	name, bag, err := e.decodeScript(data)
	if err != nil {
		return err
	}
	s := d.Scene()
	td, ok := s.Registry().ByName(name)
	if !ok || td.Kind != schema.TypeScript {
		return errs.New(errs.EditorCommand, "script "+name+" is not compiled, the preset cannot be applied")
	}
	tx := d.History.Begin(label)
	for _, n := range nodes {
		h := s.Handle(n)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		if scs, ok := scriptcomp.ScriptsOf(s, h); !ok || scs.Index(name) < 0 {
			if err := tx.Exec(&commands.AddScript{Node: n, TypeName: name, Props: schema.PropertyBag{Props: slices.Clone(bag.Props)}}); err != nil {
				tx.Abort()
				return err
			}
			continue
		}
		for _, pr := range bag.Props {
			fd, ok := td.Field(pr.Name)
			if !ok || fd.Flags.Has(schema.FlagReadOnly) {
				continue
			}
			if err := d.Props.Set(tx, []ids.NodeID{n}, td.ID, fd.ID, pr.Value); err != nil {
				tx.Abort()
				return err
			}
		}
	}
	tx.Commit()
	return nil
}

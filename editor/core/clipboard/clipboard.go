package clipboard

import (
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
)

const MIME = "application/x-aego-fragment"

func TopLevel(s *scene.Scene, nodes []ids.NodeID) []scene.NodeHandle {
	root := s.Root()
	set := make(map[scene.NodeHandle]bool, len(nodes))
	for _, id := range nodes {
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) && h != root {
			set[h] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	var out []scene.NodeHandle
	for _, h := range s.PreOrder() {
		if !set[h] {
			continue
		}
		nested := false
		for q := s.Parent(h); !q.IsZero(); q = s.Parent(q) {
			if set[q] {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, h)
		}
	}
	return out
}

func Fragment(s *scene.Scene, nodes []ids.NodeID) ([]byte, []scene.NodeHandle, error) {
	tops := TopLevel(s, nodes)
	if len(tops) == 0 {
		return nil, nil, errs.New(errs.EditorCommand, "nothing is selected")
	}
	root := &aetext.Node{Head: "prefab"}
	root.Attrs = append(root.Attrs, aetext.Field{Name: "version", Value: aetext.Int(sceneio.FormatVersion)})
	for _, h := range tops {
		doc, err := aetext.Parse(sceneio.SaveSubtree(s, h, true), "fragment")
		if err != nil {
			return nil, nil, err
		}
		root.Blocks = append(root.Blocks, doc.Blocks...)
	}
	return aetext.Write(root), tops, nil
}

func Valid(data []byte) bool {
	doc, err := aetext.Parse(data, "fragment")
	return err == nil && (doc.Head == "prefab" || doc.Head == "scene")
}

func remapKey(s string, mapping map[string]string) string {
	parts := strings.Split(s, "/")
	changed := false
	for i, p := range parts {
		if fresh, ok := mapping[p]; ok {
			parts[i] = fresh
			changed = true
		}
	}
	if !changed {
		return s
	}
	return strings.Join(parts, "/")
}

func Remap(data []byte) ([]byte, error) {
	doc, err := aetext.Parse(data, "fragment")
	if err != nil {
		return nil, err
	}
	if doc.Head != "prefab" && doc.Head != "scene" {
		return nil, errs.New(errs.EditorCommand, "clipboard does not hold scene nodes", log.F("head", doc.Head))
	}
	mapping := map[string]string{}
	var collect func(n *aetext.Node)
	collect = func(n *aetext.Node) {
		switch n.Head {
		case "node":
			if n.Label != "" {
				mapping[n.Label] = ids.NewNodeID().String()
			}
		case "id":
			if v, ok := n.Attr("node"); ok && v.Str != "" {
				mapping[v.Str] = ids.NewNodeID().String()
			}
		}
		for _, b := range n.Blocks {
			collect(b)
		}
	}
	collect(doc)
	var rewrite func(n *aetext.Node)
	rewrite = func(n *aetext.Node) {
		switch n.Head {
		case "node":
			if fresh, ok := mapping[n.Label]; ok {
				n.Label = fresh
			}
		case "override", "addComponent", "removeComponent", "addNode", "remove", "order":
			n.Label = remapKey(n.Label, mapping)
		}
		for i := range n.Attrs {
			if n.Head == "order" && n.Attrs[i].Name == "children" {
				for j := range n.Attrs[i].Value.List {
					item := &n.Attrs[i].Value.List[j]
					item.Str = remapKey(item.Str, mapping)
				}
				continue
			}
			remapValue(&n.Attrs[i].Value, mapping)
		}
		for i := range n.Fields {
			remapValue(&n.Fields[i].Value, mapping)
		}
		for _, b := range n.Blocks {
			rewrite(b)
		}
	}
	rewrite(doc)
	return aetext.Write(doc), nil
}

func remapValue(v *aetext.Value, mapping map[string]string) {
	switch v.Kind {
	case aetext.VNode:
		if fresh, ok := mapping[v.Str]; ok {
			v.Str = fresh
		}
	case aetext.VList:
		for i := range v.List {
			remapValue(&v.List[i], mapping)
		}
	case aetext.VStruct:
		for i := range v.Obj {
			remapValue(&v.Obj[i].Value, mapping)
		}
	}
}

func Copy(ctx *context.Editor) error {
	s := ctx.Scene()
	if s == nil || ctx.Clipboard == nil {
		return nil
	}
	data, _, err := Fragment(s, ctx.Selected())
	if err != nil {
		return err
	}
	ctx.Clipboard.SetData(MIME, data, string(data))
	return nil
}

func Cut(ctx *context.Editor) error {
	if err := Copy(ctx); err != nil {
		return err
	}
	return Delete(ctx)
}

func clipboardData(ctx *context.Editor) ([]byte, error) {
	if ctx.Clipboard == nil {
		return nil, errs.New(errs.EditorCommand, "clipboard is unavailable")
	}
	if data, ok := ctx.Clipboard.Data(MIME); ok && len(data) > 0 {
		return data, nil
	}
	text := strings.TrimSpace(ctx.Clipboard.Text())
	if strings.HasPrefix(text, "prefab") && Valid([]byte(text)) {
		return []byte(text), nil
	}
	return nil, errs.New(errs.EditorCommand, "clipboard does not hold scene nodes")
}

func pasteTarget(ctx *context.Editor) (ids.NodeID, int) {
	d := ctx.Doc()
	s := d.Scene()
	it, ok := ctx.Selection.Active()
	if !ok || it.Doc != d.ID {
		return ids.NodeID{}, -1
	}
	h := s.Handle(it.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return ids.NodeID{}, -1
	}
	return s.ID(s.Parent(h)), commands.ChildIndex(s, h) + 1
}

func Paste(ctx *context.Editor) error {
	d := ctx.Doc()
	if d == nil {
		return nil
	}
	switch Peek(ctx) {
	case KindComponent:
		nodes := append([]ids.NodeID(nil), ctx.Selected()...)
		if len(nodes) == 0 {
			ctx.Notify(context.LevelInfo, i18n.Note("editor.clipboard.select_the_nodes_to_paste", "Select the nodes to paste the component onto"))
			return nil
		}
		return PasteComponent(ctx, d, nodes)
	case KindComponents:
		nodes := append([]ids.NodeID(nil), ctx.Selected()...)
		if len(nodes) == 0 {
			ctx.Notify(context.LevelInfo, i18n.Note("editor.clipboard.select_the_nodes_to_paste_2", "Select the nodes to paste the components onto"))
			return nil
		}
		return EnvOf(ctx).PasteComponents(d, nodes)
	case KindNodes:
	default:
		ctx.Notify(context.LevelInfo, i18n.Note("editor.clipboard.the_clipboard_holds_nothing_that", "The clipboard holds nothing that can be pasted into the scene"))
		return nil
	}
	data, err := clipboardData(ctx)
	if err != nil {
		return err
	}
	data, err = Remap(data)
	if err != nil {
		return err
	}
	frags, err := commands.Fragments(data)
	if err != nil {
		return err
	}
	parent, index := pasteTarget(ctx)
	ops := make([]mutate.Op, len(frags))
	for i, f := range frags {
		at := -1
		if index >= 0 {
			at = index + i
		}
		ops[i] = commands.Insert(parent, at, f)
	}
	cmd := commands.NewMutation(i18n.Tr("editor.history.paste", "Paste"), commands.EnvFor(d.Scene()), ops...)
	if err := d.History.Exec(cmd); err != nil {
		return err
	}
	ctx.Select(cmd.Created()...)
	return nil
}

func Duplicate(ctx *context.Editor) error {
	d := ctx.Doc()
	if d == nil {
		return nil
	}
	s := d.Scene()
	tops := TopLevel(s, ctx.Selected())
	if len(tops) == 0 {
		return nil
	}
	tx := d.History.Begin(i18n.Tr("editor.history.duplicate", "Duplicate"))
	var created []ids.NodeID
	for _, h := range tops {
		data, _, err := Fragment(s, []ids.NodeID{s.ID(h)})
		if err == nil {
			data, err = Remap(data)
		}
		if err != nil {
			tx.Abort()
			return err
		}
		c := commands.NewMutation(i18n.Tr("editor.history.create", "Create {name}", i18n.Str("name", s.Name(h))), commands.EnvFor(s),
			commands.Insert(s.ID(s.Parent(h)), commands.ChildIndex(s, h)+1, string(data)))
		if err := tx.Exec(c); err != nil {
			tx.Abort()
			return err
		}
		created = append(created, c.Created()...)
	}
	tx.Commit()
	ctx.Select(created...)
	return nil
}

func Delete(ctx *context.Editor) error {
	d := ctx.Doc()
	if d == nil {
		return nil
	}
	s := d.Scene()
	tops := TopLevel(s, ctx.Selected())
	if len(tops) == 0 {
		return nil
	}
	ops := make([]mutate.Op, len(tops))
	for i, h := range tops {
		ops[i] = mutate.RemoveNode(commands.Ref(s.ID(h)))
	}
	if err := d.History.Exec(commands.NewMutation(i18n.Tr("editor.history.delete", "Delete"), commands.EnvFor(s), ops...)); err != nil {
		return err
	}
	ctx.Select()
	return nil
}

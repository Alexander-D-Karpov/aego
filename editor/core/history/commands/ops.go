package commands

import (
	"strings"

	"aego/core/ids"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

func WithOnAdd(env mutate.Env, fn func(*schema.TypeDescriptor) func(*scene.Scene, scene.NodeHandle)) mutate.Env {
	if fn == nil {
		return env
	}
	env.OnAdd = func(td *schema.TypeDescriptor, s *scene.Scene, h scene.NodeHandle) {
		if f := fn(td); f != nil {
			f(s, h)
		}
	}
	return env
}

func values(reg *schema.Registry, td *schema.TypeDescriptor, list []FieldValue, readOnly bool) *aetext.Node {
	if len(list) == 0 {
		return nil
	}
	n := &aetext.Node{Head: "values"}
	for _, fv := range list {
		fd, ok := td.FieldByID(fv.Field)
		if !ok || !fd.Flags.Has(schema.FlagSerialize) || (!readOnly && fd.Flags.Has(schema.FlagReadOnly)) {
			continue
		}
		n.Fields = append(n.Fields, aetext.Field{Name: fd.Name, Value: sceneio.WriteValue(reg, fd, fv.Value)})
	}
	return n
}

func AddComp(s *scene.Scene, node ids.NodeID, td *schema.TypeDescriptor, list []FieldValue) mutate.Op {
	op := mutate.AddComp(Ref(node), td.FullName)
	op.Values = values(s.Registry(), td, list, false)
	return op
}

func AddRaw(s *scene.Scene, node ids.NodeID, td *schema.TypeDescriptor, list []FieldValue) mutate.Op {
	op := mutate.AddComp(Ref(node), td.FullName)
	op.Raw = true
	op.Values = values(s.Registry(), td, list, true)
	return op
}

func RemoveComp(node ids.NodeID, td *schema.TypeDescriptor) mutate.Op {
	return mutate.RemoveComp(Ref(node), td.FullName)
}

func Insert(parent ids.NodeID, index int, fragment string) mutate.Op {
	op := mutate.Insert(Ref(parent), fragment)
	if index >= 0 {
		op = op.At(index)
	}
	return op
}

func Move(node, parent ids.NodeID, index int, keepWorld bool) mutate.Op {
	op := mutate.Move(Ref(node), Ref(parent))
	if index >= 0 {
		op = op.At(index)
	}
	if keepWorld {
		op = op.KeepingWorld()
	}
	return op
}

func Fragments(data []byte) ([]string, error) {
	doc, err := aetext.Parse(data, "fragment")
	if err != nil {
		return nil, err
	}
	var nodes, rest []*aetext.Node
	for _, b := range doc.Blocks {
		if b.Head == "node" {
			nodes = append(nodes, b)
		} else {
			rest = append(rest, b)
		}
	}
	if len(nodes) <= 1 {
		return []string{string(data)}, nil
	}
	out := make([]string, len(nodes))
	for i, n := range nodes {
		one := &aetext.Node{Head: doc.Head, Label: doc.Label, Attrs: doc.Attrs, Fields: doc.Fields}
		one.Blocks = append(append(one.Blocks, rest...), n)
		out[i] = string(aetext.Write(one))
	}
	return out, nil
}

func (c *Mutation) Created() []ids.NodeID {
	if c.res == nil {
		return nil
	}
	out := make([]ids.NodeID, 0, len(c.res.Created))
	for _, r := range c.res.Created {
		if id, err := ids.ParseNodeID(strings.TrimPrefix(r, "#")); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func (c *Mutation) Changes() []mutate.Change {
	if c.res == nil {
		return nil
	}
	return c.res.Changes
}

package sceneio

import (
	"aego/format/aetext"
	"aego/scene/prefab"
)

type OutlineField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Kind  string `json:"kind,omitempty"`
}

type OutlineBlock struct {
	Kind   string         `json:"kind"`
	Type   string         `json:"type,omitempty"`
	Fields []OutlineField `json:"fields,omitempty"`
}

type OutlineNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Blocks   []OutlineBlock `json:"blocks,omitempty"`
	Children []OutlineNode  `json:"children,omitempty"`
}

// Outline reads a scene file without resolving types, for read-only views of it.
func Outline(src []byte) ([]OutlineNode, error) {
	root, err := aetext.Parse(src, "scene.aescene")
	if err != nil {
		return nil, err
	}
	return outlineNodes(root.Blocks), nil
}

// OutlineKey names one field across outlines so a changed value can be matched to its row.
func OutlineKey(node string, b OutlineBlock, field string) string {
	return node + "/" + b.Kind + ":" + b.Type + "/" + field
}

func OutlineValues(nodes []OutlineNode) map[string]string {
	out := map[string]string{}
	var walk func([]OutlineNode)
	walk = func(list []OutlineNode) {
		for _, n := range list {
			for _, b := range n.Blocks {
				for _, f := range b.Fields {
					out[OutlineKey(n.ID, b, f.Name)] = f.Value
				}
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

func outlineNodes(blocks []*aetext.Node) []OutlineNode {
	var out []OutlineNode
	for _, n := range blocks {
		if n.Head != "node" {
			continue
		}
		on := OutlineNode{ID: n.Label, Enabled: true}
		for _, a := range n.Attrs {
			switch a.Name {
			case prefab.FieldName:
				on.Name = a.Value.Str
			case prefab.FieldEnabled:
				on.Enabled = a.Value.Bool
			}
		}
		for _, b := range n.Blocks {
			if b.Head == "node" {
				continue
			}
			ob := OutlineBlock{Kind: b.Head, Type: b.Label}
			for _, f := range b.Fields {
				ob.Fields = append(ob.Fields, OutlineField{Name: f.Name, Value: aetext.FormatValue(f.Value), Kind: valueKind(f.Value)})
			}
			on.Blocks = append(on.Blocks, ob)
		}
		on.Children = outlineNodes(n.Blocks)
		out = append(out, on)
	}
	return out
}

func valueKind(v aetext.Value) string {
	switch v.Kind {
	case aetext.VColor:
		return "color"
	case aetext.VAsset:
		return "asset"
	case aetext.VNode:
		return "node"
	case aetext.VBool:
		return "bool"
	case aetext.VString:
		return "string"
	case aetext.VIdent:
		return "enum"
	}
	return ""
}

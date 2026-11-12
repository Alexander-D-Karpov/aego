package audiosidecar

import (
	"errors"
	"strconv"

	"aego/format/aetext"
)

// writeSize mirrors aetext.Write's byte count so deeply indented or escaped
// untrusted metadata is rejected before the writer allocates its output.
func writeSize(root *aetext.Node, limit int64) (int64, error) {
	b := outputBudget{limit: limit}
	if err := b.node(root, 0); err != nil {
		return 0, err
	}
	return b.used, nil
}

type outputBudget struct {
	used  int64
	limit int64
}

func (b *outputBudget) add(n int64) error {
	if n < 0 || n > b.limit-b.used {
		return errors.New("audio sidecar output exceeds metadata budget")
	}
	b.used += n
	return nil
}

func (b *outputBudget) quoted(s string) error {
	if err := b.add(2); err != nil {
		return err
	}
	for i := 0; i < len(s); i++ {
		n := int64(1)
		switch c := s[i]; {
		case c == '"' || c == '\\' || c == '\n' || c == '\t':
			n = 2
		case c < 0x20:
			n = 6
		}
		if err := b.add(n); err != nil {
			return err
		}
	}
	return nil
}

func (b *outputBudget) node(n *aetext.Node, depth int) error {
	if n == nil || depth > maxNesting {
		return errors.New("invalid audio sidecar output nesting")
	}
	if err := b.add(int64(2*depth + len(n.Head))); err != nil {
		return err
	}
	if n.Label != "" {
		if err := b.add(1); err != nil {
			return err
		}
		if err := b.quoted(n.Label); err != nil {
			return err
		}
	}
	for _, a := range n.Attrs {
		if err := b.add(int64(len(a.Name) + 2)); err != nil {
			return err
		}
		if err := b.value(a.Value, depth+1); err != nil {
			return err
		}
	}
	if len(n.Fields) == 0 && len(n.Blocks) == 0 {
		return b.add(4) // " {}\n"
	}
	if err := b.add(3); err != nil { // " {\n"
		return err
	}
	for _, field := range n.Fields {
		if err := b.add(int64(2*(depth+1) + len(field.Name) + 2)); err != nil {
			return err
		}
		if err := b.value(field.Value, depth+1); err != nil {
			return err
		}
	}
	for _, block := range n.Blocks {
		if err := b.node(block, depth+1); err != nil {
			return err
		}
	}
	return b.add(int64(2*depth + 2)) // indentation and "}\n"
}

func (b *outputBudget) value(v aetext.Value, depth int) error {
	if depth > maxNesting {
		return errors.New("invalid audio sidecar value nesting")
	}
	switch v.Kind {
	case aetext.VInt:
		return b.add(int64(len(strconv.FormatInt(v.Int, 10))))
	case aetext.VFloat:
		bits := 64
		if v.Single {
			bits = 32
		}
		return b.add(int64(len(strconv.FormatFloat(v.Float, 'g', -1, bits))))
	case aetext.VBool:
		if v.Bool {
			return b.add(4)
		}
		return b.add(5)
	case aetext.VString:
		return b.quoted(v.Str)
	case aetext.VColor:
		return b.add(9)
	case aetext.VIdent:
		return b.add(int64(len(v.Ident)))
	case aetext.VAsset, aetext.VNode:
		prefix := int64(7) // asset( and )
		if v.Kind == aetext.VNode {
			prefix = 6
		}
		if err := b.add(prefix); err != nil {
			return err
		}
		return b.quoted(v.Str)
	case aetext.VList:
		if err := b.add(2); err != nil {
			return err
		}
		for i, item := range v.List {
			if i > 0 {
				if err := b.add(2); err != nil {
					return err
				}
			}
			if err := b.value(item, depth+1); err != nil {
				return err
			}
		}
		return nil
	case aetext.VStruct:
		if len(v.Obj) == 0 {
			return b.add(2)
		}
		if err := b.add(4); err != nil { // "{ " and " }"
			return err
		}
		for i, field := range v.Obj {
			extra := 1 // '='
			if i > 0 {
				extra++
			}
			if err := b.add(int64(len(field.Name) + extra)); err != nil {
				return err
			}
			if err := b.value(field.Value, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return b.add(5) // aetext's invalid-value fallback is false.
	}
}

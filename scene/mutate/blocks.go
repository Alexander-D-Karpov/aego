package mutate

import (
	"slices"
	"strconv"

	"aego/core/errs"
	"aego/format/aetext"
)

type blocks struct {
	root   *aetext.Node
	splice string
}

func cloneBlocks(bs []*aetext.Node) []*aetext.Node {
	out := make([]*aetext.Node, len(bs))
	for i, b := range bs {
		out[i] = b.Clone()
	}
	return out
}

func spliceOp(kind string, at, n int, put []*aetext.Node) Op {
	return Op{Kind: kind, Index: at, HasIndex: true, Field: strconv.Itoa(n), Values: &aetext.Node{Head: "values", Blocks: cloneBlocks(put)}}
}

func (b *blocks) replace(at, n int, put []*aetext.Node, ch Change) (step, error) {
	if at < 0 || n < 0 || at+n > len(b.root.Blocks) {
		return step{}, errs.New(errs.EditorCommand, "block range is outside the file")
	}
	old := cloneBlocks(b.root.Blocks[at : at+n])
	b.root.Blocks = slices.Replace(b.root.Blocks, at, at+n, cloneBlocks(put)...)
	return step{inv: spliceOp(b.splice, at, len(put), old), redo: spliceOp(b.splice, at, n, put), change: ch}, nil
}

func (b *blocks) applySplice(op Op) (step, error) {
	n, err := strconv.Atoi(op.Field)
	if err != nil || !op.HasIndex {
		return step{}, errs.New(errs.EditorCommand, op.Kind+" needs an index and a count")
	}
	var put []*aetext.Node
	if op.Values != nil {
		put = op.Values.Blocks
	}
	return b.replace(op.Index, n, put, Change{Op: op.Kind})
}

func (b *blocks) first(head string) int {
	for i, n := range b.root.Blocks {
		if n.Head == head {
			return i
		}
	}
	return len(b.root.Blocks)
}

func (b *blocks) nth(head string, k int) int {
	if k < 0 {
		return -1
	}
	for i, n := range b.root.Blocks {
		if n.Head != head {
			continue
		}
		if k == 0 {
			return i
		}
		k--
	}
	return -1
}

func attrAt(n *aetext.Node, name string) int {
	for i := range n.Attrs {
		if n.Attrs[i].Name == name {
			return i
		}
	}
	return -1
}

func setAttr(n *aetext.Node, name string, v aetext.Value) {
	if i := attrAt(n, name); i >= 0 {
		n.Attrs[i].Value = v
		return
	}
	n.Attrs = append(n.Attrs, aetext.Field{Name: name, Value: v})
}

func dropAttr(n *aetext.Node, name string) {
	if i := attrAt(n, name); i >= 0 {
		n.Attrs = slices.Delete(n.Attrs, i, i+1)
	}
}

func wordOf(v aetext.Value) string {
	if v.Kind == aetext.VIdent {
		return v.Ident
	}
	return v.Str
}

func (o Op) hasValue() bool {
	return o.HasValue || o.Text != ""
}

func runChecked(d doer, tx Tx, dry bool, problems func() []Problem) (*Result, error) {
	res, err := run(d, tx)
	if err != nil {
		return nil, err
	}
	res.Problems = append(res.Problems, problems()...)
	if dry {
		replay(d, res.Inverse.Ops)
		res.Inverse, res.Redo, res.DryRun = Tx{}, Tx{}, true
	}
	return res, nil
}

func knownKind(k string) bool {
	return sceneKinds[k] || dataKinds[k] || animKinds[k] || ctrlKinds[k]
}

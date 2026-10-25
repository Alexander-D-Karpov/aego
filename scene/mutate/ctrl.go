package mutate

import (
	"strconv"

	"aego/anim"
	"aego/core/errs"
	"aego/core/log"
	"aego/format/aetext"
)

const (
	KindCtrlPut    = "ctrl-put"
	KindCtrlRemove = "ctrl-remove"
	KindCtrlAttr   = "ctrl-attr"
	KindCtrlRoot   = "ctrl-root"
	KindCtrlSplice = "ctrl-splice"
	KindCtrlMove   = "ctrl-move"
)

var ctrlKinds = map[string]bool{
	KindCtrlPut: true, KindCtrlRemove: true, KindCtrlAttr: true, KindCtrlRoot: true, KindCtrlSplice: true, KindCtrlMove: true,
}

var ctrlRank = map[string]int{"param": 0, "state": 1, "transition": 2}

func CtrlPut(comp, label string, body *aetext.Node) Op {
	v := body.Clone()
	if v == nil {
		v = &aetext.Node{}
	}
	v.Head, v.Label = "values", ""
	return Op{Kind: KindCtrlPut, Comp: comp, Node: label, Values: v}
}

func CtrlRemove(comp, label string) Op {
	return Op{Kind: KindCtrlRemove, Comp: comp, Node: label}
}

func CtrlAttr(comp, label, attr string, v aetext.Value) Op {
	return Op{Kind: KindCtrlAttr, Comp: comp, Node: label, Path: attr, Value: v, HasValue: true}
}

func CtrlAttrClear(comp, label, attr string) Op {
	return Op{Kind: KindCtrlAttr, Comp: comp, Node: label, Path: attr}
}

// CtrlMove swaps transition index with the one delta places away; transitions are tried in order.
func CtrlMove(index, delta int) Op {
	return Op{Kind: KindCtrlMove, Comp: "transition", Index: index, HasIndex: true, Value: aetext.Int(int64(delta)), HasValue: true}
}

func CtrlRoot(path string, v aetext.Value) Op {
	return Op{Kind: KindCtrlRoot, Path: path, Value: v, HasValue: true}
}

type CtrlDocument struct {
	Name string
	b    blocks
}

func LoadCtrl(src []byte, name string) (*CtrlDocument, error) {
	root, err := aetext.Parse(src, name)
	if err != nil {
		return nil, err
	}
	if _, _, err := anim.DecodeGraph(root); err != nil {
		return nil, err
	}
	return &CtrlDocument{Name: name, b: blocks{root: root, splice: KindCtrlSplice}}, nil
}

func (d *CtrlDocument) Root() *aetext.Node {
	return d.b.root
}

func (d *CtrlDocument) Bytes() []byte {
	return aetext.Write(d.b.root)
}

func (d *CtrlDocument) Graph() (*anim.Graph, error) {
	g, _, err := anim.DecodeGraph(d.b.root)
	return g, err
}

func (d *CtrlDocument) Apply(tx Tx) (*Result, error) {
	return runChecked(d, tx, false, d.Problems)
}

func (d *CtrlDocument) DryRun(tx Tx) (*Result, error) {
	return runChecked(d, tx, true, d.Problems)
}

func (d *CtrlDocument) Problems() []Problem {
	g, probs, err := anim.DecodeGraph(d.b.root)
	if err != nil {
		return []Problem{{Error: true, Message: err.Error()}}
	}
	if len(probs) == 0 {
		probs = g.Check(nil)
	}
	out := make([]Problem, 0, len(probs))
	for _, p := range probs {
		out = append(out, Problem{Error: true, Message: graphProblem(p)})
	}
	return out
}

func graphProblem(p anim.GraphProblem) string {
	at := ""
	if p.Transition >= 0 {
		at = "transition " + strconv.Itoa(p.Transition+1) + ": "
	}
	switch p.Kind {
	case anim.GraphEmpty:
		return "ANC002: the controller has no states"
	case anim.GraphState:
		if p.Name != "" {
			return "ANC002: " + at + "there is no state " + p.Name
		}
		return "ANC002: " + at + "names a state that does not exist"
	case anim.GraphParam:
		if p.Name != "" {
			return "ANC001: " + at + "there is no parameter " + p.Name
		}
		return "ANC001: " + at + "a condition does not fit the kind of its parameter"
	case anim.GraphSelf:
		return "ANC003: " + at + "a transition into its own state needs exitTime"
	}
	return "ANC005: transitions without duration, conditions or exit time form a loop"
}

func (d *CtrlDocument) do(op Op, _ *Result) (step, error) {
	backup := d.b.root.Clone()
	st, err := d.apply(op)
	if err == nil {
		_, _, err = anim.DecodeGraph(d.b.root)
	}
	if err != nil {
		d.b.root = backup
		return step{}, err
	}
	return st, nil
}

func (d *CtrlDocument) apply(op Op) (step, error) {
	switch op.Kind {
	case KindCtrlPut:
		return d.put(op)
	case KindCtrlRemove:
		i, err := d.locate(op)
		if err != nil {
			return step{}, err
		}
		if i < 0 {
			return step{}, errs.New(errs.EditorProps, "the controller has no "+op.Comp+" "+op.Node)
		}
		return d.b.replace(i, 1, nil, Change{Op: op.Kind, Node: blockName(d.b.root.Blocks[i]), Path: op.Comp})
	case KindCtrlAttr:
		return d.attr(op)
	case KindCtrlRoot:
		return d.rootSet(op)
	case KindCtrlSplice:
		return d.b.applySplice(op)
	case KindCtrlMove:
		return d.move(op)
	}
	return step{}, errs.New(errs.EditorCommand, op.Kind+" does not apply to animation controllers")
}

func blockName(n *aetext.Node) string {
	if n.Head != "transition" {
		return n.Label
	}
	to := ""
	if v, ok := n.Attr("to"); ok {
		to = wordOf(v)
	}
	return n.Label + " → " + to
}

func (d *CtrlDocument) locate(op Op) (int, error) {
	switch op.Comp {
	case "param", "state":
		for i, b := range d.b.root.Blocks {
			if b.Head == op.Comp && b.Label == op.Node {
				return i, nil
			}
		}
		return -1, nil
	case "transition":
		if !op.HasIndex {
			return -1, nil
		}
		if i := d.b.nth("transition", op.Index); i >= 0 {
			return i, nil
		}
		return -1, errs.New(errs.EditorProps, "the controller has no transition "+strconv.Itoa(op.Index+1))
	}
	return -1, errs.New(errs.EditorCommand, "a controller block is a param, a state or a transition", log.F("block", op.Comp))
}

func (d *CtrlDocument) insertAt(comp string) int {
	r := ctrlRank[comp]
	for i, b := range d.b.root.Blocks {
		if rank, ok := ctrlRank[b.Head]; ok && rank > r {
			return i
		}
	}
	return len(d.b.root.Blocks)
}

func (d *CtrlDocument) put(op Op) (step, error) {
	if op.Values == nil {
		return step{}, errs.New(errs.EditorCommand, op.Kind+" needs the block")
	}
	i, err := d.locate(op)
	if err != nil {
		return step{}, err
	}
	n := op.Values.Clone()
	n.Head, n.Label = op.Comp, op.Node
	if i >= 0 && op.Comp == "transition" && op.Node == "" {
		n.Label = d.b.root.Blocks[i].Label
	}
	ch := Change{Op: op.Kind, Node: blockName(n), Path: op.Comp}
	if i >= 0 {
		return d.b.replace(i, 1, []*aetext.Node{n}, ch)
	}
	if n.Label == "" {
		return step{}, errs.New(errs.EditorProps, "a new "+op.Comp+" needs a name")
	}
	return d.b.replace(d.insertAt(op.Comp), 0, []*aetext.Node{n}, ch)
}

func (d *CtrlDocument) attr(op Op) (step, error) {
	i, err := d.locate(op)
	if err != nil {
		return step{}, err
	}
	if i < 0 {
		return step{}, errs.New(errs.EditorProps, "the controller has no "+op.Comp+" "+op.Node)
	}
	n := d.b.root.Blocks[i].Clone()
	ch := Change{Op: op.Kind, Node: blockName(n), Path: op.Path}
	if v, ok := n.Attr(op.Path); ok {
		ch.Before = FormatValue(v)
	}
	if op.hasValue() {
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		setAttr(n, op.Path, v.Clone())
		ch.After = FormatValue(v)
	} else {
		dropAttr(n, op.Path)
	}
	return d.b.replace(i, 1, []*aetext.Node{n}, ch)
}

func (d *CtrlDocument) rootSet(op Op) (step, error) {
	root := d.b.root
	var old aetext.Value
	var had bool
	switch op.Path {
	case "entry":
		old, had = root.Field("entry")
	case "ease", "entryPos", "anyPos":
		old, had = root.Attr(op.Path)
	default:
		return step{}, errs.New(errs.EditorProps, "a controller has entry, ease, entryPos and anyPos", log.F("attr", op.Path))
	}
	inv := Op{Kind: op.Kind, Path: op.Path}
	ch := Change{Op: op.Kind, Path: op.Path}
	if had {
		inv.Value, inv.HasValue = old.Clone(), true
		ch.Before = FormatValue(old)
	}
	redo := Op{Kind: op.Kind, Path: op.Path}
	switch {
	case op.hasValue():
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		if op.Path == "entry" {
			root.SetField("entry", v.Clone())
		} else {
			setAttr(root, op.Path, v.Clone())
		}
		ch.After = FormatValue(v)
		redo.Value, redo.HasValue = v.Clone(), true
	case !had:
		return step{}, ErrNothing
	case op.Path == "entry":
		root.RemoveField("entry")
	default:
		dropAttr(root, op.Path)
	}
	return step{inv: inv, redo: redo, change: ch}, nil
}

func (d *CtrlDocument) move(op Op) (step, error) {
	v, err := op.value()
	if err != nil {
		return step{}, err
	}
	i := d.b.nth("transition", op.Index)
	j := d.b.nth("transition", op.Index+int(v.AsInt()))
	if i < 0 || j < 0 {
		return step{}, ErrNothing
	}
	lo, hi := min(i, j), max(i, j)
	put := cloneBlocks(d.b.root.Blocks[lo : hi+1])
	put[0], put[len(put)-1] = put[len(put)-1], put[0]
	return d.b.replace(lo, hi-lo+1, put, Change{Op: op.Kind, Node: blockName(d.b.root.Blocks[i]), Path: "transition"})
}

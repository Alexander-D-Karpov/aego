package mutate

import "aego/format/aetext"

const (
	KindAddNode           = "add-node"
	KindRemoveNode        = "remove-node"
	KindRestoreNode       = "restore-node"
	KindInsert            = "insert"
	KindReplace           = "replace"
	KindMove              = "move"
	KindRename            = "rename"
	KindAddComp           = "add-comp"
	KindRemoveComp        = "remove-comp"
	KindSet               = "set"
	KindReset             = "reset"
	KindPrefabInstantiate = "prefab-instantiate"
	KindPrefabRevert      = "prefab-revert"
	KindPrefabUnpack      = "prefab-unpack"
	KindDataSet           = "data-set"
	KindDataReset         = "data-reset"
	KindDataBase          = "data-base"
	KindRowAdd            = "row-add"
	KindRowRemove         = "row-remove"
	KindRowRestore        = "row-restore"
	KindRowRename         = "row-rename"
)

var sceneKinds = map[string]bool{
	KindAddNode: true, KindRemoveNode: true, KindRestoreNode: true, KindInsert: true, KindReplace: true,
	KindMove: true, KindRename: true, KindAddComp: true, KindRemoveComp: true, KindSet: true, KindReset: true,
	KindPrefabInstantiate: true, KindPrefabRevert: true, KindPrefabUnpack: true,
}

var dataKinds = map[string]bool{
	KindDataSet: true, KindDataReset: true, KindDataBase: true, KindRowAdd: true, KindRowRemove: true, KindRowRestore: true, KindRowRename: true,
}

type Op struct {
	Kind      string
	Node      string
	Parent    string
	Name      string
	ID        string
	Space     string
	Comp      string
	Path      string
	Text      string
	Value     aetext.Value
	HasValue  bool
	Index     int
	HasIndex  bool
	Raw       bool
	Live      bool
	KeepWorld bool
	Deep      bool
	Prefab    string
	Asset     string
	Level     string
	Key       string
	Owner     string
	Field     string
	Fragment  string
	Values    *aetext.Node
}

func (o Op) At(index int) Op {
	o.Index, o.HasIndex = index, true
	return o
}

func (o Op) WithID(id string) Op {
	o.ID = id
	return o
}

func (o Op) In(space string) Op {
	o.Space = space
	return o
}

func (o Op) KeepingWorld() Op {
	o.KeepWorld = true
	return o
}

func (o Op) Runtime() Op {
	o.Live = true
	return o
}

func AddNode(parent, name string) Op {
	return Op{Kind: KindAddNode, Parent: parent, Name: name}
}

func RemoveNode(node string) Op {
	return Op{Kind: KindRemoveNode, Node: node}
}

func Insert(parent, fragment string) Op {
	return Op{Kind: KindInsert, Parent: parent, Fragment: fragment}
}

func Replace(node, fragment string) Op {
	return Op{Kind: KindReplace, Node: node, Fragment: fragment}
}

func Move(node, parent string) Op {
	return Op{Kind: KindMove, Node: node, Parent: parent}
}

func Rename(node, name string) Op {
	return Op{Kind: KindRename, Node: node, Name: name}
}

func AddComp(node, comp string) Op {
	return Op{Kind: KindAddComp, Node: node, Comp: comp}
}

func RemoveComp(node, comp string) Op {
	return Op{Kind: KindRemoveComp, Node: node, Comp: comp}
}

func Set(node, path, text string) Op {
	return Op{Kind: KindSet, Node: node, Path: path, Text: text}
}

func SetValue(node, path string, v aetext.Value) Op {
	return Op{Kind: KindSet, Node: node, Path: path, Value: v, HasValue: true}
}

func Reset(node, path string) Op {
	return Op{Kind: KindReset, Node: node, Path: path}
}

func Instantiate(parent, prefab string) Op {
	return Op{Kind: KindPrefabInstantiate, Parent: parent, Prefab: prefab}
}

func RevertPrefab(node, level string) Op {
	return Op{Kind: KindPrefabRevert, Node: node, Level: level}
}

func Unpack(node string, deep bool) Op {
	return Op{Kind: KindPrefabUnpack, Node: node, Deep: deep}
}

func DataSet(row, path, text string) Op {
	return Op{Kind: KindDataSet, Node: row, Path: path, Text: text}
}

func DataReset(row, path string) Op {
	return Op{Kind: KindDataReset, Node: row, Path: path}
}

func DataSetValue(row, path string, v aetext.Value) Op {
	return Op{Kind: KindDataSet, Node: row, Path: path, Value: v, HasValue: true}
}

func DataBase(asset string) Op {
	return Op{Kind: KindDataBase, Asset: asset}
}

func RowRestore(id string, fields []aetext.Field) Op {
	return Op{Kind: KindRowRestore, Node: id, Values: &aetext.Node{Head: "values", Fields: cloneFields(fields)}}
}

func RowAdd(id string) Op {
	return Op{Kind: KindRowAdd, Node: id}
}

func RowRemove(id string) Op {
	return Op{Kind: KindRowRemove, Node: id}
}

func RowRename(id, name string) Op {
	return Op{Kind: KindRowRename, Node: id, Name: name}
}

func (o Op) value() (aetext.Value, error) {
	if o.HasValue {
		return o.Value, nil
	}
	return ParseValue(o.Text)
}

package asset

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"aego/format/aetext"
)

const (
	materialMagic           = "AEMT"
	materialArtifactVersion = 1
	maxMaterialBytes        = 4 << 20
	maxMaterialItems        = 8192
	maxMaterialDepth        = 32
)

func (w *aeshW) u64(v uint64) { *w = binary.LittleEndian.AppendUint64(*w, v) }
func (r *aeshR) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

// EncodeMaterial emits a versioned binary semantic document, never source text.
// Unknown properties remain typed data and can survive a shader migration.
func EncodeMaterial(d *MaterialDoc) []byte {
	w := aeshW(append([]byte(nil), materialMagic...))
	w.u16(materialArtifactVersion)
	w.u16(MaterialVersion)
	writeMaterialNode(&w, d.documentNode())
	h := sha256.Sum256(w)
	return append(w, h[:]...)
}

func writeMaterialFields(w *aeshW, fields []aetext.Field) {
	w.u32(uint32(len(fields)))
	for _, f := range fields {
		w.str(f.Name)
		w.u32(uint32(f.Pos.Line))
		w.u32(uint32(f.Pos.Col))
		writeMaterialValue(w, f.Value)
	}
}

func writeMaterialNode(w *aeshW, n *aetext.Node) {
	w.str(n.Head)
	w.str(n.Label)
	writeMaterialFields(w, n.Attrs)
	writeMaterialFields(w, n.Fields)
	w.u32(uint32(len(n.Blocks)))
	for _, b := range n.Blocks {
		writeMaterialNode(w, b)
	}
}

func writeMaterialValue(w *aeshW, v aetext.Value) {
	w.u8(uint8(v.Kind))
	switch v.Kind {
	case aetext.VInt:
		w.u64(uint64(v.Int))
	case aetext.VFloat:
		w.u64(math.Float64bits(v.Float))
		if v.Single {
			w.u8(1)
		} else {
			w.u8(0)
		}
	case aetext.VBool:
		if v.Bool {
			w.u8(1)
		} else {
			w.u8(0)
		}
	case aetext.VString, aetext.VAsset, aetext.VNode:
		w.str(v.Str)
	case aetext.VIdent:
		w.str(v.Ident)
	case aetext.VColor:
		w.u32(v.Color)
	case aetext.VList:
		w.u32(uint32(len(v.List)))
		for _, x := range v.List {
			writeMaterialValue(w, x)
		}
	case aetext.VStruct:
		writeMaterialFields(w, v.Obj)
	}
}

// DecodeMaterial bypasses the aetext parser/import pipeline, bounds nested allocations and checks artifact integrity (checksum, not trust/signature).
func DecodeMaterial(b []byte, name string) (*MaterialDoc, error) {
	if len(b) < 40 || len(b) > maxMaterialBytes || string(b[:4]) != materialMagic {
		return nil, badMaterial(name, "invalid binary artifact header")
	}
	var want [32]byte
	copy(want[:], b[len(b)-32:])
	if sha256.Sum256(b[:len(b)-32]) != want {
		return nil, badMaterial(name, "artifact checksum mismatch")
	}
	r := &aeshR{b: b[4 : len(b)-32]}
	if r.u16() != materialArtifactVersion || r.u16() != MaterialVersion {
		return nil, badMaterial(name, "unsupported artifact version; reimport the source")
	}
	items := maxMaterialItems
	n := readMaterialNode(r, 0, &items)
	if r.bad || len(r.b) != 0 {
		return nil, badMaterial(name, "truncated, oversized or invalid binary document")
	}
	return materialFromNode(n, name)
}

func materialItem(r *aeshR, depth int, items *int) bool {
	*items = *items - 1
	if r.bad || depth > maxMaterialDepth || *items < 0 {
		r.bad = true
		return false
	}
	return true
}

func readMaterialFields(r *aeshR, depth int, items *int) []aetext.Field {
	n := r.count(maxMaterialItems)
	if n > *items {
		r.bad = true
		return nil
	}
	fields := make([]aetext.Field, n)
	for i := range fields {
		if !materialItem(r, depth, items) {
			return fields
		}
		f := &fields[i]
		f.Name = r.str()
		f.Pos.Line = int(r.u32())
		f.Pos.Col = int(r.u32())
		if f.Name == "" || f.Pos.Line < 0 || f.Pos.Col < 0 {
			r.bad = true
		}
		f.Value = readMaterialValue(r, depth+1, items)
	}
	return fields
}

func readMaterialNode(r *aeshR, depth int, items *int) *aetext.Node {
	if !materialItem(r, depth, items) {
		return nil
	}
	n := &aetext.Node{Head: r.str(), Label: r.str()}
	n.Attrs = readMaterialFields(r, depth+1, items)
	n.Fields = readMaterialFields(r, depth+1, items)
	count := r.count(maxMaterialItems)
	if count > *items {
		r.bad = true
		return n
	}
	n.Blocks = make([]*aetext.Node, count)
	for i := range n.Blocks {
		n.Blocks[i] = readMaterialNode(r, depth+1, items)
	}
	return n
}

func readMaterialValue(r *aeshR, depth int, items *int) aetext.Value {
	if !materialItem(r, depth, items) {
		return aetext.Value{}
	}
	v := aetext.Value{Kind: aetext.ValueKind(r.u8())}
	switch v.Kind {
	case aetext.VInt:
		v.Int = int64(r.u64())
	case aetext.VFloat:
		v.Float = math.Float64frombits(r.u64())
		single := r.u8()
		v.Single = single == 1
		if single > 1 || math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			r.bad = true
		}
	case aetext.VBool:
		b := r.u8()
		v.Bool = b == 1
		if b > 1 {
			r.bad = true
		}
	case aetext.VString, aetext.VAsset, aetext.VNode:
		v.Str = r.str()
	case aetext.VIdent:
		v.Ident = r.str()
	case aetext.VColor:
		v.Color = r.u32()
	case aetext.VList:
		n := r.count(maxMaterialItems)
		if n > *items {
			r.bad = true
			return v
		}
		v.List = make([]aetext.Value, n)
		for i := range v.List {
			v.List[i] = readMaterialValue(r, depth+1, items)
		}
	case aetext.VStruct:
		v.Obj = readMaterialFields(r, depth+1, items)
	default:
		r.bad = true
	}
	return v
}

// A lexical nesting guard runs before the generic source parser. Quoted text
// cannot consume the nesting budget.
func boundedMaterialSource(src []byte) bool {
	depth := 0
	quote := false
	for i := 0; i < len(src); i++ {
		if quote {
			if src[i] == '\\' {
				i++
				continue
			}
			if src[i] == '"' {
				quote = false
			}
			continue
		}
		switch src[i] {
		case '"':
			quote = true
		case '{', '[':
			depth++
			if depth > maxMaterialDepth {
				return false
			}
		case '}', ']':
			depth--
		}
	}
	return true
}

func checkMaterialNode(n *aetext.Node, depth int, items *int) bool {
	*items = *items - 1
	if n == nil || depth > maxMaterialDepth || *items < 0 {
		return false
	}
	if !checkMaterialFields(n.Attrs, depth+1, items) || !checkMaterialFields(n.Fields, depth+1, items) {
		return false
	}
	for _, b := range n.Blocks {
		if !checkMaterialNode(b, depth+1, items) {
			return false
		}
	}
	return true
}
func checkMaterialFields(fields []aetext.Field, depth int, items *int) bool {
	for _, f := range fields {
		*items = *items - 1
		if depth > maxMaterialDepth || *items < 0 || f.Name == "" || !checkMaterialValue(f.Value, depth+1, items) {
			return false
		}
	}
	return true
}
func checkMaterialValue(v aetext.Value, depth int, items *int) bool {
	*items = *items - 1
	if depth > maxMaterialDepth || *items < 0 || v.Kind < aetext.VInt || v.Kind > aetext.VStruct {
		return false
	}
	if v.Kind == aetext.VFloat && (math.IsNaN(v.Float) || math.IsInf(v.Float, 0)) {
		return false
	}
	if len(v.Str) > maxMaterialBytes/2 || len(v.Ident) > maxMaterialBytes/2 {
		return false
	}
	for _, x := range v.List {
		if !checkMaterialValue(x, depth+1, items) {
			return false
		}
	}
	return checkMaterialFields(v.Obj, depth+1, items)
}

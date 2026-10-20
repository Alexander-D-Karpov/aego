package aetext

import (
	"bytes"
	"strconv"
	"strings"
)

const indentUnit = "  "

func Write(n *Node) []byte {
	var b bytes.Buffer
	writeNode(&b, n, 0)
	return b.Bytes()
}

func writeNode(b *bytes.Buffer, n *Node, depth int) {
	indent := strings.Repeat(indentUnit, depth)
	b.WriteString(indent)
	b.WriteString(n.Head)
	if n.Label != "" {
		b.WriteByte(' ')
		writeString(b, n.Label)
	}
	for _, a := range n.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.Name)
		b.WriteByte('=')
		writeValue(b, a.Value)
	}
	if len(n.Fields) == 0 && len(n.Blocks) == 0 {
		b.WriteString(" {}\n")
		return
	}
	b.WriteString(" {\n")
	inner := strings.Repeat(indentUnit, depth+1)
	for _, f := range n.Fields {
		b.WriteString(inner)
		b.WriteString(f.Name)
		b.WriteByte('=')
		writeValue(b, f.Value)
		b.WriteByte('\n')
	}
	for _, blk := range n.Blocks {
		writeNode(b, blk, depth+1)
	}
	b.WriteString(indent)
	b.WriteString("}\n")
}

func writeValue(b *bytes.Buffer, v Value) {
	switch v.Kind {
	case VInt:
		b.WriteString(strconv.FormatInt(v.Int, 10))
	case VFloat:
		bits := 64
		if v.Single {
			bits = 32
		}
		b.WriteString(strconv.FormatFloat(v.Float, 'g', -1, bits))
	case VBool:
		if v.Bool {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case VString:
		writeString(b, v.Str)
	case VColor:
		b.WriteByte('#')
		hex := strconv.FormatUint(uint64(v.Color), 16)
		for i := len(hex); i < 8; i++ {
			b.WriteByte('0')
		}
		b.WriteString(hex)
	case VIdent:
		b.WriteString(v.Ident)
	case VAsset:
		b.WriteString("asset(")
		writeString(b, v.Str)
		b.WriteByte(')')
	case VNode:
		b.WriteString("node(")
		writeString(b, v.Str)
		b.WriteByte(')')
	case VList:
		b.WriteByte('[')
		for i, item := range v.List {
			if i > 0 {
				b.WriteString(", ")
			}
			writeValue(b, item)
		}
		b.WriteByte(']')
	case VStruct:
		if len(v.Obj) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{ ")
		for i, f := range v.Obj {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(f.Name)
			b.WriteByte('=')
			writeValue(b, f.Value)
		}
		b.WriteString(" }")
	default:
		b.WriteString("false")
	}
}

func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString("\\\"")
		case c == '\\':
			b.WriteString("\\\\")
		case c == '\n':
			b.WriteString("\\n")
		case c == '\t':
			b.WriteString("\\t")
		case c < 0x20:
			b.WriteString("\\u")
			hex := strconv.FormatUint(uint64(c), 16)
			for j := len(hex); j < 4; j++ {
				b.WriteByte('0')
			}
			b.WriteString(hex)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}

func FormatValue(v Value) string {
	var b bytes.Buffer
	writeValue(&b, v)
	return b.String()
}

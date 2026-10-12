package langsvc

import (
	"aego/script/checker"
	"aego/script/lexer"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/workspace"
)

var builtinOrder = []string{
	"bool", "int", "float", "string", "Vec2", "Vec3", "Color", "Quat", "Rect",
	"Transform2D", "Transform3D", "NodeRef", "AssetRef", "ComponentRef",
	"Coroutine", "Connection", "ScriptRef",
}

var builtinDecl = map[string]string{
	"Vec2":         "type Vec2 struct { X, Y float }",
	"Vec3":         "type Vec3 struct { X, Y, Z float }",
	"Color":        "type Color struct { R, G, B, A float }",
	"Quat":         "type Quat struct { X, Y, Z, W float }",
	"Rect":         "type Rect struct { X, Y, W, H float }",
	"Transform2D":  "type Transform2D struct { Position Vec2; Rotation float; Scale Vec2 }",
	"Transform3D":  "type Transform3D struct { Position Vec3; Rotation Quat; Scale Vec3 }",
	"AssetRef":     "type AssetRef<Texture | Scene | Prefab | Mesh>",
	"ComponentRef": "type ComponentRef<T>",
	"Coroutine":    "type Coroutine",
	"Connection":   "type Connection",
	"ScriptRef":    "type ScriptRef<Script>",
}

func builtinText(snap *workspace.Snapshot, key string) string {
	return snap.Docs.Keyword(key)
}

const KeyRefPrefix = "key:"

func KeyRef(key string) string {
	return KeyRefPrefix + key
}

func builtinKey(o *resolver.Object) string {
	switch o.Kind {
	case resolver.ObjType:
		if o.Ident == nil {
			return o.Name
		}
	case resolver.ObjGeneric, resolver.ObjBuiltin, resolver.ObjNode:
		return o.Name
	case resolver.ObjFunc:
		if f, ok := o.Data.(*checker.Func); ok && f.Script != nil {
			return f.Name
		}
	}
	return ""
}

func objectDoc(snap *workspace.Snapshot, o *resolver.Object) string {
	if d := docOf(o); d != "" {
		return d
	}
	if k := builtinKey(o); k != "" {
		return builtinText(snap, k)
	}
	return ""
}

func objectRef(snap *workspace.Snapshot, o *resolver.Object) string {
	if k := builtinKey(o); k != "" && builtinText(snap, k) != "" {
		return KeyRef(k)
	}
	return ""
}

func keywordHover(snap *workspace.Snapshot, file source.FileID, off uint32) (Hover, bool) {
	toks, _, _ := lexer.TokenizeAll(file, snap.Sources.Source(file), source.NewInterner())
	for i, t := range toks {
		if t.Start > off {
			break
		}
		if off > t.End {
			continue
		}
		key := ""
		switch {
		case t.Kind.IsKeyword():
			key = t.Text
		case t.Kind == token.Ident && t.Text == "start" && i+1 < len(toks) && toks[i+1].Kind == token.Ident:
			key = "start"
		case t.Kind == token.Ident && i > 0 && toks[i-1].Kind == token.Yield:
			key = "yield " + t.Text
		}
		if key == "" {
			continue
		}
		if d := builtinText(snap, key); d != "" {
			return Hover{Signature: key, Doc: d, Ref: KeyRef(key), Span: source.Span{File: file, Start: t.Start, End: t.End}}, true
		}
	}
	return Hover{}, false
}

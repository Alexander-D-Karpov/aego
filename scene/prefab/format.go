package prefab

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"aego/schema"
)

func f32(f float32) string {
	return strconv.FormatFloat(float64(f), 'g', -1, 32)
}

func floats(parts ...float32) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = f32(p)
	}
	return "[" + strings.Join(s, ", ") + "]"
}

func FormatValue(v schema.Value) string {
	switch v.Kind {
	case schema.KindBool:
		return strconv.FormatBool(v.I != 0)
	case schema.KindInt32, schema.KindInt64, schema.KindEnum:
		return v.Kind.String() + ":" + strconv.FormatInt(v.I, 10)
	case schema.KindFloat32:
		return "f32:" + f32(float32(v.F))
	case schema.KindFloat64:
		return "f64:" + strconv.FormatFloat(v.F, 'g', -1, 64)
	case schema.KindVec2:
		return floats(v.V.X, v.V.Y)
	case schema.KindVec3:
		return floats(v.V3.X, v.V3.Y, v.V3.Z)
	case schema.KindVec4:
		return "v4" + floats(v.V4.X, v.V4.Y, v.V4.Z, v.V4.W)
	case schema.KindQuat:
		return "q" + floats(v.Q.X, v.Q.Y, v.Q.Z, v.Q.W)
	case schema.KindRect:
		return "r" + floats(v.R.X, v.R.Y, v.R.W, v.R.H)
	case schema.KindColor:
		return fmt.Sprintf("#%08x", ColorKey(v.C))
	case schema.KindString:
		return strconv.Quote(v.S)
	case schema.KindAssetRef:
		return "asset(" + v.Asset.String() + ")"
	case schema.KindNodeRef:
		return "node(" + v.Node.String() + ")"
	case schema.KindList:
		s := make([]string, len(v.List))
		for i, e := range v.List {
			s[i] = FormatValue(e)
		}
		return "list[" + strings.Join(s, ", ") + "]"
	case schema.KindStruct:
		if v.Obj == nil {
			return "{}"
		}
		s := make([]string, len(v.Obj.Fields))
		for i, e := range v.Obj.Fields {
			s[i] = FormatValue(e)
		}
		return "{" + strconv.FormatUint(uint64(v.Obj.Type), 16) + " " + strings.Join(s, " ") + "}"
	}
	return "invalid"
}

func Format(n *Node) string {
	var sb strings.Builder
	formatNode(&sb, n, 0)
	return sb.String()
}

func FormatOverrides(o Overrides) string {
	var sb strings.Builder
	formatOverrides(&sb, o, 0)
	return sb.String()
}

func formatComp(sb *strings.Builder, ind string, c *Comp) {
	sb.WriteString(ind + c.Owner)
	fields := slices.Clone(c.Fields)
	slices.SortFunc(fields, func(a, b Field) int { return strings.Compare(a.Name, b.Name) })
	for _, f := range fields {
		sb.WriteString(" " + f.Name + "=" + FormatValue(f.Value))
	}
	sb.WriteByte('\n')
}

func formatNode(sb *strings.Builder, n *Node, depth int) {
	ind := strings.Repeat("  ", depth)
	k := n.Key
	if k == "" {
		k = LocalKey(n.Local)
	}
	sb.WriteString(ind + "node " + string(k))
	if n.Space3D {
		sb.WriteString(" 3d")
	}
	switch {
	case n.Instance != nil:
		sb.WriteString(" prefab=" + n.Instance.Prefab.String())
	case n.Missing && n.Origin != nil:
		sb.WriteString(" missing=" + n.Origin.Prefab.String())
	}
	sb.WriteByte('\n')
	comps := slices.Clone(n.Comps)
	slices.SortFunc(comps, func(a, b Comp) int { return strings.Compare(a.Owner, b.Owner) })
	for i := range comps {
		formatComp(sb, ind+"  ", &comps[i])
	}
	if n.Instance != nil {
		formatOverrides(sb, n.Instance.Overrides, depth+1)
	}
	for _, c := range n.Children {
		formatNode(sb, c, depth+1)
	}
}

func orphanTag(orphan bool) string {
	if orphan {
		return " orphan"
	}
	return ""
}

func formatOverrides(sb *strings.Builder, o Overrides, depth int) {
	o = o.Clone()
	o.Sort()
	ind := strings.Repeat("  ", depth)
	for _, p := range o.Props {
		sb.WriteString(ind + "prop " + string(p.Node) + " " + p.Prop.String() + "=" + FormatValue(p.Value) + orphanTag(p.Orphan) + "\n")
	}
	for _, a := range o.AddComps {
		sb.WriteString(ind + "add-comp " + string(a.Node) + orphanTag(a.Orphan) + "\n")
		formatComp(sb, ind+"  ", &a.Comp)
	}
	for _, r := range o.RemoveComps {
		sb.WriteString(ind + "remove-comp " + string(r.Node) + " " + r.Owner + "\n")
	}
	for _, a := range o.AddNodes {
		sb.WriteString(ind + "add-node " + string(a.Parent) + orphanTag(a.Orphan) + "\n")
		formatNode(sb, a.Node, depth+1)
	}
	for _, k := range o.Remove {
		sb.WriteString(ind + "remove " + string(k) + "\n")
	}
	for _, ord := range o.Order {
		keys := make([]string, len(ord.Children))
		for i, k := range ord.Children {
			keys[i] = string(k)
		}
		sb.WriteString(ind + "order " + string(ord.Parent) + " [" + strings.Join(keys, ", ") + "]\n")
	}
}

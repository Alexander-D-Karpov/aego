package edit

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"aego/app/agent"
	"aego/core/ids"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/scene/mutate"
	"aego/schema"
)

// view turns tool calls and their results into what the Agent panel shows: names instead of ids, Inspector labels instead of type paths (11b5a).
type agentView struct {
	ctx *context.Editor
}

var refValue = regexp.MustCompile(`(asset|node)\("([0-9a-f]*)"\)`)

// value is a field value as the Inspector shows it: asset and node references by name, empty ones as none.
func (v agentView) value(file, s string) string {
	return refValue.ReplaceAllStringFunc(s, func(m string) string {
		sub := refValue.FindStringSubmatch(m)
		if strings.Trim(sub[2], "0") == "" {
			return i18n.Tr("editor.agent.view.none", "none")
		}
		if sub[1] == "asset" {
			return v.assetName(sub[2])
		}
		return v.nodeName(file, sub[2])
	})
}

func (v agentView) assetName(hex string) string {
	id, err := ids.ParseAssetID(hex)
	if err != nil || v.ctx == nil || v.ctx.Pipeline == nil {
		return short(hex)
	}
	if e, ok := v.ctx.Pipeline.Index().Lookup(id); ok {
		if e.Source != "" {
			return path.Base(e.Source)
		}
		return e.Name
	}
	return short(hex)
}

func (v agentView) nodeName(file, hex string) string {
	id, err := ids.ParseNodeID(hex)
	if err != nil || v.ctx == nil || v.ctx.Docs == nil {
		return short(hex)
	}
	if d := v.ctx.Docs.Find(file); d != nil {
		s := d.Scene()
		if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
			return s.Name(h)
		}
	}
	return short(hex)
}

func short(hex string) string {
	if len(hex) > 8 {
		return "#" + hex[:8]
	}
	return "#" + hex
}

// field is a change path as the Inspector labels it: engine.SpriteRenderer.sprite is Sprite Renderer › Sprite.
func (v agentView) field(p string) (string, *schema.TypeDescriptor, *schema.FieldDescriptor) {
	if p == "" {
		return "", nil, nil
	}
	parts := strings.Split(p, ".")
	if parts[0] == "transform" && len(parts) > 1 {
		return i18n.Tr("editor.agent.view.transform", "Transform") + " › " + schema.Humanize(parts[1]) + suffix(parts[2:]), nil, nil
	}
	if v.ctx == nil || v.ctx.Types == nil {
		return p, nil, nil
	}
	reg := v.ctx.Types.Schema()
	for i := len(parts) - 1; i > 0; i-- {
		td, ok := reg.ByName(strings.Join(parts[:i], "."))
		if !ok {
			continue
		}
		fd, ok := td.Field(parts[i])
		if !ok {
			return schema.Title(td) + " › " + strings.Join(parts[i:], "."), td, nil
		}
		return schema.Title(td) + " › " + schema.FieldLabel(td, fd) + suffix(parts[i+1:]), td, fd
	}
	return p, nil, nil
}

func suffix(rest []string) string {
	if len(rest) == 0 {
		return ""
	}
	return "." + strings.Join(rest, ".")
}

// typeTitle is a component's display name from its full name.
func (v agentView) typeTitle(name string) string {
	if v.ctx != nil && v.ctx.Types != nil {
		if td, ok := v.ctx.Types.Schema().ByName(name); ok {
			return schema.Title(td)
		}
	}
	return name
}

// rows are the changes of a card, grouped by node: new nodes and components first, then fields with their old and new values.
func (v agentView) rows(file string, chs []mutate.Change) []agentui.Row {
	var out []agentui.Row
	for _, c := range chs {
		node := strings.TrimPrefix(c.Node, "Root/")
		r := agentui.Row{Group: node, Title: node}
		switch c.Op {
		case mutate.KindAddNode, mutate.KindPrefabInstantiate:
			r.Icon, r.Text = "edit.add", i18n.Tr("editor.agent.view.new_node", "new node")
		case mutate.KindRemoveNode:
			r.Icon, r.Text = "edit.remove", i18n.Tr("editor.agent.view.removed_node", "removed")
		case mutate.KindAddComp:
			r.Icon, r.Text = "edit.add", "+ "+v.typeTitle(c.Path)
		case mutate.KindRemoveComp:
			r.Icon, r.Text = "edit.remove", "− "+v.typeTitle(c.Path)
		case "script-edit", "script-new":
			r.Group, r.Title, r.Icon, r.Text = c.Node, path.Base(c.Node), "file.script", c.Path
			r.Before, r.After = c.Before, c.After
		default:
			label, _, _ := v.field(c.Path)
			if label == "" {
				label = c.Op
			}
			r.Icon, r.Text = "edit.set", label
			r.Before, r.After = v.value(file, c.Before), v.value(file, c.After)
		}
		if c.Op == mutate.KindAddComp || c.Op == mutate.KindRemoveComp {
			if r.Text == "+ " || r.Text == "− " {
				r.Text += c.After
			}
		}
		out = append(out, r)
	}
	return out
}

func docIcon(kind string) string {
	switch kind {
	case "type":
		return "cat.node"
	case "function", "method", "signal", "event", "const", "enum":
		return "code.block"
	case "code":
		return "log.warn"
	}
	return "doc.page"
}

// Describe is how a tool call looks: the command's look, and the result laid out by what it holds.
func (h *agentHost) Describe(tool, args, result string, failed bool) agentui.Step {
	var a agent.Args
	_ = json.Unmarshal([]byte(args), &a)
	if a == nil {
		a = agent.Args{}
	}
	st := agentui.Step{Icon: "code.block", Title: tool}
	if l, ok := agent.Present(tool); ok {
		st.Icon, st.Title = l.Icon, l.Title(a)
	}
	for _, c := range agent.Commands() {
		if c.Tool() == tool {
			st.Write = c.Write
		}
	}
	if result == "" {
		return st
	}
	if failed {
		st.Summary = firstLine(result)
		st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: result}
		return st
	}
	v := agentView{}
	if h.bridge != nil {
		v.ctx = h.bridge.Editor()
	}
	file := a.Str("file")
	switch tool {
	case "docs_search":
		var res struct {
			Results []struct{ ID, Title, Kind, Summary string } `json:"results"`
		}
		_ = json.Unmarshal([]byte(result), &res)
		st.Summary = i18n.TrN("editor.agent.step.results", int64(len(res.Results)), "{n} result", "{n} results")
		d := agentui.Detail{Kind: agentui.DetailList}
		for _, r := range res.Results {
			d.Rows = append(d.Rows, agentui.Row{Icon: docIcon(r.Kind), Title: r.Title, Text: r.Summary, Link: "doc:" + r.ID})
		}
		st.Detail = d
		return st
	case "docs_read":
		st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: clipText(result, 1200)}
		st.Summary = firstHeading(result)
		return st
	case "ask_user":
		st.Summary = clipText(strings.TrimPrefix(strings.TrimPrefix(result, "The user chose: "), "The user answered: "), 60)
		return st
	}
	out, ok := agent.DecodeOutput(result)
	if !ok {
		st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: clipText(result, 1200)}
		return st
	}
	if l, ok := agent.Present(tool); ok {
		st.Summary = l.Summary(out)
	}
	raw, _ := out.Value.(json.RawMessage)
	switch tool {
	case "scene_tree":
		var root agent.NodeView
		if json.Unmarshal(raw, &root) == nil {
			d := agentui.Detail{Kind: agentui.DetailTree}
			var walk func(n *agent.NodeView, depth int)
			walk = func(n *agent.NodeView, depth int) {
				for i := range n.Children {
					c := &n.Children[i]
					var comps []string
					for _, cv := range c.Components {
						comps = append(comps, v.typeTitle(cv.Type))
					}
					icon := "cat.node"
					if c.Prefab != "" {
						icon = "file.prefab"
					}
					d.Rows = append(d.Rows, agentui.Row{Icon: icon, Title: c.Name, Text: strings.Join(comps, ", "), Depth: depth, Link: "node:" + file + "#" + c.ID})
					walk(c, depth+1)
				}
			}
			walk(&root, 0)
			st.Detail = d
		}
	case "scene_get":
		var n agent.NodeView
		if json.Unmarshal(raw, &n) == nil && n.Name != "" {
			d := agentui.Detail{Kind: agentui.DetailTable}
			for _, k := range sortedKeys(n.Transform) {
				d.Rows = append(d.Rows, agentui.Row{Title: i18n.Tr("editor.agent.view.transform", "Transform") + " › " + schema.Humanize(k), Text: n.Transform[k]})
			}
			for _, c := range n.Components {
				for _, k := range sortedKeys(c.Fields) {
					label, _, _ := v.field(c.Type + "." + k)
					d.Rows = append(d.Rows, agentui.Row{Title: label, Text: v.value(file, c.Fields[k])})
				}
			}
			st.Detail = d
		}
	case "schema_list":
		var list []agent.TypeView
		if json.Unmarshal(raw, &list) == nil {
			d := agentui.Detail{Kind: agentui.DetailChips}
			for _, t := range list {
				group := t.Category
				if group == "" {
					group = t.Kind
				}
				d.Rows = append(d.Rows, agentui.Row{Title: v.typeTitle(t.Name), Group: group, Text: t.Doc, Link: "doc:" + t.Name})
			}
			st.Detail = d
		}
	case "schema_show":
		var t agent.TypeView
		if json.Unmarshal(raw, &t) == nil {
			d := agentui.Detail{Kind: agentui.DetailTable}
			for _, f := range t.Fields {
				label, _, _ := v.field(t.Name + "." + f.Name)
				d.Rows = append(d.Rows, agentui.Row{Title: label, Text: f.Kind})
			}
			st.Detail = d
		}
	case "asset_list":
		var list []agent.AssetView
		if json.Unmarshal(raw, &list) == nil {
			d := agentui.Detail{Kind: agentui.DetailGrid}
			for _, x := range list {
				d.Rows = append(d.Rows, agentui.Row{Icon: assetIcon(x.Kind), Title: path.Base(x.Path), Text: path.Dir(x.Path), Link: "asset:" + x.Path})
			}
			st.Detail = d
		}
	case "project_get":
		var secs map[string]string
		if json.Unmarshal(raw, &secs) == nil {
			d := agentui.Detail{Kind: agentui.DetailTable}
			for _, k := range sortedKeys(secs) {
				d.Rows = append(d.Rows, agentui.Row{Title: k, Text: strings.TrimSpace(secs[k])})
			}
			st.Detail = d
		}
	case "scene_run":
		var r agent.RunReport
		_ = json.Unmarshal(raw, &r)
		d := agentui.Detail{Kind: agentui.DetailLog}
		for _, l := range r.Log {
			d.Rows = append(d.Rows, agentui.Row{Text: l, Level: logLevel(l)})
		}
		for _, p := range out.Problems {
			d.Rows = append(d.Rows, problemRow(p))
		}
		if r.Hash != "" {
			d.Rows = append(d.Rows, agentui.Row{Title: i18n.Tr("editor.agent.view.hash", "State hash"), Text: r.Hash})
		}
		st.Detail = d
	case "script_read":
		var text string
		if json.Unmarshal(raw, &text) == nil {
			st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: clipText(text, 2000)}
		}
	default:
		switch {
		case len(out.Changes) > 0:
			st.Detail = agentui.Detail{Kind: agentui.DetailDiff, Rows: v.rows(file, out.Changes)}
		case len(out.Problems) > 0 || len(out.Notes) > 0:
			d := agentui.Detail{Kind: agentui.DetailLog}
			for _, p := range out.Problems {
				d.Rows = append(d.Rows, problemRow(p))
			}
			for _, n := range out.Notes {
				d.Rows = append(d.Rows, agentui.Row{Text: n})
			}
			st.Detail = d
		case out.Text != "":
			st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: clipText(out.Text, 1200)}
		}
	}
	return st
}

func problemRow(p mutate.Problem) agentui.Row {
	r := agentui.Row{Title: strings.TrimPrefix(p.Node, "Root/"), Text: p.Message, Level: 1}
	if p.Error {
		r.Level = 2
	}
	return r
}

func logLevel(l string) int {
	switch {
	case strings.Contains(l, " ERROR "), strings.Contains(l, "error:"):
		return 2
	case strings.Contains(l, " WARN "), strings.Contains(l, "warning:"):
		return 1
	}
	return 0
}

func assetIcon(kind string) string {
	switch kind {
	case "texture":
		return "file.texture"
	case "scene":
		return "file.scene"
	case "prefab":
		return "file.prefab"
	case "audio":
		return "file.audio"
	case "script":
		return "file.script"
	case "data", "table":
		return "file.data"
	}
	return "file.unknown"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clipText(s, 80)
}

func firstHeading(md string) string {
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(l, "# ") {
			return clipText(strings.TrimPrefix(l, "# "), 60)
		}
	}
	return ""
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

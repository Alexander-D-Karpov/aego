package site

import (
	"strconv"
	"strings"

	"aego/scene/sceneio"
)

func (b *builder) sceneTree(src, root string) string {
	nodes, err := sceneio.Outline([]byte(src))
	if err != nil || len(nodes) == 0 {
		return ""
	}
	count := 0
	var walk func([]sceneio.OutlineNode)
	walk = func(list []sceneio.OutlineNode) {
		for _, n := range list {
			count++
			walk(n.Children)
		}
	}
	walk(nodes)
	var c strings.Builder
	label := strconv.Itoa(count) + " nodes"
	if count == 1 {
		label = "1 node"
	}
	c.WriteString(`<details class="scene-tree"><summary>` + b.icon(root, "file.scene", "") + `<span>Scene</span><span class="count">` + label + `</span>` + b.icon(root, "ui.chevron_right", "chev") + `</summary><div class="st-body">`)
	b.treeNodes(&c, nodes, root)
	c.WriteString("</div></details>\n")
	return c.String()
}

func (b *builder) treeNodes(c *strings.Builder, nodes []sceneio.OutlineNode, root string) {
	for _, n := range nodes {
		name := n.Name
		if name == "" {
			name = "Node"
		}
		c.WriteString(`<div class="st-node"><div class="st-head">` + b.icon(root, "node.empty", "") + esc(name))
		if !n.Enabled {
			c.WriteString(` <span class="note">disabled</span>`)
		}
		c.WriteString(`</div>`)
		for _, blk := range n.Blocks {
			b.treeBlock(c, n.ID, blk, root)
		}
		if len(n.Children) > 0 {
			c.WriteString(`<div class="st-children">`)
			b.treeNodes(c, n.Children, root)
			c.WriteString(`</div>`)
		}
		c.WriteString("</div>")
	}
}

func (b *builder) treeBlock(c *strings.Builder, node string, blk sceneio.OutlineBlock, root string) {
	id, title, icon := blk.Type, blk.Type, "code.block"
	switch blk.Kind {
	case "transform":
		id, title, icon = "engine.Transform", "Transform", "cat.transform"
	case "component":
		if e, ok := b.byID[id]; ok {
			title, icon = e.Title, b.entryIcon(e)
		}
	case "script":
		icon = "file.script"
	}
	c.WriteString(`<div class="st-comp">`)
	head := b.icon(root, icon, "") + esc(title)
	if u, ok := b.urls[id]; ok {
		c.WriteString(`<a class="st-ctitle" href="` + esc(root+u) + `">` + head + `</a>`)
	} else {
		c.WriteString(`<span class="st-ctitle">` + head + `</span>`)
	}
	c.WriteString(`<div class="st-fields">`)
	for _, f := range blk.Fields {
		fe := b.fieldEntry(id, f.Name)
		name, typ := esc(f.Name), ""
		if fe != nil {
			if u, ok := b.urls[fe.ID]; ok {
				name = `<a href="` + esc(root+u) + `">` + name + `</a>`
			}
			if fe.Field != nil {
				typ = fe.Field.Type
				for _, n := range fe.Field.Notes {
					if strings.HasPrefix(n, "one of:") {
						typ = "enum"
					}
				}
			}
		}
		val := esc(f.Value)
		if f.Kind == "color" {
			val = `<i class="sw" style="background:` + esc(f.Value) + `"></i>` + val
		}
		c.WriteString(`<span class="k">` + name + `</span><span class="t">` + esc(typ) + `</span><span class="v" data-k="` + esc(sceneio.OutlineKey(node, blk, f.Name)) + `" data-kind="` + f.Kind + `">` + val + `</span>`)
	}
	c.WriteString("</div></div>")
}

func (b *builder) fieldEntry(owner, name string) *Entry {
	for _, k := range b.kids[owner] {
		if i := strings.LastIndexByte(k, '.'); i >= 0 && strings.EqualFold(k[i+1:], name) {
			return b.byID[k]
		}
	}
	return nil
}

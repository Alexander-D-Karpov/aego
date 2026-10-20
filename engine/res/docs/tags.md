---
id: tags
icon: comp.tags
group: scenes
title: Tags and queries
order: 2
---
# Tags and queries

![The Tags component in the Inspector](shots/tags.png)

Tags group nodes by meaning: `Enemy`, `Enemy.Flying`, `Pickup.Coin`. Add the Tags component in the Inspector or call `node.AddTag` from a script; find nodes with `scene.WithTag`.

---
id: tags/names
icon: edit.rename
parent: tags
order: 1
title: Tag names
---
# Tag names

A tag is segments of letters, digits and `_` joined by dots. A node tagged `Enemy.Flying` is also found by `Enemy`, so broad and narrow groups need one tag. Tags are case-sensitive. A malformed tag is ignored and Problems shows TAG001; a tag listed twice shows TAG002.

---
id: tags/scripts
icon: file.script
parent: tags
order: 2
title: In scripts
---
# In scripts

    var enemies []NodeRef

    func Ready() {
      for i := 0; i < 64; i++ {
        enemies = append(enemies, nil)
      }
    }

    func Update(dt float) {
      n := scene.WithTag("Enemy", enemies)
      for i := 0; i < n; i++ {
        debug.Circle(enemies[i].WorldPosition(), 0.5, Color{1, 0, 0, 1})
      }
    }

| Function | |
|---|---|
| `node.HasTag(t)`, `node.AddTag(t)`, `node.RemoveTag(t)` | tags of one node |
| `scene.WithTag(tag, out) int` | enabled nodes with the tag, in the order they got it |
| `scene.WithComponent(name, out) int` | enabled nodes with the component (`"Body2D"` or the full name) |
| `scene.InArea(rect, mask, out) int` | bodies touching the rectangle on the layers in `mask` when physics runs; otherwise enabled 2D nodes whose position lies in it |
| `scene.ByID(id) NodeRef` | the node with an id from the scene file |

Queries write at most `len(out)` nodes and allocate nothing; size the slice once. They see the enabled state of the last transform pass. Without physics a node is a point: the core does not know the size of a sprite or a mesh, and `InArea` ignores `mask`.

---
id: tags/go
icon: code.brackets
parent: tags
order: 3
title: In Go
---
# In Go

    s.AddTag(h, "Enemy.Flying")
    n := s.WithTag("Enemy", buf)

`WithTagAll` includes disabled nodes, `WithComponent(typeID, out)` and `InRect(rect, out)` are the other queries. When Go code changes `Tags.Tags` directly, call `s.TagsChanged(h)` so the index sees it.

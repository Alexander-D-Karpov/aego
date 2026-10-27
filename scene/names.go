package scene

import (
	"aego/core/ids"
	"strconv"
	"strings"

	"aego/core/strs"
)

func (s *Scene) ID(h NodeHandle) ids.NodeID {
	n, ok := s.nodes.Get(h)
	if !ok {
		return ids.NodeID{}
	}
	return n.id
}

func (s *Scene) Handle(id ids.NodeID) NodeHandle {
	return s.byID[id]
}

func (s *Scene) Name(h NodeHandle) string {
	n, ok := s.nodes.Get(h)
	if !ok {
		return ""
	}
	return s.interner.String(n.name)
}

func (s *Scene) NameID(h NodeHandle) strs.StringID {
	n, ok := s.nodes.Get(h)
	if !ok {
		return strs.Empty
	}
	return n.name
}

func (s *Scene) SetName(h NodeHandle, name string) string {
	n, ok := s.node(h)
	if !ok {
		return ""
	}
	if name == "" {
		name = "Node"
	}
	s.removeNameIndex(h, n)
	unique := s.uniqueName(n.parent, name, h)
	n.name = s.interner.Intern(unique)
	s.insertNameIndex(h, n)
	s.touch()
	s.emit(Event{Kind: EvNodeRenamed, Node: h, ID: n.id})
	return unique
}

func (s *Scene) uniqueName(parent NodeHandle, name string, self NodeHandle) string {
	id, ok := s.interner.Lookup(name)
	if ok {
		if existing, taken := s.byName[nameKey{parent: parent, name: id}]; !taken || existing == self {
			return name
		}
	} else {
		return name
	}
	base := name
	if i := strings.LastIndexByte(name, '_'); i > 0 {
		if _, err := strconv.Atoi(name[i+1:]); err == nil {
			base = name[:i]
		}
	}
	for n := 2; n < 1<<20; n++ {
		candidate := base + "_" + strconv.Itoa(n)
		cid, ok := s.interner.Lookup(candidate)
		if !ok {
			return candidate
		}
		if existing, taken := s.byName[nameKey{parent: parent, name: cid}]; !taken || existing == self {
			return candidate
		}
	}
	return base + "_" + ids.NewNodeID().String()[:8]
}

func (s *Scene) insertNameIndex(h NodeHandle, n *nodeData) {
	if n.name == strs.Empty {
		return
	}
	s.byName[nameKey{parent: n.parent, name: n.name}] = h
}

func (s *Scene) removeNameIndex(h NodeHandle, n *nodeData) {
	key := nameKey{parent: n.parent, name: n.name}
	if existing, ok := s.byName[key]; ok && existing == h {
		delete(s.byName, key)
	}
}

func (s *Scene) Parent(h NodeHandle) NodeHandle {
	n, ok := s.nodes.Get(h)
	if !ok {
		return NodeHandle{}
	}
	return n.parent
}

func (s *Scene) FirstChild(h NodeHandle) NodeHandle {
	n, ok := s.nodes.Get(h)
	if !ok {
		return NodeHandle{}
	}
	return n.firstChild
}

func (s *Scene) NextSibling(h NodeHandle) NodeHandle {
	n, ok := s.nodes.Get(h)
	if !ok {
		return NodeHandle{}
	}
	return n.nextSib
}

func (s *Scene) PrevSibling(h NodeHandle) NodeHandle {
	n, ok := s.nodes.Get(h)
	if !ok {
		return NodeHandle{}
	}
	return n.prevSib
}

func (s *Scene) ChildCount(h NodeHandle) int {
	n, ok := s.nodes.Get(h)
	if !ok {
		return 0
	}
	return int(n.childCount)
}

func (s *Scene) Children(h NodeHandle, fn func(NodeHandle) bool) {
	n, ok := s.nodes.Get(h)
	if !ok {
		return
	}
	for c := n.firstChild; !c.IsZero(); {
		child, ok := s.nodes.Get(c)
		if !ok {
			return
		}
		next := child.nextSib
		if !fn(c) {
			return
		}
		c = next
	}
}

func (s *Scene) Child(parent NodeHandle, name string) NodeHandle {
	id, ok := s.interner.Lookup(name)
	if !ok {
		return NodeHandle{}
	}
	return s.byName[nameKey{parent: parent, name: id}]
}

func (s *Scene) Find(from NodeHandle, path string) NodeHandle {
	if path == "" {
		return from
	}
	cur := from
	if path[0] == '/' {
		cur = s.root
		path = path[1:]
		if path == "" {
			return cur
		}
	}
	for path != "" {
		part := path
		if i := strings.IndexByte(path, '/'); i >= 0 {
			part, path = path[:i], path[i+1:]
		} else {
			path = ""
		}
		switch part {
		case "", ".":
			continue
		case "..":
			cur = s.Parent(cur)
		default:
			cur = s.Child(cur, part)
		}
		if cur.IsZero() {
			return NodeHandle{}
		}
	}
	return cur
}

func (s *Scene) Path(h NodeHandle) string {
	if h == s.root {
		return "/"
	}
	var parts []string
	for cur := h; !cur.IsZero() && cur != s.root; {
		parts = append(parts, s.Name(cur))
		cur = s.Parent(cur)
	}
	var sb strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		sb.WriteByte('/')
		sb.WriteString(parts[i])
	}
	return sb.String()
}

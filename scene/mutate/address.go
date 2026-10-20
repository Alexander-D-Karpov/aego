package mutate

import (
	"strconv"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/scene"
	"aego/schema"
)

const (
	pathTransform = "transform"
	pathEnabled   = "enabled"
)

type target struct {
	h     scene.NodeHandle
	td    *schema.TypeDescriptor
	fd    *schema.FieldDescriptor
	pose  string
	base  string
	lane  string
	index int
}

func (d *Document) node(ref string) (scene.NodeHandle, error) {
	s := d.Scene
	ref = strings.TrimSpace(ref)
	switch {
	case ref == "" || ref == "/" || ref == RootName:
		return s.Root(), nil
	case strings.HasPrefix(ref, "#"):
		id, err := ids.ParseNodeID(ref[1:])
		if err != nil {
			return scene.NodeHandle{}, errs.Wrap(err, errs.SceneDeadNode, "bad node id").With(log.F("node", ref))
		}
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			return h, errs.New(errs.SceneDeadNode, "no node with this id", log.F("node", ref))
		}
		return h, nil
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(ref, "/"), RootName+"/")
	h := s.Find(s.Root(), rest)
	if h.IsZero() || !s.IsAlive(h) {
		return h, errs.New(errs.SceneDeadNode, "no node at this path", log.F("node", ref))
	}
	return h, nil
}

func (d *Document) Node(ref string) (scene.NodeHandle, error) {
	return d.node(ref)
}

func (d *Document) nodeName(h scene.NodeHandle) string {
	s := d.Scene
	if h == s.Root() {
		return RootName
	}
	return RootName + s.Path(h)
}

func idRef(s *scene.Scene, h scene.NodeHandle) string {
	if h == s.Root() {
		return RootName
	}
	return "#" + s.ID(h).String()
}

func (d *Document) resolve(name string) []*schema.TypeDescriptor {
	reg := d.reg
	if td, ok := reg.ByName(name); ok && td.Kind == schema.TypeComponent {
		return []*schema.TypeDescriptor{td}
	}
	var found []*schema.TypeDescriptor
	for _, td := range reg.Components() {
		if strings.HasSuffix(td.FullName, "."+name) || strings.EqualFold(td.DisplayName, name) {
			found = append(found, td)
		}
	}
	return found
}

func ambiguous(name string, found []*schema.TypeDescriptor) error {
	names := make([]string, len(found))
	for i, td := range found {
		names[i] = td.FullName
	}
	return errs.New(errs.SchemaUnknownType, "ambiguous component, use the full name", log.F("component", name), log.F("candidates", strings.Join(names, ", ")))
}

func (d *Document) component(name string) (*schema.TypeDescriptor, error) {
	switch found := d.resolve(name); len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, errs.New(errs.SchemaUnknownType, "unknown component", log.F("component", name))
	default:
		return nil, ambiguous(name, found)
	}
}

func field(td *schema.TypeDescriptor, name string) (*schema.FieldDescriptor, bool) {
	if fd, ok := td.Field(name); ok {
		return fd, true
	}
	for i := range td.Fields {
		fd := &td.Fields[i]
		if strings.EqualFold(fd.Name, name) {
			return fd, true
		}
		for _, f := range fd.Former {
			if strings.EqualFold(f, name) {
				return fd, true
			}
		}
	}
	return nil, false
}

func splitIndex(seg string) (string, int, error) {
	open := strings.IndexByte(seg, '[')
	if open < 0 {
		return seg, -1, nil
	}
	if !strings.HasSuffix(seg, "]") {
		return "", 0, errs.New(errs.EditorProps, "bad index in field path", log.F("segment", seg))
	}
	n, err := strconv.Atoi(seg[open+1 : len(seg)-1])
	if err != nil || n < 0 {
		return "", 0, errs.New(errs.EditorProps, "bad index in field path", log.F("segment", seg))
	}
	return seg[:open], n, nil
}

func (d *Document) target(nodeRef, path string) (target, error) {
	h, err := d.node(nodeRef)
	if err != nil {
		return target{}, err
	}
	t := target{h: h, index: -1}
	parts := strings.Split(path, ".")
	switch strings.ToLower(parts[0]) {
	case pathEnabled:
		if len(parts) != 1 {
			return t, errs.New(errs.EditorProps, "enabled has no parts", log.F("path", path))
		}
		t.pose, t.base = pathEnabled, pathEnabled
		return t, nil
	case pathTransform:
		if len(parts) < 2 || len(parts) > 3 {
			return t, errs.New(errs.EditorProps, "use transform.position, transform.rotation or transform.scale", log.F("path", path))
		}
		t.pose = strings.ToLower(parts[1])
		switch t.pose {
		case "position", "rotation", "scale":
		default:
			return t, errs.New(errs.EditorProps, "use transform.position, transform.rotation or transform.scale", log.F("path", path))
		}
		t.base = pathTransform + "." + t.pose
		if len(parts) == 3 {
			t.lane = strings.ToLower(parts[2])
		}
		return t, nil
	}
	var miss error
	for i := len(parts) - 1; i >= 1; i-- {
		name := strings.Join(parts[:i], ".")
		found := d.resolve(name)
		if len(found) != 1 {
			if len(found) > 1 && miss == nil {
				miss = ambiguous(name, found)
			}
			continue
		}
		td := found[0]
		name, idx, err := splitIndex(parts[i])
		if err != nil {
			return t, err
		}
		fd, ok := field(td, name)
		if !ok {
			return t, errs.New(errs.EditorProps, td.FullName+" has no field "+name, log.F("path", path))
		}
		t.td, t.fd, t.index = td, fd, idx
		t.base = td.FullName + "." + fd.Name
		switch rest := parts[i+1:]; len(rest) {
		case 0:
		case 1:
			t.lane = strings.ToLower(rest[0])
		default:
			return t, errs.New(errs.EditorProps, "nested struct fields are not addressable yet", log.F("path", path))
		}
		if t.lane != "" && t.index >= 0 {
			return t, errs.New(errs.EditorProps, "a path takes a list index or a lane, not both", log.F("path", path))
		}
		return t, nil
	}
	if miss != nil {
		return t, miss
	}
	return t, errs.New(errs.EditorProps, "the path names no component", log.F("path", path))
}

package mutate

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/schema"
)

// Read is the value at a path set takes (transform.position.y, Comp.field, Comp.list[2], script/Enemy.hp, enabled); a lane or element comes alone.
func (d *Document) Read(node, path string) (schema.Value, *schema.FieldDescriptor, error) {
	t, err := d.target(node, path)
	if err != nil {
		return schema.Value{}, nil, err
	}
	var v schema.Value
	var fd *schema.FieldDescriptor
	if t.pose != "" {
		v, fd = d.poseValue(t)
	} else {
		if v, err = d.Scene.GetProperty(t.h, t.td.ID, t.fd.ID); err != nil {
			return schema.Value{}, nil, errs.New(errs.SceneNoComponent, "node has no "+t.td.FullName, log.F("node", d.nodeName(t.h)))
		}
		fd = t.fd
		if t.index >= 0 {
			if fd.Kind != schema.KindList || t.index >= len(v.List) {
				return schema.Value{}, nil, errs.New(errs.EditorProps, "index out of range", log.F("path", path))
			}
			v, fd = v.List[t.index], elemOf(fd)
		}
	}
	if t.lane != "" {
		p := lane(&v, t.lane)
		if p == nil {
			return schema.Value{}, nil, errs.New(errs.EditorProps, "no such part of the value", log.F("path", path))
		}
		return schema.Float32(*p), plain(schema.KindFloat32), nil
	}
	return v, fd, nil
}

// Text is a value as scenes write it, for reports.
func (d *Document) Text(fd *schema.FieldDescriptor, v schema.Value) string {
	return d.text(fd, v)
}

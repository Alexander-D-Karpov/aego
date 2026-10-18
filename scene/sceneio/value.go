package sceneio

import (
	"aego/format/aetext"
	"aego/schema"
)

func ReadValue(reg *schema.Registry, fd *schema.FieldDescriptor, v aetext.Value) (schema.Value, error) {
	l := &loader{reg: reg, report: &Report{}, name: "value"}
	return l.convert(fd, v)
}

func WriteValue(reg *schema.Registry, fd *schema.FieldDescriptor, v schema.Value) aetext.Value {
	w := &writer{reg: reg}
	return w.value(fd, v)
}

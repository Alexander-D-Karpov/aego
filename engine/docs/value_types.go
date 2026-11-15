//go:build !aego_runtime

package docs

import (
	"aego/schema"
	"aego/script/bind/manifest"
	"strings"
)

// Public value structures already exist in the binding manifest; publishing
// their members does not introduce a scripting type or change serialization.
func (r *Registry) valueTypes(m *manifest.Manifest, owners map[string]string) {
	for _, value := range m.Structs {
		id := value.Module + "." + value.Name
		owner := r.ownerOf(value.Module, owners)
		r.add(Entry{ID: id, Parent: value.Module, Kind: Type, Owner: owner, Title: value.Name, ValueType: true,
			Signature: "struct " + value.Name, Body: value.Doc, Location: Location{Stub: StubPath(value.Module), Symbol: "struct:" + value.Name}})
		for _, field := range value.Fields {
			r.add(Entry{ID: id + "." + field.Name, Parent: id, Kind: Field, Owner: owner, Title: field.Name,
				Signature: "field " + field.Name + " " + publicType(field.Type.String())})
		}
	}
}

func schemaFieldType(s *schema.Registry, f *schema.FieldDescriptor) string {
	if f.Kind == schema.KindStruct || f.Kind == schema.KindList && f.Elem == schema.KindStruct {
		if td, ok := s.ByID(f.Struct); ok {
			prefix := ""
			if f.Kind == schema.KindList {
				prefix = "[]"
			}
			return prefix + td.FullName
		}
	}
	return kindName(f)
}

func (r *Registry) componentValueTypes(s *schema.Registry) {
	seen := map[schema.TypeID]bool{}
	var visit func(*schema.TypeDescriptor)
	visit = func(td *schema.TypeDescriptor) {
		if seen[td.ID] {
			return
		}
		seen[td.ID] = true
		owner := td.Owner
		if entry, ok := r.byID[td.FullName]; ok && r.entries[entry].Owner != "" {
			owner = r.entries[entry].Owner
		}
		if owner == "" {
			owner = EngineOwner
		}
		for i := range td.Fields {
			f := &td.Fields[i]
			if f.Kind != schema.KindStruct && !(f.Kind == schema.KindList && f.Elem == schema.KindStruct) {
				continue
			}
			child, ok := s.ByID(f.Struct)
			if !ok {
				continue
			}
			id := td.FullName + "." + exportName(f.Name)
			if index, ok := r.byID[id]; ok && r.entries[index].Field != nil {
				r.entries[index].Field.Type = schemaFieldType(s, f)
			}
			if _, exists := r.byID[child.FullName]; !exists {
				childOwner := child.Owner
				if childOwner == "" {
					childOwner = owner
				}
				title := child.DisplayName
				if title == "" {
					title = child.FullName[strings.LastIndexByte(child.FullName, '.')+1:]
				}
				r.add(Entry{ID: child.FullName, Parent: r.modulePage(childOwner), Kind: Type, Owner: childOwner, ValueType: true,
					Title: title, Signature: "struct " + child.FullName, Body: child.Doc})
				for j := range child.Fields {
					member := &child.Fields[j]
					name := exportName(member.Name)
					r.add(Entry{ID: child.FullName + "." + name, Parent: child.FullName, Kind: Field, Owner: childOwner,
						Title: name, Signature: "field " + name + " " + schemaFieldType(s, member), Body: member.Doc})
				}
			}
			visit(child)
		}
	}
	for _, td := range s.Components() {
		visit(td)
	}
}

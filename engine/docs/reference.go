//go:build !aego_runtime

package docs

import (
	"aego/script/types"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"aego/core/ids"
	"aego/engine/module"
	"aego/engine/settings"
	"aego/format/aetext"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind/manifest"
)

const (
	EventsID       = "events"
	SectionsPrefix = "section/"
	PlayerID       = SectionsPrefix + "player"
	SettingPrefix  = "setting/"
	maxRelated     = 3
	maxChoices     = 8
)

type FieldInfo struct {
	Type    string
	Default string
	Range   string
	Notes   []string
	// Index is the field's place in the schema, the Inspector order; Runtime fields only have values during Play (D347).
	Index   int
	Runtime bool
}

type ComponentInfo struct {
	Category string
	Module   string
	Space    string
	Unique   bool
	Requires []string
	Create   string
	Events   []string
}

func defaultsOf(td *schema.TypeDescriptor) unsafe.Pointer {
	switch {
	case td.Default != nil:
		return td.Default()
	case td.New != nil:
		return td.New()
	}
	return nil
}

func kindName(fd *schema.FieldDescriptor) string {
	switch fd.Kind {
	case schema.KindEnum:
		if fd.EnumName != "" {
			return fd.EnumName
		}
	case schema.KindAssetRef:
		if fd.AssetKind != "" {
			return "AssetRef<" + fd.AssetKind + ">"
		}
		return "AssetRef"
	case schema.KindNodeRef:
		if fd.Component != "" {
			return "ComponentRef<" + fd.Component + ">"
		}
		return "NodeRef"
	case schema.KindList:
		return "[]" + fd.Elem.String()
	}
	return fd.Kind.String()
}

func fnum(f float64, bits int) string {
	return strconv.FormatFloat(f, 'g', 6, bits)
}

func f32s(vs ...float32) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fnum(float64(v), 32)
	}
	return strings.Join(parts, ", ")
}

func formatValue(fd *schema.FieldDescriptor, v schema.Value) string {
	switch v.Kind {
	case schema.KindBool:
		return strconv.FormatBool(v.I != 0)
	case schema.KindInt32, schema.KindInt64:
		return strconv.FormatInt(v.I, 10)
	case schema.KindEnum:
		if fd != nil {
			if n, ok := fd.EnumNameOf(v.I); ok {
				return n
			}
		}
		return strconv.FormatInt(v.I, 10)
	case schema.KindFloat32:
		return fnum(v.F, 32)
	case schema.KindFloat64:
		return fnum(v.F, 64)
	case schema.KindVec2:
		return "(" + f32s(v.V.X, v.V.Y) + ")"
	case schema.KindVec3:
		return "(" + f32s(v.V3.X, v.V3.Y, v.V3.Z) + ")"
	case schema.KindVec4:
		return "(" + f32s(v.V4.X, v.V4.Y, v.V4.Z, v.V4.W) + ")"
	case schema.KindQuat:
		return "(" + f32s(v.Q.X, v.Q.Y, v.Q.Z, v.Q.W) + ")"
	case schema.KindColor:
		return "rgba(" + f32s(v.C.R, v.C.G, v.C.B, v.C.A) + ")"
	case schema.KindRect:
		return "(" + f32s(v.R.X, v.R.Y, v.R.W, v.R.H) + ")"
	case schema.KindString:
		return strconv.Quote(v.S)
	case schema.KindAssetRef:
		if v.Asset.IsZero() {
			return "none"
		}
		return "asset " + v.Asset.String()
	case schema.KindNodeRef:
		if v.Node == (ids.NodeID{}) {
			return "none"
		}
		return "node " + v.Node.String()
	case schema.KindList:
		if len(v.List) == 0 {
			return "[]"
		}
		return "[" + strconv.Itoa(len(v.List)) + " items]"
	case schema.KindCurve:
		return "curve"
	case schema.KindStruct:
		return "{…}"
	}
	return ""
}

func rangeOf(hasMin bool, lo float64, hasMax bool, hi float64, unit string) string {
	s := ""
	switch {
	case hasMin && hasMax:
		s = fnum(lo, 64) + " – " + fnum(hi, 64)
	case hasMin:
		s = "≥ " + fnum(lo, 64)
	case hasMax:
		s = "≤ " + fnum(hi, 64)
	}
	switch {
	case unit == "":
		return s
	case s == "":
		return unit
	}
	return s + " " + unit
}

func source(h schema.HintSource) string {
	if h.List == "" {
		return h.Section
	}
	return h.Section + "." + h.List
}

func fieldNotes(td *schema.TypeDescriptor, fd *schema.FieldDescriptor, component bool) []string {
	var out []string
	if component {
		if !fd.Flags.Has(schema.FlagEditor) {
			out = append(out, "scripts only")
		} else if !td.Scripted(fd) {
			out = append(out, "not in scripts")
		}
	}
	if fd.Flags.Has(schema.FlagReadOnly) && !fd.Hints.Runtime {
		out = append(out, "read-only")
	}
	if fd.Hints.Hidden {
		out = append(out, "hidden in the Inspector")
	}
	if fd.Flags.Has(schema.FlagPersist) {
		out = append(out, "saved with the game")
	}
	if fd.Flags.Has(schema.FlagLive) {
		out = append(out, "live tuning")
	}
	if fd.Hints.Choices.Section != "" {
		out = append(out, "one of "+source(fd.Hints.Choices))
	}
	if fd.Hints.Bitset.Section != "" {
		out = append(out, "mask over "+source(fd.Hints.Bitset))
	}
	if n := len(fd.Enum); n > 0 && n <= maxChoices {
		names := make([]string, n)
		for i, e := range fd.Enum {
			names[i] = e.Name
		}
		out = append(out, "one of: "+strings.Join(names, ", "))
	}
	if fd.Hints.Gizmo != "" {
		out = append(out, "gizmo: "+fd.Hints.Gizmo)
	}
	if fd.Hints.Step > 0 {
		out = append(out, "step "+fnum(fd.Hints.Step, 64))
	}
	return out
}

func fieldInfo(td *schema.TypeDescriptor, fd *schema.FieldDescriptor, base unsafe.Pointer, component bool) *FieldInfo {
	fi := &FieldInfo{
		Type:    kindName(fd),
		Range:   rangeOf(fd.HasMin, fd.Min, fd.HasMax, fd.Max, fd.Hints.Unit),
		Notes:   fieldNotes(td, fd, component),
		Runtime: fd.Hints.Runtime,
	}
	switch {
	case base != nil && fd.Get != nil:
		var v schema.Value
		fd.Get(base, &v)
		fi.Default = formatValue(fd, v)
	case fd.HasDefault:
		fi.Default = formatValue(fd, fd.Default)
	}
	return fi
}

func spaceName(k schema.SpaceKind) string {
	switch k {
	case schema.Space2D:
		return "2D"
	case schema.Space3D:
		return "3D"
	}
	return ""
}

func (r *Registry) componentInfo(td *schema.TypeDescriptor, owner string) *ComponentInfo {
	ci := &ComponentInfo{
		Category: td.Category,
		Module:   r.moduleTitle(owner),
		Space:    spaceName(td.Space),
		Unique:   td.SceneUnique,
		Requires: slices.Clone(td.Requires),
	}
	if t := td.Template; t != nil {
		parts := []string{"Create"}
		menu := t.Menu
		if menu == "" {
			menu = t.Category
		}
		if menu != "" {
			parts = append(parts, menu)
		}
		ci.Create = strings.Join(append(parts, t.Name), " → ")
	}
	for name, o := range r.evOwners {
		if o == owner {
			ci.Events = append(ci.Events, EventsID+"."+name)
		}
	}
	sort.Strings(ci.Events)
	return ci
}

func (r *Registry) compMethods(m *manifest.Manifest) {
	for _, cm := range m.CompMethods {
		i, ok := r.byID[cm.Comp]
		if !ok {
			continue
		}
		short := cm.Comp[strings.LastIndexByte(cm.Comp, '.')+1:]
		if mc, ok := m.Component(cm.Comp); ok && mc.Short != "" {
			short = mc.Short
		}
		id, title := cm.Comp+"."+cm.Name, cm.Name
		if _, dup := r.byID[id]; dup {
			id, title = id+"()", title+"()"
		}
		ps := params(cm.Sig, cm.Params)
		r.add(Entry{
			ID: id, Parent: cm.Comp, Kind: Method, Owner: r.entries[i].Owner, Title: title,
			Signature: signature("func ("+short+") "+cm.Name, ps, cm.Sig.Result), Params: ps, Body: cm.Doc, Example: cm.Example,
			Location: Location{Path: cm.Loc, Stub: StubPath(ComponentsStub), Symbol: "method:" + cm.Comp + "." + cm.Name},
		})
	}
}

func (r *Registry) scriptEvents(m *manifest.Manifest) {
	if len(m.Events) == 0 {
		return
	}
	r.add(Entry{
		ID: EventsID, Kind: Module, Owner: scriptOwner, Group: GroupScriptAPI, Order: -1, Title: "Script events",
		Body: "A script method with one of these names is an event handler: the engine calls it when the event happens, it is never called directly, and it cannot be a coroutine.",
	})
	for _, e := range m.Events {
		ps := params(abi.Signature{Params: e.Params}, e.Names)
		r.add(Entry{
			ID: EventsID + "." + e.Name, Parent: EventsID, Kind: Function, Owner: r.evOwners[e.Name], Title: e.Name,
			Signature: signature("func "+e.Name, ps, abi.T(abi.Void)), Params: ps, Body: e.Doc,
		})
	}
}

func (r *Registry) projectSections(list []*module.Section) {
	for _, s := range list {
		id := SectionsPrefix + s.ID
		var b strings.Builder
		b.WriteString("The `" + s.ID + "` block of `project.aego`, owned by the " + r.moduleTitle(s.Owner) + " module.")
		if s.Version > 1 {
			b.WriteString(" The current version is " + strconv.Itoa(s.Version) + "; older blocks are migrated when the project loads and rewritten on save.")
		}
		if len(s.Former) > 0 {
			b.WriteString(" Older names of the block: `" + strings.Join(s.Former, "`, `") + "`.")
		}
		if n := s.DefaultBlock(); n != nil && len(n.Fields)+len(n.Blocks) > 0 {
			b.WriteString("\n\n## Default block\n\n```aetext\n" + strings.TrimRight(string(aetext.Write(n)), "\n") + "\n```")
		}
		r.add(Entry{ID: id, Kind: Page, Owner: s.Owner, Group: GroupSettings, Order: s.Order, Title: s.Title, Body: b.String()})
		if s.Type == nil {
			continue
		}
		var base unsafe.Pointer
		if v := reflect.ValueOf(s.DefaultValue()); v.Kind() == reflect.Pointer && !v.IsNil() {
			base = v.UnsafePointer()
		}
		for i := range s.Type.Fields {
			fd := &s.Type.Fields[i]
			if !fd.Flags.Has(schema.FlagSerialize) {
				continue
			}
			r.add(Entry{ID: id + "." + fd.Name, Parent: id, Kind: Field, Owner: s.Owner, Title: fd.Name, Body: fd.Doc, Field: fieldInfo(s.Type, fd, base, false)})
		}
	}
}

func settingValue(d *settings.Desc, v schema.Value) string {
	for _, c := range d.Choices {
		if settings.Equal(c.Value, v) {
			return c.Name
		}
	}
	return formatValue(nil, v)
}

func settingInfo(d *settings.Desc) *FieldInfo {
	fi := &FieldInfo{Type: d.Kind.String(), Default: settingValue(d, d.Default), Range: rangeOf(d.HasMin, d.Min, d.HasMax, d.Max, d.Unit)}
	if n := len(d.Choices); n > 0 {
		fi.Type = "choice"
		names := make([]string, 0, min(n, maxChoices))
		for _, c := range d.Choices[:min(n, maxChoices)] {
			names = append(names, c.Name)
		}
		fi.Notes = append(fi.Notes, "one of: "+strings.Join(names, ", "))
	}
	if d.ChoicesFrom.Section != "" {
		fi.Notes = append(fi.Notes, "one of "+source(d.ChoicesFrom))
	}
	for _, f := range [...]struct {
		flag settings.Flags
		note string
	}{
		{settings.User, "player setting"},
		{settings.Project, "project default"},
		{settings.Restart, "applies after a restart"},
		{settings.Confirm, "asks to keep the change"},
		{settings.Transient, "not saved"},
		{settings.Accessibility, "accessibility option"},
		{settings.Controls, "control binding"},
	} {
		if d.Flags.Has(f.flag) {
			fi.Notes = append(fi.Notes, f.note)
		}
	}
	if d.Preset {
		fi.Notes = append(fi.Notes, "preset")
	}
	if d.Category != "" {
		fi.Notes = append(fi.Notes, "category "+d.Category)
	}
	return fi
}

func defaultsExample(list []settings.Desc) string {
	for i := range list {
		d := &list[i]
		switch d.Kind {
		case schema.KindBool, schema.KindInt64, schema.KindFloat64:
			return "\n\n```aetext\ndefaults {\n  " + d.Owner + " {\n    " + d.Path() + "=" + formatValue(nil, d.Default) + "\n  }\n}\n```"
		}
	}
	return ""
}

func (r *Registry) playerSettings(in []settings.Desc) {
	if len(in) == 0 {
		return
	}
	list := slices.Clone(in)
	sort.SliceStable(list, func(i, j int) bool {
		a, b := &list[i], &list[j]
		if oa, ob := r.docsOrder(a.Owner), r.docsOrder(b.Owner); oa != ob {
			return oa < ob
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	r.add(Entry{
		ID: PlayerID, Kind: Page, Owner: EngineOwner, Group: GroupSettings, Order: 1 << 20, Title: "Player settings",
		Body: "Options a player changes on the game's settings screen and the editor keeps in the player profile. A project gives them its own starting values in the `defaults` block of `project.aego`; a value there is used until the player changes it." + defaultsExample(list),
	})
	for i := range list {
		d := &list[i]
		body := d.Doc
		if body == "" {
			body = d.Label() + "."
		}
		r.add(Entry{ID: SettingPrefix + d.ID, Parent: PlayerID, Kind: Field, Owner: d.Owner, Title: d.ID, Body: body, Field: settingInfo(d)})
	}
}

func (r *Registry) related() {
	back := map[string][]string{}
	for i := range r.entries {
		e := &r.entries[i]
		if e.Kind != Page {
			continue
		}
		for _, l := range e.Links {
			target := l
			if j, ok := r.byID[l]; ok && r.entries[j].Kind == Field {
				target = r.entries[j].Parent
			}
			if !slices.Contains(back[target], e.ID) {
				back[target] = append(back[target], e.ID)
			}
		}
	}
	for i := range r.entries {
		e := &r.entries[i]
		section := e.Kind == Page && e.Parent == "" && e.ID != PlayerID && strings.HasPrefix(e.ID, SectionsPrefix)
		if e.Component == nil && !section {
			continue
		}
		rel := back[e.ID]
		if e.Owner != EngineOwner {
			for j := range r.entries {
				p := &r.entries[j]
				if p.Kind != Page || p.Parent != "" || p.Owner != e.Owner || slices.Contains(rel, p.ID) {
					continue
				}
				if g, ok := r.group(p.Group); ok && g.Section != SectionReference {
					rel = append(rel, p.ID)
				}
			}
		}
		if len(rel) > maxRelated {
			rel = rel[:maxRelated]
		}
		e.Related = rel
	}
}

// displayType names a field's type as the reader knows it: enum, layer, layer mask or the script name of an asset kind (D347).
func displayType(fd *schema.FieldDescriptor, t string) string {
	switch {
	case len(fd.Enum) > 0 || fd.Kind == schema.KindEnum:
		return "enum"
	case fd.Hints.Bitset.List == "layers":
		return "layer mask"
	case fd.Hints.Choices.List == "layers":
		return "layer"
	case fd.Kind == schema.KindAssetRef && fd.AssetKind != "":
		if name, ok := types.AssetTypeName(fd.AssetKind); ok {
			return name
		}
	}
	return t
}

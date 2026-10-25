package animation

import (
	"aego/anim"
	"aego/core/errs"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/format/aetext"
)

type PayloadKind uint8

const (
	PayloadNone PayloadKind = iota
	PayloadInt
	PayloadFloat
	PayloadBool
)

var payloadNames = []string{"none", "int", "float", "bool"}

type EventDesc struct {
	Name    string
	Payload PayloadKind
}

type Settings struct {
	Events []EventDesc
}

func sectionErr(msg string) error {
	return errs.New(errs.ProjectBadFile, "animation: "+msg)
}

func eventSection() module.SectionDesc[Settings] {
	return module.SectionDesc[Settings]{
		ID: ID, Title: "Animation", Order: 450,
		Decode: decodeSettings, Encode: encodeSettings, Validate: validateSettings,
		Names: func(s *Settings, list string) []string {
			if list != "events" {
				return nil
			}
			out := make([]string, len(s.Events))
			for i, e := range s.Events {
				out[i] = e.Name
			}
			return out
		},
	}
}

func decodeSettings(n *aetext.Node, s *Settings) error {
	for _, b := range n.Blocks {
		if b.Head != "event" {
			return sectionErr("unknown block " + b.Head)
		}
		e := EventDesc{Name: b.Label}
		if v, ok := b.Attr("payload"); ok {
			name := v.Ident
			if v.Kind == aetext.VString {
				name = v.Str
			}
			found := false
			for i, p := range payloadNames {
				if p == name {
					e.Payload, found = PayloadKind(i), true
				}
			}
			if !found {
				return sectionErr("event " + e.Name + " has unknown payload " + name)
			}
		}
		s.Events = append(s.Events, e)
	}
	return nil
}

func encodeSettings(s *Settings, n *aetext.Node) {
	for _, e := range s.Events {
		b := &aetext.Node{Head: "event", Label: e.Name}
		if e.Payload != PayloadNone && int(e.Payload) < len(payloadNames) {
			b.Attrs = append(b.Attrs, aetext.Field{Name: "payload", Value: aetext.Ident(payloadNames[e.Payload])})
		}
		n.Blocks = append(n.Blocks, b)
	}
}

func validateSettings(s *Settings) error {
	seen := map[string]bool{}
	for _, e := range s.Events {
		switch {
		case e.Name == "":
			return sectionErr("an event needs a name")
		case seen[e.Name]:
			return sectionErr("event " + e.Name + " is declared twice")
		case int(e.Payload) >= len(payloadNames):
			return sectionErr("event " + e.Name + " has an unknown payload")
		}
		seen[e.Name] = true
	}
	return nil
}

func (m *Module) checkClip(c *module.AssetCheck) []problems.Diagnostic {
	clip, err := anim.Unmarshal(c.Source)
	if err != nil {
		return []problems.Diagnostic{{Code: CodeTrack, Severity: problems.Error, Message: err.Error()}}
	}
	var out []problems.Diagnostic
	for _, e := range clip.Events {
		if _, ok := m.names[e.Name]; !ok {
			out = append(out, problems.Diagnostic{Code: CodeEvent, Severity: problems.Error,
				Message: "event " + e.Name + " is not declared in Project Settings → Animation"})
		}
	}
	return out
}

func (m *Module) checkCtrl(c *module.AssetCheck) []problems.Diagnostic {
	g, err := anim.UnmarshalGraph(c.Source)
	if err != nil {
		return []problems.Diagnostic{{Code: CodeCtrlState, Severity: problems.Error, Message: err.Error()}}
	}
	var out []problems.Diagnostic
	for _, st := range g.States {
		if !st.Clip.IsZero() && c.Exists != nil && !c.Exists(st.Clip) {
			out = append(out, problems.Diagnostic{Code: CodeCtrlClip, Severity: problems.Warning,
				Message: "state " + st.Name + ": the clip " + st.Clip.String() + " is not imported"})
		}
	}
	return out
}

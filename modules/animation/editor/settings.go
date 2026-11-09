package editor

import (
	"slices"
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/ext"
	"aego/engine/i18n"
	"aego/modules/animation"
	"aego/ui/widgets"
)

// Hook adds the custom event list to the module's project section.
func Hook(r *ext.Registry) error {
	labels := []i18n.Text{
		i18n.T("editor.animation_settings.events", "Animation Events"),
		i18n.T("editor.animation_settings.payload", "Payload"),
	}
	// Search by either language or the technical section ID, independently of
	// the UI language. Values are snapshots, so no global language switch is
	// needed and no mutable translation catalog is retained.
	keywords := []i18n.Text{{En: animation.ID}}
	seen := map[string]bool{animation.ID: true}
	for _, label := range labels {
		for _, text := range append(i18n.Default().Translations(label.Key), label.En) {
			if text != "" && !seen[text] {
				keywords = append(keywords, i18n.Text{En: text})
				seen[text] = true
			}
		}
	}
	if err := r.SettingsPage(animation.ID, draw, keywords...); err != nil {
		return err
	}
	for i := range r.Editor().Pages {
		if r.Editor().Pages[i].Section == animation.ID {
			r.Editor().Pages[i].Label = i18n.T("editor.animation.title", "Animation")
			break
		}
	}
	return nil
}

func nextName(events []animation.EventDesc) string {
	for i := 1; ; i++ {
		name := "Event" + strconv.Itoa(i)
		if !slices.ContainsFunc(events, func(e animation.EventDesc) bool { return e.Name == name }) {
			return name
		}
	}
}

func validationMessage(s *animation.Settings) string {
	seen := make(map[string]bool, len(s.Events))
	for _, event := range s.Events {
		switch {
		case event.Name == "":
			return i18n.Tr("editor.animation_settings.empty_name", "An event needs a name")
		case seen[event.Name]:
			return i18n.Tr("editor.animation_settings.duplicate", "Event {name} is declared twice", i18n.Str("name", event.Name))
		case event.Payload > animation.PayloadBool:
			return i18n.Tr("editor.animation_settings.invalid_payload", "Event {name} has an unknown payload", i18n.Str("name", event.Name))
		}
		seen[event.Name] = true
	}
	return ""
}

func draw(ed *context.Editor, draft any) bool {
	s, ok := draft.(*animation.Settings)
	if !ok || ed == nil || ed.UI == nil {
		return false
	}
	ui := ed.UI
	widgets.Label(ui, i18n.Tr("editor.animation_settings.events", "Animation Events"))
	widgets.LabelDim(ui, i18n.Tr("editor.animation_settings.built_in", "Built in: Footstep (none), Hit (float)"))
	widgets.LabelDim(ui, i18n.Tr("editor.animation_settings.built_in_readonly", "Built-in events cannot be overridden"))
	changed := false
	if widgets.ButtonRect(ui, ui.ID("add-event"), ui.Cursor().Next(0), i18n.Tr("editor.animation_settings.add_event", "Add event")) {
		s.Events = append(s.Events, animation.EventDesc{Name: nextName(s.Events)})
		changed = true
	}
	for i := range s.Events {
		ui.PushIDInt(i)
		widgets.Separator(ui)
		event := &s.Events[i]
		// Live edits avoid losing the most recent text when Apply is clicked.
		if edited, _ := widgets.TextFieldRect(ui, ui.ID("name"), widgets.Field(ui, i18n.Tr("editor.animation_settings.name", "Name")), &event.Name, true); edited {
			changed = true
		}
		payload := int(event.Payload)
		if widgets.Combo(ui, ui.ID("payload"), widgets.Field(ui, i18n.Tr("editor.animation_settings.payload", "Payload")), &payload,
			[]string{i18n.Tr("editor.animation_settings.none", "None"), "int", "float", "bool"}, false) {
			event.Payload = animation.PayloadKind(payload)
			changed = true
		}
		if widgets.ButtonRect(ui, ui.ID("remove"), ui.Cursor().Next(0), i18n.Tr("editor.animation_settings.delete_event", "Delete event")) {
			ui.CancelFocus()
			s.Events = slices.Delete(s.Events, i, i+1)
			ui.PopID()
			changed = true
			break
		}
		ui.PopID()
	}
	if msg := validationMessage(s); msg != "" {
		r := ui.Cursor().Next(0)
		ui.PushClip(r)
		ui.Text(msg, mathx.Vec2{X: r.X, Y: ui.TextY(r)}, ui.Theme.Error)
		ui.PopClip()
	}
	return changed
}

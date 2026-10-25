package base

import (
	"slices"
	"strconv"

	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
)

const CoreID = "engine"

type Core struct{}

func (Core) Info() module.Info {
	return module.Info{ID: CoreID, Version: engine.Version, Builtin: true, DisplayName: "Core", DocsOrder: 0, DocsIcon: "cat.node"}
}

func (Core) Register(r *module.Registry) error {
	r.Template(schema.TemplateDesc{ID: CoreID + ".empty", Name: "Empty", Node: "Node", Icon: "node.empty", Setup: empty})
	r.Template(schema.TemplateDesc{ID: CoreID + ".empty3d", Name: "Empty", Space: schema.Space3D, Icon: "node.empty", Setup: empty})
	r.System(engine.SystemDesc{ID: engine.AnchorIntents, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, Before: []string{engine.AnchorMotion}, System: anchor{}})
	r.System(engine.SystemDesc{ID: engine.AnchorMotion, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, System: anchor{}})
	if err := registerSettings(r); err != nil {
		return err
	}
	return module.Component[scene.Tags](r, schema.Name(scene.TagsName),
		schema.Doc("Tags names groups this node belongs to; scripts find them with scene.WithTag."),
		schema.FieldDoc("tags", "Dot-separated names like Enemy.Flying; a node tagged Enemy.Flying is also found by Enemy.")).
		Category("Engine").
		Display("Tags").
		Icon("comp.tags").
		Validate(validateTags).
		Err()
}

type anchor struct{}

func (anchor) Run(*engine.Context) {}

func validateTags(_ module.ValidateContext, t *scene.Tags) []problems.Diagnostic {
	var out []problems.Diagnostic
	for i, tag := range t.Tags {
		switch {
		case !scene.ValidTag(tag):
			out = append(out, problems.Diagnostic{Code: "TAG001", Severity: problems.Warning,
				Message: "tag " + strconv.Quote(tag) + " is malformed and ignored; use letters, digits and _ joined by dots"})
		case slices.Contains(t.Tags[:i], tag):
			out = append(out, problems.Diagnostic{Code: "TAG002", Severity: problems.Warning,
				Message: "tag " + strconv.Quote(tag) + " is listed twice"})
		}
	}
	return out
}

func empty(*scene.Scene, scene.NodeHandle) {}

func RegisterTypes(t *scene.Types) error {
	if err := frame.RegisterTypes(t); err != nil {
		return err
	}
	return sceneio.RegisterUnknown(t)
}

func NewTypes() (*scene.Types, error) {
	t := scene.NewTypes(schema.NewRegistry())
	if err := RegisterTypes(t); err != nil {
		return nil, err
	}
	return t, nil
}

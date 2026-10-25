package editor

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/history"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/engine/i18n"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/settings"
	"aego/modules/physics/systems"
	"aego/schema"
)

const (
	CmdDebug     = "physics.debug"
	FlagShapes   = "physics.debug.shapes"
	FlagContacts = "physics.debug.contacts"
	FlagAABBs    = "physics.debug.aabbs"
	FlagJoints   = "physics.debug.joints"
	FlagIslands  = "physics.debug.islands"
	FlagBroad    = "physics.debug.broadphase"
	CmdSettings  = "physics.settings"
	PageID       = settings.ID
	Category     = "Physics"
	layerForces  = "forces"
	partBase     = 32
)

type issueKey struct {
	node ids.NodeID
	comp string
}

type plugin struct {
	ed       *context.Editor
	types    [components.KindCount]*schema.TypeDescriptor
	body     *schema.TypeDescriptor
	distance *schema.TypeDescriptor
	revolute *schema.TypeDescriptor
	index    map[issueKey]string
	rev      uint64
	tx       *history.Transaction
	col      components.Collider
	pts      []mathx.Vec2
	focus    string
}

func lookup(sr *schema.Registry, name string) (*schema.TypeDescriptor, error) {
	td, ok := sr.ByName(name)
	if !ok {
		return nil, errs.New(errs.SchemaUnknownType, "physics component is not registered", log.F("type", name))
	}
	return td, nil
}

func newPlugin(ed *context.Editor, sr *schema.Registry) (*plugin, error) {
	p := &plugin{ed: ed, index: map[issueKey]string{}}
	var err error
	for k := components.Kind(0); k < components.KindCount; k++ {
		if p.types[k], err = lookup(sr, components.Names[k]); err != nil {
			return nil, err
		}
	}
	if p.body, err = lookup(sr, components.NameBody); err != nil {
		return nil, err
	}
	if p.distance, err = lookup(sr, components.NameDistance); err != nil {
		return nil, err
	}
	if p.revolute, err = lookup(sr, components.NameRevolute); err != nil {
		return nil, err
	}
	return p, nil
}

func Hook(e *ext.Registry) error {
	p, err := newPlugin(e.Editor(), e.Schema())
	if err != nil {
		return err
	}
	for k := components.Kind(0); k < components.KindCount; k++ {
		name := components.Names[k]
		if err := e.Gizmo(name, &colliderGizmo{p: p, kind: k, td: p.types[k]}); err != nil {
			return err
		}
		if err := e.Bounds(name, bounds(k)); err != nil {
			return err
		}
		if err := e.Inspector(name, inspect.CustomFunc(p.colliderInspector(k))); err != nil {
			return err
		}
	}
	if err := e.Gizmo(components.NameDistance, &jointGizmo{p: p, td: p.distance}); err != nil {
		return err
	}
	if err := e.Gizmo(components.NameRevolute, &jointGizmo{p: p, td: p.revolute, revolute: true}); err != nil {
		return err
	}
	if err := e.Inspector(components.NameBody, inspect.CustomFunc(p.bodyInspector)); err != nil {
		return err
	}
	e.GameOverlay(p.overlay)
	for _, l := range []struct {
		id    string
		title i18n.Text
		fn    func(*context.VectorSink)
	}{
		{"velocity", i18n.T("i18n.physics.vectors.velocity", "Velocity"), p.velocity},
		{"angular", i18n.T("i18n.physics.vectors.angular_velocity", "Angular Velocity"), p.angular},
		{layerForces, i18n.T("i18n.physics.vectors.forces", "Forces"), p.forces},
		{"contacts", i18n.T("i18n.physics.vectors.contact_impulses", "Contact Impulses"), p.contacts},
		{"gravity", i18n.T("i18n.physics.vectors.gravity", "Gravity"), p.gravity},
	} {
		if err := e.Vectors(l.id, l.title, l.fn); err != nil {
			return err
		}
	}
	if err := e.SettingsPage(settings.ID, p.settingsPage, i18n.T("i18n.physics.gravity", "Gravity"), i18n.T("i18n.physics.substeps", "Substeps"), i18n.T("i18n.physics.velocity_iterations", "Velocity Iterations"), i18n.T("i18n.physics.relax_iterations", "Relax Iterations"), i18n.T("i18n.physics.contact_hertz", "Contact Hertz"), i18n.T("i18n.physics.contact_damping_ratio", "Contact Damping Ratio"), i18n.T("i18n.physics.contact_push_velocity", "Contact Push Velocity"), i18n.T("i18n.physics.restitution_threshold", "Restitution Threshold"), i18n.T("i18n.physics.linear_slop", "Linear Slop"), i18n.T("i18n.physics.sleep_linear", "Sleep Linear"), i18n.T("i18n.physics.sleep_angular", "Sleep Angular"), i18n.T("i18n.physics.sleep_time", "Sleep Time"), i18n.T("i18n.physics.layers", "Layers"), i18n.T("i18n.physics.collision_matrix", "Collision Matrix")); err != nil {
		return err
	}
	e.Profiler(i18n.T("i18n.physics.profiler", "Physics"), true, p.profile)
	if p.ed != nil {
		e.Tick(p.syncChannels)
		o := &p.ed.Overlay
		o.Set(CmdDebug, true)
		o.Set(FlagShapes, true)
		o.Set(FlagJoints, true)
	}
	return p.commands(e)
}

func (p *plugin) commands(e *ext.Registry) error {
	o := &p.ed.Overlay
	toggle := func(id, name string, order int) command.Desc {
		return command.Desc{
			ID: id, Name: name, Category: Category, Order: order,
			Checked: func() bool { return o.On(id) },
			Run: func() error {
				o.Toggle(id)
				return nil
			},
		}
	}
	list := []command.Desc{
		toggle(CmdDebug, i18n.Tr("i18n.physics.debug_overlay", "Debug Overlay"), 100),
		toggle(FlagShapes, i18n.Tr("i18n.physics.shapes", "Shapes"), 200),
		toggle(FlagContacts, i18n.Tr("i18n.physics.contacts", "Contacts"), 210),
		toggle(FlagAABBs, "AABBs", 220),
		toggle(FlagJoints, i18n.Tr("i18n.physics.joints", "Joints"), 230),
		toggle(FlagIslands, i18n.Tr("i18n.physics.islands", "Islands"), 240),
		toggle(FlagBroad, i18n.Tr("i18n.physics.broadphase_tree", "Broadphase Tree"), 250),
		{ID: CmdSettings, Name: "Physics Settings…", Category: Category, Order: 900, Run: func() error {
			if p.ed.ShowSettings != nil {
				p.ed.ShowSettings(PageID)
			}
			return nil
		}},
	}
	for _, d := range list {
		if err := e.Command(d); err != nil {
			return err
		}
	}
	return nil
}

func (p *plugin) sim() *systems.Sim {
	if p.ed == nil || p.ed.Docs == nil {
		return nil
	}
	return physics.SimOf(p.ed.Docs.ActiveWorld())
}

func (p *plugin) playSim() *systems.Sim {
	if p.ed == nil {
		return nil
	}
	return physics.SimOf(p.ed.PlayWorld())
}

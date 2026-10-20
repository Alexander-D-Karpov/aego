package physics

import (
	"aego/contracts/debugdraw"
	"aego/core/mathx"
	"aego/engine"
	"aego/modules/physics/world"
)

const (
	SystemDebug       = ID + ".debug"
	ChannelPrefix     = ID + "."
	ChannelShapes     = ChannelPrefix + "shapes"
	ChannelContacts   = ChannelPrefix + "contacts"
	ChannelAABBs      = ChannelPrefix + "aabbs"
	ChannelJoints     = ChannelPrefix + "joints"
	ChannelIslands    = ChannelPrefix + "islands"
	ChannelBroadphase = ChannelPrefix + "broadphase"
)

var debugChannels = [...]struct {
	name string
	flag world.DebugFlags
}{
	{ChannelShapes, world.DrawShapes},
	{ChannelContacts, world.DrawContacts},
	{ChannelAABBs, world.DrawAABBs},
	{ChannelJoints, world.DrawJoints},
	{ChannelIslands, world.DrawIslands | world.DrawShapes},
}

var treeColors = [...]mathx.Color{
	{R: 0.95, G: 0.45, B: 0.35, A: 0.5}, {R: 0.35, G: 0.75, B: 0.95, A: 0.5}, {R: 0.55, G: 0.9, B: 0.4, A: 0.5},
	{R: 0.95, G: 0.8, B: 0.3, A: 0.5}, {R: 0.8, G: 0.45, B: 0.95, A: 0.5}, {R: 0.3, G: 0.9, B: 0.8, A: 0.5},
}

type drawSink struct {
	d  debugdraw.Draw
	st debugdraw.Style
}

func (s *drawSink) Line(a, b mathx.Vec2, c mathx.Color) {
	s.st.Color = c
	s.d.Line(a, b, &s.st)
}

func (s *drawSink) Circle(center mathx.Vec2, r float32, _ int, c mathx.Color) {
	s.st.Color = c
	s.d.Circle(center, r, &s.st)
}

func (s *drawSink) tree(b world.AABB, height int32, leaf bool) {
	if leaf {
		return
	}
	s.st.Color = treeColors[int(height)%len(treeColors)]
	s.d.AABB(b.Min, b.Max, &s.st)
}

type debugSystem struct {
	sink *drawSink
}

func (ds debugSystem) Run(ctx *engine.Context) {
	sim := SimOf(ctx.World)
	d, ok := engine.ServiceOf[debugdraw.Draw](ctx.World, debugdraw.ServiceID)
	if sim == nil || !ok || !d.Enabled() {
		return
	}
	d.HideByDefault(ChannelPrefix)
	d.ClearChannel(ChannelPrefix)
	s := ds.sink
	s.d, s.st.Life = d, debugdraw.Forever()
	w := sim.World()
	for _, c := range debugChannels {
		if d.Wants(c.name) {
			s.st.Channel = c.name
			w.DebugDraw(s, c.flag)
		}
	}
	if d.Wants(ChannelBroadphase) {
		s.st.Channel = ChannelBroadphase
		w.DebugTree(s.tree)
	}
}

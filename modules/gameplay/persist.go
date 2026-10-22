package gameplay

import (
	"aego/engine"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
)

const respawnBlock = "respawn"

// respawnSection keeps the respawn points marked Persist in save games.
type respawnSection struct{}

func (respawnSection) Version() int {
	return 1
}

func (respawnSection) Migrate(*aetext.Node, int, int) error {
	return nil
}

func (respawnSection) Save(w *engine.World, out *save.Writer) error {
	s := Of(w)
	if s == nil {
		return nil
	}
	scene.Query1[Respawn](s.scene).Each(func(h scene.NodeHandle, r *Respawn) {
		if !r.Persist || !r.st.hasPoint {
			return
		}
		ref, ok := out.RefValue(h)
		if !ok {
			return
		}
		b := out.Block(respawnBlock, "")
		b.Set("node", ref)
		b.Vec2("point", r.st.point)
		b.Int("priority", int64(r.st.priority))
		b.Bool("ranked", r.st.hasPri)
	})
	return nil
}

func (respawnSection) Load(w *engine.World, in *save.Reader) error {
	s := Of(w)
	if s == nil {
		return nil
	}
	for _, b := range in.Blocks(respawnBlock) {
		h, ok := b.Ref("node")
		if !ok {
			continue
		}
		r, ok := scene.Get[Respawn](s.scene, h)
		if !ok {
			continue
		}
		r.st.point, r.st.hasPoint = b.Vec2("point", r.st.point), true
		r.st.priority, r.st.hasPri = int32(b.Int("priority", 0)), b.Bool("ranked", false)
	}
	return nil
}

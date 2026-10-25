package animedit

import (
	"aego/anim"
	"aego/core/curve"
	"aego/core/ids"
	"aego/engine/i18n"
	"aego/format/aetext"
	"aego/scene/mutate"
	"aego/schema"
)

// NewClipText is a new empty clip: one second, looping.
func NewClipText() []byte {
	return aetext.Write(anim.Encode(&anim.Clip{Length: 1, Loop: anim.LoopRepeat}))
}

// NewControllerText is a new controller with one Idle state, laid out.
func NewControllerText() ([]byte, error) {
	root, err := aetext.Parse([]byte("controller version=1 {\n  entry=\"Idle\"\n  state \"Idle\" {}\n}\n"), "new.aectrl")
	if err != nil {
		return nil, err
	}
	g, _, err := anim.DecodeGraph(root)
	if err != nil {
		return nil, err
	}
	g.Layout()
	return aetext.Write(anim.EncodeGraph(g)), nil
}

// FlipbookFPS is the frame rate of a flipbook made from sprites.
const FlipbookFPS = 12

// FlipbookOps key the sprite of the clip's node with one frame per sprite, as step keys.
func FlipbookOps(frames []ids.AssetID) []mutate.Op {
	extra := &aetext.Node{Head: "values", Attrs: []aetext.Field{{Name: "interp", Value: aetext.Ident(curve.InterpStep.String())}}}
	ops := make([]mutate.Op, len(frames))
	for i, id := range frames {
		op := mutate.AnimKeySet("", anim.SpriteProp, timeText(float32(i)/FlipbookFPS), aetext.AssetRef(id.String()))
		op.Comp, op.Values = schema.KindAssetRef.String(), extra
		ops[i] = op
	}
	return ops
}

// NewFlipbookText is a looping clip that shows the sprites one after another.
func NewFlipbookText(frames []ids.AssetID) []byte {
	c := &anim.Clip{Length: float32(len(frames)) / FlipbookFPS, FPS: FlipbookFPS, Loop: anim.LoopRepeat}
	tr := anim.Track{Property: anim.SpriteProp, Kind: schema.KindAssetRef}
	for i, id := range frames {
		tr.Keys = append(tr.Keys, anim.Key{T: float32(i) / FlipbookFPS, V: anim.Asset(id), Interp: curve.InterpStep})
	}
	c.Tracks = append(c.Tracks, tr)
	return aetext.Write(anim.Encode(c))
}

// FlipbookInto adds the flipbook to the open clip, lengthening it when the frames run past its end.
func (s *Service) FlipbookInto(frames []ids.AssetID) error {
	if s.clip == nil {
		return errNoClip
	}
	ops := FlipbookOps(frames)
	if end := float32(len(frames)) / FlipbookFPS; end > s.clip.Length {
		ops = append(ops, mutate.AnimClipSet("length", aetext.Float32(end)))
	}
	return s.EditClip(i18n.Tr("editor.history.flipbook", "Flipbook"), ops...)
}

// KeySprites keys sprite track i with the frames from time at on, one per FlipbookFPS step, lengthening the clip when they run past its end.
func (s *Service) KeySprites(i int, at float32, frames []ids.AssetID) error {
	if s.clip == nil {
		return errNoClip
	}
	if i < 0 || i >= len(s.clip.Tracks) || len(frames) == 0 {
		return nil
	}
	tr := &s.clip.Tracks[i]
	extra := &aetext.Node{Head: "values", Attrs: []aetext.Field{{Name: "interp", Value: aetext.Ident(curve.InterpStep.String())}}}
	ops := make([]mutate.Op, 0, len(frames)+1)
	for k, id := range frames {
		op := mutate.AnimKeySet(tr.Target, tr.Property, timeText(at+float32(k)/FlipbookFPS), aetext.AssetRef(id.String()))
		op.Comp, op.Values = schema.KindAssetRef.String(), extra
		ops = append(ops, op)
	}
	if end := at + float32(len(frames)-1)/FlipbookFPS; end > s.clip.Length {
		ops = append(ops, mutate.AnimClipSet("length", aetext.Float32(end)))
	}
	label := i18n.Tr("editor.animedit.key_sprite", "Key Sprite")
	if len(frames) > 1 {
		label = i18n.Tr("editor.animedit.key_sprites", "Key Sprites")
	}
	return s.EditClip(label, ops...)
}

package mutate

import (
	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
	"bytes"
	"testing"
)

func TestTextureMutationsAtomicStableFramesUnknownUndoAndMissingRefs(t *testing.T) {
	id, frame, normal, missing := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	m := asset.NewSourceMeta("texture", 5, asset.TextureDefaults())
	m.ID = id
	m.Subs = []asset.SourceFrame{{ID: frame, Name: "idle", Rect: [4]int{0, 0, 4, 4}}}
	m.Settings["unknown"] = aetext.String("keep")
	env := TextureEnv{Width: 8, Height: 4, Resolve: func(id ids.AssetID) (asset.TextureSourceInfo, bool) {
		return asset.TextureSourceInfo{ID: id, Width: 4, Height: 4, Role: asset.RoleNormal, Pivot: [2]float32{.5, .5}}, id == normal
	}}
	d, err := LoadTexture(asset.MarshalSourceMeta(m), "hero.png.meta", env)
	if err != nil {
		t.Fatal(err)
	}
	before := d.Bytes()
	tx := Tx{Label: "surface", Ops: []Op{TextureFrameRename(frame, "run"), TextureChannel(frame, "normal", normal)}}
	encoded, err := tx.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTx(encoded, "tx")
	if err != nil {
		t.Fatal(err)
	}
	res, err := d.Apply(parsed)
	if err != nil {
		t.Fatal(err)
	}
	after := d.Bytes()
	if ref, _, _ := asset.TextureChannel(d.Doc(), frame, "normal"); ref != normal {
		t.Fatal("lost stable frame link")
	}
	if _, err = d.Apply(res.Inverse); err != nil || !bytes.Equal(before, d.Bytes()) {
		t.Fatal("undo did not restore metadata exactly", err)
	}
	if _, err = d.Apply(res.Redo); err != nil || !bytes.Equal(after, d.Bytes()) {
		t.Fatal("redo changed frame IDs", err)
	}
	if _, err = d.Apply(Tx{Ops: []Op{TextureSetting("filter", aetext.Ident("linear")), TextureFrameRect(frame, [4]int{7, 0, 4, 4})}}); err == nil || !bytes.Equal(after, d.Bytes()) {
		t.Fatal("failed metadata batch partially mutated source")
	}
	if _, err = d.Apply(Tx{Ops: []Op{TextureChannel(frame, "normal", missing)}}); err != nil {
		t.Fatal("missing reference should retain ID for repair", err)
	}
	if ref, explicit, _ := asset.TextureChannel(d.Doc(), frame, "normal"); ref != missing || !explicit || len(d.Problems()) == 0 {
		t.Fatal("missing reference or diagnostic lost")
	}
	if _, err = d.Apply(Tx{Ops: []Op{TextureFrameRemove(frame)}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(d.Bytes(), []byte(missing.String())) {
		t.Fatal("deleting a frame erased unknown channel data")
	}
}
func TestTextureChannelsRejectKnownIncompatibleRoleLogicalSizeAndPivot(t *testing.T) {
	m := asset.NewSourceMeta("texture", 5, asset.TextureDefaults())
	target := ids.NewAssetID()
	for _, info := range []asset.TextureSourceInfo{{Width: 8, Height: 8, Role: asset.RoleColor, Pivot: [2]float32{.5, .5}}, {Width: 4, Height: 8, Role: asset.RoleNormal, Pivot: [2]float32{.5, .5}}, {Width: 8, Height: 8, Role: asset.RoleNormal, Pivot: [2]float32{0, 0}}} {
		d, _ := LoadTexture(asset.MarshalSourceMeta(m), "hero.png.meta", TextureEnv{Width: 8, Height: 8, Resolve: func(ids.AssetID) (asset.TextureSourceInfo, bool) { return info, true }})
		before := d.Bytes()
		if _, err := d.Apply(Tx{Ops: []Op{TextureChannel(m.ID, "normal", target)}}); err == nil || !bytes.Equal(before, d.Bytes()) {
			t.Fatal("incompatible channel accepted", info)
		}
	}
}

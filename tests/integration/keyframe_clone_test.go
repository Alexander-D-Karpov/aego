package integration

import (
	"reflect"
	"slices"
	"testing"

	"aego/app/assembly"
	playmods "aego/app/assembly/playground"
	"aego/asset"
	"aego/core/curve"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/format/aetext"
	"aego/modules/particles"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func TestShapeSnapshotClonesCoverMutableFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"occluder", reflect.TypeFor[render2d.LightOccluder2D](), []string{"Polygon"}},
		{"edge", reflect.TypeFor[components.EdgeCollider2D](), []string{"Points"}},
	} {
		var found []string
		var walk func(reflect.Type, string)
		walk = func(typ reflect.Type, name string) {
			switch typ.Kind() {
			case reflect.Slice:
				found = append(found, name)
				walk(typ.Elem(), name+"[]")
			case reflect.Array:
				walk(typ.Elem(), name+"[]")
			case reflect.Struct:
				for i := 0; i < typ.NumField(); i++ {
					f := typ.Field(i)
					field := f.Name
					if name != "" {
						field = name + "." + field
					}
					walk(f.Type, field)
				}
			case reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
				found = append(found, name+" ("+typ.Kind().String()+")")
			}
		}
		walk(tc.typ, "")
		slices.Sort(found)
		if !slices.Equal(found, tc.want) {
			t.Errorf("%s mutable fields changed: %v; update its clone and snapshot tests", tc.name, found)
		}
	}
}

func TestKeyframeComponentsOwnTheirMutableData(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := playmods.Catalog().Load(types, bind.New(), assembly.Options{Seed: 1}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Clone isolation")
	p := scene.Add[particles.ParticleSystem2D](s, h)
	*p = particles.DefaultParticleSystem2D()
	p.Frames = []asset.Ref[asset.Texture]{asset.NewRef[asset.Texture](ids.AssetID{Lo: 123})}
	p.Bursts = []mathx.Vec2{{X: .1, Y: 2}}
	p.ColorOverLife = []particles.ColorKey{{Time: .2, Color: mathx.White}}
	p.VelocityOverLife = curve.Constant(3)
	p.SizeOverLife = curve.Constant(4)
	p.AlphaOverLife = curve.Constant(5)
	o := scene.Add[render2d.LightOccluder2D](s, h)
	*o = render2d.LightOccluder2D{Polygon: []mathx.Vec2{{X: 9, Y: 10}}}
	e := scene.Add[components.EdgeCollider2D](s, h)
	*e = components.EdgeCollider2D{Points: []mathx.Vec2{{X: 11, Y: 12}}, Loop: true}
	pi := scene.Add[sceneio.PrefabInstance](s, h)
	nodeID := ids.NodeID{Lo: 77}
	*pi = sceneio.PrefabInstance{Owner: true, Keys: map[ids.NodeID]prefab.Key{nodeID: "key"},
		Orphans: prefab.Overrides{Order: []prefab.ChildOrder{{Parent: "parent", Children: []prefab.Key{"child"}}}},
		Extra:   []*aetext.Node{{Head: "future", Fields: []aetext.Field{{Name: "v", Value: aetext.List(aetext.Int(13))}}}},
		Block:   &aetext.Node{Head: "block", Blocks: []*aetext.Node{{Head: "nested"}}},
	}
	s.Flush()
	key := s.Capture(nil)
	for pass := 0; pass < 3; pass++ {
		p, _ = scene.Get[particles.ParticleSystem2D](s, h)
		o, _ = scene.Get[render2d.LightOccluder2D](s, h)
		e, _ = scene.Get[components.EdgeCollider2D](s, h)
		pi, _ = scene.Get[sceneio.PrefabInstance](s, h)
		p.Frames[0].ID.Lo = 999
		p.Bursts[0].X = 99
		p.ColorOverLife[0].Time = 99
		p.VelocityOverLife.Keys[0].V = 99
		p.SizeOverLife.Keys[0].V = 99
		p.AlphaOverLife.Keys[0].V = 99
		o.Polygon[0].X = 99
		e.Points[0].X = 99
		e.Loop = false
		pi.Keys[nodeID] = "changed"
		pi.Orphans.Order[0].Children[0] = "changed"
		pi.Extra[0].Fields[0].Value.List[0].Int = 99
		pi.Block.Blocks[0].Head = "changed"
		s.Restore(key)
		p, _ = scene.Get[particles.ParticleSystem2D](s, h)
		o, _ = scene.Get[render2d.LightOccluder2D](s, h)
		e, _ = scene.Get[components.EdgeCollider2D](s, h)
		pi, _ = scene.Get[sceneio.PrefabInstance](s, h)
		if p.Frames[0].ID.Lo != 123 || p.Bursts[0].X != .1 || p.ColorOverLife[0].Time != .2 || p.VelocityOverLife.Keys[0].V != 3 || p.SizeOverLife.Keys[0].V != 4 || p.AlphaOverLife.Keys[0].V != 5 {
			t.Fatalf("particle snapshot aliased on restore %d: %+v", pass, p)
		}
		if o.Polygon[0].X != 9 || e.Points[0].X != 11 || !e.Loop {
			t.Fatalf("shape snapshot aliased on restore %d", pass)
		}
		if pi.Keys[nodeID] != "key" || pi.Orphans.Order[0].Children[0] != "child" || pi.Extra[0].Fields[0].Value.List[0].Int != 13 || pi.Block.Blocks[0].Head != "nested" {
			t.Fatalf("prefab snapshot aliased on restore %d", pass)
		}
	}
	// Reusing a snapshot must replace old lists/maps rather than retaining them.
	p.Frames = nil
	p.Bursts = nil
	p.ColorOverLife = nil
	p.VelocityOverLife.Keys = nil
	p.SizeOverLife.Keys = nil
	p.AlphaOverLife.Keys = nil
	o.Polygon = nil
	e.Points = nil
	pi.Keys = nil
	pi.Extra = nil
	pi.Block = nil
	pi.Orphans = prefab.Overrides{}
	key = s.Capture(key)
	p.Frames = []asset.Ref[asset.Texture]{{ID: ids.AssetID{Lo: 999}}}
	o.Polygon = []mathx.Vec2{{X: 99}}
	pi.Keys = map[ids.NodeID]prefab.Key{nodeID: "changed"}
	s.Restore(key)
	p, _ = scene.Get[particles.ParticleSystem2D](s, h)
	o, _ = scene.Get[render2d.LightOccluder2D](s, h)
	e, _ = scene.Get[components.EdgeCollider2D](s, h)
	pi, _ = scene.Get[sceneio.PrefabInstance](s, h)
	if p.Frames != nil || p.Bursts != nil || p.ColorOverLife != nil || p.VelocityOverLife.Keys != nil || p.SizeOverLife.Keys != nil || p.AlphaOverLife.Keys != nil || o.Polygon != nil || e.Points != nil || pi.Keys != nil || pi.Extra != nil || pi.Block != nil || !pi.Orphans.Empty() {
		t.Fatal("reused key retained previous mutable values")
	}
}

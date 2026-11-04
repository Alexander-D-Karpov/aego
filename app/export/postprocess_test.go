package export

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/asset"
	"aego/asset/importers/scenefile"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/render2d"
	"aego/project"
	"aego/render/postfx"
	"aego/runtime/bundle"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/script/bind"
)

func TestPostProcessStrictExport(t *testing.T) {
	const file = "scenes/main.aescene"
	const node = "d777a4e656b70fdff88f889abc615020"
	const active = `{typeID="postfx.vignette" instanceID="edges" enabled=true version=1 parameters=[{name="Intensity" kind=float value=[0.33,0,0,0]}]}`
	unknown := fmt.Sprintf(`{typeID="future.effect" instanceID="future" enabled=false version=3 parameters=[{name="FutureMap" kind=texture asset=asset(%q)}]}`, asset.BuiltinWhite.String())
	fixture := func(fields string) string {
		return fmt.Sprintf(`scene version=1 {
 node %q name="Player camera" enabled=true {
  component "engine.Camera2D" version=1 { active=true }
  component "render2d.PostProcess" version=1 { %s }
 }
}`, node, fields)
	}
	p, err := project.Create(t.TempDir(), "Output", false)
	if err != nil {
		t.Fatal(err)
	}
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(scenefile.NewScene()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()
	for _, tc := range []struct{ name, fields, code, want string }{
		{"disabled unknown", "enabled=true effects=[" + active + "," + unknown + "]", "", ""},
		{"required unknown", "enabled=true effects=[" + strings.Replace(unknown, "enabled=false", "enabled=true", 1) + "]", "PFX001", "future.effect"},
		{"mismatched version", "enabled=true effects=[" + strings.Replace(active, "version=1", "version=3", 1) + "]", "PFX001", "edges"},
		{"wrong parameter kind", "enabled=true effects=[" + strings.Replace(active, "kind=float", "kind=vec3", 1) + "]", "PFX001", "Intensity"},
		{"duplicate identity", "enabled=true effects=[" + active + "," + active + "]", "PFX001", "edges"},
		{"malformed list", `enabled=true effects="not a list"`, CodeScene, "effects"},
		{"malformed checkbox", `enabled="typo" effects=[]`, CodeScene, "enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeMaterialExportSource(t, p.Root, file, fixture(tc.fields))
			if _, _, err := pl.Scan(); err != nil {
				t.Fatal(err)
			}
			if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
				t.Fatalf("fixture import: %v %+v", err, batch.Failed)
			}
			p.MainScene, _ = pl.Lookup(file)
			snap := Take(pl)
			ds := Check(p, snap, bundle.Release, false)
			if tc.code == "" {
				if Blocking(ds) != 0 {
					t.Fatal(ds)
				}
				checkUnknownEffectRoundTrip(t, p, snap)
				return
			}
			for _, d := range ds {
				if d.Code == tc.code && d.Severity == problems.Error && d.Subject.File == file && strings.Contains(d.Message, tc.want) {
					if tc.code == "PFX001" && d.Subject.Node.String() != node {
						t.Fatalf("diagnostic lost camera identity: %+v", d)
					}
					return
				}
			}
			t.Fatalf("missing %s containing %q: %+v", tc.code, tc.want, ds)
		})
	}
}

func checkUnknownEffectRoundTrip(t *testing.T, p *project.Project, snap *Snapshot) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Runtime().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	res := &resolver{s: snap}
	src, ok := res.blob(p.MainScene, asset.ArtScene)
	if !ok {
		t.Fatal("missing frozen scene")
	}
	var id ids.NodeID
	var want []postfx.Instance
	for i := 0; i < 2; i++ {
		sc, rep, err := sceneio.Load(src, "scenes/main.aescene", types, res, sceneio.LoadOptions{KeepIDs: true})
		if err != nil {
			t.Fatal(err)
		}
		var got []postfx.Instance
		scene.Store[render2d.PostProcess](sc).EachAll(func(h scene.NodeHandle, pp *render2d.PostProcess) {
			got = pp.Effects
			if i == 0 {
				id = sc.ID(h)
			} else if sc.ID(h) != id {
				t.Fatal("save changed camera identity")
			}
		})
		if len(got) != 2 || got[0].InstanceID != "edges" || got[1].TypeID != "future.effect" || got[1].Enabled || got[1].Version != 3 || len(got[1].Parameters) != 1 || got[1].Parameters[0].Asset.ID != asset.BuiltinWhite {
			t.Fatalf("unknown typed effect not preserved: %+v; report=%+v fixture=%s", got, rep, src)
		}
		if i == 0 {
			want, src = got, sceneio.Save(sc)
		} else if !reflect.DeepEqual(got, want) {
			t.Fatalf("save/load changed stack: %+v vs %+v", got, want)
		}
	}
}

type opaquePostEffect struct{}

func (opaquePostEffect) Info() module.Info { return module.Info{ID: "thirdparty"} }

func (opaquePostEffect) Register(r *module.Registry) error {
	r.Provide(postfx.ProvisionKind, postfx.Descriptor{
		ID: "thirdparty.opaque", Label: "Opaque effect", Version: 1, Phase: postfx.DisplayLinear, Alpha: postfx.OpaqueOnly,
		Fragment: "vec4 aego_effect(vec4 c, vec2 uv, vec2 size, vec4 p[4]) { return vec4(c.rgb, 1.0); }",
	})
	return nil
}

func TestPostProcessTargetModuleOutputProfile(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Runtime().Load(types, bind.New(), assembly.Options{}, opaquePostEffect{}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "Camera")
	cam := scene.Add[render2d.Camera2D](s, h)
	*cam = render2d.DefaultCamera2D()
	c := scene.Add[render2d.PostProcess](s, h)
	*c = render2d.PostProcess{Enabled: true, Effects: []postfx.Instance{{TypeID: "thirdparty.opaque", InstanceID: "glow", Enabled: true, Version: 1}}}
	s.Flush()
	if ds := module.ValidateScene(s, nil); Blocking(ds) != 0 {
		t.Fatalf("registered module's opaque camera: %+v", ds)
	}
	cam, _ = scene.Get[render2d.Camera2D](s, h)
	cam.ClearColor.A = .5
	ds := module.ValidateScene(s, nil)
	if !hasExportError(ds, "PFX001", "opaque output") {
		t.Fatalf("accepted incompatible transparent output: %+v", ds)
	}
}

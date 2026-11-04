package particles

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/rng"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/res"
	"aego/engine/settings"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render"
	"aego/render/material"
	"aego/render/shader"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
	"aego/vfs"
	"testing"
)

type proofAttachDevice struct{ gpu.Device }
type proofAttachAssets struct {
	asset.Resolver
	t         *testing.T
	allow     bool
	reads     int
	materials map[ids.AssetID]*material.Material
}

func (r *proofAttachAssets) Generation() uint32 {
	if !r.allow {
		r.t.Fatal("inactive attach queried asset generation")
	}
	return 1
}
func (r *proofAttachAssets) Material(id ids.AssetID) (*material.Material, bool) {
	if !r.allow {
		r.t.Fatalf("inactive attach resolved %s", id)
	}
	r.reads++
	m, ok := r.materials[id]
	return m, ok
}

func TestIndependentAttachDoesNotTouchDeviceOrResolveInInactiveScenes(t *testing.T) {
	for _, name := range []string{"empty", "disabled-parent", "disabled-project", "active-two-materials"} {
		t.Run(name, func(t *testing.T) {
			live := name == "active-two-materials"
			types, e := base.NewTypes()
			if e != nil {
				t.Fatal(e)
			}
			m := New()
			if e = m.Register(module.New(types, nil)); e != nil {
				t.Fatal(e)
			}
			if e = types.Schema().Freeze(); e != nil {
				t.Fatal(e)
			}
			assets := &proofAttachAssets{t: t, materials: map[ids.AssetID]*material.Material{}}
			for _, s := range []struct {
				m, sh ids.AssetID
				b     shader.Blend
			}{
				{asset.BuiltinMaterialUnlit, asset.BuiltinShaderUnlit, shader.BlendPremultiplied},
				{asset.BuiltinMaterialAdditive, asset.BuiltinShaderAdditive, shader.BlendAdditive},
			} {
				raw, ok := asset.BuiltinShader(s.sh)
				if !ok {
					t.Fatal("missing builtin shader")
				}
				def, err := asset.DecodeShader(raw)
				if err != nil {
					t.Fatal(err)
				}
				mat, ok := material.New(s.m.String(), def.Runtime(s.sh, s.sh.String()), 0, s.b)
				if !ok {
					t.Fatal("missing default variant")
				}
				assets.materials[s.m] = mat
			}
			sc := scene.New(types, assets, scene.Options{})
			w := engine.NewWorld("attach-proof")
			w.Random = rng.New(42)
			if name != "empty" {
				parent := sc.Create(sc.Root(), "parent")
				for _, sub := range []string{"ordinary-manual", "explicit-additive", "same-additive-again"} {
					n := sc.Create(parent, sub)
					c := scene.Add[ParticleSystem2D](sc, n)
					*c = DefaultParticleSystem2D()
					c.PlayOnStart = false
					c.Rate = 0
					if sub != "ordinary-manual" {
						c.Material.ID = asset.BuiltinMaterialAdditive
					}
				}
				if name == "disabled-parent" {
					sc.SetEnabled(parent, false)
				}
			}
			sc.Flush()
			sc.UpdateTransforms()
			store := settings.New()
			_, e = store.Register(settings.Desc{ID: KeyEnabled, Owner: ID, Kind: schema.KindBool, Default: schema.Bool(name != "disabled-project"), Flags: settings.Project | settings.User})
			if e != nil {
				t.Fatal(e)
			}
			store.Freeze()
			store.Start()
			if e = m.Init(&module.InitContext{Settings: store}); e != nil {
				t.Fatal(e)
			}
			files, e := vfs.New(vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true})
			if e != nil {
				t.Fatal(e)
			}
			dev := null.New()
			defer dev.Close()
			proxy := &proofAttachDevice{Device: dev}
			caps := dev.Caps()
			caps.ESSL = true
			rnd, e := render.New(proxy, caps, files, log.Nop(), render.Options{MaxSprites: 16})
			if e != nil {
				t.Fatal(e)
			}
			defer func() { proxy.Device = dev; rnd.Close() }()
			r := frame.New()
			r.Bind(sc, rnd, assets)
			w.SetView(frame.ServiceID, r)
			rngAtEntry := w.Random.State()
			before := dev.ResourceStats()
			// Forward no GPU methods in a negative case. An accidental Caps(),
			// Create*(), BeginFrame() or draw-through method will panic here.
			assets.allow = live
			if !live {
				proxy.Device = nil
			}
			ctx := &module.AttachContext{Play: true, World: w, Scene: sc}
			if e = m.AttachWorld(ctx); e != nil {
				t.Fatal(e)
			}
			proxy.Device = dev
			defer m.DetachWorld(ctx)
			after := dev.ResourceStats()
			ng := after.Shaders - before.Shaders
			service := Of(w)
			if service == nil || len(service.systems) != 0 || service.allocated != 0 || service.pendingBirths != 0 || w.Random.State() != rngAtEntry {
				t.Fatal("attach changed simulation or rng")
			}
			if live {
				if assets.reads < 2 || ng != 2 {
					t.Fatalf("active control did not resolve the real pair, reads=%d shaderDelta=%d", assets.reads, ng)
				}
			} else if before != after || assets.reads != 0 {
				t.Fatalf("inactive attach changed resources: %+v -> %+v resolverReads=%d", before, after, assets.reads)
			}
			t.Logf("allowGPU=%v materialReads=%d shaderDelta=%d pipelineDelta=%d allocatedParticles=%d RNG unchanged", live, assets.reads, ng, after.Pipelines-before.Pipelines, service.allocated)
		})
	}
}

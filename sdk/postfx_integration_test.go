package sdk_test

import (
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/render2d"
	"aego/scene"
	"aego/sdk"
	"testing"
)

func TestProjectSDKOnlyPostEffectRegistersAndAnimates(t *testing.T) {
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	m, err := module.Load(types, nil, render2d.New(), sdk.Adapt(outlineGrade{}))
	if err != nil {
		t.Fatal(err)
	}
	if err = types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	value, ok := m.RegisteredService(render2d.PostEffectsService)
	if !ok {
		t.Fatal("no registry")
	}
	reg := value.(*sdk.PostEffectRegistry)
	sc := scene.New(types, nil, scene.Options{})
	camera := sc.Create(sc.Root(), "Camera")
	*scene.Add[sdk.Camera2D](sc, camera) = render2d.DefaultCamera2D()
	a, err := reg.NewInstance("demo.gain", "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := reg.NewInstance("demo.gain", "b")
	if err != nil {
		t.Fatal(err)
	}
	c := scene.Add[sdk.PostProcess](sc, camera)
	*c = sdk.PostProcess{Enabled: true, Effects: []sdk.PostEffect{b, a}}
	sc.Flush()
	if err = animateGain(sc, camera, "a"); err != nil {
		t.Fatal(err)
	}
	c, _ = scene.Get[sdk.PostProcess](sc, camera)
	if c.Effects[0].Parameters[0].Value.X != 1 || c.Effects[1].Parameters[0].Value.X != 1.5 {
		t.Fatal("SDK write targeted wrong instance")
	}
	if err := sdk.SetPostEffectParameter(sc, camera, "a", "Gain", sdk.PostIntValue(1)); err == nil {
		t.Fatal("type mismatch was accepted")
	}
}

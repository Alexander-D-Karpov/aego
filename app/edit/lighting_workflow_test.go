package edit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	gameexport "aego/app/export"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/editor/core/templates"
	"aego/engine/res"
	"aego/modules/render2d"
	"aego/modules/tween"
	"aego/platform/input"
	"aego/render/lighting"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
	"aego/schema"
)

func TestLightingAuthoringWorkflowHeadless(t *testing.T) { lightingAuthoringWorkflow(t, headless()) }

func lightingAuthoringWorkflow(t *testing.T, cfg Config) {
	t.Helper()
	p := newProject(t)
	const scenePath = "scenes/lighting-workflow.aescene"
	const shaderPath = "assets/workflow-lit.aeshader"
	const materialPath = "assets/materials/workflow-lit.aemat"
	source, err := fs.ReadFile(res.FS(), "shaders/materials/lit.aeshader")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(p.Root, shaderPath), source, 0644); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		path  string
		color color.NRGBA
	}{
		{"assets/workflow.png", color.NRGBA{R: 210, G: 180, B: 140, A: 220}},
		{"assets/workflow_n.png", color.NRGBA{R: 191, G: 223, B: 223, A: 255}},
	} {
		im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				im.SetNRGBA(x, y, entry.color)
			}
		}
		var data bytes.Buffer
		if err = png.Encode(&data, im); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(p.Root, entry.path), data.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		for i, name := range []string{"SurfaceA", "SurfaceB"} {
			h := sc.Create(sc.Root(), name)
			sr := scene.Add[render2d.SpriteRenderer](sc, h)
			*sr = render2d.DefaultSpriteRenderer()
			sr.Size = mathx.Vec2{X: 2, Y: 2}
			pose := sc.Local(h)
			pose.Position.X = float32(i * 3)
			sc.SetLocal(h, pose)
		}
	})
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	doc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	if (ed.Renderer.Caps().Renderer == "null") != cfg.Headless {
		t.Fatal("workflow did not use the requested real/headless backend")
	}
	sc := doc.Scene()
	lookup := func(path string) ids.AssetID {
		t.Helper()
		id, ok := ed.Pipeline.Lookup(path)
		if !ok {
			t.Fatalf("asset was not imported: %s", path)
		}
		return id
	}
	shaderID, normalID, albedoID := lookup(shaderPath), lookup("assets/workflow_n.png"), lookup("assets/workflow.png")
	p.MainScene = lookup(scenePath)

	// Exercise actual searchable Create palette selection with Enter; the action
	// executed by the UI is the same template/history path used interactively.
	create := func(query string) ids.NodeID {
		t.Helper()
		var created []ids.NodeID
		palette := s.App.AddPalette()
		palette.CreateNode(ids.NodeID{}, templates.Place{}, nil, func(nodes []ids.NodeID) { created = append(created, nodes...) })
		palette.Palette().SetQuery(query)
		selected := palette.Palette().Selected()
		if selected < 0 || palette.Palette().Item(selected).Label != query {
			t.Fatalf("palette cannot find %q", query)
		}
		state := &input.State{}
		state.BeginFrame()
		ed.UI.Begin(state, mathx.Vec2{X: 1600, Y: 900}, 1)
		palette.Draw()
		ed.UI.End()
		if ed.UI.Focus() == 0 {
			t.Fatal("Create search did not receive keyboard focus")
		}
		state.BeginFrame()
		state.ApplyKeyDown(input.KeyEnter, 0, 0, false)
		ed.UI.Begin(state, mathx.Vec2{X: 1600, Y: 900}, 1)
		palette.Draw()
		ed.UI.End()
		if len(created) != 1 || palette.Active() {
			t.Fatalf("Enter did not create/close %q: %v", query, created)
		}
		return created[0]
	}
	lightID := create("Spot Light")
	light, _ := scene.Get[render2d.Light2D](sc, sc.Handle(lightID))
	if light == nil || light.Type != lighting.Spot || light.Height <= 0 {
		t.Fatal("Spot template defaults are invalid")
	}
	if err = doc.History.Undo(); err != nil || !sc.Handle(lightID).IsZero() {
		t.Fatal("Create undo failed", err)
	}
	if err = doc.History.Redo(); err != nil || sc.Handle(lightID).IsZero() {
		t.Fatal("Create redo changed its node identity", err)
	}
	occluderID, environmentID := create("Light Occluder"), create("Lighting Environment")
	set := func(node ids.NodeID, component, field string, value schema.Value) {
		t.Helper()
		td, ok := sc.Registry().ByName(component)
		if !ok {
			t.Fatal(component)
		}
		fd, ok := td.Field(field)
		if !ok {
			t.Fatal(field)
		}
		if err = doc.Props.Set(nil, []ids.NodeID{node}, td.ID, fd.ID, value); err != nil {
			t.Fatal(err)
		}
	}
	set(lightID, "engine.Light2D", "height", schema.Float32(2))
	set(lightID, "engine.Light2D", "shadows", schema.Bool(true))
	set(lightID, "engine.Light2D", "range", schema.Float32(6))
	set(environmentID, "engine.LightingEnvironment2D", "ambientIntensity", schema.Float32(.15))
	set(environmentID, "engine.LightingEnvironment2D", "maxShadowedLights", schema.Int64(3))
	beforePose := sc.Local(sc.Handle(lightID))
	afterPose := beforePose
	afterPose.Position = mathx.Vec2{X: -2, Y: -1}
	if err = doc.History.Exec(&commands.SetTransform{Node: lightID, Before: beforePose, After: afterPose}); err != nil {
		t.Fatal(err)
	}
	poly := render2d.DefaultLightOccluder2D()
	poly.Polygon = []mathx.Vec2{{X: -.2, Y: -.8}, {X: .2, Y: -.8}, {X: .2, Y: .8}, {X: -.2, Y: .8}}
	ocType, _ := sc.Registry().ByName("engine.LightOccluder2D")
	polyField, _ := ocType.Field("polygon")
	var polyValue schema.Value
	polyField.Get(unsafe.Pointer(&poly), &polyValue)
	set(occluderID, ocType.FullName, "polygon", polyValue)
	lightType, _ := sc.Registry().ByName("engine.Light2D")
	rangeField, _ := lightType.Field("range")
	drag := doc.History.Begin("Light range")
	for _, value := range []float32{3, 4, 5} {
		if err = doc.Props.Set(drag, []ids.NodeID{lightID}, lightType.ID, rangeField.ID, schema.Float32(value)); err != nil {
			t.Fatal(err)
		}
	}
	drag.Commit()
	if err = doc.History.Undo(); err != nil {
		t.Fatal(err)
	}
	light, _ = scene.Get[render2d.Light2D](sc, sc.Handle(lightID))
	if light.Range != 6 {
		t.Fatal("one range drag did not undo in one step")
	}
	if err = doc.History.Redo(); err != nil {
		t.Fatal(err)
	}
	light, _ = scene.Get[render2d.Light2D](sc, sc.Handle(lightID))
	if light.Range != 5 {
		t.Fatal("range redo")
	}

	found := false
	for _, action := range ed.CreateActions {
		if action.Key == "render2d.material-from-shader" {
			if err = action.Run(ids.NodeID{}); err != nil {
				t.Fatal(err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Material from Shader is absent from Create")
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("material creation form missing")
	}
	questions[0].Submit([]string{materialPath, shaderID.String()})
	materialID := lookup(materialPath)
	md, err := ed.Docs.OpenScript(materialPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.ApplyMaterialMutation(md, ed.Loader.Shader, mutate.Tx{Label: "Lit surface", Ops: []mutate.Op{
		mutate.MaterialSet("NormalMap", fmt.Sprintf("asset(%q)", normalID.String())), mutate.MaterialSet("NormalStrength", "0.65"), mutate.MaterialSet("ReceiveShadows", "true"),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = ed.Docs.SaveScript(md); err != nil {
		t.Fatal(err)
	}
	cache := ed.Renderer.Materials()
	commit := func() { cache.Begin(); cache.End() }
	commit()
	live, ok := ed.Loader.Material(materialID)
	if !ok {
		t.Fatal("Lit material did not load through artifacts")
	}
	if region, explicit, resolved := live.Normal(); !explicit || !resolved || region.IsZero() {
		t.Fatal("explicit imported normal did not resolve")
	}
	for _, name := range []string{"SurfaceA", "SurfaceB"} {
		id := sc.ID(sc.Find(sc.Root(), name))
		set(id, "engine.SpriteRenderer", "material", schema.Asset(materialID))
		set(id, "engine.SpriteRenderer", "sprite", schema.Asset(albedoID))
	}
	if err = ed.Docs.Save(doc.ID); err != nil {
		t.Fatal(err)
	}
	s.Step()
	beforeScene := sceneio.Save(sc)
	for _, name := range []string{"render2d.lighting.toggle", "render2d.lighting.toggle", "render2d.lighting.normals", "render2d.lighting.normals"} {
		c, ok := ed.Commands.Get(name)
		if !ok || c.Run == nil {
			t.Fatal("lighting toolbar command missing", name)
		}
		if err = c.Run(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(sceneio.Save(sc), beforeScene) || doc.Dirty() {
		t.Fatal("lighting inspection modes mutated the scene")
	}
	pc := s.App.Play()
	if err = pc.Start(doc); err != nil {
		t.Fatal(err)
	}
	service := tween.Of(pc.World())
	if service == nil {
		t.Fatal("Play lacks Tween")
	}
	if _, err = service.ToFloat(pc.Scene().Handle(lightID), "Light2D.intensity", 2.5, .5, tween.Options{}); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		pc.Tick(.05)
	}
	animated, _ := scene.Get[render2d.Light2D](pc.Scene(), pc.Scene().Handle(lightID))
	if animated.Intensity < 2.4 {
		t.Fatal("Light propref did not animate in Play")
	}
	pc.Stop()
	if !bytes.Equal(sceneio.Save(sc), beforeScene) || doc.Dirty() {
		t.Fatal("Play lighting animation modified EditScene")
	}

	// Current broken Lit source must block strict native/web export even while
	// the editor retains a functioning last-good program/schema/normal binding.
	sd, err := ed.Docs.OpenScript(shaderPath)
	if err != nil {
		t.Fatal(err)
	}
	saveSource := func(text string) {
		t.Helper()
		sd.Code.ReplaceSnapshot([]byte(text), false)
		if err = ed.Docs.SaveScript(sd); err != nil {
			t.Fatal(err)
		}
	}
	checkExport := func(blocked bool) {
		t.Helper()
		for _, target := range []string{"host", "web"} {
			rep, e := gameexport.Export(gameexport.Options{Project: p, Target: target, Profile: "release", CheckOnly: true, Snapshot: gameexport.Take(ed.Pipeline)})
			if rep == nil || rep.Blocked != blocked || (e != nil) != blocked {
				t.Fatalf("strict %s export validation blocked=%v: %v %+v", target, blocked, e, rep)
			}
		}
	}
	checkExport(false)
	lastGood, generation := live.Shader(), live.Generation()
	saveSource("#include \"missing-lighting-workflow.glsl\"\n" + string(source))
	commit()
	if live.Shader() != lastGood || live.Generation() != generation {
		t.Fatal("broken Lit changed last-good")
	}
	checkExport(true)
	fixed := strings.Replace(string(source), "#pragma end", "uniform float WorkflowMarker; // @default(0.2)\n#pragma end", 1)
	saveSource(fixed)
	commit()
	if live.Shader() == lastGood || live.Generation() == generation {
		t.Fatal("fixed Lit did not atomically publish new generation")
	}
	value, _ := live.Get("NormalStrength")
	if math.Abs(float64(value[0]-.65)) > 1e-6 {
		t.Fatal("reload lost explicit material parameter")
	}
	if region, explicit, resolved := live.Normal(); !explicit || !resolved || region.IsZero() {
		t.Fatal("reload lost normal texture")
	}
	checkExport(false)
	s.Step()
	if !cfg.Headless && (cache.Stats().Programs == 0 || ed.Renderer.Stats().LightingFailures != 0) {
		t.Fatal("native authoring did not render valid lighting", cache.Stats(), ed.Renderer.LightingError())
	}
}

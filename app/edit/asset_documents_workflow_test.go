package edit

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"aego/app/agent"
	"aego/asset"
	textureimport "aego/asset/importers/texture"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/format/aetext"
	"aego/modules/render2d"
	"aego/render/lighting"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
)

func TestImageAssetAuthoringWorkflowHeadless(t *testing.T) {
	imageAssetAuthoringWorkflow(t, headless())
}

func imageAssetAuthoringWorkflow(t *testing.T, cfg Config) {
	t.Helper()
	p := newProject(t)
	heightID, albedoID, agentID := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	imp := textureimport.New()
	for _, source := range []struct {
		path string
		id   ids.AssetID
		role string
	}{{"assets/sculpted_height.png", heightID, "height"}, {"assets/sculpted.png", albedoID, "color"}, {"assets/agent_height.png", agentID, "height"}} {
		im := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		for y := range 32 {
			for x := range 32 {
				v := uint8(220 * math.Max(0, 1-math.Hypot(float64(x-16), float64(y-16))/21))
				c := color.NRGBA{R: v, G: v, B: v, A: 255}
				if source.role == "color" {
					c = color.NRGBA{R: 100 + v/3, G: 130 + v/3, B: 170 + v/3, A: 255}
				}
				im.SetNRGBA(x, y, c)
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, im); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Root, source.path), data.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		meta := asset.NewSourceMeta("texture", imp.Version(), imp.Defaults())
		meta.ID = source.id
		meta.Settings["role"] = aetext.String(source.role)
		if source.role == "height" {
			meta.Settings["generate_normal"] = aetext.Bool(true)
			meta.Settings["height_strength"] = aetext.Float32(1)
		} else {
			meta.Settings["normal"] = aetext.AssetRef(heightID.String())
		}
		if err := os.WriteFile(filepath.Join(p.Root, source.path+".meta"), asset.MarshalSourceMeta(meta), 0644); err != nil {
			t.Fatal(err)
		}
	}
	const scenePath = "scenes/asset-workflow.aescene"
	writeScene(t, p, scenePath, func(sc *scene.Scene) {
		cam := sc.Create(sc.Root(), "Camera")
		*scene.Add[render2d.Camera2D](sc, cam) = render2d.DefaultCamera2D()
		surface := sc.Create(sc.Root(), "Lit Surface")
		sr := scene.Add[render2d.SpriteRenderer](sc, surface)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Sprite.ID, sr.Material.ID, sr.Size = albedoID, asset.BuiltinMaterialLit, mathx.Vec2{X: 4, Y: 4}
		light := sc.Create(sc.Root(), "Point")
		l := scene.Add[render2d.Light2D](sc, light)
		*l = render2d.DefaultLight2D()
		l.Type, l.Height, l.Range = lighting.Point, 2, 9
		pose := sc.Local(light)
		pose.Position = mathx.Vec2{X: -2, Y: -1}
		sc.SetLocal(light, pose)
		e := sc.Create(sc.Root(), "Environment")
		env := scene.Add[render2d.LightingEnvironment2D](sc, e)
		*env = render2d.DefaultLightingEnvironment2D()
		env.AmbientIntensity = .15
	})
	s, err := Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ed := s.App.Context()
	sc, err := ed.Docs.Open(scenePath)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	beforeScene := sceneio.Save(sc.Scene())
	readNormal := func() []byte {
		t.Helper()
		entry, ok := ed.Loader.Index().Lookup(heightID)
		if !ok {
			t.Fatal("working normal entry missing")
		}
		a, ok := entry.Artifact(asset.ArtTexture)
		if !ok {
			t.Fatal("normal artifact missing")
		}
		raw, err := ed.Loader.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		normal, err := asset.DecodeTexture(raw)
		if err != nil || normal.Semantic != asset.TextureNormal {
			t.Fatal("height source did not produce a normal artifact", err)
		}
		return bytes.Clone(normal.Pixels)
	}
	initial := readNormal()
	// Asset Browser and search both use the generic document location route.
	ed.Open(context.AssetDocumentAt("assets/sculpted_height.png"))
	d := ed.Docs.AssetDocument(heightID)
	if d == nil || d.Source != nil || d.Metadata == nil || d.Capabilities.Has(document.EditSource) {
		t.Fatal("PNG did not open as a metadata-owning image document")
	}
	panelID := "render2d.image/" + heightID.String()
	ws := s.App.Workspace()
	if !ws.Visible(panelID) || ws.Focused() != panelID {
		t.Fatal("image open did not create/focus a normal workspace document tab")
	}
	shared, err := ed.DocumentForAsset(heightID)
	if err != nil || shared != d {
		t.Fatal("Inspector and image tab use different documents", err)
	}
	beforePixels, _ := os.ReadFile(ed.Docs.Abs(d.Path))
	env, err := ed.TextureAuthoringEnv(heightID)
	if err != nil {
		t.Fatal(err)
	}
	for i, strength := range []float32{3, 5, 7} {
		_, err := document.ApplyTextureMutation(d.Metadata, env, mutate.Tx{Label: "Height strength", Ops: []mutate.Op{mutate.TextureSetting("height_strength", aetext.Float32(strength))}}, i > 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	run := func(id string) {
		t.Helper()
		ws.Focus(panelID)
		if err := ed.Commands.Run(id); err != nil {
			t.Fatal(id, err)
		}
	}
	run(command.FileSave)
	s.Step()
	saved := readNormal()
	if bytes.Equal(initial, saved) || d.Dirty() || d.Metadata.SavedEdits == 0 {
		t.Fatal("generic Save did not import the changed height settings")
	}
	diskSaved, _ := os.ReadFile(ed.Docs.Abs(d.Metadata.Path))
	if err := s.App.Play().Start(sc); err != nil {
		t.Fatal(err)
	}
	previous := d.WorkingPreview.Applied
	run(command.EditUndo)
	deadline := time.Now().Add(8 * time.Second)
	for d.WorkingPreview.Applied == previous && d.WorkingPreview.Error == "" && time.Now().Before(deadline) {
		s.Step()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if d.WorkingPreview.Applied == previous || d.WorkingPreview.Pending || d.WorkingPreview.Error != "" || !bytes.Equal(initial, readNormal()) {
		t.Fatalf("post-Save Undo did not publish earlier normals during Play: %+v", d.WorkingPreview)
	}
	if current, _ := os.ReadFile(ed.Docs.Abs(d.Metadata.Path)); !bytes.Equal(current, diskSaved) || !d.Dirty() {
		t.Fatal("live Undo silently wrote metadata or lost dirty state")
	}
	normalID, normalRegion, ok := ed.Loader.NormalFor(albedoID)
	heightRegion, heightOK := ed.Loader.Texture(heightID)
	if !ok || !heightOK || normalID != heightID || normalRegion.Texture != heightRegion.Texture {
		t.Fatal("shared albedo consumer retained a stale normal binding")
	}
	previous = d.WorkingPreview.Applied
	run(command.EditRedo)
	deadline = time.Now().Add(8 * time.Second)
	for d.WorkingPreview.Applied == previous && d.WorkingPreview.Error == "" && time.Now().Before(deadline) {
		s.Step()
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if d.Dirty() || d.WorkingPreview.Applied == previous || !bytes.Equal(saved, readNormal()) {
		t.Fatal("generic Redo did not restore the saved generated normal")
	}
	s.App.Play().Stop()
	if sc.Dirty() || !bytes.Equal(sceneio.Save(sc.Scene()), beforeScene) {
		t.Fatal("asset metadata Save/Undo/Play dirtied the scene or prefab state")
	}
	if current, _ := os.ReadFile(ed.Docs.Abs(d.Path)); !bytes.Equal(current, beforePixels) {
		t.Fatal("metadata authoring modified image pixels")
	}

	// Editor agent mutations must acquire the same journal even with no viewer
	// open. The filesystem-only CLI remains a separate file transaction.
	bridge, err := s.agentBridge()
	if err != nil {
		t.Fatal(err)
	}
	const agentMeta = "assets/agent_height.png.meta"
	raw, err := os.ReadFile(ed.Docs.Abs(agentMeta))
	if err != nil {
		t.Fatal(err)
	}
	m, err := asset.ParseSourceMeta(raw, agentMeta)
	if err != nil {
		t.Fatal(err)
	}
	m.Settings["height_strength"] = aetext.Float32(4)
	changed := asset.MarshalSourceMeta(m)
	if ed.Docs.AssetDocument(agentID) != nil {
		t.Fatal("agent fixture was already open")
	}
	if err := bridge.ApplyScript(agent.ScriptEdit{Rel: agentMeta, Before: string(raw), After: string(changed)}, "Adjust height"); err != nil {
		t.Fatal(err)
	}
	agentDoc := ed.Docs.AssetDocument(agentID)
	if agentDoc == nil || agentDoc.Metadata == nil || !agentDoc.Dirty() || !bytes.Equal(agentDoc.Metadata.Code.Bytes(), changed) {
		t.Fatal("agent metadata edit bypassed the shared document")
	}
	if current, _ := os.ReadFile(ed.Docs.Abs(agentMeta)); !bytes.Equal(current, raw) {
		t.Fatal("editor agent silently saved unopened metadata")
	}
	agentDoc.Metadata.Conflict = true
	if err := bridge.ApplyScript(agent.ScriptEdit{Rel: agentMeta, Before: string(changed), After: string(raw)}, "Conflict"); err == nil {
		t.Fatal("agent ignored metadata conflict")
	}
	agentDoc.Metadata.Conflict = false
	agentDoc.Metadata.ReadOnly = true
	if err := bridge.ApplyScript(agent.ScriptEdit{Rel: agentMeta, Before: string(changed), After: string(raw)}, "Read only"); err == nil {
		t.Fatal("agent ignored metadata read-only state")
	}
	agentDoc.Metadata.ReadOnly = false
	if err := ed.UndoAsset(agentDoc); err != nil || agentDoc.Dirty() {
		t.Fatal("agent metadata edit is not one undo step", err)
	}

	if !cfg.Headless {
		ws.Focus(panelID)
		for range 4 {
			s.Step()
		}
		if path := os.Getenv("AEGO_IMAGE_WORKFLOW_SHOT"); path != "" {
			var capture *image.RGBA
			var captureErr error
			s.Engine.CaptureFrame(func(img *image.RGBA, err error) { capture, captureErr = img, err })
			s.Step()
			if capture == nil || captureErr != nil {
				t.Fatal("image document screenshot", captureErr)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, capture)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
	}
}

func TestAssetDocumentHandlerCoverage(t *testing.T) {
	s, err := Open(newProject(t), headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	r := s.App.Context().Docs.Handlers()
	const read = document.View | document.RevealSource
	const source = read | document.EditSource
	for _, tt := range []struct {
		kind                asset.ArtifactKind
		path, role, handler string
		caps                document.Capabilities
	}{
		{asset.ArtScene, "main.aescene", "", "editor.scene", source | document.Preview},
		{asset.ArtPrefab, "enemy.aeprefab", "", "editor.prefab", source | document.Preview},
		{asset.ArtScript, "hero.aes", "", "editor.script", source},
		{asset.ArtData, "settings.aedata", "", "editor.data", source},
		{asset.ArtTable, "balance.aetable", "", "editor.table", source},
		{asset.ArtAnim, "walk.aeanim", "", "editor.animation", source},
		{asset.ArtCtrl, "hero.aectrl", "", "editor.controller", source},
		{asset.ArtMaterial, "stone.aemat", "", "render2d.material", source | document.Preview},
		{asset.ArtShader, "lit.aeshader", "", "render2d.shader", source},
		{asset.ArtShaderInclude, "common.glsl", "", "render2d.shader-include", source},
		{asset.ArtTexture, "stone.png", "color", "render2d.image", read | document.EditMetadata | document.Preview},
		{asset.ArtTexture, "stone_n.png", "normal", "render2d.image", read | document.EditMetadata | document.Preview},
		{asset.ArtSpriteRegion, "sheet.png", "color", "render2d.image", read | document.EditMetadata | document.Preview},
		{asset.ArtAtlasPage, "atlas/page", "", "render2d.image", read | document.EditMetadata | document.Preview},
		{asset.ArtAudio, "click.wav", "", "audio.clip", read | document.EditMetadata | document.Preview},
		{asset.ArtAudio, "music.ogg", "", "audio.clip", read | document.EditMetadata | document.Preview},
		{asset.ArtAudio, "music.mp3", "", "audio.clip", read | document.EditMetadata | document.Preview},
		{asset.ArtMesh, "model.obj", "", "asset.readonly", read},
		{asset.ArtPreset, "style.aepreset", "", "asset.readonly", read},
		{"future-binary", "bundle.bin", "", "asset.readonly", read},
	} {
		t.Run(tt.path+"/"+tt.role, func(t *testing.T) {
			h := r.Resolve(tt.kind, tt.path, tt.role)
			if h.ID != tt.handler || h.Capabilities != tt.caps || h.Name.En == "" || h.Icon == "" {
				t.Fatalf("handler contract: %+v", h)
			}
			if (tt.kind == asset.ArtTexture || tt.kind == asset.ArtAudio || tt.kind == asset.ArtMesh) && h.TextSource {
				t.Fatal("binary format was advertised as a text document")
			}
		})
	}
}

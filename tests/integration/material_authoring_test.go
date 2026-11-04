package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"aego/app/agent"
	"aego/app/agent/mcp"
	"aego/app/edit"
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/prefabs"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/editor/panels/scripteditor"
	"aego/modules/animation"
	"aego/modules/render2d"
	"aego/modules/tween"
	"aego/platform"
	"aego/platform/input"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/ui/core"
)

// Both Create and Add Component are operated exclusively through keyboard
// events, including the path prompt. No registered action is invoked directly.
func TestMaterialAndShaderCreationAndOverrideUseKeyboard(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/material-keyboard.aescene"); err != nil {
		t.Fatal(err)
	}
	s.Step()
	for _, create := range []struct{ query, label, path string }{
		{"shader", "Shader", "assets/shaders/keyboard.aeshader"},
		{"material", "Material", "assets/materials/keyboard.aemat"},
	} {
		push(t, s, key(input.KeyN, input.PrimaryMod()|input.ModShift)...)
		if !s.App.AddPalette().Active() {
			t.Fatalf("Create shortcut did not open the palette for %s (modal=%v, doc=%v)", create.query, ctx.UI.ModalActive(), ctx.Doc() != nil)
		}
		push(t, s, typed(create.query)...)
		pal := s.App.AddPalette().Palette()
		if pal.Query() != create.query || pal.Selected() < 0 || pal.Item(pal.Selected()).Label != create.label {
			t.Fatalf("immediate search focus/result: query %q, selected %d", pal.Query(), pal.Selected())
		}
		push(t, s, key(input.KeyEnter, 0)...)
		// The next frame displays the prompt with its existing path selected.
		s.Step()
		push(t, s, typed(create.path)...)
		push(t, s, key(input.KeyEnter, 0)...)
		if _, ok := ctx.Pipeline.Lookup(create.path); !ok {
			t.Fatalf("keyboard creation produced no %s", create.path)
		}
		if s.App.AddPalette().Active() || ctx.UI.ModalActive() {
			t.Fatal("accepting the creation prompt left a modal open")
		}
	}

	sc, h := emptyNode(t, s)
	*scene.Add[render2d.SpriteRenderer](sc, h) = render2d.DefaultSpriteRenderer()
	sc.Flush()
	before := sceneio.Save(sc)
	push(t, s, key(input.KeyA, input.PrimaryMod()|input.ModShift)...)
	push(t, s, typed("material override")...)
	pal := s.App.AddPalette().Palette()
	if pal.Query() != "material override" || pal.Selected() < 0 || pal.Item(pal.Selected()).Label != "Material Override" {
		t.Fatal("Material Override search did not receive focus or select the component")
	}
	for _, item := range pal.Items() {
		if item.Label == "Material" || item.Label == "Shader" || item.Label == "Material from Shader" {
			t.Fatal("asset creation appeared in Add Component", item.Label)
		}
	}
	push(t, s, key(input.KeyEnter, 0)...)
	if _, ok := scene.Get[render2d.MaterialOverride](sc, h); !ok || s.App.AddPalette().Active() {
		t.Fatal("Enter did not add Material Override and close the palette")
	}
	if err := ctx.Doc().History.Undo(); err != nil || !bytes.Equal(before, sceneio.Save(sc)) {
		t.Fatal("one Undo did not remove the keyboard-added override", err)
	}
	if err := ctx.Doc().History.Redo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := scene.Get[render2d.MaterialOverride](sc, h); !ok {
		t.Fatal("Redo did not restore Material Override")
	}
}

// Semantic widget IDs locate controls by moving the real mouse through the
// visible panel; the test does not duplicate the Inspector's layout arithmetic.
func materialWidgetPoint(t *testing.T, s *edit.Session, id core.ID) mathx.Vec2 {
	t.Helper()
	r, ok := s.App.Workspace().Tree.PanelRect(workspace.Inspector)
	if !ok {
		t.Fatal("material Inspector is not visible")
	}
	for y := r.Y + 4; y < r.Y+r.H-4; y += 4 {
		at := mathx.Vec2{X: r.X + r.W - 36, Y: y}
		push(t, s, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}})
		if s.App.Context().UI.Hot() == id {
			return at
		}
	}
	t.Fatalf("material widget %x is not reachable in the visible panel", id)
	return mathx.Vec2{}
}

func TestMaterialTextureDropAndVariantEachHaveOneUndo(t *testing.T) {
	t.Setenv("AEGO_LANG", "en")
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/material-picker.aescene"); err != nil {
		t.Fatal(err)
	}
	ctx.Code.(*scripteditor.Host).Autosave = false
	const source = `#pragma aego abi 1
#pragma domain sprite
#include "engine/sprite.glsl"
#pragma variant ALT
#pragma material
uniform sampler2D Noise;
#pragma end
void fragment() { aego_out = aego_sample(); }
`
	shaderID, err := ctx.CreateAsset("assets/picker.aeshader", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	const path = "assets/picker.aemat"
	materialID, err := ctx.CreateAsset(path, []byte(fmt.Sprintf("material version=1 shader=asset(%q) { params {} future { keep=true } }", shaderID.String())))
	if err != nil {
		t.Fatal(err)
	}
	textureID := writeSheet(t, s, "picker-noise.png", 4, 4)
	md, err := ctx.Docs.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), md.Code.Bytes()...)
	ctx.InspectAsset(materialID)
	s.App.Workspace().Show(workspace.Assets)
	s.App.Workspace().Show(workspace.Inspector)
	ap := s.App.AssetsPanel()
	ap.SetFolder("assets")
	ap.SetTileSize(48)
	for range 4 {
		s.Step()
	}
	prefix := workspace.Inspector + "material:" + materialID.String()
	noiseAt := materialWidgetPoint(t, s, core.IDOf(prefix+"Noise"+"texture"))
	tile, _, ok := ap.TileRect(textureID)
	if !ok {
		t.Fatal("the source texture has no Assets tile")
	}
	dragTo(t, s, center(tile), noiseAt)
	parsed, err := asset.ParseMaterial(md.Code.Bytes(), path)
	if err != nil || parsed.Textures["Noise"] != textureID {
		t.Fatal("dragging the Assets tile did not assign the material sampler", err)
	}
	if s.App.Data().Inspected() != materialID {
		t.Fatal("the texture drag replaced its material Inspector drop target")
	}
	if !md.Code.Undo() || !bytes.Equal(md.Code.Bytes(), before) || md.Code.CanUndo() {
		t.Fatal("one texture drop did not produce exactly one reversible document edit")
	}
	if !md.Code.Redo() {
		t.Fatal("texture drop redo")
	}
	afterDrop := append([]byte(nil), md.Code.Bytes()...)
	s.Step()
	variantID := core.IDOf(prefix + "variant")
	click(t, s, materialWidgetPoint(t, s, variantID))
	if !ctx.UI.PopupOpen(variantID) {
		t.Fatal("variant click did not open the Combo")
	}
	click(t, s, materialWidgetPoint(t, s, core.IDSeq(variantID, 1)))
	parsed, err = asset.ParseMaterial(md.Code.Bytes(), path)
	if err != nil || len(parsed.Variants) != 1 || parsed.Variants[0] != "ALT" {
		t.Fatal("the variant menu did not select ALT", err)
	}
	if !md.Code.Undo() || !bytes.Equal(md.Code.Bytes(), afterDrop) {
		t.Fatal("one Undo of the variant also changed the texture action")
	}
	if !md.Code.Undo() || !bytes.Equal(md.Code.Bytes(), before) {
		t.Fatal("the second Undo did not restore the original material")
	}
	if disk, err := os.ReadFile(ctx.Pipeline.Abs(path)); err != nil || !bytes.Equal(disk, before) {
		t.Fatal("Inspector actions wrote the saved material without Save", err)
	}
}

func TestMaterialPlayPreservesExplicitPrefabOverridesAndDirtyState(t *testing.T) {
	s, ctx := openEditor(t)
	if err := ctx.NewScene("scenes/material-prefab.aescene"); err != nil {
		t.Fatal(err)
	}
	clipID, err := ctx.CreateAsset("assets/material-pulse.aeanim", []byte(`anim version=1 length=1 {
  track "" prop="MaterialOverride.Flash" kind=float32 {
    key t=0 v=0.2 interp=linear {}
    key t=1 v=1 {}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	d := ctx.Doc()
	sc := d.Scene()
	h := sc.Create(sc.Root(), "MaterialEnemy")
	sr := scene.Add[render2d.SpriteRenderer](sc, h)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Material.ID, _ = asset.BuiltinMaterialNamed("Flash")
	m, ok := ctx.Loader.Material(sr.Material.ID)
	if !ok {
		t.Fatal("builtin Flash did not resolve")
	}
	o := scene.Add[render2d.MaterialOverride](sc, h)
	if err := o.Set(m, "Flash", schema.Float32(.2)); err != nil {
		t.Fatal(err)
	}
	player := animation.DefaultPlayer()
	player.PlayOnStart = false
	player.Clip = animation.ClipRef{ID: clipID}
	*scene.Add[animation.AnimationPlayer](sc, h) = player
	sc.Flush()
	aID := sc.ID(h)
	ps := prefabs.New(ctx)
	if err = ps.CreateFrom(d, []ids.NodeID{aID}, ""); err != nil {
		t.Fatal(err)
	}
	pid := assetAt(t, ctx, "prefabs/MaterialEnemy.aeprefab")
	tpl, ok := s.App.Templates().ForAsset(asset.ArtPrefab, nil, pid, "MaterialEnemy")
	if !ok {
		t.Fatal("material prefab has no existing template")
	}
	created, err := templates.Instantiate(ctx, tpl, ids.NodeID{}, -1, nil)
	if err != nil || len(created) != 1 {
		t.Fatal("second prefab instance", err)
	}
	bID := created[0]
	td, _ := sc.Registry().ByName("engine.MaterialOverride")
	fd, _ := td.Field("values")
	candidate := render2d.MaterialOverride{}
	if err = candidate.Set(m, "Flash", schema.Float32(.35)); err != nil {
		t.Fatal(err)
	}
	var value schema.Value
	fd.Get(unsafe.Pointer(&candidate), &value)
	if err = d.Props.Set(nil, []ids.NodeID{aID}, td.ID, fd.ID, value); err != nil {
		t.Fatal(err)
	}
	info, ok := ps.Info(d, aID)
	if !ok || !info.Overridden(aID, "engine.MaterialOverride", "values") {
		t.Fatal("fixture did not contain an explicit prefab material override")
	}
	beforeOverrides := prefab.FormatOverrides(info.Overrides)
	if err = ctx.Docs.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	beforeScene := sceneio.Save(sc)
	beforePrefab := readRel(t, ctx, "prefabs/MaterialEnemy.aeprefab")
	pc := s.App.Play()
	if err = pc.Start(d); err != nil {
		t.Fatal(err)
	}
	play := pc.Scene()
	if _, err = tween.Of(pc.World()).ToFloat(play.Handle(aID), "MaterialOverride.Flash", 1, 1, tween.Options{}); err != nil {
		t.Fatal(err)
	}
	playPlayer, ok := scene.Get[animation.AnimationPlayer](play, play.Handle(bID))
	if !ok || animation.Of(pc.World()) == nil {
		t.Fatal("Play did not attach the animation module")
	}
	playPlayer.Play(animation.ClipRef{})
	for range 10 {
		pc.Tick(.05)
	}
	for _, id := range []ids.NodeID{aID, bID} {
		v, err := render2d.GetMaterialParameter(play, play.Handle(id), "Flash")
		if err != nil || v.F <= .5 {
			t.Fatal("Play Tween/Animation did not drive its prefab instance", id, v, err)
		}
	}
	pc.Stop()
	info, ok = ps.Info(d, aID)
	if !ok || prefab.FormatOverrides(info.Overrides) != beforeOverrides || !bytes.Equal(sceneio.Save(sc), beforeScene) || d.Dirty() {
		t.Fatal("Play added/changed prefab overrides or dirtied EditScene")
	}
	if readRel(t, ctx, "prefabs/MaterialEnemy.aeprefab") != beforePrefab {
		t.Fatal("Play modified the saved prefab definition")
	}
	for id, want := range map[ids.NodeID]float64{aID: .35, bID: .2} {
		v, err := render2d.GetMaterialParameter(sc, sc.Handle(id), "Flash")
		if err != nil || math.Abs(v.F-want) > 1e-5 {
			t.Fatal("Play changed an edit instance value", id, v, err)
		}
	}
}

func TestMaterialCLIAndMCPUseOpenDocumentAndSceneHistory(t *testing.T) {
	s, p := proxySession(t)
	ctx := s.App.Context()
	ctx.Code.(*scripteditor.Host).Autosave = false
	run := func(args ...string) []byte {
		t.Helper()
		var out, errors bytes.Buffer
		code := -1
		background(t, s, func() { code = agent.Main(p, append(args, "--json"), &out, &errors, agent.Options{}) }, nil)
		if code != 0 {
			t.Fatalf("CLI %v exit %d: %s%s", args, code, out.String(), errors.String())
		}
		return append([]byte(nil), out.Bytes()...)
	}
	const file = "assets/transport-flash.aemat"
	run("material", "create", file, "engine/shaders/flash")
	md, err := ctx.Docs.OpenScript(file)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), md.Code.Bytes()...)
	var state struct {
		Value struct {
			SourceHash string `json:"sourceHash"`
		} `json:"value"`
	}
	if err = json.Unmarshal(run("material", "get", file), &state); err != nil || state.Value.SourceHash == "" {
		t.Fatal("CLI did not return its material document revision", err)
	}
	run("material", "set", file, "Flash", "0.4", "--expected", state.Value.SourceHash)
	fromCLI := append([]byte(nil), md.Code.Bytes()...)
	if bytes.Equal(fromCLI, before) || !md.Dirty() {
		t.Fatal("CLI did not edit the open material buffer")
	}
	if err = json.Unmarshal(run("material", "get", file), &state); err != nil {
		t.Fatal(err)
	}
	addr, token, ok := agent.EditorProxy(p.Root)
	if !ok {
		t.Fatal("editor proxy missing")
	}
	requests := []any{
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]any{"name": "material-test", "version": "1"}}},
		map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "material_set", "arguments": map[string]any{"file": file, "param": "Flash", "value": "0.75", "expected": state.Value.SourceHash}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "material_assign", "arguments": map[string]any{"file": bridgeScene, "node": "Player", "material": file}}},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "material_validate", "arguments": map[string]any{"file": file}}},
	}
	var in, out bytes.Buffer
	for _, request := range requests {
		if err = json.NewEncoder(&in).Encode(request); err != nil {
			t.Fatal(err)
		}
	}
	background(t, s, func() { err = mcp.Relay(&in, &out, addr, token) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
		var reply struct {
			Error  json.RawMessage `json:"error"`
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if err = json.Unmarshal(line, &reply); err != nil || len(reply.Error) != 0 || reply.Result.IsError {
			t.Fatalf("MCP material command failed: %s (%v)", line, err)
		}
	}
	parsed, err := asset.ParseMaterial(md.Code.Bytes(), file)
	v, explicit := parsed.Value("Flash")
	if err != nil || !explicit || v.AsFloat() != .75 {
		t.Fatal("MCP did not see and update the CLI's dirty material", err)
	}
	if !md.Code.Undo() || !bytes.Equal(md.Code.Bytes(), fromCLI) {
		t.Fatal("one MCP edit did not have one source Undo")
	}
	sc := ctx.Docs.Find(bridgeScene).Scene()
	player := sc.Find(sc.Root(), "Player")
	sprite, ok := scene.Get[render2d.SpriteRenderer](sc, player)
	materialID := assetAt(t, ctx, file)
	if !ok || sprite.Material.ID != materialID || !strings.HasPrefix(lastStep(t, s), "via agent: material-test: ") {
		t.Fatal("MCP material assignment bypassed scene History")
	}
	if err = ctx.Docs.Find(bridgeScene).History.Undo(); err != nil {
		t.Fatal(err)
	}
	if sprite.Material.ID == materialID {
		t.Fatal("scene Undo did not restore the previous material assignment")
	}
	if disk, err := os.ReadFile(filepath.Join(p.Root, file)); err != nil || !bytes.Equal(disk, before) {
		t.Fatal("CLI/MCP transport wrote an unsaved material buffer to disk", err)
	}
}

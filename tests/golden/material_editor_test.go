//go:build aego_golden

package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	edws "aego/editor/core/workspace"
	agentpanel "aego/editor/panels/agent"
	"aego/editor/panels/scripteditor"
	"aego/engine/i18n"
	"aego/project"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/sceneio"
)

const editorMaterialShader = `#pragma aego abi 1
#pragma domain sprite
#include "engine/sprite.glsl"
#pragma material
uniform float Opacity; // @instance @default(0.65) @range(0, 1)
uniform vec4 Tint; // @color @default(0.08, 0.45, 0.9, 1)
uniform sampler2D Noise;
#pragma end
void fragment() {
    vec4 color = aego_sample();
    aego_out = color * Opacity * vec4(Tint.rgb * Tint.a, Tint.a);
}
`

// All four states use the actual module asset inspector and code editor.
// Shader output is drawn by the real material renderer before the UI crop.
func TestMaterialEditorGoldens(t *testing.T) {
	for _, language := range []string{i18n.English, i18n.Russian} {
		for _, mode := range []string{"dark", "light"} {
			t.Run(language+"_"+mode, func(t *testing.T) {
				t.Setenv("AEGO_LANG", language)
				t.Setenv("AEGO_EDITOR_THEME", mode)
				t.Cleanup(func() { i18n.Default().SetLanguage(i18n.English) })
				s, materialID, shaderPath := editorMaterialSession(t)
				ed := s.App.Context()
				suffix := "_" + language + "_" + mode
				stepEditorMaterial(s)
				rectShot(t, s, editorPanelRect(t, s, edws.Inspector), "editor_material_inspector"+suffix)

				// The dirty document is intentionally different from the scene's
				// committed material; the labels and independent preview show it.
				md, err := ed.Docs.OpenScript("assets/materials/review.aemat")
				if err != nil {
					t.Fatal(err)
				}
				_, err = document.ApplyMaterialMutation(md, ed.Loader.Shader, mutate.Tx{Ops: []mutate.Op{mutate.MaterialSet("Opacity", "0.25")}}, false)
				if err != nil {
					t.Fatal(err)
				}
				stepEditorMaterial(s)
				scrollMaterialEditor(t, s, -100)
				rectShot(t, s, editorPanelRect(t, s, edws.Inspector), "editor_material_dirty"+suffix)
				md.Code.Undo()
				scrollMaterialEditor(t, s, 100)
				stepEditorMaterial(s)

				// An unsupported portable token gives a stable source diagnostic
				// without machine-specific temporary compiler paths in the image.
				sd, err := ed.Docs.OpenScript(shaderPath)
				if err != nil {
					t.Fatal(err)
				}
				broken := strings.Replace(editorMaterialShader, "void fragment() {", "void fragment() {\n    double unsupported = 1.0;", 1)
				sd.Code.ReplaceSnapshot([]byte(broken), false)
				if err = ed.Docs.SaveScript(sd); err != nil {
					t.Fatal(err)
				}
				ed.Open(context.At(shaderPath, 10, 0))
				ed.Open(context.ProblemsAt(""))
				stepEditorMaterial(s)
				problems, _ := ed.Problems.Collect()
				found := false
				for _, problem := range problems {
					if problem.Code == "SHD008" {
						found = true
						if problem.Subject.File != shaderPath || problem.Subject.Line != 10 {
							t.Fatal("shader problem lost its source location", problem.Subject)
						}
					}
				}
				if !found {
					t.Fatal("broken shader has no portable-profile problem")
				}
				codeRect := editorPanelRect(t, s, scripteditor.PanelID(shaderPath))
				problemRect := editorPanelRect(t, s, edws.Problems)
				combined := mathx.Rect{X: codeRect.X, Y: codeRect.Y, W: codeRect.W, H: problemRect.Y + problemRect.H - codeRect.Y}
				rectShot(t, s, combined, "editor_material_shader_error"+suffix)
				// The same broken source remains visible alongside last-good
				// preview; wheel/scrollbar can reach a long diagnostics block.
				if current, ok := ed.Loader.Material(materialID); !ok || current.Generation() != 1 {
					t.Fatal("broken source changed the committed scene generation")
				}

				sd.Code.ReplaceSnapshot([]byte(editorMaterialShader), false)
				if err = ed.Docs.SaveScript(sd); err != nil {
					t.Fatal(err)
				}
				stepEditorMaterial(s)
				shaderID, _ := ed.Pipeline.Lookup(shaderPath)
				noGPU, err := ed.CreateAsset("assets/materials/no-preview.aemat", []byte(fmt.Sprintf("material version=1 shader=asset(%q) { params { Opacity=0.65 } }", shaderID.String())))
				if err != nil {
					t.Fatal(err)
				}
				ed.InspectAsset(noGPU)
				// The UI itself still has its graphics device. This adapter state
				// deliberately has no renderer available for a material preview.
				renderer := ed.Renderer
				ed.Renderer = nil
				stepEditorMaterial(s)
				rectShot(t, s, editorPanelRect(t, s, edws.Inspector), "editor_material_no_preview"+suffix)
				ed.Renderer = renderer
			})
		}
	}
}

func editorMaterialSession(t *testing.T) (*edit.Session, ids.AssetID, string) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	p, err := project.Create(t.TempDir(), "MaterialGolden", false)
	if err != nil {
		t.Fatal(err)
	}
	sc := scene.New(goldenTypes(t), nil, scene.Options{})
	cameraScene(sc)
	sc.Flush()
	const scenePath = "scenes/golden.aescene"
	if err = os.WriteFile(filepath.Join(p.Root, scenePath), sceneio.Save(sc), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: 1400, Height: 1040, Logger: log.Nop()})
	if err != nil {
		t.Fatal("material editor goldens require a graphics device:", err)
	}
	t.Cleanup(s.Close)
	if name := s.Engine.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("material editor references require a software rasterizer, got %q", name)
	}
	s.Step()
	// These goldens exercise the full-height material inspector, not the
	// default workspace split. Keep the panel tall enough to cover its fields.
	if s.App.Workspace().Visible(agentpanel.ID) {
		s.App.Workspace().Toggle(agentpanel.ID)
	}
	s.Step()
	ed := s.App.Context()
	if host, ok := ed.Code.(*scripteditor.Host); ok {
		host.Autosave = false
	}
	if _, err = ed.Docs.Open(scenePath); err != nil {
		t.Fatal(err)
	}
	const shaderPath = "assets/shaders/review.aeshader"
	shaderID, err := ed.CreateAsset(shaderPath, []byte(editorMaterialShader))
	if err != nil {
		t.Fatal(err)
	}
	materialID, err := ed.CreateAsset("assets/materials/review.aemat", []byte(fmt.Sprintf("material version=1 shader=asset(%q) { params { Opacity=0.65 } }", shaderID.String())))
	if err != nil {
		t.Fatal(err)
	}
	ed.InspectAsset(materialID)
	return s, materialID, shaderPath
}

func stepEditorMaterial(s *edit.Session) {
	for range 5 {
		s.Step()
	}
}

func scrollMaterialEditor(t *testing.T, s *edit.Session, lines float32) {
	t.Helper()
	r := editorPanelRect(t, s, edws.Inspector)
	in := s.Engine.Input()
	in.BeginFrame()
	in.ApplyWheel(mathx.Vec2{X: r.X + 12, Y: r.Y + r.H - 24}, mathx.Vec2{Y: lines})
	// Drive the existing UI update with this real input frame before the
	// platform begins its next event poll. No widget internals are modified.
	s.App.Update(0)
	in.BeginFrame()
	stepEditorMaterial(s)
}

func editorPanelRect(t *testing.T, s *edit.Session, id string) mathx.Rect {
	t.Helper()
	tree := s.App.Workspace().Tree
	node, ok := tree.Node(tree.FindPanel(id))
	if !ok || node.Rect.W <= 0 || node.Rect.H <= 0 {
		t.Fatal("panel is not visible", id)
	}
	return node.Rect
}

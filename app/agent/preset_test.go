package agent

import (
	"aego/asset"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSceneAddComponentRejectsUnusablePresetBeforeMutation(t *testing.T) {
	const src = "assets/invalid.aepreset"
	const main = "scenes/main.aescene"
	for _, test := range []struct {
		name    string
		invalid string
	}{
		{"wrong-rate-type", `component "particles.ParticleSystem2D" version=1 { rate="fast" }`},
		{"zero-capacity", `component "particles.ParticleSystem2D" version=1 { maxParticles=0 }`},
		{"ambiguous", `component "particles.ParticleSystem2D" version=1 {}
  component "particles.ParticleSystem2D" version=1 {}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := materialSession(t)
			before := []byte(`scene version=1 {node "77cf0000000000000000000000000021" name="Player" {}}`)
			path := filepath.Join(s.Project.Root, main)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, before, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.Project.Root, src), []byte(`prefab version=1 {node "77cf0000000000000000000000000022" name="Effect" {`+test.invalid+`}}`), 0644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.Pipeline.Scan(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pipeline.ImportAll(); err != nil {
				t.Fatal(err)
			}
			if _, err := Run(s, "scene.add-comp", Args{"file": main, "node": "Player", "comp": "ParticleSystem2D", "preset": src}); err == nil {
				t.Fatal("applied invalid component preset")
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, before) {
				t.Fatalf("failed preset changed scene: %v", err)
			}
		})
	}
}

type livePresetCamera struct {
	postCameraHost
	path   string
	source []byte
}

func (h *livePresetCamera) ScriptText(path string) (string, bool) {
	return string(h.source), path == h.path
}

func TestPostProcessPresetValidatesActualCameraAndAssets(t *testing.T) {
	const main, ref = "scenes/main.aescene", "assets/test.aepreset"
	const cameraID = "77cf0000000000000000000000000021"
	for _, mode := range []string{"disk", "live"} {
		t.Run(mode, func(t *testing.T) {
			s := materialSession(t)
			write := func(rel string, data []byte) {
				t.Helper()
				p := filepath.Join(s.Project.Root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			before := []byte(`scene version=1 { node "` + cameraID + `" name="Camera" {component "engine.Camera2D" version=1 {active=true clear=true clearColor=#000000ff}} }`)
			write(main, before)
			source := func(mat string) []byte {
				return []byte(`prefab version=1 {node "77cf0000000000000000000000000022" name="Colour" {
component "render2d.PostProcess" version=1 {enabled=true effects=[{typeID="postfx.color-grading" instanceID="lut" enabled=true version=1 lut=asset("45e397813812442d994a810000000003")},{typeID="postfx.custom" instanceID="shader" enabled=true version=1 material=asset("` + mat + `")}]}}}`)
			}
			bad, good := source(asset.BuiltinMaterialUnlit.String()), source(asset.BuiltinMaterialPostTint.String())
			write(ref, bad)
			for _, suffix := range []string{"", ".meta"} {
				b, err := os.ReadFile("../../examples/platformer/assets/postfx/identity16.png" + suffix)
				if err != nil {
					t.Fatal(err)
				}
				write("assets/identity.png"+suffix, b)
			}
			if _, _, err := s.Pipeline.Scan(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Pipeline.ImportAll(); err != nil {
				t.Fatal(err)
			}
			_, _, doc, err := s.sceneDisk(main)
			if err != nil {
				t.Fatal(err)
			}
			host := &livePresetCamera{postCameraHost: postCameraHost{doc: doc}, path: ref, source: bad}
			if mode == "live" {
				s.host = host
			}
			args := Args{"file": main, "node": "Camera", "comp": "PostProcess", "preset": ref}
			_, err = Run(s, "scene.add-comp", args)
			if err == nil || !strings.Contains(err.Error(), "postfx-domain") {
				t.Fatalf("wrong-domain material was not diagnosed: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(s.Project.Root, main)); err != nil || !bytes.Equal(got, before) || host.edits != 0 {
				t.Fatal("rejected preset changed destination", err)
			}
			if mode == "live" {
				host.source = good
			} else {
				write(ref, good)
			}
			args["dry-run"] = true
			if _, err = Run(s, "scene.add-comp", args); err != nil || host.edits != 0 {
				t.Fatal("preview", err)
			}
			delete(args, "dry-run")
			if _, err = Run(s, "scene.add-comp", args); err != nil {
				t.Fatal("valid camera+LUT+Custom was rejected", err)
			}
			if mode == "disk" {
				_, _, doc, err = s.sceneDisk(main)
				if err != nil {
					t.Fatal(err)
				}
			} else if host.edits != 1 {
				t.Fatal("preset not applied in one edit")
			}
			stack, err := cameraStack(doc, "Camera")
			if err != nil || len(stack.Effects) != 2 || stack.Effects[0].InstanceID != "lut" || stack.Effects[1].Material.ID != asset.BuiltinMaterialPostTint {
				t.Fatal("applied wrong preset", err)
			}
		})
	}
}

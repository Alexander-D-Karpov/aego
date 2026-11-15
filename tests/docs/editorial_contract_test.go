//go:build !aego_runtime

package docs_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestDocumentedExportTargetsMatchCommandUsage(t *testing.T) {
	command, err := os.ReadFile("../../cmd/aego/export.go")
	if err != nil {
		t.Fatal(err)
	}
	const targets = "host|linux|windows|darwin[/amd64|/arm64]|web"
	if !strings.Contains(string(command), "--target "+targets) {
		t.Fatal("export command contract changed; review the documentation")
	}
	for _, name := range []string{"cli.md", "cli.ru.md"} {
		data, err := os.ReadFile("../../engine/res/docs/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ReplaceAll(string(data), `\|`, "|"), "--target "+targets) {
			t.Errorf("%s omits a supported export target", name)
		}
	}
}

func TestPhysicsAdviceKeepsQueuedActionEdges(t *testing.T) {
	r, _ := build(t)
	e, ok := r.Get("physics/pitfalls")
	if !ok {
		t.Fatal("missing physics pitfalls")
	}
	if strings.Contains(e.Body, "Read it in `Update`") || strings.Contains(e.Translations["ru"].Body, "Читайте его в `Update`") {
		t.Fatal("obsolete workaround for action edges returned")
	}
	for _, body := range []string{e.Body, e.Translations["ru"].Body} {
		if !strings.Contains(body, `input.Pressed("Jump")`) || !strings.Contains(body, "`FixedUpdate`") || !strings.Contains(body, "`input.Down`") {
			t.Fatal("advice must distinguish a queued edge from holding")
		}
	}
}

func TestPostProcessDocumentsNamedParametersAndDiagnosticCauses(t *testing.T) {
	r, _ := build(t)
	for _, method := range []string{"SetFloat", "SetInt", "SetBool", "SetVec2", "SetVec3", "SetVec4", "SetEffectEnabled"} {
		e, ok := r.Get("render2d.PostProcess." + method)
		if !ok || len(e.Params) != 2 || e.Params[0].Name == "p0" || e.Params[1].Name == "p1" || strings.Contains(e.Signature, "struct:") {
			t.Errorf("undocumented parameters: %+v", e)
		}
	}
	for _, id := range []string{"MAT004", "SHD009"} {
		e, ok := r.Get(id)
		if !ok || e.Diag == nil || e.Translations["ru"].Body == "" {
			t.Errorf("missing diagnostic %s or its translation", id)
		}
	}
}

func TestOwnSourceAndRuntimeFormatsHaveBilingualReferencePages(t *testing.T) {
	r, _ := build(t)
	for _, name := range []string{"aego", "aescene", "aeprefab", "aes", "aeart", "aeanim", "aectrl", "aepreset", "aedata", "aetable", "aemat", "aeshader", "meta", "aetext"} {
		e, ok := r.Get("formats/" + name)
		if !ok || e.Parent != "formats/source" || e.Summary == "" || e.Translations["ru"].Body == "" {
			t.Errorf("missing source-format page or translation: %s: %+v", name, e)
		}
	}
	for _, name := range []string{"artifacts", "aetry", "aesave"} {
		e, ok := r.Get("formats/" + name)
		if !ok || e.Parent != "formats/runtime" || e.Translations["ru"].Body == "" {
			t.Errorf("runtime format %s is missing or mixed with authoring sources", name)
		}
	}
	if _, exists := r.Get("module/"); exists {
		t.Fatal("a schema value created an empty module group")
	}
	for _, id := range []string{"particles.ColorKey", "render2d.PostEffect", "render2d.PostParameter"} {
		e, ok := r.Get(id)
		if !ok || !e.ValueType || e.Owner == "" || e.Component != nil {
			t.Errorf("value type masquerades as a component: %s: %+v", id, e)
		}
	}
}

func TestPublicValueFieldsHaveBilingualDescriptions(t *testing.T) {
	r, _ := build(t)
	for parent, fields := range map[string][]string{
		"aego/audio.PlayOptions":   {"Bus", "FadeIn", "Loop", "Pan", "Pitch", "Priority", "Unscaled", "Volume"},
		"aego/camera.Pose":         {"Position", "Rotation", "Zoom"},
		"aego/data.AudioEvent":     {"Bus", "Cooldown", "Loop", "MaxDistance", "MaxVoices", "MinDistance", "PanWidth", "PauseWith", "Pitch", "PitchRand", "Priority", "Spatial", "Steal", "Volume", "VolumeRand"},
		"aego/material.Vec4":       {"X", "Y", "Z", "W"},
		"aego/physics.Contact":     {"Point", "Normal", "NormalImpulse", "RelativeVelocity", "SelfCollider", "OtherCollider"},
		"aego/physics.RayHit":      {"Hit", "Point", "Normal", "Distance", "Node", "Collider"},
		"engine.MaterialParameter": {"Name", "Kind", "Value", "Integer", "Boolean"},
	} {
		for _, field := range fields {
			id := parent + "." + field
			e, ok := r.Get(id)
			if !ok || e.Summary == "" || e.Body == "" || e.Translations["ru"].Body == "" {
				t.Errorf("public value field %s needs EN/RU prose", id)
			}
		}
	}
}

func TestMaterialOverrideDocumentsParameterScopesAndCheckedScene(t *testing.T) {
	r, _ := build(t)
	for method, names := range map[string][]string{
		"SetFloat": {"name", "value"}, "SetInt": {"name", "value"}, "SetBool": {"name", "value"},
		"SetVec2": {"name", "value"}, "SetVec3": {"name", "value"}, "SetVec4": {"name", "value"}, "SetColor": {"name", "value"},
		"GetFloat": {"name"}, "GetInt": {"name"}, "GetBool": {"name"}, "GetVec2": {"name"}, "GetVec3": {"name"},
		"GetVec4": {"name"}, "GetColor": {"name"}, "GetVec4Lane": {"name", "lane"}, "Reset": {"name"},
	} {
		e, ok := r.Get("engine.MaterialOverride." + method)
		var got []string
		for _, p := range e.Params {
			got = append(got, p.Name)
		}
		if !ok || !slices.Equal(got, names) || e.Translations["ru"].Body == "" {
			t.Errorf("MaterialOverride.%s has unnamed parameters or no RU explanation: %+v", method, e)
		}
	}
	e, _ := r.Get("engine.MaterialOverride")
	checked := map[string]bool{}
	for _, snippet := range e.Snippets {
		if snippet.Check {
			checked[snippet.Lang] = true
		}
	}
	if !checked["aes"] || !checked["aescene"] || !strings.Contains(e.Body, "FlashProbe") {
		t.Fatal("MaterialOverride example must compile and run with its actual scene setup")
	}
	for _, body := range []string{e.Body, e.Translations["ru"].Body} {
		for _, token := range []string{"@instance", "int32", "Reset", "PostProcess"} {
			if !strings.Contains(body, token) {
				t.Errorf("material override guide omits %q", token)
			}
		}
	}
	data, _ := r.Get("data")
	if strings.Contains(data.Body, `asset("…")`) || strings.Contains(data.Translations["ru"].Body, `asset("…")`) {
		t.Fatal("data guide presents an invalid AssetID as a source example")
	}
}

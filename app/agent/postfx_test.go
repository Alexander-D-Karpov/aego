package agent

import (
	"os"
	"path/filepath"
	"testing"

	"aego/modules/render2d"
	"aego/scene/mutate"
)

type postCameraHost struct {
	Host
	doc   *mutate.Document
	edits int
}

func (h *postCameraHost) OpenScene(string) (*mutate.Document, bool) { return h.doc, true }
func (h *postCameraHost) ApplyScene(_ string, _ string, tx mutate.Tx) (*mutate.Result, error) {
	r, err := h.doc.Apply(tx)
	if err == nil {
		h.edits++
	}
	return r, err
}

func TestPostEffectCommandTargetsDirtyCameraByStableID(t *testing.T) {
	s := materialSession(t)
	const file = "scenes/test.aescene"
	disk := []byte(`scene version=1 {
 node "43980000000000000000000000000001" name="Camera" {
  component "engine.Camera2D" version=1 {}
  component "render2d.PostProcess" version=1 { enabled=true effects=[{typeID="postfx.bloom" instanceID="first" enabled=true version=1 parameters=[]}] }
 }
}`)
	full := filepath.Join(s.Project.Root, file)
	if err := os.WriteFile(full, disk, 0644); err != nil {
		t.Fatal(err)
	}
	_, _, doc, err := s.sceneDisk(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = doc.Apply(mutate.Tx{Label: "Unsaved rename", Ops: []mutate.Op{mutate.Rename("Camera", "UnsavedCamera")}}); err != nil {
		t.Fatal(err)
	}
	h := &postCameraHost{doc: doc}
	s.host = h
	a := Args{"file": file, "node": "UnsavedCamera", "instance": "first", "field": "Intensity", "value": "0.37", "dry-run": true}
	if _, err := Run(s, "postfx.set", a); err != nil || h.edits != 0 {
		t.Fatalf("dry run: %v / %d", err, h.edits)
	}
	delete(a, "dry-run")
	if _, err := Run(s, "postfx.set", a); err != nil || h.edits != 1 {
		t.Fatalf("apply: %v / %d", err, h.edits)
	}
	o, err := Run(s, "postfx.get", Args{"file": file, "node": "UnsavedCamera"})
	if err != nil {
		t.Fatal(err)
	}
	stack := o.Value.(render2d.PostProcess)
	e, ok := stack.Effect("first")
	if !ok || len(e.Parameters) != 1 || e.Parameters[0].Value.X != 0.37 {
		t.Fatal(stack)
	}
	for _, bad := range []string{"-1", "true", "[1,2]"} {
		a["value"] = bad
		if _, err := Run(s, "postfx.set", a); err == nil || h.edits != 1 {
			t.Fatalf("invalid %q wrote scene", bad)
		}
	}
	a["instance"] = "deleted"
	if _, err := Run(s, "postfx.set", a); err == nil {
		t.Fatal("missing instance accepted")
	}
	if b, err := os.ReadFile(full); err != nil || string(b) != string(disk) {
		t.Fatal("saved document changed")
	}
}

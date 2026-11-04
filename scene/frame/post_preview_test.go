package frame

import (
	"testing"

	"aego/core/ids"
	"aego/render/postfx"
	"aego/render/view"
)

func TestPostPreviewBypassScopesToOutputCameraAndInstance(t *testing.T) {
	r := New()
	owner := ids.NewNodeID()
	p := &postfx.Plan{Passes: []postfx.Pass{{InstanceID: "grade"}, {InstanceID: "bloom"}}}
	r.SetPostPreview(PostPreview{Camera: owner, Instance: "grade"})
	out := r.previewPostPlan(r.OutputID(), owner, p)
	if out == p || len(out.Passes) != 1 || out.Passes[0].InstanceID != "bloom" || len(p.Passes) != 2 {
		t.Fatal("instance audition mutated the camera plan")
	}
	if r.previewPostPlan(view.ID{Scope: r.OutputID().Scope + 1}, owner, p) != p || r.previewPostPlan(r.OutputID(), ids.NewNodeID(), p) != p {
		t.Fatal("other camera/output changed")
	}
	r.SetPostPreview(PostPreview{Bypass: true})
	if r.previewPostPlan(r.OutputID(), owner, p) != nil || r.previewPostPlan(view.ID{Scope: r.OutputID().Scope + 1}, owner, p) != p {
		t.Fatal("whole-output bypass scope")
	}
}

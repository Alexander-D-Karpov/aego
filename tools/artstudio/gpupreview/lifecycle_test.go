package gpupreview

import (
	"testing"

	"aego/gpu"
)

func TestPreviewCloseReleasesResourcesAndRejectsFurtherWork(t *testing.T) {
	for _, step := range []bool{false, true} {
		name := "before-first-step"
		if step {
			name = "after-first-step"
		}
		t.Run(name, func(t *testing.T) {
			d, data := fixture(t)
			a, err := New(true)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.Publish(1, d.Revision(), data); err != nil {
				a.Close()
				t.Fatal(err)
			}
			if step && !a.Step() {
				a.Close()
				t.Fatal("preview stopped before close")
			}
			if a.Resources().Textures == 0 {
				a.Close()
				t.Fatal("fixture did not allocate preview textures")
			}
			a.Close()
			a.Close()
			if a.Status().State != "closed" || a.Resources() != (gpu.ResourceStats{}) || a.data != nil || a.loader != nil || a.runner != nil || a.detach != nil {
				t.Fatalf("preview retained live resources after repeated close: status=%+v resources=%+v", a.Status(), a.Resources())
			}
			if a.Step() || a.Publish(2, d.Revision(), data) == nil || a.Configure(DefaultOptions()) == nil || a.RestartParticles() == nil {
				t.Fatal("closed preview accepted further work")
			}
		})
	}
}

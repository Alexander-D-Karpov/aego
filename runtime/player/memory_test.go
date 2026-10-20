package player

import (
	"testing"

	"aego/asset"
	"aego/runtime/bundle"
)

func TestMountBytes(t *testing.T) {
	raw, err := bundle.EncodeData(&bundle.Data{Engine: "test", Profile: bundle.Release, Target: "web", Index: asset.NewIndex()})
	if err != nil {
		t.Fatal(err)
	}
	d, v, err := MountBytes(&Bundle{Manifest: raw, Files: map[string][]byte{"artifacts/x.bin": []byte("hi")}})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release() {
		t.Fatalf("profile %q", d.Profile)
	}
	got, err := v.ReadFile(bundle.ArtifactPath("x.bin"))
	if err != nil || string(got) != "hi" {
		t.Fatalf("read %q %v", got, err)
	}
	if _, _, err := MountBytes(&Bundle{Manifest: []byte("not a manifest")}); err == nil {
		t.Fatal("a broken manifest mounted")
	}
}

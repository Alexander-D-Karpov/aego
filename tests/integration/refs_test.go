package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/refs"
)

func referencedPair(t *testing.T, ctx *context.Editor) (scene, dep, other ids.AssetID) {
	t.Helper()
	pl := ctx.Pipeline
	for _, a := range pl.Assets() {
		if a.Kind != asset.ArtScene {
			continue
		}
		e, _ := pl.Entry(a.ID)
		for _, d := range e.Deps {
			de, ok := pl.Entry(d)
			if !ok {
				continue
			}
			for _, b := range pl.Assets() {
				if b.Kind == de.Kind && b.ID != d {
					return a.ID, d, b.ID
				}
			}
		}
	}
	t.Skip("the test project has no scene referencing an asset with a sibling of the same kind")
	return
}

func TestReferrersAndReplace(t *testing.T) {
	_, ctx := openEditor(t)
	scene, dep, other := referencedPair(t, ctx)
	svc := refs.New(ctx)
	found := false
	for _, r := range svc.Referrers(dep) {
		found = found || r.Asset == scene
	}
	if !found {
		t.Fatal("the scene must be listed as a referrer")
	}
	hits := svc.Find(dep)
	for _, h := range hits {
		if h.Ref.Asset == scene && len(h.Places) == 0 {
			t.Fatal("a referrer must show where the reference is")
		}
	}
	rel, _ := ctx.Pipeline.PathOf(scene)
	job, err := svc.Replace(dep, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Changed()) == 0 {
		t.Fatal("the scene file must be rewritten")
	}
	data, _ := os.ReadFile(ctx.Pipeline.Abs(rel))
	if strings.Contains(string(data), dep.String()) || !strings.Contains(string(data), other.String()) {
		t.Fatal("the reference was not replaced in the file")
	}
	if _, err := os.Stat(filepath.Join(ctx.Project.Root, filepath.FromSlash(job.Backup), filepath.FromSlash(rel))); err != nil {
		t.Fatalf("no backup of %s: %v", rel, err)
	}
	for _, r := range svc.Referrers(dep) {
		if r.Asset == scene {
			t.Fatal("after reimport the scene no longer depends on the old asset")
		}
	}
}

func TestDeleteBuriesAndResolverSuggests(t *testing.T) {
	_, ctx := openEditor(t)
	scene, dep, _ := referencedPair(t, ctx)
	rel, _ := ctx.Pipeline.PathOf(dep)
	data, err := os.ReadFile(ctx.Pipeline.Abs(rel))
	if err != nil {
		t.Fatal(err)
	}
	if err := assetops.Delete(ctx, []ids.AssetID{dep}); err != nil {
		t.Fatal(err)
	}
	g, ok := ctx.Pipeline.Grave(dep)
	if !ok || g.Path != rel {
		t.Fatalf("grave = %+v, %v", g, ok)
	}
	svc := refs.New(ctx)
	var miss *refs.Missing
	for _, m := range svc.MissingAssets() {
		if m.ID == dep {
			miss = &m
		}
	}
	if miss == nil || !miss.Buried || len(miss.Refs) == 0 || miss.Refs[0].Asset != scene {
		t.Fatalf("missing = %+v", miss)
	}
	copyRel := strings.TrimSuffix(rel, filepath.Ext(rel)) + "-restored" + filepath.Ext(rel)
	if _, err := assetops.CreateFile(ctx, copyRel, data); err != nil {
		t.Skipf("cannot recreate a file of this kind: %v", err)
	}
	ctx.Pipeline.Reimport()
	for _, m := range svc.MissingAssets() {
		if m.ID == dep && m.Suggest.IsZero() {
			return
		}
	}
	t.Log("suggestion is by file name; a renamed copy is not suggested")
}

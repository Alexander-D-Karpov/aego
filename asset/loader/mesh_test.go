package loader

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/render/mesh"
)

func TestBuiltinMeshWithoutIndex(t *testing.T) {
	l, _ := newLoader(t)
	m, ok := l.Mesh(asset.BuiltinCube)
	if !ok || m.Count != 36 {
		t.Fatalf("builtin cube = %+v %v", m, ok)
	}
	if again, _ := l.Mesh(asset.BuiltinCube); again != m {
		t.Fatal("builtin mesh was not cached")
	}
	if _, ok := l.Mesh(ids.NewAssetID()); ok {
		t.Fatal("unknown mesh resolved")
	}
}

func TestMeshArtifactSwap(t *testing.T) {
	id := ids.NewAssetID()
	fs := newFS(t, map[string][]byte{"m.aems": mesh.Encode(mesh.Cube(1))})
	idx := asset.NewIndex()
	entry := asset.Entry{
		ID: id, Kind: asset.ArtMesh, Name: "m",
		Artifacts: []asset.Artifact{{Kind: asset.ArtMesh, Path: "cache://m.aems"}},
	}
	idx.Put(entry)
	l, _ := newLoader(t)
	l.Mount(idx, fs)
	m, ok := l.Mesh(id)
	if !ok || m.Count != 36 {
		t.Fatal("mesh artifact did not load")
	}
	fs.WriteFile("cache://m.aems", mesh.Encode(mesh.Plane(1, 1, 1)))
	if err := l.Swap([]asset.Entry{entry}, nil); err != nil {
		t.Fatal(err)
	}
	next, ok := l.Mesh(id)
	if !ok || next == m || next.Count != 6 {
		t.Fatalf("swap did not replace the mesh: %+v", next)
	}
	fs.WriteFile("cache://m.aems", []byte("garbage"))
	if err := l.Swap([]asset.Entry{entry}, nil); err == nil {
		t.Fatal("broken mesh swap succeeded")
	}
	if kept, _ := l.Mesh(id); kept != next || kept.Count != 6 {
		t.Fatal("failed swap must keep the previous mesh")
	}
}

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"aego/format/aetext"
)

func TestSetSettingsWritesMeta(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	pl := open(t, p, imps)
	id, ok := pl.Lookup("assets/a.txt")
	if !ok {
		t.Fatal("a.txt is not imported")
	}
	if err := pl.SetSettings(id, Settings{"gain": aetext.Float64(0.5)}); err != nil {
		t.Fatal(err)
	}
	m, ok := pl.MetaOf(id)
	if !ok || m.Settings.Float("gain", 1) != 0.5 {
		t.Fatalf("MetaOf = %v, %v", m.Settings, ok)
	}
	m.Settings["gain"] = aetext.Float64(2)
	if again, _ := pl.MetaOf(id); again.Settings.Float("gain", 1) != 0.5 {
		t.Fatal("MetaOf shares the settings map")
	}
	path := filepath.Join(p.Root, "assets", "a.txt")
	data, err := os.ReadFile(path + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := ParseMeta(data, path)
	if err != nil {
		t.Fatal(err)
	}
	if disk.ID != id || disk.Settings.Float("gain", 1) != 0.5 {
		t.Fatalf("meta on disk %v %v", disk.ID, disk.Settings)
	}
}

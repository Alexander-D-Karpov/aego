package winres

import (
	"slices"
	"testing"
)

func TestSysoHasResourceTypes(t *testing.T) {
	data, err := Syso("amd64", []Icon{{Size: 16, PNG: []byte{1, 2, 3}}, {Size: 256, PNG: []byte{4, 5}}},
		Info{Product: "Game", Original: "game.exe", Version: ParseVersion("1.2.3-beta")}, true)
	if err != nil {
		t.Fatal(err)
	}
	types, err := Types(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(types, []uint16{rtIcon, rtGroupIcon, rtVersion, rtManifest}) {
		t.Fatalf("resource types %v", types)
	}
}

func TestParseVersion(t *testing.T) {
	if v := ParseVersion("v2.10.3-rc1"); v != (Version{2, 10, 3, 0}) {
		t.Fatalf("got %v", v)
	}
}

func TestVersionInfoIsAligned(t *testing.T) {
	b := versionInfo(Info{Product: "G", Version: Version{1, 0, 0, 0}})
	if int(b[0])|int(b[1])<<8 != len(b) {
		t.Fatalf("wLength %d, size %d", int(b[0])|int(b[1])<<8, len(b))
	}
}

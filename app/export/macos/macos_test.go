package macos

import (
	"encoding/binary"
	"image"
	"strings"
	"testing"
)

func TestICNSHeader(t *testing.T) {
	data, err := ICNS(image.NewNRGBA(image.Rect(0, 0, 64, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:])) != len(data) {
		t.Fatal("bad icns header")
	}
}

func TestPlistEscapesAndBundleID(t *testing.T) {
	p := string(Plist(Info{Name: "A & B", Executable: "ab", ID: BundleID("My_Game/x"), Version: "1.0"}, "icon"))
	if !strings.Contains(p, "A &amp; B") || !strings.Contains(p, "dev.aego.my.game.x") {
		t.Fatal(p)
	}
}

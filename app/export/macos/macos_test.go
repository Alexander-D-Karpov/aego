package macos

import (
	"encoding/binary"
	"encoding/xml"
	"image"
	"io"
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

func TestPlistURLSchemesAreExplicitAndEscaped(t *testing.T) {
	game := string(Plist(Info{Name: "Game", ID: "dev.aego.game"}, ""))
	if strings.Contains(game, "CFBundleURLTypes") {
		t.Fatal("a game without explicit schemes registered a URL handler")
	}
	data := Plist(Info{Name: "Editor", ID: "dev.aego.editor", URLSchemes: []string{"aego", "example&test"}}, "")
	out := string(data)
	for _, want := range []string{"<key>CFBundleURLTypes</key>", "<key>CFBundleURLName</key>", "dev.aego.editor.examples", "<key>CFBundleURLSchemes</key>", "<string>aego</string>", "<string>example&amp;test</string>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plist has no %s:\n%s", want, out)
		}
	}
	decoder := xml.NewDecoder(strings.NewReader(out))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("URL plist is not XML: %v", err)
		}
	}
}

func TestPlistEscapesAndBundleID(t *testing.T) {
	p := string(Plist(Info{Name: "A & B", Executable: "ab", ID: BundleID("My_Game/x"), Version: "1.0"}, "icon"))
	if !strings.Contains(p, "A &amp; B") || !strings.Contains(p, "dev.aego.my.game.x") {
		t.Fatal(p)
	}
}

func TestPlistDocumentTypes(t *testing.T) {
	out := string(Plist(Info{Name: "Aego", Executable: "aego", ID: "ru.akarpov.aego", Documents: []DocType{{Name: "Aego Scene", Extensions: []string{"aescene"}}}}, ""))
	for _, want := range []string{"<key>CFBundleDocumentTypes</key>", "<string>Aego Scene</string>", "<string>Editor</string>", "<string>aescene</string>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("plist has no %s:\n%s", want, out)
		}
	}
}

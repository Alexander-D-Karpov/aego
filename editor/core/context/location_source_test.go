package context

import "testing"

func TestShaderAndMaterialSourceLocationsOpenCode(t *testing.T) {
	var opened string
	var revealed string
	ed := &Editor{
		OpenScript: func(name string, line int) { opened = name },
		RevealPath: func(name string) { revealed = name },
	}
	for _, name := range []string{"assets/test.aeshader", "assets/include.glsl", "assets/test.vert", "assets/test.frag", "assets/test.aemat", "scripts/test.aes"} {
		opened, revealed = "", ""
		ed.Open(At(name, 7, 0))
		if opened != name || revealed != "" {
			t.Fatalf("source location %s was revealed instead of opened in code", name)
		}
	}
	opened, revealed = "", ""
	ed.Open(AssetAt("assets/test.aemat"))
	if opened != "" || revealed != "assets/test.aemat" {
		t.Fatal("explicit asset navigation must still reveal Assets")
	}
}

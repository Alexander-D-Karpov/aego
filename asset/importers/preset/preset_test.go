package preset

import (
	"testing"

	"aego/format/aetext"
)

func TestTypeFindsComponentAndScript(t *testing.T) {
	comp := "prefab version=1 {\n  node \"n\" name=\"p\" {\n    component \"engine.SpriteRenderer\" version=1 {\n    }\n  }\n}\n"
	scr := "prefab version=1 {\n  node \"n\" name=\"p\" {\n    component \"engine.Script\" version=1 {\n      script \"game.Spinner\" enabled=true {\n      }\n    }\n  }\n}\n"
	for _, c := range []struct {
		src    string
		name   string
		script bool
	}{{comp, "engine.SpriteRenderer", false}, {scr, "game.Spinner", true}} {
		doc, err := aetext.Parse([]byte(c.src), "p.aepreset")
		if err != nil {
			t.Fatal(err)
		}
		name, script, ok := Type(doc)
		if !ok || name != c.name || script != c.script {
			t.Fatalf("Type = %q %v %v", name, script, ok)
		}
	}
}

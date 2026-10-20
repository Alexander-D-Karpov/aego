package refs

import (
	"testing"

	"aego/core/ids"
	"aego/format/aetext"
)

func TestTextPlaces(t *testing.T) {
	id := ids.AssetID{Hi: 7, Lo: 9}
	src := "scene version=1 {\n  node \"n1\" name=\"Player\" {\n    component \"engine.SpriteRenderer\" version=1 {\n      sprite = " +
		string(Token(id)) + "\n    }\n  }\n}\n"
	doc, err := aetext.Parse([]byte(src), "x.aescene")
	if err != nil {
		t.Fatal(err)
	}
	got := textPlaces(doc, vAsset, id.String())
	if len(got) != 1 || got[0].Path != "Player" || got[0].Owner != "engine.SpriteRenderer" || got[0].Field != "sprite" {
		t.Fatalf("places = %+v", got)
	}
}

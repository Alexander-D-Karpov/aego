package refs

import (
	"testing"

	"aego/core/ids"
	"aego/format/aetext"
	"aego/schema"
)

func TestRewriteNested(t *testing.T) {
	a, b := ids.AssetID{Hi: 1, Lo: 1}, ids.AssetID{Hi: 2, Lo: 2}
	v := schema.Value{Kind: schema.KindList, List: []schema.Value{schema.Asset(ids.AssetID{}), schema.Asset(a)}}
	nv, ok := rewrite(v, swapAsset(a, b))
	if !ok || nv.List[1].Asset != b || v.List[1].Asset != a {
		t.Fatal("rewrite must replace inside lists without touching the original")
	}
	if leafAny(nv, isAsset(a)) {
		t.Fatal("no old reference may remain")
	}
}

func TestTextPlaces(t *testing.T) {
	id := ids.AssetID{Hi: 7, Lo: 9}
	src := "scene version=1 {\n  node \"n1\" name=\"Player\" {\n    component \"engine.SpriteRenderer\" version=1 {\n      sprite = " +
		string(token(id)) + "\n    }\n  }\n}\n"
	doc, err := aetext.Parse([]byte(src), "x.aescene")
	if err != nil {
		t.Fatal(err)
	}
	got := textPlaces(doc, vAsset, id.String())
	if len(got) != 1 || got[0].Path != "Player" || got[0].Owner != "engine.SpriteRenderer" || got[0].Field != "sprite" {
		t.Fatalf("places = %+v", got)
	}
}

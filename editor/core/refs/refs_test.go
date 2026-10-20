package refs

import (
	"testing"

	"aego/core/ids"
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

package manifest

import (
	"testing"

	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
)

func TestComponentMethodsRoundTrip(t *testing.T) {
	r := bind.New()
	bind.MethodProc1(r.ComponentMethods("test.Body"), "Push", schema.TypeID(1), func(*bind.Ctx, *struct{}, float32) {},
		bind.Doc("Push moves the body."), bind.Params("force"))
	r.Component(&abi.Component{Name: "test.Body"})
	m, err := FromRegistry(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(Encode(m))
	if err != nil {
		t.Fatal(err)
	}
	cm, ok := back.CompMethodDoc("test.Body", "Push")
	if !ok || cm.Doc != "Push moves the body." || len(cm.Params) != 1 || cm.Sig.String() != "(float)" {
		t.Fatalf("method %+v %v", cm, ok)
	}
	tab, err := back.Table()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tab.ComponentMethod("test.Body", "Push"); !ok {
		t.Fatal("decoded table lost the method")
	}
}

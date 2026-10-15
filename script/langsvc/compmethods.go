package langsvc

import (
	"aego/script/abi"
	"aego/script/bind/manifest"
	"aego/script/workspace"
)

func compMethodDoc(docs *manifest.Manifest, comp, name string) manifest.CompMethod {
	cm, _ := docs.CompMethodDoc(comp, name)
	return cm
}

func uniqueCompMethod(snap *workspace.Snapshot, name string) (*abi.Component, abi.Signature, bool) {
	var found *abi.Component
	var sig abi.Signature
	for _, me := range snap.Bindings.ComponentMethodList() {
		if me.Name != name {
			continue
		}
		if found != nil {
			return nil, abi.Signature{}, false
		}
		list := snap.Bindings.Components(me.Comp)
		if len(list) != 1 {
			return nil, abi.Signature{}, false
		}
		found, sig = list[0], me.Sig
	}
	return found, sig, found != nil
}

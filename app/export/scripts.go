package export

import (
	"aego/asset/loader"
	"aego/schema"
	"aego/script/abi"
	scriptruntime "aego/script/runtime"
)

// Scene loading must see the same project script descriptors as the player.
// Read the frozen prepared bytecode, never an editor workspace or current source.
func publishScriptSchemas(s *Snapshot, reg *schema.Registry, surface abi.Surface) error {
	l := loader.New(nil, nil, nil)
	if err := l.Mount(s.idx, s.files); err != nil {
		return err
	}
	blobs, err := l.Scripts()
	if err != nil {
		return err
	}
	mods, err := scriptruntime.DecodeAll(blobs)
	if err != nil {
		return err
	}
	return scriptruntime.Publish(reg, mods, surface)
}

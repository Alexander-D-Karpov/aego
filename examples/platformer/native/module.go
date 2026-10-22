package native

import (
	"aego/core/mathx"
	"aego/sdk"
)

type EnemyStats struct {
	Speed    float32     `aego:"speed,min=0,max=10"`
	Cooldown float32     `aego:"cooldown,min=0,max=5"`
	Damage   int64       `aego:"damage,min=0"`
	Tint     mathx.Color `aego:"tint"`
}

type Module struct{}

func (Module) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "mygame", Version: "0.1.0"}
}

func (Module) Register(r *sdk.Registry) error {
	if err := sdk.DataType[EnemyStats](r, sdk.Name("mygame.EnemyStats"), sdk.Doc("movement and attack numbers of one kind of enemy")).Err(); err != nil {
		return err
	}
	sdk.Live[EnemyStats](r, "speed")
	return nil
}

func Modules() []sdk.Module {
	return []sdk.Module{Module{}}
}

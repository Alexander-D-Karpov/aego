package sdk

import (
	"aego/contracts/inputactions"
	"aego/engine"
	"aego/modules/input"
)

type (
	InputActions   = input.Service
	InputConfig    = input.Config
	InputRecord    = input.RecordMeta
	InputActionSet = inputactions.Set
	InputAction    = inputactions.Action
	InputContext   = inputactions.Context
)

func Actions(w *engine.World) *InputActions {
	return input.Of(w)
}

func (r *Registry) ProvideInputActions(s InputActionSet) {
	r.r.Provide(inputactions.Kind, s)
}

func ProvideInputActions(r *Registry, s InputActionSet) {
	r.ProvideInputActions(s)
}

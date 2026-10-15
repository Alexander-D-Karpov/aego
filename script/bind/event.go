package bind

import (
	"fmt"

	"aego/script/abi"
)

const EventModule = "#event"

func (r *Registry) Event(e abi.EventDef, opts ...Option) error {
	if r.frozen {
		return fmt.Errorf("event %s: registry is frozen", e.Name)
	}
	if err := r.table.AddEvent(e); err != nil {
		return err
	}
	return r.describe(EventModule, e.Name, opts, len(e.Params))
}

func (r *Registry) Events() []abi.EventDef {
	return r.table.EventList()
}

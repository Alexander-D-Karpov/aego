package engine

type serviceEntry struct {
	id    string
	value any
}

func (e *Engine) SetService(id string, v any) {
	e.main.SetService(id, v)
}

func (e *Engine) ServiceAny(id string) (any, bool) {
	return e.main.ServiceAny(id)
}

func (e *Engine) MainWorld() *World {
	return e.main
}

func Service[T any](e *Engine, id string) (T, bool) {
	return ServiceOf[T](e.main, id)
}

package inputactions

const Kind = "inputactions"

type Action struct {
	Name        string
	Type        string
	Bindings    []string
	Interaction string
	Time        float32
	Buffer      float32
	Deadzone    float32
}

type Context struct {
	Name     string
	Priority int
	Consume  bool
	Release  bool
	Actions  []string
	Default  bool
}

type Set struct {
	Actions  []Action
	Contexts []Context
}

const ReaderID = "inputactions.reader"

type Reader interface {
	Has(action string) bool
	Pressed(action string) bool
}

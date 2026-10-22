package platform

type DialogKind uint8

const (
	DialogFolder DialogKind = iota
	DialogOpenFile
	DialogSaveFile
)

type FileDialog struct {
	Kind    DialogKind
	Start   string
	Filter  string
	Pattern string
}

type FileDialogs interface {
	ShowFileDialog(d FileDialog, done func(path string, ok bool)) error
}

var alertFn func(title, msg string) bool

func SetAlert(fn func(title, msg string) bool) {
	alertFn = fn
}

func Alert(title, msg string) bool {
	if alertFn == nil {
		return false
	}
	return alertFn(title, msg)
}

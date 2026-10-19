package scenes

import "aego/core/errs"

func errInvalid(name string) error {
	return errs.New(errs.EditorCommand, "\""+name+"\" is not a valid scene name")
}

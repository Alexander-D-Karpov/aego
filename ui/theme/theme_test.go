package theme

import (
	"reflect"
	"testing"

	"aego/core/mathx"
)

func TestCodeThemeComplete(t *testing.T) {
	colorT := reflect.TypeOf(mathx.Color{})
	for name, th := range map[string]Theme{"default": Default(), "editor": Editor()} {
		v := reflect.ValueOf(th.Code)
		for i := 0; i < v.NumField(); i++ {
			f, field := v.Field(i), v.Type().Field(i)
			switch {
			case f.Type() == colorT:
				if f.Interface().(mathx.Color).A == 0 {
					t.Errorf("%s: Code.%s is empty", name, field.Name)
				}
			case f.Kind() == reflect.Array && f.Type().Elem() == colorT:
				for j := 0; j < f.Len(); j++ {
					if f.Index(j).Interface().(mathx.Color).A == 0 {
						t.Errorf("%s: Code.%s[%d] is empty", name, field.Name, j)
					}
				}
			case f.Kind() == reflect.Float32:
				if f.Float() == 0 {
					t.Errorf("%s: Code.%s is zero", name, field.Name)
				}
			}
		}
	}
}

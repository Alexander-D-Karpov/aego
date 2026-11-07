package particles

import (
	"reflect"
	"slices"
	"testing"
)

// FullClone must be revisited if a mutable reference is added to the emitter.
func TestParticleSnapshotCloneCoversMutableFields(t *testing.T) {
	var found []string
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, name string) {
		switch typ.Kind() {
		case reflect.Slice:
			found = append(found, name)
			walk(typ.Elem(), name+"[]")
		case reflect.Array:
			walk(typ.Elem(), name+"[]")
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				field := f.Name
				if name != "" {
					field = name + "." + field
				}
				walk(f.Type, field)
			}
		case reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
			found = append(found, name+" ("+typ.Kind().String()+")")
		}
	}
	walk(reflect.TypeFor[ParticleSystem2D](), "")
	slices.Sort(found)
	want := []string{"AlphaOverLife.Keys", "Bursts", "ColorOverLife", "Frames", "SizeOverLife.Keys", "VelocityOverLife.Keys"}
	if !slices.Equal(found, want) {
		t.Fatalf("mutable emitter fields changed: %v; update cloneParticleSystem and the snapshot tests", found)
	}
}

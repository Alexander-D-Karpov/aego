//go:build darwin && amd64

package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../../.cache/native/darwin_amd64/include
#cgo LDFLAGS: -L${SRCDIR}/../../.cache/native/darwin_amd64/lib -lSDL3
*/
import "C"

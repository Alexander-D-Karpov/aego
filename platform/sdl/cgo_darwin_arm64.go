//go:build darwin && arm64

package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../../.cache/native/darwin_arm64/include
#cgo LDFLAGS: -L${SRCDIR}/../../.cache/native/darwin_arm64/lib -lSDL3
*/
import "C"

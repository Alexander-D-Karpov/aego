//go:build linux && amd64

package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../../.cache/native/linux_amd64/include
#cgo LDFLAGS: -L${SRCDIR}/../../.cache/native/linux_amd64/lib -lSDL3 -lm -ldl -lpthread -lrt
*/
import "C"

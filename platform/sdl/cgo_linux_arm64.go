//go:build linux && arm64

package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../../.cache/native/linux_arm64/include
#cgo LDFLAGS: -L${SRCDIR}/../../.cache/native/linux_arm64/lib -lSDL3 -lm -ldl -lpthread -lrt
*/
import "C"

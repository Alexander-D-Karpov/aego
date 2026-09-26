//go:build windows && amd64

package sdl

/*
#cgo CFLAGS: -I${SRCDIR}/../../.cache/native/windows_amd64/include
#cgo LDFLAGS: -L${SRCDIR}/../../.cache/native/windows_amd64/lib -lSDL3 -lsetupapi -lwinmm -limm32 -lversion -lole32 -loleaut32 -lgdi32 -luser32 -lkernel32 -lshell32 -luuid -lcfgmgr32 -ladvapi32 -ldinput8 -ldxguid -lhid
*/
import "C"

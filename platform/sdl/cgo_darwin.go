//go:build darwin

package sdl

/*
#cgo LDFLAGS: -liconv -framework Cocoa -framework IOKit -framework CoreVideo -framework CoreAudio -framework AudioToolbox -framework Carbon -framework ForceFeedback -framework GameController -framework Metal -framework QuartzCore -framework CoreHaptics -framework AVFoundation -framework CoreMedia -framework Foundation -framework UniformTypeIdentifiers
*/
import "C"

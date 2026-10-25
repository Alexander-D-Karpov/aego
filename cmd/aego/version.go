package main

import (
	"fmt"
	"runtime/debug"

	"aego/engine"
)

func cmdVersion() int {
	fmt.Println(versionLine())
	return 0
}

// versionLine is the engine version and the commit the binary was built from.
func versionLine() string {
	s := "aego " + engine.Version
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return s
	}
	var rev, at string
	dirty := false
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value
		case "vcs.time":
			at = kv.Value
		case "vcs.modified":
			dirty = kv.Value == "true"
		}
	}
	if rev == "" {
		return s
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	if at != "" {
		rev += ", " + at
	}
	return s + " (" + rev + ")"
}

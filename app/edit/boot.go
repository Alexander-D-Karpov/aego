package edit

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"aego/engine"
	"aego/platform"
)

func startBoot(verb string) {
	wd, _ := os.Getwd()
	exe, _ := os.Executable()
	gobin, err := exec.LookPath("go")
	if err != nil {
		gobin = "not on PATH"
	}
	_ = platform.StartBoot(platform.DefaultBootPath(),
		"aego "+engine.Version+" "+runtime.GOOS+"/"+runtime.GOARCH+" "+verb,
		"exe "+exe,
		"args "+strings.Join(os.Args[1:], " "),
		"cwd "+wd,
		"go "+gobin,
	)
}

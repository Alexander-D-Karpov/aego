package backend

import (
	"os/exec"
	"syscall"
)

func hideCatalogProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

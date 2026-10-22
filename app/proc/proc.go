package proc

import "os/exec"

func Hide(cmd *exec.Cmd) {
	hide(cmd)
}

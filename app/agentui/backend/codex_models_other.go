//go:build !windows

package backend

import "os/exec"

func hideCatalogProcess(*exec.Cmd) {}

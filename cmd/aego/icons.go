package main

import (
	"fmt"
	"os"

	"aego/engine"
	"aego/engine/bootstrap"
	"aego/engine/builtin"
)

func cmdIcons(args []string) int {
	dir, _ := firstArg(args)
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	code := 0
	if bootstrap.NeedsNative(p) {
		code, err = bootstrap.RunIcons(p, engine.Version, os.Stdout)
		if err != nil {
			return fail(err)
		}
	} else {
		rep, err := builtin.AuditIcons()
		if err != nil {
			return fail(err)
		}
		code = rep.Write(os.Stdout)
	}
	if code == 0 {
		fmt.Println("icons: ok")
	}
	return code
}

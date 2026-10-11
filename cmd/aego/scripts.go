package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"aego/app/run"
	"aego/engine"
	"aego/engine/bootstrap"
	"aego/project"
)

func scriptTool(p *project.Project, args ...string) int {
	if !bootstrap.NeedsNative(p) {
		return run.Tool(p, args, os.Stdout, os.Stderr)
	}
	binary, err := bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine.Version, nil)
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(binary, append([]string{"--tool"}, args...)...)
	cmd.Dir = p.Root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return exitCode(cmd.Run())
}

func cmdTest(args []string) int {
	dir, rest := firstArg(args)
	p, err := openProject(dir)
	if err != nil {
		return fail(err)
	}
	return scriptTool(p, append([]string{"test"}, rest...)...)
}

func cmdDump(args []string) int {
	if len(args) == 0 || args[0] != "script" {
		fmt.Fprintln(os.Stderr, "usage: aego dump script <package|Type> [--func name] [--project dir]")
		return 2
	}
	fs := flag.NewFlagSet("dump script", flag.ContinueOnError)
	fn := fs.String("func", "", "only print this function")
	dir := fs.String("project", "", "project directory")
	target, rest := firstArg(args[1:])
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: aego dump script <package|Type> [--func name] [--project dir]")
		return 2
	}
	p, err := openProject(*dir)
	if err != nil {
		return fail(err)
	}
	ws, err := openWorkspace(p, "", false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aego:", err)
		return 2
	}
	return run.Dump(ws, target, *fn, os.Stdout, os.Stderr)
}

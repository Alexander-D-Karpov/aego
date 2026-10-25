package main

import (
	"aego/engine/i18n"
	"errors"
	"flag"
	"fmt"
	"os"

	"aego/app/bootstrap"
	"aego/app/docstool"
	"aego/app/projects"
	"aego/project"
)

func docsUsage() string {
	return i18n.Tr("cli.docs.usage", `usage: aego docs <command> [flags]

  export [--out file]          write the documentation registry as JSON
  check [--strict]             broken links, style rules (DOC010–DOC023), undocumented bindings
  report [--json]              every page with its opening paragraph, examples and findings
  build [--out site] [--demo] generate the static site with the playground
  serve [--addr host:port]     build, serve on localhost and rebuild on changes

flags: --project dir, --engine dir, --playground=false`)
}

func docsEnv(dir, engineDir string) (*docstool.Env, error) {
	env := &docstool.Env{Stdout: os.Stdout, Stderr: os.Stderr}
	if root, err := projects.EngineRoot(engineDir); err == nil {
		env.Engine = root
	}
	start := dir
	if start == "" {
		start = "."
	}
	root, err := project.Find(start)
	if err != nil {
		if dir != "" {
			return nil, err
		}
		return env, nil
	}
	p, err := project.Load(root)
	if err != nil {
		return nil, err
	}
	env.Project = p
	if bootstrap.NeedsNative(p) {
		env.Native = func(args ...string) error {
			if code := scriptTool(p, args...); code != 0 {
				return errors.New(i18n.Tr("cli.docs.tool_status", "project tool exited with status {code}", i18n.Int("code", int64(code))))
			}
			return nil
		}
	}
	return env, nil
}

func cmdDocs(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, docsUsage())
		return 2
	}
	fs := flag.NewFlagSet("docs "+args[0], flag.ContinueOnError)
	out := fs.String("out", "", "output file for export, directory for build and serve")
	lang := fs.String("lang", "", i18n.Tr("cli.docs.flag.lang", "also check the translations into this language, such as ru"))
	strict := fs.Bool("strict", false, "undocumented project bindings are errors")
	addr := fs.String("addr", "127.0.0.1:6060", "listen address for serve")
	play := fs.Bool("playground", true, "build the playground and Run buttons")
	engineDir := fs.String("engine", "", "path to the aego source tree")
	repo := fs.String("repo", "", "source repository linked from the site header")
	demo := fs.Bool("demo", false, "link the web demo at demo/ from the landing page")
	dir := fs.String("project", "", "project directory")
	asJSON := fs.Bool("json", false, "report as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	env, err := docsEnv(*dir, *engineDir)
	if err != nil {
		return fail(err)
	}
	env.Repo = *repo
	env.Demo = *demo
	site := func(def string) string {
		if *out != "" {
			return *out
		}
		return def
	}
	switch args[0] {
	case "export":
		return env.Export(*out)
	case "check":
		env.Lang = *lang
		return env.Check(*strict)
	case "report":
		return env.Report(*asJSON)
	case "build":
		if err := env.Build(site("site"), *play); err != nil {
			return fail(err)
		}
		return 0
	case "serve":
		if err := env.Serve(*addr, site(".cache/docs-site"), *play); err != nil {
			return fail(err)
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, docsUsage())
	return 2
}

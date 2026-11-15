package main

import (
	"context"
	"io"
	"os"
	"os/signal"

	"aego/app/assembly/builtin"
	"aego/engine"
	"aego/engine/i18n"
	"aego/project/remoteexample/cli"
)

// Remote-example operations only prepare or inspect data. Opening the returned
// project in the editor remains a separate explicit action.
func cmdRemoteExamples(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runRemoteExamples(ctx, args, os.Stdout, os.Stderr); err != nil {
		return fail(err)
	}
	return 0
}

func runRemoteExamples(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	return cli.Run(ctx, args, out, diagnostics, cli.Options{
		CommandName:      "aego examples",
		EngineVersion:    engine.Version,
		InstalledModules: builtin.Default().IDs(),
		HelpText: i18n.Tr("cli.examples.help", `Remote examples (data only; no code execution)
  remoteexample clone  -url HTTPS_MANIFEST -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample open   -url HTTPS_MANIFEST -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample recent [-root DIR]
  remoteexample reopen -id LOCAL_COPY_ID [-root DIR]
  remoteexample check  -id LOCAL_COPY_ID -approve-origin HTTPS_ORIGIN [-root DIR]
  remoteexample link   -url HTTPS_MANIFEST [-mode project|scene]
  remoteexample inspect -link AEGO_URL

clone always creates an independent editable copy. open reuses the most recent
local copy, including offline edits, before attempting HTTPS. check only reports
updates; it never replaces files. inspect parses a deep link but grants no trust.
No origin is implicitly trusted. -approve-origin must be an exact HTTPS origin.`),
	})
}

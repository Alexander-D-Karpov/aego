// remoteexample is the standalone data-only example transport client.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"aego/project/remoteexample/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	return cli.Run(ctx, args, out, diagnostics, cli.Options{EngineVersion: "0.12.1", AllowCompatibilityOverrides: true})
}

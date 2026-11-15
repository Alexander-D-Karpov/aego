package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"aego/asset/pipeline"
	"aego/engine/i18n"
)

func cmdSourceRecovery(args []string) int {
	if err := runSourceRecovery(args, os.Stdout); err != nil {
		return fail(err)
	}
	return 0
}

func runSourceRecovery(args []string, out io.Writer) error {
	usage := i18n.Tr("cli.source_recovery.usage", "usage: aego source-recovery status [dir] | restore [dir] --confirm")
	if len(args) == 0 || (args[0] != "status" && args[0] != "restore") {
		return errors.New(usage)
	}
	action := args[0]
	dir, rest := firstArg(args[1:])
	flags := flag.NewFlagSet("source-recovery "+action, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	confirm := flags.Bool("confirm", false, i18n.Tr("cli.source_recovery.flag.confirm", "explicitly resolve the interrupted transaction"))
	if err := flags.Parse(rest); err != nil || flags.NArg() != 0 || (action == "status" && *confirm) {
		return errors.New(usage)
	}
	if action == "restore" && !*confirm {
		return errors.New(i18n.Tr("cli.source_recovery.confirm_required", "recovery requires --confirm; inspect status first"))
	}
	p, err := openProject(dir)
	if err != nil {
		return err
	}
	info, err := pipeline.SourceRecoveryStatus(p)
	if err != nil {
		return err
	}
	if info == nil {
		fmt.Fprintln(out, i18n.Tr("cli.source_recovery.none", "No interrupted source transaction was found."))
		return nil
	}
	if action == "status" {
		fmt.Fprintln(out, i18n.Tr("cli.source_recovery.status", "Source recovery {id}: {state}", i18n.Str("id", info.ID), i18n.Str("state", info.State)))
		for _, name := range info.Paths {
			fmt.Fprintf(out, "  %q\n", name)
		}
		if info.State == "prepared" {
			fmt.Fprintln(out, i18n.Tr("cli.source_recovery.prepared", "Inspect these files and close the editor. Restore with --confirm only if you want the previous source generation."))
		} else {
			fmt.Fprintln(out, i18n.Tr("cli.source_recovery.finished", "The transaction was already resolved; restore --confirm only finishes recovery-file cleanup."))
		}
		return nil
	}
	if err := pipeline.RestoreSourceRecovery(p); err != nil {
		return err
	}
	fmt.Fprintln(out, i18n.Tr("cli.source_recovery.resolved", "Source recovery was resolved."))
	return nil
}

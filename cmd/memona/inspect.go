package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"memona/internal/memory"
)

var errInspectionFindings = errors.New("inspection found warnings, errors or incomplete results")

func runInspect(args []string) error { return inspectCommand(context.Background(), args, os.Stdout) }

func inspectCommand(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("data-dir", "", "Existing data directory (server must be stopped); read-only inspection")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid inspect flags")
	}
	if *root == "" || flags.NArg() != 0 {
		return errors.New("usage: memona inspect -data-dir existing-directory")
	}
	report, err := memory.InspectOffline(ctx, *root)
	if errors.Is(err, memory.ErrInspectBusy) {
		return errors.New("inspection refused: writer active or lock unavailable; stop normally, never remove lock")
	}
	if err != nil {
		return errors.New("inspection unavailable: verify existing private data directory, ownership, directories and lock; no files were created or repaired")
	}
	if err := json.NewEncoder(out).Encode(report); err != nil {
		return errors.New("inspection output failed")
	}
	if report.Status != "ok" || !report.Integrity.Complete {
		return errInspectionFindings
	}
	return nil
}

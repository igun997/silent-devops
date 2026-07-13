package app

import (
	"context"
	"fmt"
	"io"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer, start func(context.Context) error) int {
	if len(args) == 0 {
		if err := start(ctx); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintf(stdout, "Usage: %s [help|version]\n", name)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "%s %s (commit %s, built %s)\n", name, Version, Commit, Date)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown argument: %s\n", args[0])
		return 2
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"silent-devops/internal/app"
	"silent-devops/internal/clientapi"
	"silent-devops/internal/clientcli"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Fprintln(os.Stdout, clientcli.Usage)
			return
		case "version", "-v", "--version":
			fmt.Fprintf(os.Stdout, "client %s (commit %s, built %s)\n", app.Version, app.Commit, app.Date)
			return
		}
	}
	if err := clientcli.Validate(stripFlags(args)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, clientcli.Usage)
		os.Exit(2)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store := &clientcli.CredentialStore{Path: filepath.Join(home, ".config", "silent-devops", "token")}
	api, err := clientapi.Dial(os.Getenv("SILENT_DEVOPS_VALIDATOR"), os.Getenv("SILENT_DEVOPS_VALIDATOR_CA"), os.Getenv("SILENT_DEVOPS_SERVER_NAME"), store)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer api.Close()
	os.Exit(clientcli.Run(context.Background(), args, os.Stdout, os.Stderr, api, isTerminal(os.Stdout)))
}
func stripFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--json" && arg != "--yes" && arg != "--no-color" {
			out = append(out, arg)
		}
	}
	return out
}
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

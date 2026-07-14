package main

import (
	"context"
	"fmt"
	"net"
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
	tui := len(args) == 0 || args[0] == "tui"
	joining := len(args) > 0 && args[0] == "join"
	piping := len(args) == 2 && args[0] == "ssh-pipe"
	if !tui && !joining && !piping {
		if err := clientcli.Validate(stripFlags(args)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, clientcli.Usage)
			os.Exit(2)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	configDir := filepath.Join(home, ".config", "silent-devops")
	store := &clientcli.CredentialStore{Path: filepath.Join(configDir, "token")}
	address, caPath, serverName := os.Getenv("SILENT_DEVOPS_VALIDATOR"), os.Getenv("SILENT_DEVOPS_VALIDATOR_CA"), os.Getenv("SILENT_DEVOPS_SERVER_NAME")
	if profile, e := clientcli.LoadProfile(filepath.Join(configDir, "config.json")); e == nil {
		if address == "" {
			address = profile.Address
		}
		if caPath == "" {
			caPath = profile.CAPath
		}
		if serverName == "" {
			serverName = profile.ServerName
		}
	}
	if joining {
		if len(args) != 4 {
			fmt.Fprintln(os.Stderr, "usage: client join INVITATION USERNAME PASSWORD")
			os.Exit(2)
		}
		inv, e := clientcli.DecodeInvitation(args[1])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		certPath := filepath.Join(configDir, "validator-ca.crt")
		if e = clientapi.FetchPinnedCertificate(context.Background(), inv.Address, inv.Pin, certPath); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		host, _, e := net.SplitHostPort(inv.Address)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		api, e := clientapi.Dial(inv.Address, certPath, host, store)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		defer api.Close()
		if _, e = api.Redeem(context.Background(), inv.Secret, args[3]); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		if e = clientcli.SaveProfile(filepath.Join(configDir, "config.json"), clientcli.Profile{Address: inv.Address, ServerName: host, CAPath: certPath, Username: args[2]}); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "client joined; run silent-devops-client")
		return
	}
	api, err := clientapi.Dial(address, caPath, serverName, store)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer api.Close()
	if piping {
		if err := api.Pipe(context.Background(), args[1], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if tui {
		if err := clientcli.RunTUI(api, hasFlag(args, "--no-color")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
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
func hasFlag(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

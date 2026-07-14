// Command easypanel-migrate detects a local EasyPanel install, pulls its API
// token from the panel container's LMDB store, and migrates ("snapshots and
// transfers") a service to a remote EasyPanel — with a fail-closed preflight
// that verifies the remote project exists before calling the panel's migrate
// endpoint (which otherwise returns an opaque 500).
//
// Subcommands:
//
//	detect                 report whether EasyPanel is present on this host
//	token                  print the local panel API token (read from LMDB)
//	projects [--remote]    list local (or remote) projects
//	migrate  ...           validate and migrate a service to a remote panel
//
// The local panel is auto-detected via host Docker. The remote panel is given
// by --remote-url and --remote-token, or --remote-container to extract the
// token from a locally reachable container (used by the E2E harness).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"silent-devops/internal/easypanel"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "detect":
		err = cmdDetect(ctx)
	case "token":
		err = cmdToken(ctx)
	case "projects":
		err = cmdProjects(ctx, os.Args[2:])
	case "migrate":
		err = cmdMigrate(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `easypanel-migrate — detect EasyPanel and migrate a service between panels

Usage:
  easypanel-migrate detect
  easypanel-migrate token
  easypanel-migrate projects [--remote-url URL --remote-token TOK]
  easypanel-migrate migrate \
      --local-project P --local-service S \
      --remote-url URL (--remote-token TOK | --remote-container NAME) \
      --remote-project P --remote-service S \
      [--create-remote-project] [--overwrite-remote-service]
`)
}

func localClient(ctx context.Context) (*easypanel.Client, easypanel.Detection, error) {
	return easypanel.DetectAndToken(ctx, easypanel.ExecRunner{})
}

func cmdDetect(ctx context.Context) error {
	det, err := easypanel.Detect(ctx, easypanel.ExecRunner{})
	if err != nil {
		return err
	}
	if !det.Present {
		fmt.Println("easypanel: not detected")
		os.Exit(3)
	}
	version := det.Version
	if version == "" {
		version = "unknown"
	}
	publicURL := det.PublicURL
	if publicURL == "" {
		publicURL = "unknown"
	}
	fmt.Printf("easypanel: detected container=%s url=%s version=%s public_url=%s\n",
		det.Container, det.BaseURL, version, publicURL)
	return nil
}

func cmdToken(ctx context.Context) error {
	c, _, err := localClient(ctx)
	if err != nil {
		return err
	}
	fmt.Println(c.Token)
	return nil
}

func cmdProjects(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("projects", flag.ExitOnError)
	remoteURL := fs.String("remote-url", "", "remote panel URL (list remote instead of local)")
	remoteToken := fs.String("remote-token", "", "remote API token")
	remoteContainer := fs.String("remote-container", "", "extract remote token from this local container")
	_ = fs.Parse(args)

	c, err := selectPanel(ctx, *remoteURL, *remoteToken, *remoteContainer)
	if err != nil {
		return err
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		fmt.Println(p.Name)
	}
	return nil
}

// selectPanel returns the remote panel client when a remote is specified,
// otherwise the auto-detected local panel.
func selectPanel(ctx context.Context, url, token, container string) (*easypanel.Client, error) {
	if url == "" && container == "" {
		c, _, err := localClient(ctx)
		return c, err
	}
	return remoteClient(ctx, url, token, container)
}

func remoteClient(ctx context.Context, url, token, container string) (*easypanel.Client, error) {
	if url == "" {
		return nil, fmt.Errorf("--remote-url required")
	}
	if token == "" && container != "" {
		var err error
		token, err = easypanel.ExtractToken(ctx, easypanel.ExecRunner{}, container)
		if err != nil {
			return nil, fmt.Errorf("extract remote token: %w", err)
		}
	}
	if token == "" {
		return nil, fmt.Errorf("--remote-token or --remote-container required")
	}
	return easypanel.New(url, token), nil
}

func cmdMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	localProject := fs.String("local-project", "", "source project name")
	localService := fs.String("local-service", "", "source service name")
	remoteURL := fs.String("remote-url", "", "remote panel URL")
	remoteToken := fs.String("remote-token", "", "remote API token")
	remoteContainer := fs.String("remote-container", "", "extract remote token from this local container")
	remoteProject := fs.String("remote-project", "", "target project name")
	remoteService := fs.String("remote-service", "", "target service name")
	createProject := fs.Bool("create-remote-project", false, "auto-create remote project if missing")
	overwrite := fs.Bool("overwrite-remote-service", false, "allow overwriting an existing remote service")
	_ = fs.Parse(args)

	if *localProject == "" || *localService == "" || *remoteProject == "" || *remoteService == "" {
		return fmt.Errorf("local/remote project and service names are required")
	}

	local, det, err := localClient(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("detected local panel: %s\n", det.Container)

	remote, err := remoteClient(ctx, *remoteURL, *remoteToken, *remoteContainer)
	if err != nil {
		return err
	}

	in := easypanel.MigrateInput{
		LocalProjectName:   *localProject,
		LocalServiceName:   *localService,
		RemoteAPIToken:     remote.Token,
		RemoteEasypanelURL: *remoteURL,
		RemoteProjectName:  *remoteProject,
		RemoteServiceName:  *remoteService,
	}
	plan, err := easypanel.Preflight(ctx, local, remote, in, easypanel.PreflightOptions{
		CreateRemoteProject:    *createProject,
		OverwriteRemoteService: *overwrite,
	})
	if err != nil {
		return err
	}
	if plan.CreatedProject {
		fmt.Printf("created remote project %q\n", *remoteProject)
	}
	fmt.Printf("migrating %s/%s -> %s %s/%s ...\n",
		*localProject, *localService, *remoteURL, *remoteProject, *remoteService)
	if err := plan.Run(ctx); err != nil {
		return err
	}
	fmt.Println("migrate: ok")
	return nil
}

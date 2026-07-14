package easypanel

import (
	"context"
	"fmt"
)

// MigratePlan is a validated, ready-to-run service migration.
type MigratePlan struct {
	Local          *Client
	Remote         *Client
	RemoteURL      string
	RemoteToken    string
	LocalProject   string
	LocalService   string
	RemoteProject  string
	RemoteService  string
	CreatedProject bool // remote project was auto-created during preflight
}

// PreflightOptions tune the pre-migration checks.
type PreflightOptions struct {
	// CreateRemoteProject creates the remote project when it does not exist
	// instead of failing.
	CreateRemoteProject bool
	// OverwriteRemoteService allows migration when the remote service name is
	// already present in the remote project.
	OverwriteRemoteService bool
}

// Preflight verifies both sides before a migrate call and returns a runnable
// plan. It fails closed with a specific error rather than letting the panel
// return an opaque 500 ("Service not found.") when the remote project is
// missing.
//
// Checks:
//  1. local project exists,
//  2. local service exists in that project,
//  3. remote project exists (optionally auto-created),
//  4. remote service name is free (unless overwrite allowed).
func Preflight(ctx context.Context, local, remote *Client, in MigrateInput, opt PreflightOptions) (*MigratePlan, error) {
	// 1 + 2: local project + service.
	localInspect, err := local.InspectProject(ctx, in.LocalProjectName)
	if err != nil {
		if NotFound(err) {
			return nil, fmt.Errorf("local project %q not found", in.LocalProjectName)
		}
		return nil, fmt.Errorf("inspect local project: %w", err)
	}
	if !hasService(localInspect.Services, in.LocalServiceName) {
		return nil, fmt.Errorf("local service %q not found in project %q",
			in.LocalServiceName, in.LocalProjectName)
	}

	// 3: remote project.
	created := false
	remoteInspect, err := remote.InspectProject(ctx, in.RemoteProjectName)
	switch {
	case err == nil:
		// exists
	case NotFound(err):
		if !opt.CreateRemoteProject {
			return nil, fmt.Errorf("remote project %q not found (use create to auto-create)",
				in.RemoteProjectName)
		}
		if err := remote.CreateProject(ctx, in.RemoteProjectName); err != nil {
			return nil, fmt.Errorf("create remote project %q: %w", in.RemoteProjectName, err)
		}
		created = true
		remoteInspect = &InspectResult{Project: Project{Name: in.RemoteProjectName}}
	default:
		return nil, fmt.Errorf("inspect remote project: %w", err)
	}

	// 4: remote service collision.
	if !opt.OverwriteRemoteService && hasService(remoteInspect.Services, in.RemoteServiceName) {
		return nil, fmt.Errorf("remote service %q already exists in project %q (allow overwrite to proceed)",
			in.RemoteServiceName, in.RemoteProjectName)
	}

	return &MigratePlan{
		Local:          local,
		Remote:         remote,
		RemoteURL:      in.RemoteEasypanelURL,
		RemoteToken:    in.RemoteAPIToken,
		LocalProject:   in.LocalProjectName,
		LocalService:   in.LocalServiceName,
		RemoteProject:  in.RemoteProjectName,
		RemoteService:  in.RemoteServiceName,
		CreatedProject: created,
	}, nil
}

// Run executes the validated migration against the source (local) panel.
func (p *MigratePlan) Run(ctx context.Context) error {
	return p.Local.Migrate(ctx, MigrateInput{
		LocalProjectName:   p.LocalProject,
		LocalServiceName:   p.LocalService,
		RemoteAPIToken:     p.RemoteToken,
		RemoteEasypanelURL: p.RemoteURL,
		RemoteProjectName:  p.RemoteProject,
		RemoteServiceName:  p.RemoteService,
	})
}

func hasService(services []Service, name string) bool {
	for _, s := range services {
		if s.Name == name {
			return true
		}
	}
	return false
}

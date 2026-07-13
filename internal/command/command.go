package command

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"silent-devops/internal/app"
)

func Main(name string) {
	os.Exit(Run(name, os.Args[1:], os.Stdout, os.Stderr))
}

func Run(name string, args []string, stdout, stderr *os.File) int {
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	return app.Run(context.Background(), name, args, stdout, stderr, func(ctx context.Context) error {
		logger.InfoContext(ctx, "starting", "component", name)
		return fmt.Errorf("%s service not configured", name)
	})
}

package localcontrol

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
)

func Dial(ctx context.Context, path string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///unix", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return new(net.Dialer).DialContext(ctx, "unix", path)
	}))
}

package localcontrol

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
)

func Dial(ctx context.Context, path string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///unix", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return net.Dial("unix", path) }))
}

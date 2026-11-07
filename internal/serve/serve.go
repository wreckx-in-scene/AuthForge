package serve

import (
	"context"

	"google.golang.org/grpc"

	"github.com/wreckx-in-scene/AuthForge/internal/grpcrun"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
)

func GRPC(ctx context.Context, healthAddr, grpcAddr string, register func(*grpc.Server)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- health.Serve(ctx, healthAddr) }()
	go func() { errCh <- grpcrun.Serve(ctx, grpcAddr, register) }()
	err := <-errCh
	cancel()
	return err
}

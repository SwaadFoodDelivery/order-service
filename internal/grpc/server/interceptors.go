package server

import (
	"context"

	"order-service/internal/app"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func AuthUnaryInterceptor(c *app.Container) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		_ = c
		_ = info
		return handler(ctx, req)
	}
}

func LoggingUnaryInterceptor(c *app.Container) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		c.Logger.Info("grpc_request")
		return handler(ctx, req)
	}
}

func RecoveryUnaryInterceptor(c *app.Container) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if recover() != nil {
				c.Logger.Error("panic_recovered")
				err = status.Error(13, "internal")
			}
		}()
		return handler(ctx, req)
	}
}

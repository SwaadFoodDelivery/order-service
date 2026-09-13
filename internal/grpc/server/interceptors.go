package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const ServiceKeyMetadata = "x-order-service-key"

func authUnaryInterceptor(key string) grpc.UnaryServerInterceptor {
	expected := sha256.Sum256([]byte(key))
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get(ServiceKeyMetadata)
		if len(values) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid service credential")
		}
		supplied := sha256.Sum256([]byte(values[0]))
		if subtle.ConstantTimeCompare(supplied[:], expected[:]) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid service credential")
		}
		return handler(ctx, req)
	}
}

func timeoutUnaryInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		bounded, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		out, err := handler(bounded, req)
		if bounded.Err() != nil {
			return nil, status.FromContextError(bounded.Err()).Err()
		}
		return out, err
	}
}

func recoveryUnaryInterceptor(log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if recover() != nil {
				// Do not log request data, metadata, credentials, SQL errors, or panic values.
				log.Error("order RPC panic recovered")
				err = status.Error(codes.Internal, "order lookup failed")
			}
		}()
		return handler(ctx, req)
	}
}

package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"order-service/internal/services/order/business"
	"order-service/internal/services/order/repository"
	"order-service/pkg/config"

	orderpb "github.com/SwaadFoodDelivery/proto/order"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// New registers the generated service and accepts either the production SQL
// repository or a test repository. All RPCs except GetOrder remain Unimplemented.
func New(cfg *config.Config, repo repository.Repository, log *zap.Logger) (*grpc.Server, error) {
	if err := cfg.ValidateRPC(); err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, fmt.Errorf("order repository is required")
	}
	if log == nil {
		log = zap.NewNop()
	}
	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(64*1024),
		grpc.ChainUnaryInterceptor(
			authUnaryInterceptor(cfg.GRPC.ServiceKey),
			timeoutUnaryInterceptor(cfg.GRPC.RequestTimeout),
			recoveryUnaryInterceptor(log),
		),
	)
	orderpb.RegisterOrderServiceServer(s, &orderServer{service: business.NewService(repo)})
	return s, nil
}

type orderServer struct {
	orderpb.UnimplementedOrderServiceServer
	service business.Service
}

func (s *orderServer) GetOrder(ctx context.Context, req *orderpb.GetOrderRequest) (*orderpb.OrderResponse, error) {
	out, err := s.service.GetOrder(ctx, req.GetOrderId(), req.GetRequesterUserId(), req.GetRequesterRole())
	if err != nil {
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		switch {
		case errors.Is(err, business.ErrInvalidArgument):
			return nil, status.Error(codes.InvalidArgument, business.ErrInvalidArgument.Error())
		case errors.Is(err, business.ErrPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, business.ErrPermissionDenied.Error())
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, "order not found")
		case errors.Is(err, repository.ErrInvalidData):
			return nil, status.Error(codes.Internal, "invalid stored order data")
		default:
			return nil, status.Error(codes.Unavailable, "order lookup unavailable")
		}
	}
	enum, ok := orderpb.OrderStatus_value[strings.ToUpper(out.Status)]
	if !ok || enum == 0 {
		return nil, status.Error(codes.Internal, "invalid stored order status")
	}
	response := &orderpb.OrderResponse{
		OrderId: out.ID, Status: orderpb.OrderStatus(enum), CreatedAt: out.CreatedAt.UTC().Format(time.RFC3339Nano),
		TotalAmountMinor: out.TotalMinor, TotalAmount: float64(out.TotalMinor) / 100,
		Currency: "INR", RestaurantId: out.RestaurantID, PaymentStatus: out.PaymentStatus, PaymentMethod: out.PaymentMethod,
		Items: make([]*orderpb.OrderItem, 0, len(out.Items)),
	}
	for _, item := range out.Items {
		response.Items = append(response.Items, &orderpb.OrderItem{
			ItemId: item.ID, ItemNameSnapshot: item.Name, Quantity: item.Quantity,
			ItemPriceSnapshotMinor: item.PriceMinor, LineTotalMinor: item.LineTotalMinor,
			ItemPriceSnapshot: float64(item.PriceMinor) / 100, LineTotal: float64(item.LineTotalMinor) / 100,
		})
	}
	return response, nil
}

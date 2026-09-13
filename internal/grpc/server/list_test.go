package server_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	grpcserver "order-service/internal/grpc/server"
	"order-service/internal/services/order/models"

	orderpb "github.com/SwaadFoodDelivery/proto/order"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type listRepositoryFunc func(context.Context, uuid.UUID, int, *models.OrderPosition) ([]models.OrderSummary, error)

func (f listRepositoryFunc) GetOrder(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
	panic("list must not hydrate GetOrder")
}
func (f listRepositoryFunc) ListForUser(ctx context.Context, user uuid.UUID, n int, before *models.OrderPosition) ([]models.OrderSummary, error) {
	return f(ctx, user, n, before)
}

func TestGetUserOrdersAuthAndValidationBeforeSQL(t *testing.T) {
	var calls atomic.Int32
	client := newClient(t, listRepositoryFunc(func(context.Context, uuid.UUID, int, *models.OrderPosition) ([]models.OrderSummary, error) {
		calls.Add(1)
		return nil, nil
	}), time.Second, false)
	for _, tc := range []struct {
		name, key, user, role, cursor string
		limit                         int32
		want                          codes.Code
	}{
		{"missing_key", "", "invalid", "driver", "broken", 0, codes.Unauthenticated},
		{"wrong_key", "wrong", testUserID.String(), "client", "", 1, codes.Unauthenticated},
		{"wrong_role", testKey, testUserID.String(), "driver", "broken", 1, codes.PermissionDenied},
		{"missing_role", testKey, testUserID.String(), "", "", 1, codes.PermissionDenied},
		{"invalid_uuid", testKey, "invalid", "client", "", 1, codes.InvalidArgument},
		{"zero_limit", testKey, testUserID.String(), "client", "", 0, codes.InvalidArgument},
		{"large_limit", testKey, testUserID.String(), "client", "", 51, codes.InvalidArgument},
		{"bad_cursor", testKey, testUserID.String(), "client", "broken", 1, codes.InvalidArgument},
		{"large_cursor", testKey, testUserID.String(), "client", strings.Repeat("x", 1025), 1, codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.key != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcserver.ServiceKeyMetadata, tc.key)
			}
			_, err := client.GetUserOrders(ctx, &orderpb.GetUserOrdersRequest{UserId: tc.user, RequesterRole: tc.role, Limit: tc.limit, Cursor: tc.cursor})
			if status.Code(err) != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("invalid list request reached repository")
	}
}

func TestGetUserOrdersSummaryTransport(t *testing.T) {
	created := time.Date(2026, 9, 13, 1, 2, 3, 123456000, time.UTC)
	var calls atomic.Int32
	client := newClient(t, listRepositoryFunc(func(_ context.Context, user uuid.UUID, n int, before *models.OrderPosition) ([]models.OrderSummary, error) {
		calls.Add(1)
		if user != testUserID || n != 2 {
			t.Error("incorrect ownership or lookahead")
		}
		if before != nil {
			return nil, nil
		}
		row := models.OrderSummary{OrderPosition: models.OrderPosition{CreatedAt: created, OrderID: testOrderID}, Status: "confirmed", TotalMinor: 1005,
			RestaurantID: testOrderID.String(), RestaurantName: "Current inactive restaurant", PaymentMethod: "upi", DeliveryStatus: ""}
		return []models.OrderSummary{row, row}, nil
	}), time.Second, true)
	req := &orderpb.GetUserOrdersRequest{UserId: testUserID.String(), RequesterRole: "client", Limit: 1}
	page, err := client.GetUserOrders(authorized(context.Background()), req)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Orders) != 1 || page.NextCursor == "" {
		t.Fatalf("invalid page: %v", page)
	}
	out := page.Orders[0]
	if out.TotalAmountMinor != 1005 || out.TotalAmount != 10.05 || out.Currency != "INR" || out.RestaurantName != "Current inactive restaurant" || out.CreatedAt != created.Format(time.RFC3339Nano) ||
		out.PaymentStatus != "" || len(out.Items) != 0 || out.DeliveryStatus != "" {
		t.Fatalf("incorrect summary: %v", out)
	}
	req.Cursor = page.NextCursor
	req.UserId = uuid.NewString()
	_, err = client.GetUserOrders(authorized(context.Background()), req)
	if status.Code(err) != codes.InvalidArgument || calls.Load() != 1 {
		t.Fatal("foreign cursor reached repository")
	}
	req.UserId = testUserID.String()
	end, err := client.GetUserOrders(authorized(context.Background()), req)
	if err != nil || len(end.Orders) != 0 || end.Total != 0 || end.NextCursor != "" {
		t.Fatalf("unexpected end page: %v, %v", end, err)
	}
}

func TestGetUserOrdersDeadline(t *testing.T) {
	stopped := make(chan struct{})
	client := newClient(t, listRepositoryFunc(func(ctx context.Context, _ uuid.UUID, _ int, _ *models.OrderPosition) ([]models.OrderSummary, error) {
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}), 50*time.Millisecond, false)
	_, err := client.GetUserOrders(authorized(context.Background()), &orderpb.GetUserOrdersRequest{UserId: testUserID.String(), RequesterRole: "client", Limit: 1})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("got %v, want DeadlineExceeded", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("SQL context was not canceled")
	}
}

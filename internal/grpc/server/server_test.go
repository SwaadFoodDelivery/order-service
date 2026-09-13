package server_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	grpcserver "order-service/internal/grpc/server"
	"order-service/internal/services/order/models"
	"order-service/internal/services/order/repository"
	"order-service/pkg/config"

	orderpb "github.com/SwaadFoodDelivery/proto/order"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testKey = "test-only-order-service-key-32-bytes"

var testOrderID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
var testUserID = uuid.MustParse("22222222-2222-4222-8222-222222222222")

type repositoryFunc func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error)

func (f repositoryFunc) GetOrder(ctx context.Context, order, user uuid.UUID) (models.Order, error) {
	return f(ctx, order, user)
}

func testConfig(timeout time.Duration) *config.Config {
	cfg := &config.Config{}
	cfg.App.Env = "test"
	cfg.GRPC.OrderAddr = "127.0.0.1:50051"
	cfg.GRPC.ServiceKey = testKey
	cfg.GRPC.RequestTimeout = timeout
	return cfg
}

func newClient(t *testing.T, repo repository.Repository, timeout time.Duration, tcp bool) orderpb.OrderServiceClient {
	t.Helper()
	s, err := grpcserver.New(testConfig(timeout), repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	var listener net.Listener
	var dial func(context.Context, string) (net.Conn, error)
	if tcp {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		dial = func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
		}
	} else {
		buf := bufconn.Listen(1024 * 1024)
		listener = buf
		dial = func(ctx context.Context, _ string) (net.Conn, error) { return buf.DialContext(ctx) }
	}
	t.Cleanup(func() { s.Stop(); listener.Close() })
	go func() { _ = s.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "passthrough:///order-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dial), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return orderpb.NewOrderServiceClient(conn)
}

func authorized(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, grpcserver.ServiceKeyMetadata, testKey)
}

func validRequest() *orderpb.GetOrderRequest {
	return &orderpb.GetOrderRequest{OrderId: testOrderID.String(), RequesterUserId: testUserID.String(), RequesterRole: "client"}
}

func TestGetOrderAuthenticationFirst(t *testing.T) {
	var calls atomic.Int32
	client := newClient(t, repositoryFunc(func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
		calls.Add(1)
		return models.Order{}, nil
	}), time.Second, false)
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"missing", nil}, {"empty", []string{""}}, {"wrong", []string{strings.Repeat("x", len(testKey))}},
		{"short", []string{"x"}}, {"duplicate", []string{testKey, testKey}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := metadata.MD{}
			if tc.values != nil {
				md[grpcserver.ServiceKeyMetadata] = tc.values
			}
			ctx := metadata.NewOutgoingContext(context.Background(), md)
			// Malformed input still gets authentication failure, never validation details.
			_, err := client.GetOrder(ctx, &orderpb.GetOrderRequest{})
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("got %v, want Unauthenticated", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("unauthenticated request reached repository %d times", calls.Load())
	}
}

func TestGetOrderRejectsInvalidIdentity(t *testing.T) {
	var calls atomic.Int32
	client := newClient(t, repositoryFunc(func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
		calls.Add(1)
		return models.Order{}, nil
	}), time.Second, false)
	for _, tc := range []struct {
		name, order, user, role string
		code                    codes.Code
	}{
		{"invalid_order", "invalid", testUserID.String(), "client", codes.InvalidArgument},
		{"invalid_user", testOrderID.String(), "invalid", "client", codes.InvalidArgument},
		{"nil_order", uuid.Nil.String(), testUserID.String(), "client", codes.InvalidArgument},
		{"nil_user", testOrderID.String(), uuid.Nil.String(), "client", codes.InvalidArgument},
		{"owner", testOrderID.String(), testUserID.String(), "restaurant_owner", codes.PermissionDenied},
		{"driver", testOrderID.String(), testUserID.String(), "driver", codes.PermissionDenied},
		{"manager", testOrderID.String(), testUserID.String(), "restaurant_manager", codes.PermissionDenied},
		{"empty_role", testOrderID.String(), testUserID.String(), "", codes.PermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.GetOrder(authorized(context.Background()), &orderpb.GetOrderRequest{OrderId: tc.order, RequesterUserId: tc.user, RequesterRole: tc.role})
			if status.Code(err) != tc.code {
				t.Fatalf("got %v, want %v", err, tc.code)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid identity reached repository %d times", calls.Load())
	}
}

func TestGetOrderTransportAndCompatibility(t *testing.T) {
	for _, tcp := range []bool{false, true} {
		name := "bufconn"
		if tcp {
			name = "loopback_tcp"
		}
		t.Run(name, func(t *testing.T) {
			created := time.Date(2026, 9, 12, 10, 5, 0, 123456000, time.UTC)
			client := newClient(t, repositoryFunc(func(_ context.Context, order, user uuid.UUID) (models.Order, error) {
				if order != testOrderID || user != testUserID {
					t.Errorf("requester scope was not propagated")
				}
				return models.Order{ID: order.String(), Status: "order_created", CreatedAt: created, TotalMinor: 1005,
					RestaurantID: testOrderID.String(), PaymentStatus: "success", PaymentMethod: "upi",
					Items: []models.Item{{ID: testOrderID.String(), Name: "stored name", PriceMinor: 5, Quantity: 2, LineTotalMinor: 10}}}, nil
			}), time.Second, tcp)
			out, err := client.GetOrder(authorized(context.Background()), validRequest())
			if err != nil {
				t.Fatal(err)
			}
			if out.OrderId != testOrderID.String() || out.TotalAmountMinor != 1005 || out.TotalAmount != 10.05 || out.Currency != "INR" ||
				out.Status != orderpb.OrderStatus_ORDER_CREATED || out.CreatedAt != created.Format(time.RFC3339Nano) ||
				out.RestaurantId != testOrderID.String() || out.PaymentStatus != "success" || out.PaymentMethod != "upi" {
				t.Fatalf("unexpected response: %v", out)
			}
			if len(out.Items) != 1 || out.Items[0].ItemPriceSnapshotMinor != 5 || out.Items[0].ItemPriceSnapshot != 0.05 ||
				out.Items[0].LineTotalMinor != 10 || out.Items[0].LineTotal != 0.1 || out.Items[0].Quantity != 2 {
				t.Fatalf("unexpected item: %v", out.Items)
			}
		})
	}
}

func TestGetOrderForeignAndMissingAreIdentical(t *testing.T) {
	client := newClient(t, repositoryFunc(func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
		return models.Order{}, repository.ErrNotFound
	}), time.Second, false)
	_, foreign := client.GetOrder(authorized(context.Background()), validRequest())
	req := validRequest()
	req.OrderId = uuid.NewString()
	_, missing := client.GetOrder(authorized(context.Background()), req)
	if status.Code(foreign) != codes.NotFound || status.Code(missing) != codes.NotFound || status.Convert(foreign).Message() != status.Convert(missing).Message() {
		t.Fatalf("foreign/missing differ: %v / %v", foreign, missing)
	}
}

func TestGetOrderDeadlineAndCancel(t *testing.T) {
	for _, mode := range []string{"server_cap", "caller_deadline", "caller_cancel"} {
		t.Run(mode, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			client := newClient(t, repositoryFunc(func(ctx context.Context, _, _ uuid.UUID) (models.Order, error) {
				close(started)
				<-ctx.Done()
				close(stopped)
				return models.Order{}, ctx.Err()
			}), 100*time.Millisecond, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := codes.DeadlineExceeded
			if mode == "caller_deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			} else if mode == "caller_cancel" {
				want = codes.Canceled
				go func() {
					select {
					case <-started:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			start := time.Now()
			_, err := client.GetOrder(authorized(ctx), validRequest())
			if status.Code(err) != want {
				t.Fatalf("got %v, want %v", err, want)
			}
			if time.Since(start) > time.Second {
				t.Fatal("RPC did not respect timeout bounds")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("repository context was not canceled")
			}
		})
	}
}

func TestOtherRPCsAreUnimplemented(t *testing.T) {
	client := newClient(t, repositoryFunc(func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
		t.Error("unimplemented RPC reached repository")
		return models.Order{}, nil
	}), time.Second, false)
	ctx := authorized(context.Background())
	_, place := client.PlaceOrder(ctx, &orderpb.PlaceOrderRequest{})
	_, list := client.GetUserOrders(ctx, &orderpb.GetUserOrdersRequest{})
	_, cancel := client.CancelOrder(ctx, &orderpb.CancelOrderRequest{})
	_, update := client.UpdateOrderStatus(ctx, &orderpb.UpdateOrderStatusRequest{})
	_, tracking := client.GetOrderTracking(ctx, &orderpb.GetOrderTrackingRequest{})
	for _, err := range []error{place, list, cancel, update, tracking} {
		if status.Code(err) != codes.Unimplemented {
			t.Errorf("got %v, want Unimplemented", err)
		}
	}
}

func TestGetOrderSanitizesFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo repositoryFunc
		code codes.Code
	}{
		{"database", func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
			return models.Order{}, errors.New("sensitive database detail")
		}, codes.Unavailable},
		{"data", func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
			return models.Order{}, repository.ErrInvalidData
		}, codes.Internal},
		{"status", func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) {
			return models.Order{Status: "unknown"}, nil
		}, codes.Internal},
		{"panic", func(context.Context, uuid.UUID, uuid.UUID) (models.Order, error) { panic("sensitive panic detail") }, codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newClient(t, tc.repo, time.Second, false)
			_, err := client.GetOrder(authorized(context.Background()), validRequest())
			if status.Code(err) != tc.code || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("unexpected public error: %v", err)
			}
		})
	}
}

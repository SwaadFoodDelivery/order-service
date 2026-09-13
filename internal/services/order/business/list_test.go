package business

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"order-service/internal/services/order/models"
)

type listFunc func(context.Context, uuid.UUID, int, *models.OrderPosition) ([]models.OrderSummary, error)

func (f listFunc) ListForUser(ctx context.Context, user uuid.UUID, n int, before *models.OrderPosition) ([]models.OrderSummary, error) {
	return f(ctx, user, n, before)
}

func TestListPagination(t *testing.T) {
	user := uuid.New()
	at := time.Date(2026, 9, 13, 1, 2, 3, 123456000, time.UTC)
	rows := []models.OrderSummary{
		{OrderPosition: models.OrderPosition{CreatedAt: at, OrderID: uuid.MustParse("ffffffff-ffff-4fff-8fff-ffffffffffff")}},
		{OrderPosition: models.OrderPosition{CreatedAt: at, OrderID: uuid.MustParse("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee")}},
		{OrderPosition: models.OrderPosition{CreatedAt: at.Add(-time.Microsecond), OrderID: uuid.New()}},
	}
	for _, tc := range []struct {
		name         string
		count, limit int
		next         bool
	}{
		{"empty", 0, 2, false}, {"last_page", 1, 2, false}, {"exact_end", 2, 2, false}, {"extra", 3, 2, true}, {"max_limit", 3, 50, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewListService(listFunc(func(_ context.Context, owner uuid.UUID, fetchLimit int, before *models.OrderPosition) ([]models.OrderSummary, error) {
				if owner != user || fetchLimit != tc.limit+1 || before != nil {
					t.Fatal("incorrect query scope or lookahead limit")
				}
				return rows[:tc.count], nil
			}), "unit-cursor-key")
			page, err := svc.GetUserOrders(context.Background(), user.String(), "client", tc.limit, "")
			if err != nil {
				t.Fatal(err)
			}
			if (page.NextCursor != "") != tc.next {
				t.Fatal("incorrect next cursor presence")
			}
			want := tc.count
			if want > tc.limit {
				want = tc.limit
			}
			if len(page.Orders) != want {
				t.Fatalf("got %d results, want %d", len(page.Orders), want)
			}
			if tc.next {
				position, err := svc.cursors.decode(page.NextCursor, user)
				if err != nil || position.OrderID != rows[tc.limit-1].OrderID || !position.CreatedAt.Equal(at) {
					t.Fatal("cursor was not made from last returned row")
				}
			}
		})
	}
}

func TestListCursorAndValidationBeforeRepository(t *testing.T) {
	user := uuid.New()
	calls := 0
	svc := NewListService(listFunc(func(_ context.Context, owner uuid.UUID, n int, before *models.OrderPosition) ([]models.OrderSummary, error) {
		calls++
		if owner != user || n != 2 || before == nil || before.CreatedAt.Nanosecond() != 123456000 {
			t.Fatal("cursor position/scope was not forwarded")
		}
		return nil, nil
	}), "unit-key")
	token, _ := svc.cursors.encode(user, models.OrderPosition{CreatedAt: time.Date(2026, 9, 13, 0, 0, 0, 123456000, time.UTC), OrderID: uuid.New()})
	for _, tc := range []struct {
		user, role, cursor string
		limit              int
		err                error
	}{
		{user.String(), "driver", token, 1, ErrPermissionDenied},
		{user.String(), "", token, 1, ErrPermissionDenied},
		{"invalid", "client", token, 1, ErrInvalidList},
		{uuid.Nil.String(), "client", token, 1, ErrInvalidList},
		{user.String(), "client", "", 0, ErrInvalidList},
		{user.String(), "client", "", -1, ErrInvalidList},
		{user.String(), "client", "", 51, ErrInvalidList},
		{user.String(), "client", "tampered", 1, ErrInvalidCursor},
		{uuid.NewString(), "client", token, 1, ErrInvalidCursor},
	} {
		_, err := svc.GetUserOrders(context.Background(), tc.user, tc.role, tc.limit, tc.cursor)
		if !errors.Is(err, tc.err) {
			t.Fatalf("got %v, want %v", err, tc.err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached repository")
	}
	if _, err := svc.GetUserOrders(context.Background(), user.String(), "client", 1, token); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("valid cursor did not reach repository")
	}
}

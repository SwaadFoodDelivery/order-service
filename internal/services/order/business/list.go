package business

import (
	"context"
	"errors"

	"order-service/internal/services/order/models"
	"order-service/internal/services/order/repository"

	"github.com/google/uuid"
)

var ErrInvalidList = errors.New("user_id must be a nonzero UUID and limit must be between 1 and 50")

type ListService struct {
	repo    repository.ListRepository
	cursors cursorCodec
}

func NewListService(repo repository.ListRepository, serviceKey string) *ListService {
	return &ListService{repo: repo, cursors: newCursorCodec(serviceKey)}
}

func (s *ListService) GetUserOrders(ctx context.Context, userID, role string, limit int, cursor string) (models.OrderPage, error) {
	if role != "client" {
		return models.OrderPage{}, ErrPermissionDenied
	}
	user, err := uuid.Parse(userID)
	if err != nil || len(userID) != 36 || user == uuid.Nil || limit < 1 || limit > 50 {
		return models.OrderPage{}, ErrInvalidList
	}
	var before *models.OrderPosition
	if cursor != "" {
		before, err = s.cursors.decode(cursor, user)
		if err != nil {
			return models.OrderPage{}, err
		}
	}
	orders, err := s.repo.ListForUser(ctx, user, limit+1, before)
	if err != nil {
		return models.OrderPage{}, err
	}
	page := models.OrderPage{Orders: orders}
	if len(orders) > limit {
		page.Orders = orders[:limit]
		page.NextCursor, err = s.cursors.encode(user, page.Orders[limit-1].OrderPosition)
		if err != nil {
			return models.OrderPage{}, repository.ErrInvalidData
		}
	}
	return page, nil
}

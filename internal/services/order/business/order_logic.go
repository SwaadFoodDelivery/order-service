package business

import (
	"context"
	"github.com/google/uuid"
	"order-service/internal/services/order/models"
	"order-service/internal/services/order/repository"
)

type service struct{ repo repository.Repository }

func NewService(repo repository.Repository) Service { return &service{repo: repo} }

func (s *service) GetOrder(ctx context.Context, orderID, requesterID, role string) (models.Order, error) {
	if role != "client" {
		return models.Order{}, ErrPermissionDenied
	}
	order, orderErr := uuid.Parse(orderID)
	requester, requesterErr := uuid.Parse(requesterID)
	if len(orderID) != 36 || len(requesterID) != 36 || orderErr != nil || requesterErr != nil || order == uuid.Nil || requester == uuid.Nil {
		return models.Order{}, ErrInvalidArgument
	}
	return s.repo.GetOrder(ctx, order, requester)
}

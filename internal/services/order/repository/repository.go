package repository

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"order-service/internal/services/order/models"
)

var (
	ErrNotFound    = errors.New("order not found")
	ErrInvalidData = errors.New("invalid stored order data")
)

// Repository exposes no mutations. The backend remains the authoritative writer.
type Repository interface {
	GetOrder(ctx context.Context, orderID, requesterID uuid.UUID) (models.Order, error)
}

// ListRepository is separate so existing GetOrder adapters need not implement it.
type ListRepository interface {
	ListForUser(ctx context.Context, requesterID uuid.UUID, fetchLimit int, before *models.OrderPosition) ([]models.OrderSummary, error)
}

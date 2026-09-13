package business

import (
	"context"
	"errors"
	"order-service/internal/services/order/models"
)

var (
	ErrInvalidArgument  = errors.New("order_id and requester_user_id must be nonzero UUIDs")
	ErrPermissionDenied = errors.New("only client order reads are supported")
)

type Service interface {
	GetOrder(ctx context.Context, orderID, requesterID, role string) (models.Order, error)
}

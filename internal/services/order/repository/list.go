package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"order-service/internal/services/order/models"

	"github.com/google/uuid"
)

var _ ListRepository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ListForUser(ctx context.Context, requesterID uuid.UUID, fetchLimit int, before *models.OrderPosition) ([]models.OrderSummary, error) {
	if requesterID == uuid.Nil || fetchLimit < 1 || fetchLimit > 51 {
		return nil, fmt.Errorf("invalid order list repository arguments")
	}
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// No restaurant status filter: historical orders remain visible after closure.
	// Deliveries belong to a composite order identity; missing deliveries are valid.
	query := `SELECT o.order_id, o.created_at, o.status::text, o.total_amount::text,
		o.restaurant_id, r.name AS restaurant_name, o.payment_method,
		COALESCE(d.status::text, '') AS delivery_status
		FROM orders o
		JOIN restaurants r ON r.restaurant_id = o.restaurant_id
		LEFT JOIN deliveries d ON d.order_id = o.order_id AND d.order_created_at = o.created_at
		WHERE o.user_id = $1`
	args := []any{requesterID}
	if before != nil {
		query += ` AND (o.created_at, o.order_id) < ($2, $3)`
		args = append(args, before.CreatedAt, before.OrderID)
	}
	query += fmt.Sprintf(" ORDER BY o.created_at DESC, o.order_id DESC LIMIT $%d", len(args)+1)
	args = append(args, fetchLimit)
	var rows []struct {
		OrderID        uuid.UUID `db:"order_id"`
		CreatedAt      time.Time `db:"created_at"`
		Status         string    `db:"status"`
		Total          string    `db:"total_amount"`
		RestaurantID   string    `db:"restaurant_id"`
		RestaurantName string    `db:"restaurant_name"`
		PaymentMethod  string    `db:"payment_method"`
		DeliveryStatus string    `db:"delivery_status"`
	}
	if err := tx.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]models.OrderSummary, 0, len(rows))
	for _, row := range rows {
		total, err := numericToMinor(row.Total)
		if err != nil {
			return nil, fmt.Errorf("%w: order summary amount", ErrInvalidData)
		}
		out = append(out, models.OrderSummary{OrderPosition: models.OrderPosition{OrderID: row.OrderID, CreatedAt: row.CreatedAt},
			Status: row.Status, TotalMinor: total, RestaurantID: row.RestaurantID, RestaurantName: row.RestaurantName,
			PaymentMethod: row.PaymentMethod, DeliveryStatus: row.DeliveryStatus})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

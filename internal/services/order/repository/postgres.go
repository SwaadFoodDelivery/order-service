package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"order-service/internal/services/order/models"
	"strconv"
	"strings"
	"time"
)

type PostgresRepository struct{ db *sqlx.DB }

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) GetOrder(ctx context.Context, orderID, requesterID uuid.UUID) (models.Order, error) {
	// Both reads use one consistent snapshot, without acquiring write locks.
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return models.Order{}, err
	}
	defer tx.Rollback()
	var row struct {
		ID            string    `db:"order_id"`
		Status        string    `db:"status"`
		CreatedAt     time.Time `db:"created_at"`
		Total         string    `db:"total_amount"`
		RestaurantID  string    `db:"restaurant_id"`
		PaymentMethod string    `db:"payment_method"`
		PaymentStatus string    `db:"payment_status"`
	}
	err = tx.GetContext(ctx, &row, `
		SELECT o.order_id, o.status::text, o.created_at, o.total_amount::text,
		       o.restaurant_id, o.payment_method,
		       COALESCE((SELECT p.status FROM payments p
		         WHERE p.order_id = o.order_id AND p.order_created_at = o.created_at
		         ORDER BY (p.status = 'success') DESC, p.created_at DESC, p.payment_id DESC
		         LIMIT 1), 'unpaid') AS payment_status
		FROM orders o
		WHERE o.order_id = $1 AND o.user_id = $2
		ORDER BY o.created_at DESC LIMIT 1`, orderID, requesterID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Order{}, ErrNotFound
	}
	if err != nil {
		return models.Order{}, err
	}
	total, err := numericToMinor(row.Total)
	if err != nil {
		return models.Order{}, fmt.Errorf("%w: total amount", ErrInvalidData)
	}
	var items []struct {
		ID        string `db:"item_id"`
		Name      string `db:"item_name_snapshot"`
		Price     string `db:"item_price_snapshot"`
		Quantity  int32  `db:"quantity"`
		LineTotal string `db:"line_total"`
	}
	err = tx.SelectContext(ctx, &items, `
		SELECT item_id, item_name_snapshot, item_price_snapshot::text, quantity, line_total::text
		FROM order_items WHERE order_id = $1 AND order_created_at = $2
		ORDER BY order_item_id`, row.ID, row.CreatedAt)
	if err != nil {
		return models.Order{}, err
	}
	out := models.Order{ID: row.ID, Status: row.Status, CreatedAt: row.CreatedAt, TotalMinor: total,
		RestaurantID: row.RestaurantID, PaymentMethod: row.PaymentMethod, PaymentStatus: row.PaymentStatus,
		Items: make([]models.Item, 0, len(items))}
	for _, item := range items {
		price, priceErr := numericToMinor(item.Price)
		line, lineErr := numericToMinor(item.LineTotal)
		if priceErr != nil || lineErr != nil || item.Quantity <= 0 {
			return models.Order{}, fmt.Errorf("%w: order item", ErrInvalidData)
		}
		out.Items = append(out.Items, models.Item{ID: item.ID, Name: item.Name, PriceMinor: price, Quantity: item.Quantity, LineTotalMinor: line})
	}
	if err := tx.Commit(); err != nil {
		return models.Order{}, err
	}
	return out, nil
}

// numericToMinor parses fixed-point, nonnegative money without floating point
// or rounding. Fractional zeroes are significant. Invalid/overflowing data fails.
func numericToMinor(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalidData
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) < 1 || len(parts[1]) > 2 {
			return 0, ErrInvalidData
		}
		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}
	digits := parts[0] + fraction
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return 0, ErrInvalidData
		}
	}
	minor, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, ErrInvalidData
	}
	return minor, nil
}

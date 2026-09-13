package models

import (
	"time"

	"github.com/google/uuid"
)

type OrderPosition struct {
	CreatedAt time.Time
	OrderID   uuid.UUID
}

// OrderSummary intentionally omits item and payment-attempt hydration.
type OrderSummary struct {
	OrderPosition
	Status         string
	TotalMinor     int64
	RestaurantID   string
	RestaurantName string
	PaymentMethod  string
	DeliveryStatus string
}

type OrderPage struct {
	Orders     []OrderSummary
	NextCursor string
}

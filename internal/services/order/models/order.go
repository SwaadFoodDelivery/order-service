package models

import "time"

type Order struct {
	ID            string
	Status        string
	CreatedAt     time.Time
	TotalMinor    int64
	RestaurantID  string
	PaymentStatus string
	PaymentMethod string
	Items         []Item
}

type Item struct {
	ID             string
	Name           string
	PriceMinor     int64
	Quantity       int32
	LineTotalMinor int64
}

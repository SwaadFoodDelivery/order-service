package repository

type Repository interface {
	InsertOrder() error
	GetOrder() error
	UpdateStatus() error
	ListUserOrders() error
}

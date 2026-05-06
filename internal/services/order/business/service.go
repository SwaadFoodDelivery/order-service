package business

type Service interface {
	PlaceOrder() error
	GetOrder() error
	GetUserOrders() error
	CancelOrder() error
	UpdateOrderStatus() error
	GetOrderTracking() error
}

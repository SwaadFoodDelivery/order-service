package business

type service struct{}

var _ Service = (*service)(nil)

func NewService() Service                   { return &service{} }
func (s *service) PlaceOrder() error        { return nil }
func (s *service) GetOrder() error          { return nil }
func (s *service) GetUserOrders() error     { return nil }
func (s *service) CancelOrder() error       { return nil }
func (s *service) UpdateOrderStatus() error { return nil }
func (s *service) GetOrderTracking() error  { return nil }

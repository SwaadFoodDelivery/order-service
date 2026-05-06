package response

type APIResponse[T any] struct {
	Status string `json:"status"`
	Data   *T     `json:"data,omitempty"`
	Error  any    `json:"error,omitempty"`
}

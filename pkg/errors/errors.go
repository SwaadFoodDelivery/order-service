package errors

type AppError struct {
	Code    string   `json:"error_code"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

func (e *AppError) Error() string { return e.Message }

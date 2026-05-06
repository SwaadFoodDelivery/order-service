package logger

import (
	"strings"

	"go.uber.org/zap"
)

func New(level string) (*zap.Logger, error) {
	if strings.EqualFold(level, "debug") {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

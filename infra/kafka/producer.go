package kafka

import (
	"order-service/pkg/config"

	"github.com/segmentio/kafka-go"
)

func NewProducer(cfg *config.Config) (*kafka.Writer, error) {
	return &kafka.Writer{Addr: kafka.TCP(cfg.Kafka.Brokers...), Topic: cfg.Kafka.OrderEventsTopic, RequiredAcks: kafka.RequiredAcks(cfg.Kafka.RequiredAcks), BatchSize: cfg.Kafka.BatchSize, Balancer: &kafka.LeastBytes{}}, nil
}

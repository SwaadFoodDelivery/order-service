package kafka

import (
	"order-service/pkg/config"

	"github.com/segmentio/kafka-go"
)

func NewConsumer(cfg *config.Config, topic string) (*kafka.Reader, error) {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: cfg.Kafka.Brokers, Topic: topic, GroupID: cfg.Kafka.ClientID + "-group"}), nil
}

package main

import (
	"net"

	"order-service/infra/kafka"
	"order-service/infra/postgres"
	"order-service/infra/redis"
	"order-service/internal/app"
	grpcserver "order-service/internal/grpc/server"
	"order-service/pkg/config"
	"order-service/pkg/logger"

	"google.golang.org/grpc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log, err := logger.New(cfg.App.LogLevel)
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	db, _ := postgres.NewPostgresDB(cfg)
	rdb, _ := redis.NewRedisClient(cfg)
	kw, _ := kafka.NewProducer(cfg)
	deps := &app.Container{Config: cfg, Logger: log, DB: db, Redis: rdb, KafkaWriter: kw}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		panic(err)
	}
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(grpcserver.AuthUnaryInterceptor(deps), grpcserver.LoggingUnaryInterceptor(deps), grpcserver.RecoveryUnaryInterceptor(deps)))
	grpcserver.Register(deps)
	_ = s.Serve(lis)
}
